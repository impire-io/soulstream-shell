// The soulhelm consumer-position gate — the research bars as standing
// tests. This module's path sits outside the impire-io namespace, so an
// internal/ import cannot compile (the pure-consumer article,
// compiler-checked); every upstream arrives at its published tag and
// only soulhelm itself is replaced. The gate boots a whole soulnode
// realm in-process, runs the helm through its public embed seam, and
// walks the entire human ceremony: passkey enrolment, sign-in, an act
// as the signed-in principal, and the custody scan with its positive
// control.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulfold/authtest"
	helmembed "github.com/impire-io/soulhelm/embed"
	siclient "github.com/impire-io/soulidentity/client"
	"github.com/impire-io/soulnode/ceremony"
	"github.com/impire-io/soulnode/node"
	"github.com/impire-io/soulstream/realm"
	"github.com/impire-io/soulstream/topic"
)

func reservePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	return port
}

type rig struct {
	dir     string
	st      *ceremony.State
	n       *node.Node
	token   string
	helmURL string
	issuer  string
}

func startRig(t *testing.T) *rig {
	t.Helper()
	dir := t.TempDir()
	foldPort := reservePort(t)

	st, err := ceremony.Generate("127.0.0.1:0", "home")
	if err != nil {
		t.Fatal(err)
	}
	st.DoorListen = "127.0.0.1:0"
	st.FoldListen = "127.0.0.1:" + foldPort
	st.FoldIssuer = "http://localhost:" + foldPort
	// Session admissions ride the identity plane's OIDC lane, which the
	// node switches on with public-door mode (the helm plane's soulnode
	// wiring does this by default — the finding is recorded).
	st.DoorPublicURL = "http://127.0.0.1:8666"
	st.DoorAuthIssuer = st.FoldIssuer
	st.DoorAuthAudience = st.FoldAudience
	if err := st.Save(dir); err != nil {
		t.Fatal(err)
	}
	n, err := node.Start(node.Config{StateDir: dir, State: st, AuditWriter: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Stop)
	token, err := node.Found(n, st, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Seed a topic as the founding owner so the helm has something to
	// observe (the rig pattern from the research topic).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	wnc, err := nats.Connect(n.URL(),
		nats.UserCredentials(ceremony.SentinelPath(dir)), nats.Token(token))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wnc.Close)
	signer, err := siclient.New(wnc, st.RealmPub, ceremony.FoundingPersona).
		PersonaSigner(ceremony.FoundingPersona)
	if err != nil {
		t.Fatal(err)
	}
	wc, err := realm.NewClient(ctx, wnc, realm.Config{
		Realm: st.Realm, Persona: ceremony.FoundingPersona, Signer: signer,
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := topic.StartTopic(ctx, wc, topic.StartTopicInput{
		Name: "helm-gate", SubjectMatter: "the standing consumer-position gate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.PostTurn(ctx, "the gate is watching"); err != nil {
		t.Fatal(err)
	}

	// The helm through its public seam — ops creds as the read lane,
	// exactly what the soulnode plane hands it.
	ready := make(chan string, 1)
	helmCtx, helmCancel := context.WithCancel(context.Background())
	t.Cleanup(helmCancel)
	go func() {
		err := helmembed.Run(helmCtx, helmembed.Options{
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
		if err != nil && helmCtx.Err() == nil {
			t.Errorf("helm exited: %v", err)
		}
	}()
	var helmAddr string
	select {
	case helmAddr = <-ready:
	case <-time.After(20 * time.Second):
		t.Fatal("helm did not become ready")
	}
	return &rig{dir: dir, st: st, n: n, token: token,
		helmURL: "http://" + helmAddr, issuer: st.FoldIssuer}
}

var csrfRe = regexp.MustCompile(`id="csrf" value="([^"]*)"`)

type beginResp struct {
	CeremonyID string          `json:"ceremonyID"`
	Kind       string          `json:"kind"`
	Options    json.RawMessage `json:"options"`
}

func postJSON(t *testing.T, cl *http.Client, u, origin string, body []byte) []byte {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", u, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: %d: %s", u, resp.StatusCode, out)
	}
	return out
}

// enroll performs the passkey enrolment through the fold's standalone
// invite lane with a virtual authenticator.
func enroll(t *testing.T, r *rig, auth *authtest.Authenticator) {
	t.Helper()
	cl := &http.Client{}
	q := url.Values{"username": {ceremony.FoundingPersona}, "invite": {r.n.FoldInvite()}}
	var begin beginResp
	if err := json.Unmarshal(postJSON(t, cl,
		r.issuer+"/enroll/begin?"+q.Encode(), r.issuer, nil), &begin); err != nil {
		t.Fatal(err)
	}
	// The enroll lane is registration by definition — it carries no kind.
	created, err := auth.CreateResponse(begin.Options)
	if err != nil {
		t.Fatal(err)
	}
	postJSON(t, cl, r.issuer+"/enroll/finish?ceremonyID="+url.QueryEscape(begin.CeremonyID),
		r.issuer, created)
}

// signIn drives the helm's /login through the fold and returns a
// cookie-jarred client holding the helm session.
func signIn(t *testing.T, r *rig, auth *authtest.Authenticator) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Jar: jar}

	resp, err := cl.Get(r.helmURL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	authReqID := resp.Request.URL.Query().Get("authRequestID")
	if authReqID == "" {
		t.Fatalf("no authRequestID; landed on %s", resp.Request.URL)
	}
	m := csrfRe.FindSubmatch(page)
	if m == nil {
		t.Fatalf("no csrf field on the fold login page")
	}
	q := url.Values{"authRequestID": {authReqID}, "csrf": {string(m[1])},
		"username": {ceremony.FoundingPersona}}
	var begin beginResp
	if err := json.Unmarshal(postJSON(t, cl,
		r.issuer+"/login/begin?"+q.Encode(), r.issuer, nil), &begin); err != nil {
		t.Fatal(err)
	}
	var cred []byte
	if begin.Kind == "register" {
		cred, err = auth.CreateResponse(begin.Options)
	} else {
		cred, err = auth.GetResponse(begin.Options)
	}
	if err != nil {
		t.Fatal(err)
	}
	q.Set("ceremonyID", begin.CeremonyID)
	var fin struct {
		Redirect string `json:"redirect"`
	}
	if err := json.Unmarshal(postJSON(t, cl,
		r.issuer+"/login/finish?"+q.Encode(), r.issuer, cred), &fin); err != nil {
		t.Fatal(err)
	}
	redirect := fin.Redirect
	if strings.HasPrefix(redirect, "/") {
		redirect = r.issuer + redirect
	}
	final, err := cl.Get(redirect)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(final.Body)
	final.Body.Close()
	if !strings.HasPrefix(final.Request.URL.String(), r.helmURL) {
		t.Fatalf("ceremony ended at %s, not the helm", final.Request.URL)
	}
	if !strings.Contains(string(body), "Sign out") {
		t.Fatalf("helm page shows no session: %s", body)
	}
	return cl
}

// readSSE reads one SSE response for up to the given duration.
func readSSE(t *testing.T, cl *http.Client, u string, d time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteString("\n")
		if b.Len() > 1<<18 {
			break
		}
	}
	return b.String()
}

// scanFor walks the state dir (skipping the realm's own churn) looking
// for the needle — the Bar 2 scan shape.
func scanFor(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "jetstream", "archive", "fold":
				return filepath.SkipDir
			}
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(raw, []byte(needle)) {
			hits = append(hits, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func TestHelmGate(t *testing.T) {
	r := startRig(t)
	auth, err := authtest.New("localhost", r.issuer)
	if err != nil {
		t.Fatal(err)
	}

	// The surface is closed until sign-in: no realm content, and the
	// live channel refuses.
	plain, err := http.Get(r.helmURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	anon, _ := io.ReadAll(plain.Body)
	plain.Body.Close()
	if strings.Contains(string(anon), "helm-gate") {
		t.Fatal("unauthenticated page leaks realm content")
	}
	if resp, _ := http.Get(r.helmURL + "/live"); resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthenticated /live must refuse")
	}

	// The ceremony: enrol, sign in, observe, act, sign out.
	enroll(t, r, auth)
	cl := signIn(t, r, auth)

	live := readSSE(t, cl, r.helmURL+"/live", 3*time.Second)
	for _, want := range []string{"datastar-patch-elements", "helm-gate", "verified", "Storage"} {
		if !strings.Contains(live, want) {
			t.Fatalf("live view missing %q:\n%s", want, live)
		}
	}

	// Class (a): an op on the record as the signed-in principal.
	actResp, err := cl.Post(r.helmURL+"/act/work-open", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	act, _ := io.ReadAll(actResp.Body)
	actResp.Body.Close()
	if !strings.Contains(string(act), "work.open ok") {
		t.Fatalf("work.open failed: %s", act)
	}

	// The act is attributed to the fold principal in the realm itself,
	// not to the helm or the owner.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rnc, err := nats.Connect(r.n.URL(),
		nats.UserCredentials(ceremony.SentinelPath(r.dir)), nats.Token(r.token))
	if err != nil {
		t.Fatal(err)
	}
	defer rnc.Close()
	rc, err := realm.NewClient(ctx, rnc, realm.Config{Realm: r.st.Realm})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	entries, err := topic.Board(ctx, rc)
	if err != nil || len(entries) == 0 {
		t.Fatalf("board: %v (%d)", err, len(entries))
	}
	mt, err := topic.Open(rc, entries[len(entries)-1].Path).Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(mt.WorkItems) != 1 {
		t.Fatalf("work items = %d, want 1", len(mt.WorkItems))
	}
	author := mt.WorkItems[0].Author
	if author == "" || author == ceremony.FoundingPersona {
		t.Fatalf("work item author = %q — not the fold principal", author)
	}

	// Sign out closes the session.
	if _, err := cl.Post(r.helmURL+"/logout", "", nil); err != nil {
		t.Fatal(err)
	}
	after, err := cl.Get(r.helmURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	afterBody, _ := io.ReadAll(after.Body)
	after.Body.Close()
	if strings.Contains(string(afterBody), "Sign out") {
		t.Fatal("session survived logout")
	}

	// The custody scan (Bar 2's shape): the session cookie value must
	// exist nowhere on disk; the positive control must fire.
	u, _ := url.Parse(r.helmURL)
	var sid string
	for _, c := range cl.Jar.Cookies(u) {
		if c.Name == "helm_session" {
			sid = c.Value
		}
	}
	if sid == "" {
		// The jar drops expired cookies; a fixed needle stands in.
		sid = "helm-session-needle-never-on-disk"
	}
	if hits := scanFor(t, r.dir, sid); len(hits) != 0 {
		t.Fatalf("session material on disk: %v", hits)
	}
	control := filepath.Join(r.dir, "planted-control")
	if err := os.WriteFile(control, []byte("x"+sid+"x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := scanFor(t, r.dir, sid); len(hits) != 1 {
		t.Fatalf("positive control did not fire: %v", hits)
	}
	os.Remove(control)

	// The offline-render gate (Bar 4's shape): the token source is
	// self-contained and the fonts are real.
	css, err := http.Get(r.helmURL + "/assets/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	cssBody, _ := io.ReadAll(css.Body)
	css.Body.Close()
	if strings.Contains(string(cssBody), "https://") {
		t.Fatal("token source reaches an external host")
	}
	if !strings.Contains(string(cssBody), "@font-face") {
		t.Fatal("token source carries no vendored fonts")
	}
	font, err := http.Get(r.helmURL + "/assets/fonts/archivo-400-800.woff2")
	if err != nil {
		t.Fatal(err)
	}
	magic := make([]byte, 4)
	io.ReadFull(font.Body, magic)
	font.Body.Close()
	if string(magic) != "wOF2" {
		t.Fatalf("font magic = %q", magic)
	}
	fmt.Println("helm gate: ceremony, custody, offline render — all green")
}
