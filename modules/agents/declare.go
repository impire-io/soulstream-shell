package agents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-workloads/declaration"

	"github.com/impire-io/soulstream-shell/shell"
)

// Declaring an agent: the second lane of this screen (hq design
// soulstream-shell 0009). The other lane hands a person a block to run an
// agent on their own machine; this one places the agent on the deployment,
// where it keeps answering with nobody's laptop open.
//
// The form's output IS the declaration — the same document the command line
// takes, built here and validated by the package that owns it, so there is
// never a second schema to drift from. Every refusal on this screen is
// upstream's own sentence, printed unchanged: a shell that rewords a
// refusal is a shell that will one day reword it wrongly.

// declaredArtifact is what the declaration's required artifact field
// carries for an agent the deployment serves through its wake engine.
//
// The field is required by the declaration's own validation and means
// nothing here: an engine-served agent runs the deployment's harness and
// never the declared executable (workloads design 0007 §9 names this as an
// open, "required by Validate, meaningless for engine-served agents").
// Until that is settled upstream, every such declaration carries this same
// placeholder — the command line's own declarations do too. It is written
// once, here, and shown in the JSON view rather than hidden, because the
// document on screen has to be the document that would be submitted.
const declaredArtifact = "file:///dev/null"

// The budget the form offers when a person has said nothing about limits.
// They are the wake engine's own defaults, restated here because the engine
// exports no way to ask for them — a number a screen shows must be a number
// somebody can read, so it is written down rather than left blank.
const (
	defaultMaxHops   = 4
	defaultWindowMax = 8
	defaultWindowPer = "10m"
)

// declareRoutes is what this lane claims, mounted with the rest.
func (m *Module) mountDeclare(rt shell.Router) {
	rt.HandleFunc("POST /agents/declare-json", m.declareJSON)
	rt.HandleFunc("POST /act/agent-declare", m.actDeclare)
}

// placing reports whether this deployment places agents at all — the
// declared fact this lane is drawn by, asked once per render and never
// probed.
func (m *Module) placing() bool { return m.sp.PlacementsTopic() != "" }

// declareRead is one reading of everything this lane shows. Every part of
// it is read fresh here and nothing is kept: restart the surface mid-way
// and the same screen derives again, because there is no store to disagree
// with the record.
//
// A part that cannot be read contributes nothing rather than failing the
// screen — the pickers offer what there is, and what is missing says so
// where it would have been.
func (m *Module) declareRead(r *http.Request) declareView {
	if !m.placing() {
		return declareView{}
	}
	ctx := r.Context()
	v := declareView{On: true, Role: m.sp.CapabilityRole()}
	v.List, v.Err = m.sp.Declared(ctx)
	if entries, err := topic.Board(ctx, m.sp.Reader()); err == nil {
		v.Board = entries
	}
	v.Models, v.ModelsErr = m.sp.ModelNames(ctx)
	if v.Role != "" {
		if tools, _, err := m.sp.Tools(ctx); err == nil {
			for _, t := range tools {
				v.Tools = append(v.Tools, t.Name)
			}
		}
	}
	return v
}

// declarationFrom builds the declaration one filled-in form describes. It
// is pure: given the fields a browser posted and the name of the signing
// role this deployment declared, it says what document would be submitted,
// so what the JSON view shows and what the act submits are the same thing
// computed the same way.
//
// It shapes and never judges. Whether the result is a declaration this
// realm will accept is decided by Validate, whose words this screen prints.
func declarationFrom(f url.Values, capabilityRole string) declaration.Declaration {
	d := declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   strings.TrimSpace(f.Get("name")),
		Topic:     strings.TrimSpace(f.Get("home")),
		Artifact:  declaredArtifact,
	}
	d.Wake = wakeFrom(f)

	if topicPath, artefact := strings.TrimSpace(f.Get("instr_home")),
		strings.TrimSpace(f.Get("instr_name")); topicPath != "" || artefact != "" {
		d.Instructions = &declaration.Instructions{Topic: topicPath, Artefact: artefact}
	}
	if model := strings.TrimSpace(f.Get("model")); model != "" {
		d.Inference = &declaration.InferenceSpec{Model: model}
	}
	// Tools ride as capability names against the role the deployment
	// declared. With no role declared there is no name to resolve them
	// through, so no capability block is written at all — a block naming a
	// role nobody declared is a placement that refuses at claim time.
	if tools := trimmed(f["tools"]); len(tools) > 0 && capabilityRole != "" {
		d.Capabilities = &declaration.Capabilities{Role: capabilityRole, Tools: tools}
	}
	d.Budget = budgetFrom(f)
	return d
}

// wakeFrom reads the wake section: a mention entry unless it was turned
// off, and one entry per other kind the person filled in.
func wakeFrom(f url.Values) []declaration.WakeEntry {
	var wake []declaration.WakeEntry
	if f.Get("wake_mention") != "" {
		wake = append(wake, declaration.WakeEntry{Kind: declaration.WakeMention})
	}
	if path := strings.TrimSpace(f.Get("wake_conversation")); path != "" {
		e := declaration.WakeEntry{Kind: declaration.WakeTopic, Path: path}
		if types := splitFields(f.Get("wake_kinds")); len(types) > 0 {
			e.Types = types
		}
		wake = append(wake, e)
	}
	name, pattern := strings.TrimSpace(f.Get("sched_name")), strings.TrimSpace(f.Get("sched_pattern"))
	if name != "" || pattern != "" {
		wake = append(wake, declaration.WakeEntry{
			Kind: declaration.WakeSchedule, Name: name, Pattern: pattern,
			TTL: strings.TrimSpace(f.Get("sched_keep")),
		})
	}
	if subject := strings.TrimSpace(f.Get("wake_subject")); subject != "" {
		wake = append(wake, declaration.WakeEntry{Kind: declaration.WakeSubject, Subject: subject})
	}
	return wake
}

// budgetFrom reads the limits section. The fields are always on screen with
// their numbers in them, so an empty one is a person clearing a limit and
// is carried through as the zero the schema reads as "no bound".
func budgetFrom(f url.Values) *declaration.BudgetSpec {
	b := declaration.BudgetSpec{MaxHops: atoi(f.Get("budget_hops"))}
	most := atoi(f.Get("budget_max"))
	per := strings.TrimSpace(f.Get("budget_per"))
	if most > 0 || per != "" {
		b.Window = &declaration.WindowSpec{Max: most, Per: per}
	}
	return &b
}

// atoi reads a number a person typed; anything that is not one is nothing.
func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// splitFields is a space- or comma-separated list as a person writes it.
func splitFields(s string) []string {
	return trimmed(strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}))
}

// trimmed drops the empties a multi-select and a text field both produce.
func trimmed(in []string) []string {
	var out []string
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// declarationJSON is the document a form describes, exactly as the command
// line would take it. It is what the JSON view shows and what a person
// could save to a file and submit themselves — one schema, one shape, both
// paths learnable from the other.
func declarationJSON(d declaration.Declaration) (string, error) {
	body, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// declareForm reads the act's form and answers the declaration it
// describes, or the note explaining why the form did not arrive whole.
func (m *Module) declareForm(r *http.Request) (declaration.Declaration, string) {
	if err := r.ParseForm(); err != nil {
		return declaration.Declaration{}, "That form did not arrive whole: " + err.Error()
	}
	return declarationFrom(r.PostForm, m.sp.CapabilityRole()), ""
}

// declareJSON shows the document the form describes, and what the packages
// that own it say about it. It writes nothing: it is the form reading
// itself back, so a person can see the exact file the command line takes
// before anything is placed anywhere.
func (m *Module) declareJSON(w http.ResponseWriter, r *http.Request) {
	if m.sp.Session(r) == nil || !m.placing() {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	d, bad := m.declareForm(r)
	if bad != "" {
		shell.Patch(w, declareNote(bad))
		return
	}
	body, err := declarationJSON(d)
	if err != nil {
		shell.Patch(w, declareNote("Writing that out failed: "+err.Error()))
		return
	}
	shell.Patch(w, declarationView(body))
	// The refusal, if there is one, in the words of the package that
	// refuses — never this screen's paraphrase of them.
	if err := d.Validate(); err != nil {
		shell.Patch(w, declareNote(err.Error()))
		return
	}
	shell.Patch(w, declareNote(""))
}

// actDeclare places the agent. The person signed in is the one placing it,
// so the placement is opened on their own admission and signed with their
// own key — and the moment it is on the record this screen has nothing
// left to hold: closing the tab does not un-place it.
func (m *Module) actDeclare(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil || !m.placing() {
		shell.Patch(w, declareNote("Sign in first."))
		return
	}
	d, bad := m.declareForm(r)
	if bad != "" {
		shell.Patch(w, declareNote(bad))
		return
	}
	// Validated here as well as inside the submit, so the refusal answers
	// beside the fields it is about rather than on the screen behind them.
	if err := d.Validate(); err != nil {
		shell.Patch(w, declareNote(err.Error()))
		return
	}
	if _, err := sess.Declare(r.Context(), d); err != nil {
		shell.Patch(w, declareNote(err.Error()))
		return
	}
	// The panel goes away so the answer is in front of the person rather
	// than behind the form they just finished with.
	shell.PatchSignals(w, `{panel: false}`)
	shell.Patch(w, declareNote(""))
	shell.Patch(w, resultNote(fmt.Sprintf(
		"%s is declared. Nothing here holds it now — close this screen and it still "+
			"arrives. Its line below says where it stands.", d.Persona)))
	m.patchDeclared(w, r)
}

// patchDeclared hands back the placed agents as they now stand.
func (m *Module) patchDeclared(w http.ResponseWriter, r *http.Request) {
	list, err := m.sp.Declared(r.Context())
	shell.Patch(w, renderDeclared(list, err))
}
