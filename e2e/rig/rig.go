// Package rig boots a whole running deployment in one process — a soulnode
// (its embedded server, identity plane and bundled fold) and the shell
// through its public embed seam — and walks the human ceremony against it:
// passkey enrolment with a virtual authenticator, sign-in, a cookie-jarred
// client holding the session.
//
// The consumer-position gate and the screens helper both drive this, so
// what a person is shown in a screenshot is what the gate measures. Like
// the gate module that holds it, this package sits outside the impire-io
// path, so an internal/ import of the shell cannot compile here.
package rig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/identity"
	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/registry"
	siclient "github.com/impire-io/soulstream-identity/client"
	"github.com/impire-io/soulstream-idp/authtest"
	shellembed "github.com/impire-io/soulstream-shell/embed"
	"github.com/impire-io/soulstream/ceremony"
	"github.com/impire-io/soulstream/node"
)

// SessionCookie is the name of the cookie the shell hands a signed-in
// person. Nothing durable hangs off it — the session it names lives in the
// shell's memory only.
const SessionCookie = "helm_session"

// Rig is one running deployment. Close drains it.
type Rig struct {
	Dir      string
	State    *ceremony.State
	Node     *node.Node
	Token    string
	ShellURL string
	Issuer   string

	cancel context.CancelFunc
	conns  []*nats.Conn
	// signers is the signing capability this rig holds for each persona it
	// has admitted — what an operator needs to countersign a claim that they
	// answer for somebody (see Operated).
	signers map[string]identity.Signer
}

func reservePort() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return port, ln.Close()
}

// Start founds a realm in dir and serves it: the node, then the shell on
// the ops read lane the soulnode plane hands it. A failure leaves nothing
// running.
func Start(dir string) (*Rig, error) {
	foldPort, err := reservePort()
	if err != nil {
		return nil, fmt.Errorf("rig: reserve fold port: %w", err)
	}
	st, err := ceremony.Generate("127.0.0.1:0", "home")
	if err != nil {
		return nil, fmt.Errorf("rig: ceremony: %w", err)
	}
	st.DoorListen = "127.0.0.1:0"
	st.FoldListen = "127.0.0.1:" + foldPort
	st.FoldIssuer = "http://localhost:" + foldPort
	// The node composes a shell plane of its own on the ceremony's default
	// port. This rig drives the shell through its public embed seam instead,
	// so the plane it composes must not fight a shell already on this
	// machine — or the second rig — for 8500.
	st.HelmListen = "127.0.0.1:0"
	// Session admissions ride the identity plane's OIDC lane, which the node
	// switches on with public-door mode (the shell plane's soulnode wiring
	// does this by default — the finding is recorded).
	st.DoorPublicURL = "http://127.0.0.1:8666"
	st.DoorAuthIssuer = st.FoldIssuer
	st.DoorAuthAudience = st.FoldAudience
	if err := st.Save(dir); err != nil {
		return nil, fmt.Errorf("rig: save state: %w", err)
	}
	n, err := node.Start(node.Config{StateDir: dir, State: st, AuditWriter: io.Discard})
	if err != nil {
		return nil, fmt.Errorf("rig: node: %w", err)
	}
	token, err := node.Found(n, st, dir)
	if err != nil {
		n.Stop()
		return nil, fmt.Errorf("rig: found: %w", err)
	}

	ready := make(chan string, 1)
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		errCh <- shellembed.Run(ctx, shellembed.Options{
			Listen:       "127.0.0.1:0",
			NATSURL:      n.URL(),
			CredsPath:    ceremony.UserCredsPath(dir, "ops"),
			CredsUser:    "ops",
			SentinelPath: ceremony.SentinelPath(dir),
			Realm:        st.Realm,
			Account:      st.RealmPub,
			Issuer:       st.FoldIssuer,
			Ready:        func(addr string) { ready <- addr },
		})
	}()
	var addr string
	select {
	case addr = <-ready:
	case err := <-errCh:
		cancel()
		n.Stop()
		return nil, fmt.Errorf("rig: shell: %w", err)
	case <-time.After(20 * time.Second):
		cancel()
		n.Stop()
		return nil, errors.New("rig: the shell did not become ready")
	}
	return &Rig{
		Dir: dir, State: st, Node: n, Token: token,
		ShellURL: "http://" + addr, Issuer: st.FoldIssuer, cancel: cancel,
		signers: map[string]identity.Signer{},
	}, nil
}

// Close drains everything this rig started; the directory is the caller's.
func (r *Rig) Close() {
	r.cancel()
	for _, nc := range r.conns {
		nc.Close()
	}
	r.Node.Stop()
}

// Owner returns a realm client writing as the founding persona, signed with
// that persona's own key. The connection closes with the rig.
func (r *Rig) Owner(ctx context.Context) (*realm.Client, error) {
	nc, err := nats.Connect(r.Node.URL(),
		nats.UserCredentials(ceremony.SentinelPath(r.Dir)), nats.Token(r.Token))
	if err != nil {
		return nil, fmt.Errorf("rig: owner admission: %w", err)
	}
	r.conns = append(r.conns, nc)
	signer, err := siclient.New(nc, r.State.RealmPub, ceremony.FoundingPersona).
		PersonaSigner(ceremony.FoundingPersona)
	if err != nil {
		return nil, fmt.Errorf("rig: owner signer: %w", err)
	}
	rc, err := realm.NewClient(ctx, nc, realm.Config{
		Realm: r.State.Realm, Persona: ceremony.FoundingPersona, Signer: signer,
	})
	if err != nil {
		return nil, fmt.Errorf("rig: owner client: %w", err)
	}
	r.signers[ceremony.FoundingPersona] = signer
	return rc, nil
}

// Reader returns a realm client on the ops read lane — the position the
// shell itself reads from, with no persona and no signer.
func (r *Rig) Reader(ctx context.Context) (*realm.Client, *nats.Conn, error) {
	nc, err := nats.Connect(r.Node.URL(),
		nats.UserCredentials(ceremony.SentinelPath(r.Dir)), nats.Token(r.Token))
	if err != nil {
		return nil, nil, fmt.Errorf("rig: reader admission: %w", err)
	}
	r.conns = append(r.conns, nc)
	rc, err := realm.NewClient(ctx, nc, realm.Config{Realm: r.State.Realm})
	if err != nil {
		return nil, nil, fmt.Errorf("rig: reader client: %w", err)
	}
	return rc, nc, nil
}

// admit mints an admitted principal through the identity plane's operator
// lane and returns a realm client that writes as that persona, signed with
// that persona's own key. The signing capability is remembered, so a persona
// this rig admitted can later countersign for somebody it operates.
func (r *Rig) admit(ctx context.Context, persona string) (*realm.Client, identity.Signer, error) {
	minted, err := r.Node.Ops().MintCreds(r.State.RealmPub, persona)
	if err != nil {
		return nil, nil, fmt.Errorf("rig: mint %q: %w", persona, err)
	}
	path := filepath.Join(r.Dir, persona+".creds")
	if err := os.WriteFile(path, []byte(minted.Creds), 0o600); err != nil {
		return nil, nil, fmt.Errorf("rig: write %q creds: %w", persona, err)
	}
	nc, err := nats.Connect(r.Node.URL(), nats.UserCredentials(path))
	if err != nil {
		return nil, nil, fmt.Errorf("rig: %q admission: %w", persona, err)
	}
	r.conns = append(r.conns, nc)
	signer, err := siclient.New(nc, r.State.RealmPub, persona).PersonaSigner(persona)
	if err != nil {
		return nil, nil, fmt.Errorf("rig: %q signer: %w", persona, err)
	}
	rc, err := realm.NewClient(ctx, nc, realm.Config{
		Realm: r.State.Realm, Persona: persona, Signer: signer,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("rig: %q client: %w", persona, err)
	}
	r.signers[persona] = signer
	return rc, signer, nil
}

// Voice mints a second admitted principal and returns a realm client that
// writes as that persona. With a display name it publishes the persona's
// directory card too, so readers have a name to show. The card names no
// operator, which is the directory's way of saying this voice answers for
// itself.
func (r *Rig) Voice(ctx context.Context, persona, display string) (*realm.Client, error) {
	rc, signer, err := r.admit(ctx, persona)
	if err != nil {
		return nil, err
	}
	if display == "" {
		return rc, nil
	}
	return rc, r.publish(ctx, rc, card(persona, display, signer), persona)
}

// Operated mints a persona the record says somebody else answers for: its
// directory card names an operator and carries that operator's
// countersignature over the claim.
//
// It is the honest way to put a machine voice on the record. Soulstream has
// no human/agent field to set — the persona `kind` taxonomy was removed
// outright, because the protocol cannot verify what controls a key — and the
// operator claim is what replaced it: an audit fact about who answers for a
// voice, verifiable because the operator signs it.
//
// The hand-off is the real one, not a shortcut. The operator presses the
// stamp on its own side and only the token travels; the operated persona
// publishes its own card with the token in it. Nobody ever writes on
// somebody else's card. The operator must therefore be a persona this rig
// already holds the signing capability for — the owner, or a Voice it
// minted.
func (r *Rig) Operated(ctx context.Context, persona, display, operator string) (*realm.Client, error) {
	op, ok := r.signers[operator]
	if !ok {
		return nil, fmt.Errorf("rig: %q cannot attest for %q — this rig holds no signer for it",
			operator, persona)
	}
	rc, signer, err := r.admit(ctx, persona)
	if err != nil {
		return nil, err
	}
	token, err := registry.NewAttestationToken(op, operator, persona, signer.PublicKey())
	if err != nil {
		return nil, fmt.Errorf("rig: %q attests for %q: %w", operator, persona, err)
	}
	stamp, err := registry.ParseAttestationToken(token)
	if err != nil {
		return nil, fmt.Errorf("rig: %q reads the attestation token: %w", persona, err)
	}
	p := card(persona, display, signer)
	p.OperatedBy = operator
	p.OperatorAttestation = &registry.OperatorAttestation{
		OperatedKey: stamp.OperatedKey, Sig: stamp.Sig,
	}
	return rc, r.publish(ctx, rc, p, persona)
}

// card is a persona's own directory entry, keyed to its own signing key.
func card(persona, display string, signer identity.Signer) registry.Profile {
	p := registry.Profile{Name: persona, DisplayName: display, CreatedAt: time.Now()}
	p.SigningKey = &registry.SigningKeyInfo{Ed25519: signer.PublicKey(), Since: p.CreatedAt}
	return p
}

func (r *Rig) publish(ctx context.Context, rc *realm.Client, p registry.Profile,
	persona string,
) error {
	if err := registry.Publish(ctx, rc, p); err != nil {
		return fmt.Errorf("rig: publish %q profile: %w", persona, err)
	}
	return nil
}

// Name gives a persona a name on screen: its entry in the realm's own
// persona directory, published as that persona over a lane the operator
// mints for them.
//
// It is how a fold-issued person comes to be called something. The fold
// mints the id and keeps the name to itself — nothing in the token it hands
// the shell carries one — so the directory is the only place a human name
// can live, and the directory takes an entry from nobody but the persona it
// belongs to.
func (r *Rig) Name(ctx context.Context, persona, display string) error {
	_, err := r.Voice(ctx, persona, display)
	return err
}

var csrfRe = regexp.MustCompile(`id="csrf" value="([^"]*)"`)

type beginResp struct {
	CeremonyID string          `json:"ceremonyID"`
	Kind       string          `json:"kind"`
	Options    json.RawMessage `json:"options"`
}

func postJSON(cl *http.Client, u, origin string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	resp, err := cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST %s: %d: %s", u, resp.StatusCode, out)
	}
	return out, nil
}

// Enroll performs the passkey enrolment through the fold's standalone
// invite lane with a virtual authenticator.
func (r *Rig) Enroll(auth *authtest.Authenticator, username string) error {
	cl := &http.Client{}
	q := url.Values{"username": {username}, "invite": {r.Node.FoldInvite()}}
	raw, err := postJSON(cl, r.Issuer+"/enroll/begin?"+q.Encode(), r.Issuer, nil)
	if err != nil {
		return err
	}
	var begin beginResp
	if err := json.Unmarshal(raw, &begin); err != nil {
		return fmt.Errorf("rig: enroll begin: %w", err)
	}
	// The enroll lane is registration by definition — it carries no kind.
	created, err := auth.CreateResponse(begin.Options)
	if err != nil {
		return fmt.Errorf("rig: authenticator: %w", err)
	}
	_, err = postJSON(cl, r.Issuer+"/enroll/finish?ceremonyID="+
		url.QueryEscape(begin.CeremonyID), r.Issuer, created)
	return err
}

// SignIn drives the shell's /login through the fold and returns a
// cookie-jarred client holding the shell session, plus the page it landed
// on.
func (r *Rig) SignIn(auth *authtest.Authenticator, username string) (*http.Client, string, error) {
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Jar: jar}

	resp, err := cl.Get(r.ShellURL + "/login")
	if err != nil {
		return nil, "", fmt.Errorf("rig: /login: %w", err)
	}
	page, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	authReqID := resp.Request.URL.Query().Get("authRequestID")
	if authReqID == "" {
		return nil, "", fmt.Errorf("rig: no authRequestID; landed on %s", resp.Request.URL)
	}
	m := csrfRe.FindSubmatch(page)
	if m == nil {
		return nil, "", errors.New("rig: no csrf field on the fold login page")
	}
	q := url.Values{"authRequestID": {authReqID}, "csrf": {string(m[1])},
		"username": {username}}
	raw, err := postJSON(cl, r.Issuer+"/login/begin?"+q.Encode(), r.Issuer, nil)
	if err != nil {
		return nil, "", err
	}
	var begin beginResp
	if err := json.Unmarshal(raw, &begin); err != nil {
		return nil, "", fmt.Errorf("rig: login begin: %w", err)
	}
	var cred []byte
	if begin.Kind == "register" {
		cred, err = auth.CreateResponse(begin.Options)
	} else {
		cred, err = auth.GetResponse(begin.Options)
	}
	if err != nil {
		return nil, "", fmt.Errorf("rig: authenticator: %w", err)
	}
	q.Set("ceremonyID", begin.CeremonyID)
	raw, err = postJSON(cl, r.Issuer+"/login/finish?"+q.Encode(), r.Issuer, cred)
	if err != nil {
		return nil, "", err
	}
	var fin struct {
		Redirect string `json:"redirect"`
	}
	if err := json.Unmarshal(raw, &fin); err != nil {
		return nil, "", fmt.Errorf("rig: login finish: %w", err)
	}
	redirect := fin.Redirect
	if strings.HasPrefix(redirect, "/") {
		redirect = r.Issuer + redirect
	}
	final, err := cl.Get(redirect)
	if err != nil {
		return nil, "", fmt.Errorf("rig: callback: %w", err)
	}
	body, _ := io.ReadAll(final.Body)
	_ = final.Body.Close()
	if !strings.HasPrefix(final.Request.URL.String(), r.ShellURL) {
		return nil, "", fmt.Errorf("rig: ceremony ended at %s, not the shell", final.Request.URL)
	}
	return cl, string(body), nil
}

// Cookie returns the shell session cookie a signed-in client holds.
func (r *Rig) Cookie(cl *http.Client) (*http.Cookie, bool) {
	u, err := url.Parse(r.ShellURL)
	if err != nil {
		return nil, false
	}
	for _, c := range cl.Jar.Cookies(u) {
		if c.Name == SessionCookie {
			return c, true
		}
	}
	return nil, false
}

// Post writes a message into a conversation the way the browser does —
// the composer's form, over the person's own session.
func (r *Rig) Post(cl *http.Client, topicPath string, form url.Values) (string, error) {
	u := r.ShellURL + "/act/post-turn"
	if topicPath != "" {
		u += "?topic=" + url.QueryEscape(topicPath)
	}
	resp, err := cl.PostForm(u, form)
	if err != nil {
		return "", fmt.Errorf("rig: post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return string(out), nil
}
