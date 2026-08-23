package approvals

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

// routes records what a module claims, without serving any of it.
type routes struct{ patterns []string }

func (rt *routes) Handle(pattern string, _ http.Handler) {
	rt.patterns = append(rt.patterns, pattern)
}

func (rt *routes) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	rt.patterns = append(rt.patterns, pattern)
}

// The module claims its screen and its two acts — and nothing another
// surface could be serving.
func TestTheModuleClaimsItsOwnRoutes(t *testing.T) {
	m := New(nil, nil)
	if got := m.Identity(); got.Slug != "approvals" || got.Name != "Approvals" {
		t.Errorf("the module names itself %+v", got)
	}
	var rt routes
	m.Mount(&rt)
	want := []string{"GET /approvals", "POST /act/approval-approve", "POST /act/approval-deny"}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("the module mounts %v, want %v", rt.patterns, want)
	}
}

// A ticket row carries who, what, the rule, the window, the fingerprint —
// and never any arguments, because the ticket holds none by construction.
func TestTheRowsAndTheirHonesty(t *testing.T) {
	tk := ticket{
		InvocationID: "abcdef0123456789abcdef0123456789",
		Principal:    "ACCT/scribe-daan",
		Who:          "Scribe",
		Action:       "secrets.put",
		Rule:         "defer-secret-puts",
		ExpiresAt:    time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
	}
	acting := renderList(view{MayAct: true, Tickets: []ticket{tk}})
	for _, want := range []string{
		"Scribe", "secrets.put", "defer-secret-puts",
		"abcdef012345…",                            // the fingerprint a column can hold
		`title="abcdef0123456789abcdef0123456789"`, // the whole on the hover
		"/act/approval-approve?invocation=abcdef0123456789abcdef0123456789",
		"principal=ACCT%2Fscribe-daan",
		"/act/approval-deny?",
		"left",                     // the window in words
		`title="ACCT/scribe-daan"`, // the principal in the hover, like every raw id
		`title="until `,            // the exact deadline behind the computed window
	} {
		if !strings.Contains(acting, want) {
			t.Errorf("the list is missing %q:\n%s", want, acting)
		}
	}
	// With a name resolved, the principal string stays out of the cell: the
	// hover is its place. It stands in the cell only when it is all there is.
	if strings.Contains(acting, `>ACCT/scribe-daan</span>`) {
		t.Errorf("the principal stands beside the name it is the hover for:\n%s", acting)
	}
	unnamed := renderList(view{MayAct: true, Tickets: []ticket{{
		InvocationID: tk.InvocationID, Principal: tk.Principal,
		Action: tk.Action, Rule: tk.Rule, ExpiresAt: tk.ExpiresAt,
	}}})
	if !strings.Contains(unnamed, `<span class="mono">ACCT/scribe-daan</span>`) {
		t.Errorf("with no name resolved the principal does not stand in:\n%s", unnamed)
	}
	watching := renderList(view{MayAct: false, Tickets: []ticket{tk}})
	if strings.Contains(watching, "approval-approve") {
		t.Errorf("a session that cannot act is offered the acts:\n%s", watching)
	}
	if !strings.Contains(watching, "not yours to answer") {
		t.Errorf("the withheld acts do not say why:\n%s", watching)
	}
}

// The empty state, the privacy line said out loud, and the plain register.
func TestTheScreenSaysItsTerms(t *testing.T) {
	got := renderApprovals(view{MayAct: true})
	for _, want := range []string{
		"Nothing is waiting for a decision",
		"named by its fingerprint, never its contents",
		"signed with your own key",
		"exactly one request",
		// The how-it-works prose rests under a fold: first-visit reading,
		// out of the way of every visit after.
		`<details class="stow"><summary>How this works</summary>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the screen is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(strings.ToLower(got), "realm") {
		t.Error("the screen says the retired word")
	}
}

// The mark on the spine counts, and reads singular when one waits.
func TestTheSpineMark(t *testing.T) {
	if got := spineTally(3); !strings.Contains(got, ">3</span>") ||
		!strings.Contains(got, "3 decisions wait") {
		t.Errorf("the mark: %s", got)
	}
	if got := spineTally(1); !strings.Contains(got, "1 decision waits") {
		t.Errorf("one waiting decision reads as many: %s", got)
	}
}

// The principal's persona half, however the principal is spelled.
func TestPrincipalUser(t *testing.T) {
	for in, want := range map[string]string{
		"ACCT/scribe-daan": "scribe-daan",
		"scribe-daan":      "scribe-daan",
	} {
		if got := principalUser(in); got != want {
			t.Errorf("principalUser(%q) = %q", in, got)
		}
	}
}

// An expiring window says so instead of going negative.
func TestTheWindowNeverGoesNegative(t *testing.T) {
	tk := ticket{InvocationID: "ff00", Principal: "A/u", Action: "x", Rule: "r",
		ExpiresAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)}
	if got := renderList(view{MayAct: true, Tickets: []ticket{tk}}); !strings.Contains(got, "expiring") {
		t.Errorf("a passed window reads wrong:\n%s", got)
	}
}
