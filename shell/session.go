package shell

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Sessions: one signed-in human, held in memory and nowhere else.
//
// The shell custodies exactly two things about a person — the id the issuer
// admitted them under and the grant it issued: the access token of the
// moment and, when the issuer gave one, the refresh token that renews it.
// None of it touches disk or the browser. Everything richer is somebody
// else's: a support layer hangs what it needs off the session through a
// SessionHook, the shell hands it back on request and closes it when the
// session ends, and never looks inside it.

// Session is one signed-in human.
type Session struct {
	// Subject is the principal the issuer admitted: the id every act this
	// person takes is attributed to.
	Subject string
	// Name is what the issuer said to call them, "" when it said nothing.
	Name string

	// mu guards the grant. tok is the issuer's access token of the moment;
	// renew replaces it when it is spent, for as long as the issuer honours
	// the refresh grant — nil when it issued none, and such a session ends
	// with its one token.
	mu    sync.Mutex
	tok   *oauth2.Token
	renew oauth2.TokenSource

	attached map[any]any
}

// Bearer is the issuer's current access token for this person — never in
// the browser, never on disk, renewed from here when it can be. An error
// means no living credential can be produced: the session is over, and the
// next request through Shell.Session ends it properly.
func (sess *Session) Bearer() (string, error) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.tok.Valid() {
		return sess.tok.AccessToken, nil
	}
	if sess.renew == nil {
		return "", errors.New("session: the access token is spent and the issuer issued nothing to renew it with")
	}
	tok, err := sess.renew.Token()
	if err != nil {
		return "", fmt.Errorf("session: renew: %w", err)
	}
	sess.tok = tok
	return tok.AccessToken, nil
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

// Session is the session this request carries, nil when it carries none —
// and a session that can no longer produce a credential carries none: it is
// closed and dropped on the spot, so the person reads the sign-in card
// rather than a screen whose every act errors.
func (s *Shell) Session(r *http.Request) *Session {
	c, err := r.Cookie(s.opts.SessionCookie)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	sess := s.sessions[c.Value]
	s.mu.Unlock()
	if sess == nil {
		return nil
	}
	if _, err := sess.Bearer(); err != nil {
		s.mu.Lock()
		still := s.sessions[c.Value] == sess
		if still {
			delete(s.sessions, c.Value)
		}
		s.mu.Unlock()
		if still {
			s.closeSession(sess)
		}
		return nil
	}
	return sess
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

// redirectBase is the origin the browser can actually reach this
// surface on: the advertised PublicURL when the deployment fronts the
// loopback listener, the bound address itself otherwise.
func redirectBase(opts Options, boundAddr string) string {
	if opts.PublicURL != "" {
		return strings.TrimSuffix(opts.PublicURL, "/")
	}
	return "http://" + boundAddr
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
	redirect := redirectBase(opts, boundAddr) + "/callback"
	reg, _ := json.Marshal(map[string]any{
		"redirect_uris":              []string{redirect},
		"client_name":                opts.ClientName,
		"grant_types":                []string{"authorization_code", "refresh_token"},
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
			// offline_access asks for the refresh grant, so a session can
			// outlive its first access token. An issuer that grants none
			// simply leaves the session to end with that token.
			Scopes: []string{oidc.ScopeOpenID, "profile", oidc.ScopeOfflineAccess},
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

	sess := &Session{Subject: subject, Name: claims.Name, tok: tok,
		attached: map[any]any{}}
	if tok.RefreshToken != "" {
		// Renewals outlive the sign-in request they started on, so the
		// source is bound to no request's context.
		sess.renew = s.oidc.cfg.TokenSource(context.Background(), tok)
	}
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
