package tools

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/impire-io/soulstream-core/toolcatalog"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// routes records what a module claims, without serving any of it.
type routes struct{ patterns []string }

func (rt *routes) Handle(pattern string, _ http.Handler) {
	rt.patterns = append(rt.patterns, pattern)
}

func (rt *routes) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	rt.patterns = append(rt.patterns, pattern)
}

// The module claims its screen, the ceremony's two legs, and its acts —
// and nothing another surface could be serving.
func TestTheModuleClaimsItsOwnRoutes(t *testing.T) {
	m := New(nil, nil)
	if got := m.Identity(); got.Slug != "tools" || got.Name != "Tools" {
		t.Errorf("the module names itself %+v", got)
	}
	var rt routes
	m.Mount(&rt)
	want := []string{"GET /tools", "GET /tools/connect", "GET /tools/callback",
		"POST /act/tool-disconnect", "POST /act/tool-add", "POST /act/tool-remove"}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("the module mounts %v, want %v", rt.patterns, want)
	}
}

// One key, carrying the open conversation like every key on the spine.
func TestTheModuleContributesOneKey(t *testing.T) {
	m := New(nil, nil)
	nav := m.Nav(httptest.NewRequest(http.MethodGet, "/tools?topic=home%2Fkitchen", nil))
	if len(nav) != 1 || nav[0].Href != "/tools?topic=home%2Fkitchen" || nav[0].Foot {
		t.Fatalf("the key is %+v", nav)
	}
}

// Every honest state of a row: a remote tool without its ceremony half
// says it isn't serving; connected offers Disconnect; not connected offers
// Connect; a run-here tool offers neither; the admin sees Remove except on
// a declared entry, which says why not.
func TestTheRowsSayTheirHonestStates(t *testing.T) {
	remote := soulstream.Tool{Name: "github", Kind: toolcatalog.KindRemote,
		Endpoint: "https://api.github.invalid/mcp", OnPlane: true, Description: "GitHub"}
	half := soulstream.Tool{Name: "drafts", Kind: toolcatalog.KindRemote,
		Endpoint: "https://x.invalid/mcp"}
	declared := soulstream.Tool{Name: "dex", Kind: toolcatalog.KindRemote,
		OnPlane: true, Declared: true, Endpoint: "https://dex.invalid/mcp"}
	workload := soulstream.Tool{Name: "notes", Kind: toolcatalog.KindWorkload,
		Persona: "notes-tool", Endpoint: "http://127.0.0.1:9/mcp", Description: "notes"}

	connected := renderList(view{Admin: true,
		Tools:     []soulstream.Tool{remote, half, declared, workload},
		Connected: map[string]bool{"github": true},
	})
	for _, want := range []string{
		">connected</span>", "/act/tool-disconnect?name=github",
		"isn't serving",                // the half tool, by name
		"/tools/connect?name=dex",      // declared still connects
		"declared in configuration",    // but never removes
		"/act/tool-remove?name=github", // the runtime one removes
		"runs here", `<span class="mono">@notes-tool</span>`,
	} {
		if !strings.Contains(connected, want) {
			t.Errorf("the list is missing %q:\n%s", want, connected)
		}
	}
	if strings.Contains(connected, "/act/tool-remove?name=dex") {
		t.Error("a declared tool is offered removal")
	}
	if strings.Contains(connected, "/tools/connect?name=notes") {
		t.Error("a run-here tool is offered connecting")
	}
	if strings.Contains(connected, "/tools/connect?name=drafts") {
		t.Error("a tool without its ceremony half is offered connecting")
	}

	plain := renderList(view{Tools: []soulstream.Tool{remote}, Connected: map[string]bool{}})
	if strings.Contains(plain, "tool-remove") || strings.Contains(plain, "Add a tool") {
		t.Errorf("a plain session sees the admin's acts:\n%s", plain)
	}
	if !strings.Contains(plain, "/tools/connect?name=github") {
		t.Errorf("a plain session is not offered connecting:\n%s", plain)
	}
}

// The admin form never echoes a secret and the screen never says the
// retired word or the machine-room vocabulary.
func TestTheFormAndTheRegister(t *testing.T) {
	form := renderAddForm()
	if !strings.Contains(form, `name="client_secret" type="password"`) {
		t.Errorf("the secret field is not a password input:\n%s", form)
	}
	// No input ever echoes a value — the select's own option values are
	// the one legitimate carrier.
	for _, input := range strings.Split(form, "<input")[1:] {
		if end := strings.Index(input, ">"); end >= 0 && strings.Contains(input[:end], "value=") {
			t.Errorf("an input echoes a value:%s", input[:end])
		}
	}
	whole := renderTools(view{Admin: true, Tools: []soulstream.Tool{
		{Name: "github", Kind: toolcatalog.KindRemote, OnPlane: true, Endpoint: "https://x"},
	}, Connected: map[string]bool{}})
	lower := strings.ToLower(whole)
	for _, banned := range []string{"realm", "grant", ">link<"} {
		if strings.Contains(lower, banned) {
			t.Errorf("the screen says %q:\n%s", banned, whole)
		}
	}
	if !strings.Contains(whole, "Tools") {
		t.Error("the screen has no title")
	}
}

// The list is empty honestly, and a ceremony's message rides across the
// redirect.
func TestEmptyAndMessage(t *testing.T) {
	if got := renderTools(view{Msg: "Connected."}); !strings.Contains(got, "No tools yet") ||
		!strings.Contains(got, "Connected.") {
		t.Errorf("the empty screen: %s", got)
	}
}

// splitList reads a list the way a person types one.
func TestSplitList(t *testing.T) {
	if got := splitList("repo, read:user  gist"); !reflect.DeepEqual(got, []string{"repo", "read:user", "gist"}) {
		t.Errorf("splitList = %v", got)
	}
	if got := splitList(""); got != nil {
		t.Errorf("splitList of nothing = %v", got)
	}
}
