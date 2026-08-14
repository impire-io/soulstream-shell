package shellserver

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
	"github.com/nats-io/nats.go"
	"golang.org/x/oauth2"

	"github.com/impire-io/soulstream-core/realm"
	siclient "github.com/impire-io/soulstream-identity/client"
)

// session is one signed-in human: their own NATS admission (sentinel +
// their fold-issued bearer through the OIDC callout lane), their own
// realm client, their own signer. Delegated authority, never borrowed
// identity (S6): the shell signs as no one. Memory only — nothing here
// touches disk.
type session struct {
	Persona string // the admitted persona-shaped id
	Display string // human-facing name (id until O3 resolves richer)
	Signed  bool
	nc      *nats.Conn
	rc      *realm.Client
}

type oidcRP struct {
	provider *oidc.Provider
	cfg      *oauth2.Config
	pending  map[string]string // oauth state -> PKCE verifier
}

// newOIDCRP discovers the issuer and registers the shell through
// RFC 7591 DCR — no pre-provisioned client (design 0001 §6).
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
		"client_name":                "soulstream-shell",
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

func (s *Server) currentSession(r *http.Request) *session {
	c, err := r.Cookie("helm_session")
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[c.Value]
}

func (s *Server) closeAllSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		sess.nc.Close()
		delete(s.sessions, id)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	state, verifier := randTok(), randTok()+randTok()
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	s.mu.Lock()
	s.oidcState.pending[state] = verifier
	s.mu.Unlock()
	http.Redirect(w, r, s.oidcState.cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256")), http.StatusFound)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	fail := func(step string, err error) {
		http.Error(w, fmt.Sprintf("%s: %v", step, err), http.StatusBadGateway)
	}
	state := r.URL.Query().Get("state")
	s.mu.Lock()
	verifier, ok := s.oidcState.pending[state]
	delete(s.oidcState.pending, state)
	s.mu.Unlock()
	if !ok {
		fail("state", fmt.Errorf("unknown"))
		return
	}
	tok, err := s.oidcState.cfg.Exchange(r.Context(), r.URL.Query().Get("code"),
		oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		fail("exchange", err)
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := s.oidcState.provider.Verifier(&oidc.Config{ClientID: s.oidcState.cfg.ClientID}).
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
	persona := claims.PreferredUsername
	if persona == "" {
		persona = idt.Subject
	}
	// The name to put on screen: what the fold knows, else what the realm's
	// own persona directory publishes, else the id the record carries.
	display := claims.Name
	if display == "" {
		display = s.displayName(r.Context(), persona)
	}

	nc2, err := nats.Connect(s.opts.NATSURL,
		nats.UserCredentials(s.opts.SentinelPath), nats.Token(tok.AccessToken))
	if err != nil {
		fail("admission (oidc lane)", err)
		return
	}
	sess := &session{Persona: persona, Display: display, nc: nc2}
	cfg := realm.Config{Realm: s.opts.Realm, Persona: persona}
	if signer, serr := siclient.New(nc2, s.opts.Account, persona).PersonaSigner(persona); serr == nil {
		cfg.Signer = signer
		sess.Signed = true
	}
	rc2, err := realm.NewClient(r.Context(), nc2, cfg)
	if err != nil {
		nc2.Close()
		fail("realm client", err)
		return
	}
	sess.rc = rc2
	sid := randTok()
	s.mu.Lock()
	s.sessions[sid] = sess
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "helm_session", Value: sid,
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("helm_session"); err == nil {
		s.mu.Lock()
		if sess := s.sessions[c.Value]; sess != nil {
			sess.nc.Close()
		}
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "helm_session", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}
