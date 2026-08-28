package agents

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-workloads/declaration"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// The declare lane's standing bars (hq design soulstream-shell 0009 §6),
// the ones a hermetic test can honestly reach. The rest — that a placement
// lands on the record through the person's own admission, that it survives
// the surface being restarted, that it goes from waiting to taken-up live —
// needs a realm and is measured in the gate beside this module.

// board is the conversations the pickers offer.
func board() []topic.BoardEntry {
	return []topic.BoardEntry{
		{Path: "t-ab12", Announcement: topic.Announcement{Name: "planning"}},
		{Path: "t-cd34", Announcement: topic.Announcement{Name: "old news"},
			Lifecycle: topic.Archived},
	}
}

// filled is the form as a person would leave it: a name, a home, the
// mention wake it starts with, and one of everything else.
func filled() url.Values {
	return url.Values{
		"name": {"scribe"}, "home": {"t-ab12"},
		"wake_mention":      {"on"},
		"wake_conversation": {"t-ab12"},
		"sched_name":        {"nightly"}, "sched_pattern": {"@every 24h"},
		"sched_keep": {"48h"},
		"model":      {"house-brain"},
		"tools":      {"notes"},
		"instr_home": {"t-ab12"}, "instr_name": {"how-to-take-minutes"},
		"budget_hops": {"4"}, "budget_max": {"8"}, "budget_per": {"10m"},
	}
}

// declaredList is a placement in each state the screen shows.
func declaredList() []soulstream.Declared {
	opened := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	return []soulstream.Declared{
		{ItemID: "op-1", Name: "scribe", Home: "t-ab12", Model: "house-brain",
			State: topic.WorkOpen, Opened: opened,
			Wakes: []soulstream.Wake{
				{Kind: "mention", Delivery: declaration.WakeEntry{
					Kind: declaration.WakeMention}.DeliveryClass()},
				{Kind: "subject", Detail: "alarms.>", Delivery: declaration.WakeEntry{
					Kind: declaration.WakeSubject}.DeliveryClass()},
			},
			JSON: `{"role":"agent","persona":"scribe"}`},
		{ItemID: "op-2", Name: "nightly", Home: "t-ab12",
			State: topic.WorkClaimed, Owner: "node-a", Opened: opened,
			JSON: `{"role":"agent","persona":"nightly"}`},
	}
}

// fullDeclareView is the lane with everything on it, as the screen renders
// it for a deployment that places agents.
func fullDeclareView() declareView {
	return declareView{
		On: true, List: declaredList(), Board: board(),
		Models: []string{"house-brain", "quick"}, Tools: []string{"notes"},
		Role: "agent",
	}
}

// Criterion 1, first half. What the form describes is a document the
// package that owns declarations parses and accepts — byte for byte the
// file the command line takes. Round-tripping it is the whole proof: a
// second schema would show up here as a field that does not survive the
// trip.
func TestTheFormEmitsTheDocumentTheCommandLineTakes(t *testing.T) {
	built := declarationFrom(filled(), "agent")
	body, err := declarationJSON(built)
	if err != nil {
		t.Fatalf("writing the document out failed: %v", err)
	}
	// Parsed by the package that owns the format, strictly: an unknown
	// field refuses the document, so a shell-side invention cannot pass.
	parsed, err := declaration.Parse([]byte(body))
	if err != nil {
		t.Fatalf("the document this screen shows is not one the command line takes: %v\n%s",
			err, body)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("the document this screen shows is refused: %v\n%s", err, body)
	}
	if !reflect.DeepEqual(parsed, built) {
		t.Fatalf("what the screen shows is not what it would submit:\n%+v\n%+v", parsed, built)
	}
	// And it says the things a person filled in, in the record's own field
	// names — the point of showing it at all.
	for _, want := range []string{
		`"persona": "scribe"`, `"topic": "t-ab12"`, `"artifact": "file:///dev/null"`,
		`"kind": "mention"`, `"kind": "topic"`, `"kind": "schedule"`,
		`"pattern": "@every 24h"`, `"ttl": "48h"`,
		`"model": "house-brain"`, `"role": "agent"`, `"how-to-take-minutes"`,
		`"max_hops": 4`, `"max": 8`, `"per": "10m"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the document does not carry %s:\n%s", want, body)
		}
	}
}

// The mention wake is on unless somebody turns it off, and turning it off
// leaves the other ways standing. What wakes an agent is the one thing the
// form must not quietly decide.
func TestTheMentionWakeIsOnUnlessItIsTurnedOff(t *testing.T) {
	d := declarationFrom(url.Values{"name": {"scribe"}, "home": {"t-ab12"},
		"wake_mention": {"on"}}, "")
	if len(d.Wake) != 1 || d.Wake[0].Kind != declaration.WakeMention {
		t.Fatalf("the form does not start with the mention wake: %+v", d.Wake)
	}
	off := declarationFrom(url.Values{"name": {"scribe"}, "home": {"t-ab12"},
		"wake_mention": {""}, "wake_subject": {"alarms.fire"}}, "")
	if len(off.Wake) != 1 || off.Wake[0].Kind != declaration.WakeSubject {
		t.Fatalf("turning the mention wake off did not leave the rest: %+v", off.Wake)
	}
}

// With no signing role declared by the deployment there is no name to
// resolve tools through, so no capability block is written at all. A block
// naming a role nobody declared is a placement that refuses at claim time.
func TestToolsRideOnlyTheRoleTheDeploymentDeclared(t *testing.T) {
	f := url.Values{"name": {"scribe"}, "home": {"t-ab12"}, "tools": {"notes", "calendar"}}
	if d := declarationFrom(f, ""); d.Capabilities != nil {
		t.Errorf("tools were declared against no role: %+v", d.Capabilities)
	}
	d := declarationFrom(f, "agent")
	if d.Capabilities == nil || d.Capabilities.Role != "agent" ||
		!reflect.DeepEqual(d.Capabilities.Tools, []string{"notes", "calendar"}) {
		t.Errorf("the picked tools did not ride the declared role: %+v", d.Capabilities)
	}
}

// Criterion 1, second half. A refusal arrives in the words of the package
// that refuses — not this screen's paraphrase of them, which is how a
// surface comes to explain a rule it has stopped enforcing.
func TestUpstreamRefusalsArriveInTheirOwnWords(t *testing.T) {
	base := func() url.Values {
		return url.Values{"name": {"scribe"}, "home": {"t-ab12"}, "wake_mention": {"on"}}
	}
	cases := []struct {
		what string
		form func(url.Values) url.Values
	}{
		{"a wake nothing can read", func(f url.Values) url.Values {
			f.Set("sched_name", "nightly")
			f.Set("sched_pattern", "every other tuesday")
			return f
		}},
		{"a model that is a credential", func(f url.Values) url.Values {
			f.Set("model", "sk-live-9f2c")
			return f
		}},
		{"no name at all", func(f url.Values) url.Values {
			f.Set("name", "")
			return f
		}},
		{"no home", func(f url.Values) url.Values {
			f.Set("home", "")
			return f
		}},
	}
	for _, c := range cases {
		d := declarationFrom(c.form(base()), "agent")
		err := d.Validate()
		if err == nil {
			t.Errorf("%s was accepted", c.what)
			continue
		}
		// The note the panel shows is the error's own text, unchanged.
		note := declareNote(err.Error())
		if !strings.Contains(note, esc(err.Error())) {
			t.Errorf("%s: the note does not carry the refusal's own words: %s", c.what, note)
		}
	}
	// The credential-shaped model is worth naming on its own: it is the
	// paste a person makes once, and the refusal has to say what happened.
	sk := declarationFrom(func() url.Values {
		f := base()
		f.Set("model", "sk-live-9f2c")
		return f
	}(), "agent").Validate()
	if sk == nil || !strings.Contains(sk.Error(), "looks like a credential") {
		t.Errorf("a credential pasted as a model name is not refused by name: %v", sk)
	}
}

// Criterion 7, this lane's half. An empty list explains what declaring is
// and points at the act — never an empty box.
func TestAnEmptyDeclaredListOffersTheAct(t *testing.T) {
	body := renderDeclared(nil, nil)
	for _, want := range []string{"None yet", "runs on this soulstream", "Declare the first"} {
		if !strings.Contains(body, want) {
			t.Errorf("the empty declared list does not carry %q:\n%s", want, body)
		}
	}
	// And the key it points at is on the screen it points from.
	screen := renderAgents(nil, nil, nil, "", nil, declareView{On: true})
	if !strings.Contains(screen, ">Declare agent</button>") {
		t.Errorf("the empty lane offers an act the screen does not have:\n%s", screen)
	}
}

// Criterion 5. The picker offers exactly the names this soulstream holds
// and nothing invented; an empty catalogue offers the act that fills it, in
// words, because that act is not this screen's to take.
func TestTheModelPickerIsExactlyTheNamesThereAre(t *testing.T) {
	field := modelField(fullDeclareView())
	for _, want := range []string{`value="house-brain"`, `value="quick"`,
		`<option value="">the one this soulstream already uses</option>`} {
		if !strings.Contains(field, want) {
			t.Errorf("the model picker is missing %q:\n%s", want, field)
		}
	}
	if got := strings.Count(field, "<option "); got != 3 {
		t.Errorf("the picker offers %d choices, want the two names and the ambient one", got)
	}
	empty := modelField(declareView{On: true})
	for _, want := range []string{"No assistants are named here yet", "soulstream model set"} {
		if !strings.Contains(empty, want) {
			t.Errorf("an empty catalogue does not offer the act in words: %s", empty)
		}
	}
	if strings.Contains(empty, "<select") {
		t.Error("an empty catalogue still draws a picker with nothing in it")
	}
	// The read-only list says the same thing, and offers no act it cannot
	// perform: nothing retires a model here and nothing pretends to.
	list := modelsList(fullDeclareView())
	for _, want := range []string{"<summary>Models</summary>", "house-brain", "quick",
		"soulstream model set"} {
		if !strings.Contains(list, want) {
			t.Errorf("the models list is missing %q:\n%s", want, list)
		}
	}
	if strings.Contains(list, "@post") || strings.Contains(list, "<button") {
		t.Errorf("the models list offers an act:\n%s", list)
	}
}

// No provider secret passes through this surface — there is no field for
// one anywhere on the screen, and what there is instead is the line that
// loads one where the deployment keeps it. The block runs unchanged under
// every shell a person might be in.
func TestNoProviderSecretIsEverAskedFor(t *testing.T) {
	screen := renderAgents(agentList(), nil, names(), "", nil, fullDeclareView())
	if strings.Contains(screen, `type="password"`) {
		t.Error("the agents screen asks for a secret")
	}
	block := providerNote()
	for _, want := range []string{"never typed into a screen",
		"env SOULSTREAM_PROVIDER_KEY=", "soulstream provider set"} {
		if !strings.Contains(block, want) {
			t.Errorf("the provider line is missing %q:\n%s", want, block)
		}
	}
	for _, banned := range []string{"<<", "export ", "go install"} {
		if strings.Contains(block, banned) {
			t.Errorf("the provider line carries %q, which not every shell runs:\n%s", banned, block)
		}
	}
	if !strings.Contains(screen, "data-provider-command") {
		t.Error("the screen never says where a provider key goes")
	}
}

// A placement nothing has taken up says so, in the words the design fixed:
// honest waiting, never a spinner and never an error. One that has been
// taken up names what took it.
func TestAPlacementSaysWhereItStands(t *testing.T) {
	body := renderDeclared(declaredList(), nil)
	if !strings.Contains(body,
		"declared; nothing serves agents here yet — the deployment enables the dispatcher plane") {
		t.Errorf("a waiting placement does not say why it waits:\n%s", body)
	}
	if !strings.Contains(body, "claimed by node-a") {
		t.Errorf("a placement that was taken up does not name what took it:\n%s", body)
	}
	// Every one of them taken up: the waiting sentence goes with the wait.
	var claimed []soulstream.Declared
	for _, d := range declaredList() {
		d.State, d.Owner = topic.WorkClaimed, "node-a"
		claimed = append(claimed, d)
	}
	if strings.Contains(renderDeclared(claimed, nil), "nothing serves agents here yet") {
		t.Error("the waiting sentence outlived the wait")
	}
}

// Nothing on this screen offers to un-place an agent. Nothing in the
// ecosystem un-places one, and a key that cannot do what it says is worse
// than no key — the state shows, the act waits for the vocabulary.
func TestNothingOffersToRetireADeclaredAgent(t *testing.T) {
	body := renderDeclared(declaredList(), nil)
	for _, banned := range []string{"Retire", "Stop", "Remove", "Delete", "agent-retire"} {
		if strings.Contains(body, banned) {
			t.Errorf("the declared list offers %q, which nothing can perform:\n%s", banned, body)
		}
	}
}

// The delivery each way of waking promises is surfaced, in the words of the
// package that decides it. A shell that shortens them is a shell deciding
// what a person may know about whether a wake can go missing — and the one
// kind that can lose a wake is marked on the row rather than only in a
// hover.
func TestEveryWakeCarriesTheDeliveryItPromises(t *testing.T) {
	body := renderDeclared(declaredList(), nil)
	for _, w := range []declaration.WakeKind{declaration.WakeMention, declaration.WakeSubject} {
		want := (declaration.WakeEntry{Kind: w}).DeliveryClass()
		if !strings.Contains(body, esc(want)) {
			t.Errorf("the %s wake does not carry %q:\n%s", w, want, body)
		}
	}
	if !strings.Contains(body, `class="pill warn" title="alarms.&gt; · at-most-once"`) {
		t.Errorf("the one wake that can go missing is not marked as such:\n%s", body)
	}
	// The form says the same thing in the person's own words, since that is
	// where somebody chooses it.
	if !strings.Contains(declarePanel(fullDeclareView()),
		"one arriving while the agent is not running is gone") {
		t.Error("the form does not say which way of waking can go missing")
	}
}

// The declaration itself rides a fold under the row it belongs to: what was
// asked for is worth reading, and only when somebody asks.
func TestWhatWasAskedForIsThereOnDemand(t *testing.T) {
	body := renderDeclared(declaredList(), nil)
	if got := strings.Count(body, `<details class="stow"><summary>What was asked for</summary>`); got != 2 {
		t.Errorf("the declaration is folded under %d rows, want one per row (2)", got)
	}
	if !strings.Contains(body, `data-declaration>`) {
		t.Errorf("the fold holds no declaration:\n%s", body)
	}
}

// The two lanes share the one slide-over, each behind the key that names
// it, and the declare form has a result line of its own beside its fields.
func TestBothFormsShareTheOneSlideOver(t *testing.T) {
	body := renderAgents(agentList(), nil, names(), "", nil, fullDeclareView())
	for _, want := range []string{
		`data-on:click="$panel = true; $make = 'yours'"`,
		`data-on:click="$panel = true; $make = 'here'"`,
		`data-signals="{make:'yours'}"`,
		`<div data-show="$make == 'yours'">`,
		`<div data-show="$make == 'here'">`,
		`id="agent-add"`, `id="agent-declare"`,
		`id="agents-declare-note"`, `id="agents-declare-json"`,
		"contentType:'form'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the slide-over is missing %q", want)
		}
	}
	if got := strings.Count(body, `class="slideover"`); got != 1 {
		t.Errorf("the screen serves %d side panels, want the one every sheet has", got)
	}
	// The tables lead and the panel waits behind them, as every sheet does.
	if strings.Index(body, `id="agents-declared"`) > strings.Index(body, `class="slideover"`) {
		t.Error("the declared list does not lead the screen")
	}
}

// The pickers offer the conversations there are. An archived one is kept
// for reading, not for putting an agent in.
func TestThePickersOfferTheLivingConversations(t *testing.T) {
	sel := conversationSelect(fullDeclareView(), "home", "")
	if !strings.Contains(sel, `<option value="t-ab12">planning</option>`) {
		t.Errorf("the picker does not offer the conversation there is:\n%s", sel)
	}
	if strings.Contains(sel, "t-cd34") {
		t.Errorf("the picker offers an archived conversation:\n%s", sel)
	}
	if !strings.Contains(sel, `<select name="home" required>`) {
		t.Errorf("the home a required field takes is not required:\n%s", sel)
	}
	optional := conversationSelect(fullDeclareView(), "wake_conversation", "— optional")
	if !strings.Contains(optional, `<option value="">— optional</option>`) {
		t.Errorf("an optional picker cannot be left alone:\n%s", optional)
	}
}

// The limits an agent runs inside are on the screen with their numbers in
// them, never hidden: what keeps a colony from running away is a fact a
// person may read and change.
func TestTheLimitsAreVisibleAndEditable(t *testing.T) {
	form := declarePanel(fullDeclareView())
	for _, want := range []string{
		`name="budget_hops" type="number" min="0" value="4"`,
		`name="budget_max" type="number" min="0" value="8"`,
		`name="budget_per" autocomplete="off" spellcheck="false" value="10m"`,
	} {
		if !strings.Contains(form, want) {
			t.Errorf("the limits are not on the form with their numbers in them (%q):\n%s",
				want, form)
		}
	}
	// And they are not behind a fold: a bound nobody sees is a bound
	// nobody knows they are running under. Measured by cutting every fold
	// out and requiring the limits to survive the cut — with the control
	// that the cut really removed the things that are folded.
	unfolded, folded := withoutFolds(form)
	if !strings.Contains(unfolded, "budget_hops") {
		t.Errorf("the limits are folded away:\n%s", form)
	}
	if folded == 0 || strings.Contains(unfolded, "sched_pattern") {
		t.Error("the cut removed no fold — it cannot tell a folded field from a shown one")
	}
}

// withoutFolds removes every <details> block, and says how many it removed.
func withoutFolds(body string) (string, int) {
	var b strings.Builder
	rest, cut := body, 0
	for {
		i := strings.Index(rest, "<details")
		if i < 0 {
			b.WriteString(rest)
			return b.String(), cut
		}
		b.WriteString(rest[:i])
		j := strings.Index(rest[i:], "</details>")
		if j < 0 {
			return b.String(), cut
		}
		rest = rest[i+j+len("</details>"):]
		cut++
	}
}

// A deployment that places no agents has no such lane: no table, no key, no
// form, and the screen it does have is exactly the screen it always was.
func TestTheLaneIsAbsentWhereTheDeploymentPlacesNone(t *testing.T) {
	body := renderAgents(agentList(), nil, names(), "", nil, declareView{})
	for _, gone := range []string{"Declared agents", "agents-declared", "agent-declare",
		"Models", "$make"} {
		if strings.Contains(body, gone) {
			t.Errorf("a deployment that places no agents still shows %q", gone)
		}
	}
	if !strings.Contains(body, `data-on:click="$panel = true"`) {
		t.Error("the one act such a deployment has stopped being offered")
	}
}
