package shell

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Sessions: one signed-in human, held in memory and nowhere else.
//
// The shell custodies exactly two things about a person — the id the issuer
// admitted them under and the bearer it issued — and neither touches disk or
// the browser. Everything richer is somebody else's: a support layer hangs
// what it needs off the session through a SessionHook, the shell hands it
// back on request and closes it when the session ends, and never looks
// inside it.

// Session is one signed-in human.
type Session struct {
	// Subject is the principal the issuer admitted: the id every act this
	// person takes is attributed to.
	Subject string
	// Name is what the issuer said to call them, "" when it said nothing.
	Name string
	// Bearer is the issuer's access token. It lives here for as long as the
	// session does — never in the browser, never on disk.
	Bearer string

	attached map[any]any
}

// Attached is what the hook registered under key opened for this session,
// nil when there is none.
func (sess *Session) Attached(key any) any { return sess.attached[key] }

// A SessionHook turns a signed-in person into whatever a module-support
// layer needs to act for them. The shell holds a bearer and an id; anything
// richer — a connection admitted as this person, a client that signs as
// them — is opened here and drained when they sign out.
type SessionHook interface {
	// SignedIn is called once, before the browser is handed its cookie: a
	// failure fails the sign-in. What it returns rides the session and
	// comes back from Session.Attached.
	SignedIn(ctx context.Context, sess *Session) (any, error)
	// SignedOut drains what SignedIn opened.
	SignedOut(v any)
}

// Named is the optional face of an attachment that knows what to call the
// signed-in person — for a deployment where the issuer mints an id and the
// name people know each other by lives somewhere else. The frame asks when
// the issuer itself said nothing; "" means it has no answer either.
type Named interface {
	ScreenName(ctx context.Context) string
}

type hook struct {
	key any
	h   SessionHook
}

// Attach registers a session hook under a key of the registrar's own —
// an unexported type, so nothing else can reach what it hangs off a
// session.
func (s *Shell) Attach(key any, h SessionHook) {
	s.hooks = append(s.hooks, hook{key: key, h: h})
}

// Session is the session this request carries, nil when it carries none.
func (s *Shell) Session(r *http.Request) *Session {
	c, err := r.Cookie(s.opts.SessionCookie)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[c.Value]
}

// screenName is what to call the signed-in person: what the issuer said,
// else whatever an attachment knows, else the id the issuer minted.
func (s *Shell) screenName(ctx context.Context, sess *Session) string {
	if sess == nil {
		return ""
	}
	if sess.Name != "" {
		return sess.Name
	}
	for _, a := range s.hooks {
		if n, ok := sess.attached[a.key].(Named); ok {
			if name := n.ScreenName(ctx); name != "" {
				return name
			}
		}
	}
	return sess.Subject
}

func (s *Shell) closeSession(sess *Session) {
	for _, a := range s.hooks {
		if v := sess.attached[a.key]; v != nil {
			a.h.SignedOut(v)
		}
	}
}

func (s *Shell) closeAllSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		s.closeSession(sess)
		delete(s.sessions, id)
	}
}

type oidcRP struct {
	provider *oidc.Provider
	cfg      *oauth2.Config
	pending  map[string]string // oauth state -> PKCE verifier
}

// newOIDCRP discovers the issuer and registers the shell through
// RFC 7591 DCR — no pre-provisioned client.
func newOIDCRP(ctx context.Context, opts Options, boundAddr string) (*oidcRP, error) {
	var provider *oidc.Provider
	var err error
	for range 40 {
		provider, err = oidc.NewProvider(ctx, opts.Issuer)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("issuer discovery: %w", err)
	}
	var meta struct {
		RegistrationEndpoint string `json:"registration_endpoint"`
	}
	if err := provider.Claims(&meta); err != nil || meta.RegistrationEndpoint == "" {
		return nil, fmt.Errorf("issuer publishes no registration_endpoint (%v)", err)
	}
	redirect := "http://" + boundAddr + "/callback"
	reg, _ := json.Marshal(map[string]any{
		"redirect_uris":              []string{redirect},
		"client_name":                opts.ClientName,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	resp, err := http.Post(meta.RegistrationEndpoint, "application/json", bytes.NewReader(reg)) // #nosec G107 -- deployment-configured issuer
	if err != nil {
		return nil, fmt.Errorf("dcr: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var client struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&client); err != nil {
		return nil, fmt.Errorf("dcr decode: %w", err)
	}
	return &oidcRP{
		provider: provider,
		cfg: &oauth2.Config{
			ClientID: client.ClientID, ClientSecret: client.ClientSecret,
			Endpoint:    provider.Endpoint(),
			RedirectURL: redirect,
			Scopes:      []string{oidc.ScopeOpenID, "profile"},
		},
		pending: map[string]string{},
	}, nil
}

func randTok() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Shell) login(w http.ResponseWriter, r *http.Request) {
	state, verifier := randTok(), randTok()+randTok()
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	s.mu.Lock()
	s.oidc.pending[state] = verifier
	s.mu.Unlock()
	http.Redirect(w, r, s.oidc.cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256")), http.StatusFound)
}

func (s *Shell) callback(w http.ResponseWriter, r *http.Request) {
	fail := func(step string, err error) {
		http.Error(w, fmt.Sprintf("%s: %v", step, err), http.StatusBadGateway)
	}
	state := r.URL.Query().Get("state")
	s.mu.Lock()
	verifier, ok := s.oidc.pending[state]
	delete(s.oidc.pending, state)
	s.mu.Unlock()
	if !ok {
		fail("state", fmt.Errorf("unknown"))
		return
	}
	tok, err := s.oidc.cfg.Exchange(r.Context(), r.URL.Query().Get("code"),
		oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		fail("exchange", err)
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := s.oidc.provider.Verifier(&oidc.Config{ClientID: s.oidc.cfg.ClientID}).
		Verify(r.Context(), rawID)
	if err != nil {
		fail("verify", err)
		return
	}
	var claims struct {
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	_ = idt.Claims(&claims)
	subject := claims.PreferredUsername
	if subject == "" {
		subject = idt.Subject
	}

	sess := &Session{Subject: subject, Name: claims.Name, Bearer: tok.AccessToken,
		attached: map[any]any{}}
	// Whatever else this person needs to be admitted as themselves, opened
	// once and closed with the session. The shell does not know what any of
	// it is; it knows only that a failure here is a sign-in that failed.
	for _, a := range s.hooks {
		v, err := a.h.SignedIn(r.Context(), sess)
		if err != nil {
			s.closeSession(sess)
			fail("session", err)
			return
		}
		sess.attached[a.key] = v
	}

	sid := randTok()
	s.mu.Lock()
	s.sessions[sid] = sess
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.opts.SessionCookie, Value: sid,
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, s.opts.Home, http.StatusFound)
}

func (s *Shell) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(s.opts.SessionCookie); err == nil {
		s.mu.Lock()
		if sess := s.sessions[c.Value]; sess != nil {
			s.closeSession(sess)
		}
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: s.opts.SessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, s.opts.Home, http.StatusFound)
}
