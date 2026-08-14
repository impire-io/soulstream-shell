package agents

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

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

func agentList() []soulstream.Agent {
	added := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	return []soulstream.Agent{
		{Handle: "scribe", ShownAs: "Scribe", OperatedBy: "u-daan",
			Added: added, Credential: "9f2c"},
		{Handle: "nightly", ShownAs: "Nightly run", OperatedBy: "u-daan", Added: added},
	}
}

func names() map[string]string { return map[string]string{"u-daan": "Daan"} }

// The module claims exactly the paths it answers on, and no others. Route
// names are one namespace across every module in a build, so what this one
// takes is worth stating out loud.
func TestTheModuleClaimsItsOwnRoutes(t *testing.T) {
	m := New(nil, nil)
	if got := m.Identity(); got.Slug != "agents" || got.Name != "Agents" {
		t.Errorf("the module names itself %+v", got)
	}
	var rt routes
	m.Mount(&rt)
	want := []string{
		"GET /agents",
		"POST /act/agent-add",
		"POST /act/agent-credential",
		"POST /act/agent-revoke",
	}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("claimed %v, want %v", rt.patterns, want)
	}
}

// Every row is a machine voice, so every row carries the machine channel's
// lamp — and the lamp says the record's own fact about the voice, not the
// design's word for the colour.
func TestEveryRowIsAMachineVoice(t *testing.T) {
	body := renderTable(agentList(), nil, names(), "")
	if got := strings.Count(body, `class="led machine"`); got != 2 {
		t.Errorf("machine lamps = %d, want one per row (2)", got)
	}
	if !strings.Contains(body, `title="operated by Daan"`) {
		t.Error("the lamp does not say who answers for the voice")
	}
	if strings.Contains(body, "led human") {
		t.Error("a human channel lamp on the agents screen")
	}
}

// An agent that cannot get in any more is still on the screen: it said
// things, and the things it said still need a name against them. What
// changed is what it can do, and only the act that would be a lie is gone.
func TestAVoiceStaysOnTheScreenAfterItsCredentialIsTakenAway(t *testing.T) {
	body := renderTable(agentList(), nil, names(), "")
	if !strings.Contains(body, "nightly") {
		t.Fatal("the agent with no credential is missing from the roster")
	}
	if got := strings.Count(body, "Take the credential away"); got != 1 {
		t.Errorf("take-away offered %d times, want only for the one that can get in", got)
	}
	if got := strings.Count(body, "New credential"); got != 2 {
		t.Errorf("new-credential offered %d times, want on every row", got)
	}
}

// The credential is shown once and the screen says so, because the store
// behind it keeps a digest it cannot reverse and reloading brings nothing
// back.
func TestTheCredentialIsShownOnceAndSaysSo(t *testing.T) {
	body := renderCredential(soulstream.Credential{
		Handle: "scribe", ShownAs: "Scribe", Secret: "sit_deadbeef",
		Dial: "nats://127.0.0.1:4222", Realm: "home",
		SentinelPath: "/state/sentinel.creds", Sentinel: "-----BEGIN NATS USER JWT-----",
	}, "is ready")
	for _, want := range []string{
		"Shown once", "nothing keeps a copy", "sit_deadbeef",
		"data-credential", "data-sentinel",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the shown-once card does not carry %q", want)
		}
	}
	// The next thing that happens takes the secret off the screen.
	if !strings.Contains(resultNote("anything else"), `id="`+resultID+`"`) {
		t.Error("the result line does not replace the shown-once card")
	}
}

// The configuration is emitted in the shape the stdio server actually reads:
// the exact variable names that program resolves its lane from, spelled its
// way. Getting one wrong is an agent that does not start.
func TestTheConfigurationIsTheShapeTheAgentsOwnProgramReads(t *testing.T) {
	got := mcpConfig(soulstream.Credential{
		Handle: "scribe", Secret: "sit_deadbeef", Dial: "nats://127.0.0.1:4222",
		Realm: "home", SentinelPath: "/state/sentinel.creds",
	})
	for _, want := range []string{
		`"command": "soulstream-mcp"`,
		`"SOULSTREAM_URL": "nats://127.0.0.1:4222"`,
		`"SOULSTREAM_CREDS": "/state/sentinel.creds"`,
		`"SOULSTREAM_TOKEN": "sit_deadbeef"`,
		`"SOULSTREAM_REALM": "home"`,
		`"SOULSTREAM_PERSONA": "scribe"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the emitted configuration does not carry %s", want)
		}
	}
}

// An empty roster is a sentence, not an empty container.
func TestAnEmptyRosterSaysSo(t *testing.T) {
	if !strings.Contains(renderTable(nil, nil, nil, ""), "No agents yet.") {
		t.Error("an empty roster does not say so")
	}
}

// The screen answers the question somebody arrived with, including when the
// answer is no. Most voices on the record are not agents.
func TestTheScreenAnswersAboutTheVoiceSomebodyCameFor(t *testing.T) {
	marked := renderTable(agentList(), nil, names(), "scribe")
	if got := strings.Count(marked, `<tr class="on">`); got != 1 {
		t.Errorf("marked rows = %d, want exactly the one somebody came for", got)
	}
	stranger := lookedUpNote(agentList(), nil, "u-avery")
	if !strings.Contains(stranger, "No agent here answers to") {
		t.Errorf("a name this screen does not hold is not answered: %q", stranger)
	}
	if lookedUpNote(agentList(), nil, "") != "" {
		t.Error("a screen nobody was pointed at answers a question nobody asked")
	}
}

// Adding an agent is vouching for it, and the form says so before anybody
// clicks: the claim is signed with the reader's own key and their name stays
// on it.
func TestTheFormSaysWhatAddingAnAgentCommitsYouTo(t *testing.T) {
	form := renderAddForm()
	if !strings.Contains(form, "You vouch for what you add") {
		t.Error("the form does not say that adding is vouching")
	}
	for _, want := range []string{`name="handle"`, `name="shown"`, "contentType:'form'"} {
		if !strings.Contains(form, want) {
			t.Errorf("the form is missing %s", want)
		}
	}
}

// The words on this screen are the ones a person uses. Component bynames
// never reach a product surface.
//
// The emitted configuration is exempt and only it: those variable names
// belong to soulstream-core, which reads them and no others, so the screen
// spells them that program's way or the agent does not start. The exemption
// is narrow by construction — the screen below is rendered with no
// credential on it, which is every state but one.
func TestNothingServedSaysTheRetiredWord(t *testing.T) {
	body := renderAgents(agentList(), nil, names(), "scribe")
	for _, banned := range []string{"realm", "fold", "idp", "OIDC", "persona", "sentinel", "token"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(banned)) {
			t.Errorf("the agents screen says %q", banned)
		}
	}
}
