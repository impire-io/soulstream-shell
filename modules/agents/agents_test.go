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
		"GET /agents/live",
		"GET /agents/revoke-ask",
		"POST /act/agent-add",
		"POST /act/agent-credential",
		"POST /act/agent-revoke",
		"POST /agents/declare-json",
		"POST /act/agent-declare",
	}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("claimed %v, want %v", rt.patterns, want)
	}
}

// Every row is a machine voice, so every row carries the machine channel's
// lamp — and the lamp says the record's own fact about the voice, not the
// design's word for the colour.
func TestEveryRowIsAMachineVoice(t *testing.T) {
	body := renderTable(agentList(), nil, names(), "", nil)
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
	body := renderTable(agentList(), nil, names(), "", nil)
	if !strings.Contains(body, "nightly") {
		t.Fatal("the agent with no credential is missing from the roster")
	}
	if got := strings.Count(body, "/agents/revoke-ask?who="); got != 1 {
		t.Errorf("revoking offered %d times, want only for the one that can get in", got)
	}
	// The row offers the question, never the act itself — and the key says
	// one short word, with the whole sentence in the hover.
	if strings.Contains(body, "/act/agent-revoke") {
		t.Error("the roster offers the revoke act without its question")
	}
	if !strings.Contains(body, `title="Take the credential away"`) ||
		!strings.Contains(body, ">Revoke</button>") {
		t.Errorf("the revoke key does not read as one word with the sentence in hover:\n%s", body)
	}
	if got := strings.Count(body, "New credential"); got != 2 {
		t.Errorf("new-credential offered %d times, want on every row", got)
	}
}

// Revoking stands behind a question that says what stops and what stays,
// with both ways out.
func TestRevokingStandsBehindAQuestion(t *testing.T) {
	q := revokeConfirm("scribe")
	for _, want := range []string{
		"credential away?", "everything it said stays",
		`@post('/act/agent-revoke?who=scribe')`, "Yes, revoke it",
		`@get('/agents/revoke-ask')`, "Keep it",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("the revoke question is missing %q:\n%s", want, q)
		}
	}
}

// The screen leads with the roster; the add-form waits in the slide-over
// behind its own key, with a result line of its own beside the fields.
func TestTheAddFormWaitsInTheSlideOver(t *testing.T) {
	body := renderAgents(agentList(), nil, names(), "", nil, declareView{})
	table := strings.Index(body, `id="agents-table"`)
	panel := strings.Index(body, `class="slideover"`)
	if table < 0 || panel < 0 || panel < table {
		t.Fatalf("the roster does not lead the screen (table at %d, panel at %d)", table, panel)
	}
	for _, want := range []string{
		`data-on:click="$panel = true"`, "Add agent",
		`id="agent-add"`, `id="agents-add-note"`, "Add an agent",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen is missing %q", want)
		}
	}
}

// Six columns of names, ids and dates do not narrow past a point, so the
// table scrolls inside its own box rather than pushing the screen sideways
// and clipping its last column off the edge of the frame.
func TestTheTableScrollsInsideItsOwnBox(t *testing.T) {
	body := renderTable(agentList(), nil, names(), "", nil)
	if n := strings.Count(body, "<table"); n != 1 {
		t.Fatalf("the screen serves %d tables, want 1", n)
	}
	if !strings.Contains(body, `<div class="tablewrap"><table>`) {
		t.Errorf("the table is not inside the container that scrolls it:\n%s", body)
	}
	// Before it scrolls, it gives: the keys in the last column stack instead
	// of widening every row, and a handle that has to wrap keeps the whole of
	// itself in the hover.
	if n := strings.Count(body, `<div class="acts">`); n != 2 {
		t.Errorf("the acts stack on %d rows, want one per row (2)", n)
	}
	if !strings.Contains(body, `<td class="mono" title="scribe">scribe</td>`) {
		t.Errorf("a handle that wraps cannot be read whole:\n%s", body)
	}
	// And the form in the slide-over lays its fields out in whatever room there
	// is, rather than running every one of them the width of the screen.
	if !strings.Contains(addPanel(), `<div class="fields">`) {
		t.Error("the form's fields do not lay themselves out")
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

// The configuration is emitted in the shape an MCP client actually reads:
// the one product binary as the door (command soulstream, argument mcp) and
// the exact variable names the door resolves its lane from, spelled its
// way. Getting one wrong is an agent that does not start.
func TestTheConfigurationIsTheShapeTheAgentsOwnProgramReads(t *testing.T) {
	got := mcpConfig(soulstream.Credential{
		Handle: "scribe", Secret: "sit_deadbeef", Dial: "nats://127.0.0.1:4222",
		Realm: "home", SentinelPath: "/state/sentinel.creds",
	})
	for _, want := range []string{
		`"command": "soulstream"`,
		`"args": ["mcp"]`,
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

// The paste block is the primary path and holds the product's promise (one
// binary, one paste — design soulstream/0002 §4): it writes the credentials
// file itself so the same block works on any machine, and it stays portable
// across POSIX shells and fish — no heredoc, no export.
func TestThePasteBlockIsPortableAndWhole(t *testing.T) {
	block := wrapBlock(soulstream.Credential{
		Handle: "scribe", Secret: "sit_deadbeef", Dial: "nats://127.0.0.1:4222",
		Realm: "home", SentinelPath: "/state/sentinel.creds",
		Sentinel: "-----BEGIN NATS USER JWT-----",
	})
	for _, want := range []string{
		`mkdir -p "$HOME/.soulstream"`,
		`printf '%s' '-----BEGIN NATS USER JWT-----' > "$HOME/.soulstream/scribe.creds"`,
		`env SOULSTREAM_URL='nats://127.0.0.1:4222'`,
		`SOULSTREAM_CREDS="$HOME/.soulstream/scribe.creds"`,
		`SOULSTREAM_TOKEN='sit_deadbeef'`,
		`SOULSTREAM_REALM='home'`,
		`SOULSTREAM_PERSONA='scribe'`,
		"soulstream wrap --harness claude",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the paste block does not carry %q:\n%s", want, block)
		}
	}
	for _, banned := range []string{"<<", "export ", "go install"} {
		if strings.Contains(block, banned) {
			t.Errorf("the paste block carries %q, which not every shell runs:\n%s", banned, block)
		}
	}
	// With no sentinel to inline, the block points at the deployment's own
	// file instead of writing one — the honest local-machine fallback.
	local := wrapBlock(soulstream.Credential{
		Handle: "scribe", Secret: "sit_deadbeef", Dial: "nats://127.0.0.1:4222",
		Realm: "home", SentinelPath: "/state/sentinel.creds",
	})
	if strings.Contains(local, "mkdir") || !strings.Contains(local, `SOULSTREAM_CREDS='/state/sentinel.creds'`) {
		t.Errorf("the sentinel-less block does not fall back to the deployment's path:\n%s", local)
	}
}

// The operator cell carries a person's name with the handle beside it — and
// when the record offers no name beyond the handle, the handle stands alone
// rather than beside a copy of itself.
func TestTheOperatorCellDoesNotSayANameTwice(t *testing.T) {
	body := renderTable(agentList(), nil, nil, "", nil)
	if strings.Contains(body, "u-daan <span") || strings.Contains(body, "u-daan @u-daan") {
		t.Errorf("an unnamed operator is printed twice:\n%s", body)
	}
	if !strings.Contains(body, `<span class="mono">@u-daan</span>`) {
		t.Errorf("the handle does not stand alone when it is all there is:\n%s", body)
	}
}

// The shown-once screen leads with the paste block — running the agent is
// the primary path, above every fold — and keeps the hard paths possible
// under it: Claude Code takes the block as its own file, codex takes the
// same values as TOML, and everything else that speaks MCP gets the shape
// in plain words. Nothing on the card asks for a toolchain.
func TestTheCredentialScreenLeadsWithTheWrap(t *testing.T) {
	body := renderCredential(soulstream.Credential{
		Handle: "scribe", ShownAs: "Scribe", Secret: "sit_deadbeef",
		Dial: "nats://127.0.0.1:4222", Realm: "home",
		SentinelPath: "/state/sentinel.creds", Sentinel: "-----BEGIN NATS USER JWT-----",
	}, "is ready")
	for _, want := range []string{
		"Run your agent",
		`data-setup="wrap"`, `data-wrap-command`,
		"soulstream wrap --harness claude",
		"https://github.com/impire-io/soulstream/releases",
		"Copy the block",
		"Other ways to connect it",
		`data-setup="claude-code"`, ".mcp.json",
		`data-setup="codex"`, "[mcp_servers.soulstream]",
		`data-setup="other"`, "pi.dev",
		`data-setup="creds"`, "data-sentinel",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the credential card does not carry %q", want)
		}
	}
	if strings.Contains(body, "go install") {
		t.Error("the credential card asks for a Go toolchain")
	}
	// The paste block comes before every fold: the easy path is first.
	if strings.Index(body, "data-wrap-command") > strings.Index(body, "<details") {
		t.Error("the paste block does not lead the card")
	}
	toml := codexConfig(soulstream.Credential{
		Handle: "scribe", Secret: "sit_deadbeef", Dial: "nats://127.0.0.1:4222",
		Realm: "home", SentinelPath: "/state/sentinel.creds",
	})
	for _, want := range []string{
		`command = "soulstream"`,
		`args = ["mcp"]`,
		`SOULSTREAM_URL = "nats://127.0.0.1:4222"`,
		`SOULSTREAM_CREDS = "/state/sentinel.creds"`,
		`SOULSTREAM_TOKEN = "sit_deadbeef"`,
		`SOULSTREAM_REALM = "home"`,
		`SOULSTREAM_PERSONA = "scribe"`,
	} {
		if !strings.Contains(toml, want) {
			t.Errorf("the codex shape does not carry %s", want)
		}
	}
}

// An empty roster is a sentence, not an empty container.
func TestAnEmptyRosterSaysSo(t *testing.T) {
	if !strings.Contains(renderTable(nil, nil, nil, "", nil), "No agents yet.") {
		t.Error("an empty roster does not say so")
	}
}

// The screen answers the question somebody arrived with, including when the
// answer is no. Most voices on the record are not agents.
func TestTheScreenAnswersAboutTheVoiceSomebodyCameFor(t *testing.T) {
	marked := renderTable(agentList(), nil, names(), "scribe", nil)
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
	form := addPanel()
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
// Two things are exempt and only they, both for the same reason: they are
// documents another program reads, not sentences a person reads. The
// emitted configuration spells the five variable names soulstream-core
// reads and no others, or the agent does not start. The declaration blocks
// spell the field names the declaration package parses and no others, or
// the document a person copies to the command line is not the document the
// command line takes.
//
// The exemption is narrow by construction and proven so: the screen is
// rendered with no credential on it (every state but one), and the
// declaration blocks are cut out by the attribute that marks them, with a
// control that has to fire or the cut proves nothing.
func TestNothingServedSaysTheRetiredWord(t *testing.T) {
	banned := []string{"realm", "fold", "idp", "OIDC", "persona", "sentinel", "token"}
	check := func(what, body string) {
		t.Helper()
		for _, word := range banned {
			if strings.Contains(strings.ToLower(body), strings.ToLower(word)) {
				t.Errorf("%s says %q", what, word)
			}
		}
	}
	check("the agents screen", renderAgents(agentList(), nil, names(), "scribe", nil, declareView{}))

	// The same screen with the declare lane on: everything a person reads
	// keeps the same words, and only the documents are exempt.
	full := renderAgents(agentList(), nil, names(), "scribe", nil, fullDeclareView())
	prose, cut := withoutDeclarations(full)
	if cut == 0 {
		t.Fatal("nothing was cut out — the exemption cannot be narrow if it caught nothing")
	}
	check("the declare lane", prose)
	// The control: the cut is what makes the check pass, so the words must
	// really be in what was cut.
	if !strings.Contains(strings.ToLower(full), "persona") {
		t.Fatal("the declaration blocks carry none of the record's own words — " +
			"the exemption is measuring nothing")
	}
}

// withoutDeclarations removes every block marked as a document another
// program reads, and says how many it removed.
func withoutDeclarations(body string) (string, int) {
	var b strings.Builder
	rest, cut := body, 0
	for {
		i := strings.Index(rest, "data-declaration>")
		if i < 0 {
			b.WriteString(rest)
			return b.String(), cut
		}
		b.WriteString(rest[:i])
		rest = rest[i+len("data-declaration>"):]
		j := strings.Index(rest, "</textarea>")
		if j < 0 {
			return b.String(), cut
		}
		rest = rest[j:]
		cut++
	}
}
