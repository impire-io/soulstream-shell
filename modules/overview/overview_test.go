package overview

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/topic"
)

// routes records what a module claims, without serving any of it.
type routes struct{ patterns []string }

func (rt *routes) Handle(pattern string, _ http.Handler) {
	rt.patterns = append(rt.patterns, pattern)
}

func (rt *routes) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	rt.patterns = append(rt.patterns, pattern)
}

// board is the conversations this screen would list.
func board() []topic.BoardEntry {
	return []topic.BoardEntry{
		{Path: "home/attic", Announcement: topic.Announcement{Name: "attic"},
			Lifecycle: topic.Lifecycle("active")},
		{Path: "home/kitchen", Announcement: topic.Announcement{Name: "kitchen table"},
			Lifecycle: topic.Lifecycle("active")},
	}
}

// convo is a conversation with something said in it, for the screens that
// count what is in one without showing any of it.
func convo() *topic.MaterializedTopic {
	t0 := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	return &topic.MaterializedTopic{
		Path:         "home/kitchen",
		Announcement: &topic.Announcement{Name: "kitchen table"},
		Contributions: []topic.Contribution{
			{OpID: "op-1", Author: "avery", Timestamp: t0, Type: topic.TypeTurnPost,
				Body: "is the kettle on", Sig: topic.SigVerified},
		},
	}
}

// The module claims its two screens and the act offered from the second —
// and nothing another surface could be serving.
func TestTheModuleClaimsItsOwnRoutes(t *testing.T) {
	m := New(nil, nil)
	if got := m.Identity(); got.Slug != "overview" {
		t.Errorf("the module names itself %+v", got)
	}
	var rt routes
	m.Mount(&rt)
	want := []string{"GET /home", "GET /status", "POST /act/work-open"}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("the module mounts %v, want %v", rt.patterns, want)
	}
}

// The module's two keys: the overview at the top, the readouts at the foot
// beside the way out — and both carry the open conversation, so the way
// back from here lands where the person left.
func TestTheModuleContributesTwoKeys(t *testing.T) {
	m := New(nil, nil)
	nav := m.Nav(httptest.NewRequest(http.MethodGet, "/status?topic=home%2Fkitchen", nil))
	if len(nav) != 2 {
		t.Fatalf("the module contributes %d keys, want 2: %+v", len(nav), nav)
	}
	if nav[0].Href != "/home?topic=home%2Fkitchen" || nav[0].Foot {
		t.Errorf("the overview key is %+v", nav[0])
	}
	if nav[1].Href != "/status?topic=home%2Fkitchen" || !nav[1].Foot {
		t.Errorf("the readouts key is %+v", nav[1])
	}
	// With no conversation open there is nothing to carry.
	bare := m.Nav(httptest.NewRequest(http.MethodGet, "/home", nil))
	if bare[0].Href != "/home" || bare[1].Href != "/status" {
		t.Errorf("the keys carry a conversation nobody opened: %+v", bare)
	}
}

// A screen that settled on a conversation of its own says so, so every
// other key on the spine is built against the same one.
func TestAScreenSaysWhichConversationItSettledOn(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/home", nil)
	if got := withTopic(r, "home/kitchen").URL.Query().Get("topic"); got != "home/kitchen" {
		t.Errorf("the screen kept its default to itself: %q", got)
	}
	// What the person asked for is never overwritten, and a screen with
	// nothing to say hands the request on untouched.
	asked := httptest.NewRequest(http.MethodGet, "/home?topic=home%2Fattic", nil)
	if got := withTopic(asked, "home/attic"); got != asked {
		t.Error("a request that already says it was rewritten anyway")
	}
	if got := withTopic(r, ""); got != r {
		t.Error("a screen with no conversation rewrote the request")
	}
}

// The house readout is the segmented ladder, and it reads against the scale
// the store declares for itself. A store with no declared roof has no scale,
// and the instrument says so rather than inventing a ceiling to look empty
// against: an unroofed store is not 0% full, it is unmeasured.
func TestTheStorageReadoutIsAMeterAgainstADeclaredScale(t *testing.T) {
	roofed := renderOverview(view{StreamMsg: 400, StreamBytes: 512 << 20, StreamRoof: 1 << 30})
	if n := strings.Count(roofed, `<span class="seg`); n != vuSegments {
		t.Errorf("the ladder has %d segments, want %d", n, vuSegments)
	}
	if lit := strings.Count(roofed, " lit"); lit != vuSegments/2 {
		t.Errorf("half a budget lights %d of %d lamps", lit, vuSegments)
	}
	for _, want := range []string{`<span class="cap">of 1.0 GB</span>`,
		`<span class="mono">50%</span>`, `aria-label="50% of the store&#39;s budget used"`,
		"400 ops · 512 MB"} {
		if !strings.Contains(roofed, want) {
			t.Errorf("the storage readout is missing %q:\n%s", want, roofed)
		}
	}
	bare := renderOverview(view{StreamMsg: 400, StreamBytes: 512 << 20})
	if strings.Contains(bare, " lit") {
		t.Errorf("a store with no declared budget still reads a level:\n%s", bare)
	}
	for _, want := range []string{`<span class="cap">no budget set</span>`,
		`aria-label="the store declares no budget to measure against"`} {
		if !strings.Contains(bare, want) {
			t.Errorf("the unmeasured store does not say so (%q):\n%s", want, bare)
		}
	}
	// And nothing anywhere is a progress bar.
	if strings.Contains(bare, "<progress") || strings.Contains(roofed, "<progress") {
		t.Error("the house readout is a progress bar")
	}
}

// The overview is a way into every conversation and a look at the house.
func TestOverviewOpensOntoTheConversations(t *testing.T) {
	got := renderOverview(view{
		Board:     board(),
		StreamMsg: 12, StreamBytes: 1 << 20, FoldOK: true,
	})
	for _, want := range []string{
		"Storage", "People &amp; sign-in", "12 ops",
		`<a class="row" href="/?topic=home%2Fkitchen">`,
		`<a class="row" href="/?topic=home%2Fattic">`,
		"2 conversations",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the overview is missing %q:\n%s", want, got)
		}
	}
	if got := renderOverview(view{}); !strings.Contains(got, "No conversations yet") {
		t.Errorf("the empty overview says nothing:\n%s", got)
	}
}

// A conversation holding messages with your name in it is marked here too,
// with its own share of the count — the tray is the person's, not one
// screen's.
func TestTheOverviewMarksConversationsThatWantYou(t *testing.T) {
	got := renderOverview(view{Board: board(), Unread: map[string]int{"home/attic": 2}})
	if !strings.Contains(got, `class="tally on" title="2 messages mention you"`) {
		t.Errorf("the overview does not mark the conversation:\n%s", got)
	}
	if strings.Contains(renderOverview(view{Board: board()}), "tally") {
		t.Error("an empty tray still marks the overview")
	}
}

// The system-status screen still carries the house readouts in plain words.
func TestStatusScreenKeepsThePlaneReadouts(t *testing.T) {
	got := renderPlanes(view{StreamMsg: 12, StreamBytes: 3 << 20, FoldOK: true, Topic: convo()})
	for _, want := range []string{"Storage", "People &amp; sign-in", "Work", "12 ops"} {
		if !strings.Contains(got, want) {
			t.Errorf("status screen missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "is the kettle on") {
		t.Error("the status screen renders conversation content")
	}
}

// The word the operator retired. The Go keeps it — realm.Client, the realm
// package, the flag a deployment sets — but nothing a person reads says it.
func TestNothingServedSaysTheRetiredWord(t *testing.T) {
	v := view{Board: board(), Topic: convo(), Unread: map[string]int{"home/attic": 1},
		StreamMsg: 12, StreamBytes: 3 << 20, StreamRoof: 1 << 30, FoldOK: true}
	for what, served := range map[string]string{
		"the overview":        renderOverview(v),
		"the status readouts": renderPlanes(v),
	} {
		if strings.Contains(strings.ToLower(served), "realm") {
			t.Errorf("%s says the retired word:\n%s", what, served)
		}
	}
}

// The overview points the way to an agent of your own — on deployments that
// issue agent credentials at all. No agents yet is a pointer, a standing
// roster is its counts, an unreadable roster says so, and a deployment that
// issues nothing shows no card to a screen it does not have.
func TestTheOverviewPointsAtTheAgentsScreen(t *testing.T) {
	if got := agentsCard(view{}); got != "" {
		t.Errorf("a deployment with no agents surface grew a card:\n%s", got)
	}
	empty := agentsCard(view{AgentsOn: true})
	for _, want := range []string{`href="/agents"`, "Set one up", "none yet"} {
		if !strings.Contains(empty, want) {
			t.Errorf("the empty-roster card does not carry %q:\n%s", want, empty)
		}
	}
	standing := agentsCard(view{AgentsOn: true, AgentsNamed: 2, AgentsIn: 1})
	for _, want := range []string{"2 named", "1 can get in", `href="/agents"`} {
		if !strings.Contains(standing, want) {
			t.Errorf("the standing-roster card does not carry %q:\n%s", want, standing)
		}
	}
	unread := agentsCard(view{AgentsOn: true, AgentsUnread: true})
	if !strings.Contains(unread, "unreadable") || strings.Contains(unread, "none yet") {
		t.Errorf("an unreadable roster is not reported honestly:\n%s", unread)
	}
}
