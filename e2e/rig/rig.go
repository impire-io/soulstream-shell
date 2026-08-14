// Package rig boots a whole running deployment in one process — a soulnode
// (its embedded server, identity plane and bundled fold) and the shell
// through its public embed seam — and walks the human ceremony against it:
// passkey enrolment with a virtual authenticator, sign-in, a cookie-jarred
// client holding the session.
//
// It stands up two deployment shapes, because which modules a build runs is
// the deployment's answer and not the shell's. Start is the bundled shape:
// the node runs its own sign-in plane, and the people who sign in are the
// people it administers. StartExternalIdP is the other one: that plane is
// off, sessions ride an authorization server standing outside the product
// entirely, and there is nobody here to administer. Everything the shell is
// handed in either arm comes from the deployment's own declaration
// (ceremony.State), never from a constant written here — so what the gate
// measures is the product's wiring and not the test's opinion of it.
//
// It also stands up the least a shell can run on (StartIssuer): an
// authorization server alone, with no node, no record and no
// module-support layer beside it. That is the position a module written
// outside the product composes from, and the arm that measures one starts
// there.
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
	foldembed "github.com/impire-io/soulstream-idp/embed"
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
	// Invite is the single-use enrolment invite the deployment's sign-in
	// surface minted for the founding person — the bundled plane's in one
	// arm, the external authorization server's in the other.
	Invite string
	// AdminBase is what this deployment declared about administering its
	// own people, read back so a test can assert against the declaration
	// rather than against a guess at it. Empty in the external-IdP arm.
	AdminBase string

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

// Start founds a realm in dir and serves it with the deployment's own
// sign-in plane running: the node, then the shell on the ops read lane the
// soulnode plane hands it. A failure leaves nothing running.
func Start(dir string) (*Rig, error) {
	return start(dir, false)
}

// StartExternalIdP founds the same realm in the other deployment shape: the
// sign-in plane off, and sessions signing in against an authorization
// server outside the product altogether — a fold run standalone through its
// own public embed seam, on its own store, which this node only knows the
// URL of.
//
// It is the shape a deployment takes when its people already live
// somewhere: the node holds the record, somebody else holds the people.
func StartExternalIdP(dir string) (*Rig, error) {
	return start(dir, true)
}

// start stands up one arm. The only thing the two arms decide differently
// is where the authorization server comes from; everything the shell is
// handed is read back off the deployment's own state either way.
func start(dir string, external bool) (*Rig, error) {
	asPort, err := reservePort()
	if err != nil {
		return nil, fmt.Errorf("rig: reserve sign-in port: %w", err)
	}
	st, err := ceremony.Generate("127.0.0.1:0", "home")
	if err != nil {
		return nil, fmt.Errorf("rig: ceremony: %w", err)
	}
	st.DoorListen = "127.0.0.1:0"
	// The node composes a shell plane of its own on the ceremony's default
	// port. This rig drives the shell through its public embed seam instead,
	// so the plane it composes must not fight a shell already on this
	// machine — or the second rig — for 8500.
	st.HelmListen = "127.0.0.1:0"
	// Session admissions ride the identity plane's OIDC lane, which the node
	// switches on with public-door mode (the shell plane's soulnode wiring
	// does this by default — the finding is recorded).
	st.DoorPublicURL = "http://127.0.0.1:8666"

	ctx, cancel := context.WithCancel(context.Background())
	var invite string
	if external {
		// The authorization server this deployment does not run. It starts
		// first: the identity plane's callout validator discovers the issuer
		// at startup, and so does the shell.
		st.FoldEnabled = false
		st.DoorAuthIssuer = "http://localhost:" + asPort
		st.DoorAuthAudience = "soulstream-external"
		invite, err = startExternalAS(ctx, filepath.Join(dir, "external-as"),
			st.DoorAuthIssuer, "127.0.0.1:"+asPort, st.DoorAuthAudience,
			ceremony.FoundingPersona, []string{"admin", "realm"})
		if err != nil {
			cancel()
			return nil, err
		}
	} else {
		st.FoldListen = "127.0.0.1:" + asPort
		st.FoldIssuer = "http://localhost:" + asPort
		st.DoorAuthIssuer = st.FoldIssuer
		st.DoorAuthAudience = st.FoldAudience
	}
	if err := st.Save(dir); err != nil {
		cancel()
		return nil, fmt.Errorf("rig: save state: %w", err)
	}
	n, err := node.Start(node.Config{StateDir: dir, State: st, AuditWriter: io.Discard})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("rig: node: %w", err)
	}
	token, err := node.Found(n, st, dir)
	if err != nil {
		cancel()
		n.Stop()
		return nil, fmt.Errorf("rig: found: %w", err)
	}
	if !external {
		invite = n.FoldInvite()
	}

	issuer, _ := st.SessionIssuer()
	ready := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- shellembed.Run(ctx, shellembed.Options{
			Listen:       "127.0.0.1:0",
			NATSURL:      n.URL(),
			CredsPath:    ceremony.UserCredsPath(dir, "ops"),
			CredsUser:    "ops",
			SentinelPath: ceremony.SentinelPath(dir),
			Realm:        st.Realm,
			Account:      st.RealmPub,
			Issuer:       issuer,
			// The deployment's own declaration, not the rig's: whichever arm
			// this is, the fact reaching the shell is the one the product
			// computes for a real soulnode.
			AdminBase: st.AdminSurface(),
			Ready:     func(addr string) { ready <- addr },
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
		ShellURL: "http://" + addr, Issuer: issuer,
		Invite: invite, AdminBase: st.AdminSurface(), cancel: cancel,
		signers: map[string]identity.Signer{},
	}, nil
}

// Issuer is an authorization server and nothing else: the whole of what a
// shell needs to run, with no part of this product beside it. No node, no
// record, no module-support layer — a deployment composing the frame with
// modules of its own has exactly this much.
//
// It is the position an outside module is measured from, and the reason
// it can be: everything the frame needs is here, and everything Soulstream
// is, is not.
type Issuer struct {
	// URL is where people sign in, and what a shell composed against this
	// discovers itself through.
	URL string
	// Invite is the single-use enrolment invite it minted for the seeded
	// person.
	Invite string

	cancel context.CancelFunc
}

// StartIssuer runs one, seeding the one person who will sign in.
func StartIssuer(dir, username string) (*Issuer, error) {
	port, err := reservePort()
	if err != nil {
		return nil, fmt.Errorf("rig: reserve sign-in port: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	url := "http://localhost:" + port
	invite, err := startExternalAS(ctx, dir, url, "127.0.0.1:"+port,
		"soulstream-shell-outside", username, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	return &Issuer{URL: url, Invite: invite, cancel: cancel}, nil
}

// Close drains it.
func (i *Issuer) Close() { i.cancel() }

// Enroll spends the invite on a passkey for the seeded person.
func (i *Issuer) Enroll(auth *authtest.Authenticator, username string) error {
	return enroll(i.URL, auth, username, i.Invite)
}

// SignInTo walks the ceremony against whatever shell is serving at shellURL
// and returns a cookie-jarred client holding that shell's session, plus the
// page it landed on.
func (i *Issuer) SignInTo(shellURL string, auth *authtest.Authenticator, username string,
) (*http.Client, string, error) {
	return signInTo(shellURL, i.URL, auth, username)
}

// startExternalAS runs a fold standalone, through its own public embed
// seam, on its own embedded store — an authorization server the product
// composes no part of and can only reach over HTTP. It returns the
// single-use invite that enrols the named person's passkey there.
//
// Where it stands in for the deployment's own sign-in plane, the seeded
// groups are deliberately that plane's own, admin included: the only
// difference between those two arms is the shape of the deployment, never
// the standing of the person signing in.
func startExternalAS(ctx context.Context, stateDir, issuer, listen, audience, username string,
	roles []string,
) (string, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", fmt.Errorf("rig: external AS state: %w", err)
	}
	ready := make(chan string, 1)
	errCh := make(chan error, 1)
	invites := make(chan string, 1)
	go func() {
		errCh <- foldembed.Run(ctx, foldembed.Options{
			Issuer: issuer, Listen: listen, StateDir: stateDir,
			TokenAudience: audience, EnableDCR: true,
			SeedUsers: []foldembed.SeedUser{{
				Username: username, DisplayName: username, Roles: roles,
			}},
			InviteSink: func(_, token string) { invites <- token },
			Ready:      func(addr string) { ready <- addr },
		})
	}()
	select {
	case <-ready:
	case err := <-errCh:
		return "", fmt.Errorf("rig: external AS: %w", err)
	case <-time.After(20 * time.Second):
		return "", errors.New("rig: the external authorization server did not become ready")
	}
	select {
	case token := <-invites:
		return token, nil
	default:
		return "", errors.New("rig: the external authorization server minted no invite")
	}
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

// Enroll performs the passkey enrolment through the sign-in surface's
// standalone invite lane with a virtual authenticator, spending the invite
// the deployment minted for the founding person.
func (r *Rig) Enroll(auth *authtest.Authenticator, username string) error {
	return r.EnrollWith(auth, username, r.Invite)
}

// EnrollWith spends a named invite instead — the way somebody handed one
// uses it, and the only way to find out whether the sign-in surface really
// issued the one a screen showed: a token it never minted is refused here.
func (r *Rig) EnrollWith(auth *authtest.Authenticator, username, invite string) error {
	return enroll(r.Issuer, auth, username, invite)
}

// SignIn drives the shell's /login through the fold and returns a
// cookie-jarred client holding the shell session, plus the page it landed
// on.
func (r *Rig) SignIn(auth *authtest.Authenticator, username string) (*http.Client, string, error) {
	return signInTo(r.ShellURL, r.Issuer, auth, username)
}

// enroll and signInTo are the ceremony itself, against whichever
// authorization server and whichever shell they are pointed at: the rig's
// own pair in every arm that runs the product, and an authorization server
// standing alone in the arm that runs none of it (see Issuer).
func enroll(issuer string, auth *authtest.Authenticator, username, invite string) error {
	cl := &http.Client{}
	q := url.Values{"username": {username}, "invite": {invite}}
	raw, err := postJSON(cl, issuer+"/enroll/begin?"+q.Encode(), issuer, nil)
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
	_, err = postJSON(cl, issuer+"/enroll/finish?ceremonyID="+
		url.QueryEscape(begin.CeremonyID), issuer, created)
	return err
}

func signInTo(shellURL, issuer string, auth *authtest.Authenticator, username string,
) (*http.Client, string, error) {
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Jar: jar}

	resp, err := cl.Get(shellURL + "/login")
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
	raw, err := postJSON(cl, issuer+"/login/begin?"+q.Encode(), issuer, nil)
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
	raw, err = postJSON(cl, issuer+"/login/finish?"+q.Encode(), issuer, cred)
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
		redirect = issuer + redirect
	}
	final, err := cl.Get(redirect)
	if err != nil {
		return nil, "", fmt.Errorf("rig: callback: %w", err)
	}
	body, _ := io.ReadAll(final.Body)
	_ = final.Body.Close()
	if !strings.HasPrefix(final.Request.URL.String(), shellURL) {
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
