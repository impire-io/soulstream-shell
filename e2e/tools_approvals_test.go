// The tools and approvals gates (hq designs soulstream-shell 0005/0006):
// the two graduated-and-built arcs get their human ends walked against a
// real deployment at published tags — soulnode v0.13.0-rc.10, whose
// identity plane runs the runtime catalog, the guardrail, and the ticket
// store.
//
// Tools: an administrator adds a remote tool from the screen (both halves,
// one act), a person connects their own account through the real OAuth
// redirect ceremony (out to the provider, back through the module's
// callback, completed on the session that started it), the standing shows,
// disconnecting revokes — and the client secret exists nowhere any scan
// reads, control fired.
//
// Approvals: a defer rule loaded live, a real op tripped into a ticket,
// the ticket on the screen, one tap approving it — the person's own signed
// yes, delivered to the originator's tail — and the originator's retry
// serving; the ticket reading spent.
package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	siclient "github.com/impire-io/soulstream-identity/client"
	"github.com/impire-io/soulstream-idp/authtest"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

// opsIdentity is the gate's own management lane: the same ops creds the
// deployment hands the shell, used here to stand in for the operator.
func opsIdentity(t *testing.T, r *rig.Rig) (*siclient.Client, *nats.Conn) {
	t.Helper()
	nc, err := nats.Connect(r.Node.URL(),
		nats.UserCredentials(ceremony.UserCredsPath(r.Dir, "ops")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return siclient.New(nc, r.State.RealmPub, "ops", siclient.WithTimeout(10*time.Second)), nc
}

// noRedirect returns a client sharing cl's cookies that reports redirects
// instead of following them — the ceremony's legs are the assertion.
func noRedirect(cl *http.Client) *http.Client {
	c := *cl
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

func TestToolsGate(t *testing.T) {
	r, _ := startRig(t)
	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		t.Fatal(err)
	}
	cl := signIn(t, r, auth)

	// The stand-in provider: the AS half only — the ceremony's authorize
	// leg is the browser's, which this gate plays by hand.
	var asMu sync.Mutex
	live := ""
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/token" {
			http.NotFound(w, req)
			return
		}
		_ = req.ParseForm()
		asMu.Lock()
		defer asMu.Unlock()
		switch req.Form.Get("grant_type") {
		case "authorization_code":
			live = "rt-0"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at-0", "refresh_token": live, "expires_in": 3600})
		case "refresh_token":
			if req.Form.Get("refresh_token") != live {
				http.Error(w, "stale", http.StatusBadRequest)
				return
			}
			live = "rt-1"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at-1", "refresh_token": live, "expires_in": 3600})
		default:
			http.Error(w, "unsupported", http.StatusBadRequest)
		}
	}))
	defer as.Close()

	// The key is on the spine for everyone, and the screen answers before
	// any tool exists — the empty state is the way to add one.
	screen := get(t, cl, r.ShellURL+"/tools")
	for _, want := range []string{`class="ir on" href="/tools`, "Tools", "No tools yet",
		"Add a tool"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the tools screen is missing %q: %s", want, screen)
		}
	}

	// The administrator adds a remote tool from the screen: both halves,
	// one act, the secret crossing once.
	const secret = "tool-secret-never-anywhere-readable"
	added := postForm(t, cl, r.ShellURL+"/act/tool-add", url.Values{
		"name": {"github"}, "kind": {"remote"},
		"endpoint":    {"https://api.github.invalid/mcp"},
		"description": {"GitHub"},
		"auth_url":    {as.URL + "/auth"},
		"token_url":   {as.URL + "/token"},
		"client_id":   {"shell"}, "client_secret": {secret},
		"redirect_uri": {r.ShellURL + "/tools/callback"},
	})
	if !strings.Contains(added, "github is available now") {
		t.Fatalf("adding did not answer: %s", added)
	}

	// The person connects: out to the provider…
	bare := noRedirect(cl)
	resp, err := bare.Get(r.ShellURL + "/tools/connect?name=github")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, err := resp.Location()
	if err != nil {
		t.Fatalf("connect did not redirect: %v", err)
	}
	if !strings.HasPrefix(loc.String(), as.URL) ||
		loc.Query().Get("code_challenge") == "" || loc.Query().Get("state") == "" {
		t.Fatalf("the authorize leg is malformed: %s", loc)
	}
	if loc.Query().Get("redirect_uri") != r.ShellURL+"/tools/callback" {
		t.Fatalf("the ceremony returns somewhere else: %s", loc.Query().Get("redirect_uri"))
	}
	// …and back, the provider's own redirect played by hand.
	back := get(t, cl, r.ShellURL+"/tools/callback?state="+
		url.QueryEscape(loc.Query().Get("state"))+"&code=consented")
	if !strings.Contains(back, "Connected.") {
		t.Fatalf("the callback did not land connected: %s", back)
	}
	if !strings.Contains(back, ">connected</span>") ||
		!strings.Contains(back, "/act/tool-disconnect?name=github") {
		t.Fatalf("the standing does not show: %s", back)
	}

	// The secret exists nowhere any scan reads: not in any served page,
	// not on the deployment's disk unsealed — control fired.
	if strings.Contains(back, secret) || strings.Contains(added, secret) {
		t.Fatal("the client secret was served back")
	}
	if hits := scanFor(t, r.Dir, secret); len(hits) != 0 {
		t.Fatalf("the client secret rests readable: %v", hits)
	}
	control := r.Dir + "/planted-control"
	if err := writeFile(control, "x"+secret+"x"); err != nil {
		t.Fatal(err)
	}
	if hits := scanFor(t, r.Dir, secret); len(hits) != 1 {
		t.Fatalf("positive control did not fire: %v", hits)
	}
	removeFile(control)

	// Disconnecting revokes the person's own custody.
	gone := post(t, cl, r.ShellURL+"/act/tool-disconnect?name=github")
	if !strings.Contains(gone, "Disconnected from github") {
		t.Fatalf("disconnecting did not answer: %s", gone)
	}
	after := get(t, cl, r.ShellURL+"/tools")
	if !strings.Contains(after, "/tools/connect?name=github") {
		t.Fatalf("the standing did not return to not-connected: %s", after)
	}

	// Removing reverses both halves.
	removed := post(t, cl, r.ShellURL+"/act/tool-remove?name=github")
	if !strings.Contains(removed, "github is gone") {
		t.Fatalf("removing did not answer: %s", removed)
	}
	if final := get(t, cl, r.ShellURL+"/tools"); !strings.Contains(final, "No tools yet") {
		t.Fatalf("the tool survived removal: %s", final)
	}
}

func TestApprovalsGate(t *testing.T) {
	r, _ := startRig(t)
	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		t.Fatal(err)
	}
	cl := signIn(t, r, auth)

	// The gate's operator hand: a defer rule loaded live, naming the
	// signed-in person's persona as its approver. The persona id is the
	// session's own — read off the screen's top bar.
	page := get(t, cl, r.ShellURL+"/")
	who := personaOf(t, page)
	ops, _ := opsIdentity(t, r)
	if err := ops.GuardrailLoad([]siclient.GuardrailRule{{
		Name: "defer-secret-puts", When: `action == "secrets.put"`, Effect: "defer",
		Approvers: []string{who},
	}}); err != nil {
		t.Fatalf("loading the rule: %v", err)
	}

	// A real op trips the rule: the originator (ops) is refused with a
	// ticket, machine-readably.
	_, err = ops.SecretPut("plans/launch", []byte("the planted argument"), 0)
	deferral, ok := siclient.ParseDeferral(err)
	if !ok {
		t.Fatalf("the defer did not emit a parseable ticket: %v", err)
	}

	// The screen: the key on the spine with its count, the ticket on the
	// list — who, what, rule, window — and never the arguments.
	screen := get(t, cl, r.ShellURL+"/approvals")
	for _, want := range []string{`class="ir on" href="/approvals`, "Approvals",
		"secrets.put", "defer-secret-puts", "Approve", "Deny",
		"never its contents"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the approvals screen is missing %q: %s", want, screen)
		}
	}
	if strings.Contains(screen, "the planted argument") {
		t.Fatal("the deferred arguments reached the screen")
	}

	// One tap: the person's own signed yes, delivered — and the
	// originator's retry serves.
	answered := post(t, cl, r.ShellURL+"/act/approval-approve?invocation="+
		url.QueryEscape(deferral.InvocationID)+"&principal="+
		url.QueryEscape(r.State.RealmPub+"/ops"))
	if !strings.Contains(answered, "Approved.") {
		t.Fatalf("approving did not answer: %s", answered)
	}
	if _, err := ops.SecretPut("plans/launch", []byte("the planted argument"), 0); err != nil {
		t.Fatalf("the approved retry did not serve: %v", err)
	}
	if tk, err := ops.ApprovalStatus(deferral.InvocationID); err != nil || tk.State != "spent" {
		t.Fatalf("the conversion is not witnessed: %+v %v", tk, err)
	}
	if empty := get(t, cl, r.ShellURL+"/approvals"); !strings.Contains(empty,
		"Nothing is waiting for a decision") {
		t.Fatalf("the answered ticket still shows: %s", empty)
	}

	// The deny arm: a fresh ask, the human's no, the ticket ending denied.
	_, err = ops.SecretPut("plans/other", []byte("another value"), 0)
	d2, ok := siclient.ParseDeferral(err)
	if !ok {
		t.Fatalf("no second deferral: %v", err)
	}
	denied := post(t, cl, r.ShellURL+"/act/approval-deny?invocation="+
		url.QueryEscape(d2.InvocationID)+"&principal="+
		url.QueryEscape(r.State.RealmPub+"/ops"))
	if !strings.Contains(denied, "Denied.") {
		t.Fatalf("denying did not answer: %s", denied)
	}
	if tk, err := ops.ApprovalStatus(d2.InvocationID); err != nil || tk.State != "denied" {
		t.Fatalf("the no is not witnessed: %+v %v", tk, err)
	}
}

// personaOf reads the session's own persona id off the top bar's hover —
// the one place the raw id deliberately remains.
func personaOf(t *testing.T, page string) string {
	t.Helper()
	const mark = `<span class="who" title="`
	i := strings.Index(page, mark)
	if i < 0 {
		t.Fatalf("no top-bar identity on the page: %s", page)
	}
	rest := page[i+len(mark):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("unterminated top-bar identity")
	}
	return rest[:j]
}

// writeFile and removeFile keep the custody-control plumbing in one place.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

func removeFile(path string) { _ = os.Remove(path) }
