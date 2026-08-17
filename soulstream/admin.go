package soulstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// The people-and-sign-in administration surface, reached as the person
// looking at the screen.
//
// Two rules hold this whole file up. The first is authority: every call
// carries the bearer the issuer handed that person at sign-in, and nothing
// else. This layer holds no credential of its own to lend anybody, so what
// a person may do here is what the sign-in surface says they may do — the
// refusal comes back from there and goes on the screen as it was said.
//
// The second is that a deployment may not have one. Where the surface is
// arrives as a declared deployment fact (Config.AdminBase); a deployment
// signing its people in against an authorization server it does not run
// declares none, and Session.Admin is nil there. That absence is what the
// module above reads to decide it is not part of this deployment at all.
//
// It speaks the surface's JSON rather than importing its Go: the shapes
// below are the wire contract as published, so this layer stays a consumer
// of an HTTP surface rather than of somebody's package.

// Person is one human the sign-in surface knows.
type Person struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Status      string   `json:"status"`
	Groups      []string `json:"groups"`
	// Credentials is how many passkeys they have enrolled. Zero means
	// somebody who exists but cannot yet sign in.
	Credentials int `json:"credentials"`
	// LastAdmin marks the one person left who can administer sign-ins
	// here. Taking that away from them would leave the deployment with
	// nobody to administer it, so the sign-in surface refuses it — and
	// says which person it is holding, rather than leaving a screen to
	// work it out from the group names, which would mean this layer
	// deciding which group administers somebody else's deployment.
	LastAdmin bool `json:"last_admin"`
}

// Active reports whether this person may sign in at all.
func (p Person) Active() bool { return p.Status == "active" }

// Invite is a single-use enrolment invite. The token is in the answer that
// mints it and nowhere else ever again — the surface keeps only its digest
// — so a screen that fails to show it has thrown it away.
type Invite struct {
	Token string `json:"invite"`
	URL   string `json:"enroll_url"`
}

// Refusal is the surface's own no, kept whole: the status it answered with
// and the words it used. A screen says what was actually said rather than
// inventing a reason on its behalf.
type Refusal struct {
	Status int
	Msg    string
}

func (e *Refusal) Error() string { return e.Msg }

// Denied separates "not yours to do" from "that did not work": the
// sign-in surface refuses an absent or unprivileged bearer with 401/403,
// and a person reading that deserves to be told they lack the standing
// rather than shown a fault.
func (e *Refusal) Denied() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

// Rule is the third kind of no, and it is neither of the other two: the
// request was well formed and the person had every standing for it, and
// the surface refused the state it would have been left in. Its own
// sentence is the whole explanation, so a screen shows that and adds
// nothing.
func (e *Refusal) Rule() bool { return e.Status == http.StatusConflict }

// Admin is one signed-in person's own reach into the administration
// surface: their bearer, their standing, nothing borrowed. The bearer is
// fetched per call from the shell's custody, so a session that has
// outlived its first access token still acts with a living one.
type Admin struct {
	base   string
	bearer func() (string, error)
	cl     *http.Client
}

// People is everyone the sign-in surface knows, with the groups their
// token would carry and whether they have a passkey yet.
func (a *Admin) People(ctx context.Context) ([]Person, error) {
	var out []Person
	if err := a.do(ctx, http.MethodGet, "/api/admin/users", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// MintInvite asks for a fresh single-use enrolment invite for somebody who
// already exists, with the surface's own default lifetime.
func (a *Admin) MintInvite(ctx context.Context, username string) (Invite, error) {
	var out Invite
	err := a.do(ctx, http.MethodPost, "/api/admin/invites",
		map[string]any{"username": username}, &out)
	return out, err
}

// Disable takes somebody's sign-in away. It is the surface's own word for
// it, and it is reversible there — nothing is destroyed here.
func (a *Admin) Disable(ctx context.Context, username string) error {
	return a.do(ctx, http.MethodPost,
		"/api/admin/users/"+url.PathEscape(username)+"/status",
		map[string]any{"status": "disabled"}, nil)
}

// Enable is Disable undone: the surface accepts them again.
func (a *Admin) Enable(ctx context.Context, username string) error {
	return a.do(ctx, http.MethodPost,
		"/api/admin/users/"+url.PathEscape(username)+"/status",
		map[string]any{"status": "active"}, nil)
}

// Create names a new person. They exist from here on but cannot sign in
// until an invite enrolls their passkey — creation grants existence, never
// admission, which is the surface's own rule and stays there.
func (a *Admin) Create(ctx context.Context, username, displayName string, groups []string) error {
	return a.do(ctx, http.MethodPost, "/api/admin/users", map[string]any{
		"username": username, "display_name": displayName, "groups": groups,
	}, nil)
}

// SetGroups replaces somebody's group memberships — the names their next
// token carries as roles. The surface's own rules apply, the last-admin
// refusal among them.
func (a *Admin) SetGroups(ctx context.Context, username string, groups []string) error {
	return a.do(ctx, http.MethodPost,
		"/api/admin/users/"+url.PathEscape(username)+"/groups",
		map[string]any{"groups": groups}, nil)
}

// Groups is every group name the surface knows — what a screen offers
// where a person types memberships.
func (a *Admin) Groups(ctx context.Context) ([]string, error) {
	var out []string
	if err := a.do(ctx, http.MethodGet, "/api/admin/groups", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Client is one application that signs people in through this deployment's
// sign-in service — the shape the surface publishes, consumed as JSON.
type Client struct {
	ClientID     string   `json:"client_id"`
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
}

// Clients is every application registered to sign people in.
func (a *Admin) Clients(ctx context.Context) ([]Client, error) {
	var out []Client
	if err := a.do(ctx, http.MethodGet, "/api/admin/clients", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateClient registers an application: an id, a name to show people,
// and the exact redirect URIs it may return them to.
func (a *Admin) CreateClient(ctx context.Context, id, name string, redirectURIs []string) error {
	return a.do(ctx, http.MethodPost, "/api/admin/clients", map[string]any{
		"client_id": id, "name": name, "redirect_uris": redirectURIs,
	}, nil)
}

// DeleteClient unregisters an application. Sign-ins it already completed
// are history and stay; new ones stop.
func (a *Admin) DeleteClient(ctx context.Context, id string) error {
	return a.do(ctx, http.MethodDelete, "/api/admin/clients/"+url.PathEscape(id), nil, nil)
}

// do carries one call as this person. A refusal comes back as a *Refusal
// carrying what the surface said; anything else is the surface being
// unreachable, which is a different thing and reads differently on screen.
func (a *Admin) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, rdr)
	if err != nil {
		return err
	}
	bearer, err := a.bearer()
	if err != nil {
		return fmt.Errorf("this session's credential is spent: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.cl.Do(req)
	if err != nil {
		return fmt.Errorf("the sign-in surface did not answer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		var said struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&said)
		msg := said.Error
		if msg == "" {
			msg = resp.Status
		}
		return &Refusal{Status: resp.StatusCode, Msg: msg}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("the sign-in surface answered something unreadable: %w", err)
	}
	return nil
}
