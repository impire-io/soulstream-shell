package agents

import (
	"fmt"
	"strings"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-workloads/declaration"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// The declare lane's markup: the table of agents this deployment runs
// itself, the form that puts one there, and the read-only list of the names
// an agent can be told to think through.
//
// The words are the person's throughout. What the record calls these things
// stays on the record; the one place the machine room's own spelling
// reaches this screen is the JSON view, which has to be exactly the
// document the command line takes or it is worth nothing.

// This lane's patch targets. The declare form's result line is its own,
// inside the slide-over beside the fields it is about.
const (
	declaredID    = "agents-declared"
	declareNoteID = "agents-declare-note"
	declareJSONID = "agents-declare-json"
)

// declareView is everything this lane reads for one render, gathered by the
// module and shaped here.
type declareView struct {
	// On says the deployment places agents at all — the declared fact this
	// lane is drawn by. Off, none of it is served.
	On bool
	// List is the placements as the log has them; Err is what went wrong
	// reading them, said rather than blanked.
	List []soulstream.Declared
	Err  error
	// Board is the conversations a declared agent can be given a home in
	// and pointed at.
	Board []topic.BoardEntry
	// Models are the names this deployment has given the assistants an
	// agent may think through; ModelsErr is what went wrong reading them.
	Models    []string
	ModelsErr error
	// Tools are the names of what this soulstream can reach, and Role is
	// the signing role they resolve through — no role, no picker.
	Tools []string
	Role  string
}

// renderDeclared is the table of agents this deployment runs itself, and a
// patch target of its own so the act that places one — and the live channel
// that watches it arrive — can hand back what is now true.
//
// No row is marked. The act that places an agent names it in the result
// line, and a highlight the next tick would take away again reads as a
// fault rather than as an answer.
func renderDeclared(list []soulstream.Declared, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, declaredID)
	switch {
	case err != nil:
		fmt.Fprintf(&b, `<p class="blank">Reading the agents this soulstream runs failed: %s</p>`,
			esc(err.Error()))
	case len(list) == 0:
		b.WriteString(`<p class="blank">None yet. An agent declared here runs on this ` +
			`soulstream instead of on somebody&#39;s machine — it keeps answering with ` +
			`nobody&#39;s laptop open. Declare the first with the key above.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Name</th><th>Wakes on</th><th>Thinks with</th><th>State</th><th>Declared</th>` +
			`</tr></thead><tbody>`)
		for _, d := range list {
			b.WriteString(declaredRow(d))
		}
		b.WriteString(`</tbody></table></div>`)
		b.WriteString(waitingNote(list))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// declaredRow is one placed agent. The declaration itself rides a fold
// under the row's own name: what was asked for is worth reading, and it is
// worth reading only when somebody asks.
func declaredRow(d soulstream.Declared) string {
	model := `<span class="dim">the assistant already set up here</span>`
	if d.Model != "" {
		model = fmt.Sprintf(`<span class="mono">%s</span>`, esc(d.Model))
	}
	declared := "—"
	declaredFull := ""
	if !d.Opened.IsZero() {
		declared = d.Opened.Format("2006-01-02")
		declaredFull = d.Opened.UTC().Format("2006-01-02 15:04Z")
	}
	return fmt.Sprintf(`<tr><td><span class="led machine"></span> %s%s</td>`+
		`<td>%s</td><td>%s</td><td>%s</td><td class="mono" title="%s">%s</td></tr>`,
		esc(d.Name), declarationFold(d), wakeCell(d.Wakes), model,
		stateCell(d), esc(declaredFull), esc(declared))
}

// wakeCell says what wakes an agent, one short word per way — with the
// delivery each way promises in the hover, in the words the package that
// decides it uses. A shell that shortens those words is a shell deciding
// what a person may know about whether a wake can go missing.
func wakeCell(wakes []soulstream.Wake) string {
	if len(wakes) == 0 {
		return `<span class="dim">nothing</span>`
	}
	var out []string
	for _, w := range wakes {
		title := w.Delivery
		if w.Detail != "" {
			title = w.Detail + " · " + title
		}
		cls := "pill"
		if w.Kind == "subject" {
			// The one kind that can lose a wake. It is marked, because a
			// person reading a row should not have to hover to learn that.
			cls = "pill warn"
		}
		out = append(out, fmt.Sprintf(`<span class="%s" title="%s">%s</span>`,
			cls, esc(title), esc(wakeWord(w.Kind))))
	}
	return `<div class="acts">` + strings.Join(out, "") + `</div>`
}

// wakeWord is a wake kind in the person's own word. An unknown kind arrives
// as itself: a newer record outranks this list.
func wakeWord(kind string) string {
	switch kind {
	case "mention":
		return "mentions"
	case "topic":
		return "a conversation"
	case "schedule":
		return "a schedule"
	case "subject":
		return "a message"
	default:
		return kind
	}
}

// stateCell is where the placement stands, in the person's words. Open is
// not a failure and is not a spinner: nothing has taken this on yet, which
// the note under the table says in full.
func stateCell(d soulstream.Declared) string {
	switch d.State {
	case topic.WorkClaimed:
		if d.Owner != "" {
			return fmt.Sprintf(`<span class="pill ok"><span class="led ok"></span>claimed by %s</span>`,
				esc(d.Owner))
		}
		return `<span class="pill ok"><span class="led ok"></span>claimed</span>`
	case topic.WorkDone:
		return `<span class="pill">finished</span>`
	case topic.WorkOpen:
		return `<span class="pill warn">declared</span>`
	default:
		return fmt.Sprintf(`<span class="pill">%s</span>`, esc(string(d.State)))
	}
}

// waitingNote is the truth about an agent nothing has picked up, said once
// under the table rather than repeated in every row: honest waiting, never
// a spinner and never an error.
func waitingNote(list []soulstream.Declared) string {
	for _, d := range list {
		if d.State == topic.WorkOpen {
			return `<p class="note">declared; nothing serves agents here yet — ` +
				`the deployment enables the dispatcher plane</p>`
		}
	}
	return ""
}

// declarationFold is what was asked for, under the row it belongs to. It is
// the same document the command line takes, so a person who reads it once
// has learned the other path too.
func declarationFold(d soulstream.Declared) string {
	if d.JSON == "" {
		return ""
	}
	return fmt.Sprintf(`<details class="stow"><summary>What was asked for</summary>`+
		`<textarea readonly rows="%d" data-declaration>%s</textarea></details>`,
		jsonRows(d.JSON), esc(d.JSON))
}

// declarePanel is the declare form: the whole of what a person says to put
// an agent on this deployment, and nothing the record would not carry.
//
// It is a three-step wizard (the calm pass, the kit's own shape): name
// it, wake it, instruct it — with the limits an agent starts under
// folded as the step's advanced matter, prefilled and editable, because
// the bound an agent runs inside is a fact a person may read. Steps are
// visibility, not pages: the form stays one and every field submits at
// the end.
func declarePanel(v declareView) string {
	var b strings.Builder
	b.WriteString(`<div data-show="$make == 'here'">`)
	b.WriteString(`<p class="lede">This one runs here, on this soulstream. It keeps ` +
		`answering with nobody&#39;s laptop open, and it stops when you say so.</p>`)
	b.WriteString(`<form id="agent-declare" data-signals="{dstep:0}" ` +
		`data-on:submit="@post('/act/agent-declare', {contentType:'form'})">`)
	b.WriteString(shell.Steps("$dstep", "Name it", "Wake it", "Instruct it"))

	b.WriteString(`<div data-show="$dstep == 0">`)
	b.WriteString(`<div class="fields">`)
	b.WriteString(`<label class="field">Name<input name="name" required autocomplete="off" ` +
		`spellcheck="false" placeholder="lowercase, no spaces"></label>`)
	b.WriteString(`<label class="field">Home conversation` + conversationSelect(v, "home", "") +
		`</label>`)
	b.WriteString(`</div></div>`)

	b.WriteString(`<div data-show="$dstep == 1">`)
	b.WriteString(`<div class="fields">` +
		`<label class="field">Whenever somebody says its name<select name="wake_mention">` +
		`<option value="on">yes</option><option value="">no</option></select></label>` +
		`</div>`)
	b.WriteString(`<p class="note">Every mention reaches it, including ones sent while it ` +
		`was not running.</p>`)
	b.WriteString(`<details class="stow"><summary>More ways to wake it</summary>` +
		`<div class="fields">` +
		`<label class="field">Anything said in a conversation` +
		conversationSelect(v, "wake_conversation", "— optional") + `</label>` +
		`<label class="field">Only these kinds of message<input name="wake_kinds" ` +
		`autocomplete="off" spellcheck="false" ` +
		`placeholder="everything said, unless you narrow it — optional"></label>` +
		`<label class="field">On a schedule, called<input name="sched_name" ` +
		`autocomplete="off" spellcheck="false" placeholder="lowercase, no spaces — optional"></label>` +
		`<label class="field">Which runs<input name="sched_pattern" autocomplete="off" ` +
		`spellcheck="false" placeholder="@every 1h — optional"></label>` +
		`<label class="field">Catching up at most<input name="sched_keep" autocomplete="off" ` +
		`spellcheck="false" placeholder="24h — optional"></label>` +
		`<label class="field">When a message arrives about<input name="wake_subject" ` +
		`autocomplete="off" spellcheck="false" placeholder="— optional"></label>` +
		`</div>` +
		`<p class="note">A schedule catches up on what it missed, up to the time you give. ` +
		`A message is the one way that can go missing: one arriving while the agent is not ` +
		`running is gone.</p></details></div>`)

	b.WriteString(`<div data-show="$dstep == 2">`)
	b.WriteString(`<p class="note">What it is told to do lives in a document kept in a ` +
		`conversation. Rewrite it and the next answer follows the new one — nothing to restart.</p>`)
	b.WriteString(`<div class="fields">` +
		`<label class="field">Kept in` + conversationSelect(v, "instr_home", "— optional") +
		`</label>` +
		`<label class="field">Called<input name="instr_name" autocomplete="off" ` +
		`spellcheck="false" placeholder="the document&#39;s name — optional"></label>` +
		`</div>`)

	b.WriteString(`<h3 class="label">How it thinks</h3>`)
	b.WriteString(modelField(v))

	if v.Role != "" {
		b.WriteString(`<h3 class="label">What it may use</h3>`)
		b.WriteString(toolsField(v))
	}

	// The limits stay unfolded inside their step — design 0009's own rule,
	// kept through the calm pass: a bound nobody sees is a bound nobody
	// knows they are running under. The wizard does the calming; the
	// numbers stay on the screen.
	b.WriteString(`<h3 class="label">Limits</h3>`)
	b.WriteString(`<p class="note">What keeps an agent that sets off other agents from ` +
		`running away. These are the ones it starts with; change them if you know better.</p>`)
	fmt.Fprintf(&b, `<div class="fields">`+
		`<label class="field">Agents it may set off in a chain<input name="budget_hops" `+
		`type="number" min="0" value="%d"></label>`+
		`<label class="field">Answers in one conversation<input name="budget_max" `+
		`type="number" min="0" value="%d"></label>`+
		`<label class="field">Within<input name="budget_per" autocomplete="off" `+
		`spellcheck="false" value="%s"></label>`+
		`</div></div>`, declaration.DefaultBudget.MaxHops,
		declaration.DefaultBudget.Window.Max, declaration.DefaultBudget.Window.Per)

	b.WriteString(`<div class="wiz-foot">` +
		`<p class="whisper" data-show="$dstep == 0">Step 1 of 3 — what wakes it comes next.</p>` +
		`<p class="whisper" data-show="$dstep == 1">Step 2 of 3</p>` +
		`<p class="whisper" data-show="$dstep == 2">Step 3 of 3 — instructions can change ` +
		`any time after.</p>` +
		`<span class="keys">` +
		`<button class="btn ghost" type="button" data-show="$dstep > 0" ` +
		`data-on:click="$dstep = $dstep - 1">Back</button>` +
		`<button class="btn" type="button" data-show="$dstep < 2" ` +
		`data-on:click="$dstep = $dstep + 1">Next</button>` +
		`<button class="btn ghost" type="button" data-show="$dstep == 2" ` +
		`data-on:click="@post('/agents/declare-json', {contentType:'form'})">Show as JSON</button>` +
		`<button class="btn" type="submit" data-show="$dstep == 2">Declare agent</button>` +
		`</span></div>`)
	b.WriteString(declareNote(""))
	b.WriteString(declarationView(""))
	b.WriteString(`</form></div>`)
	return b.String()
}

// conversationSelect offers the conversations there are. An archived one is
// kept for reading, not for putting an agent in, so it is not offered.
func conversationSelect(v declareView, field, optional string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<select name="%s"`, field)
	if optional == "" {
		b.WriteString(` required`)
	}
	b.WriteString(`>`)
	if optional != "" {
		fmt.Fprintf(&b, `<option value="">%s</option>`, esc(optional))
	}
	for _, e := range v.Board {
		if e.Lifecycle == topic.Archived {
			continue
		}
		name := e.Announcement.Name
		if name == "" {
			name = e.Path
		}
		fmt.Fprintf(&b, `<option value="%s">%s</option>`, esc(e.Path), esc(name))
	}
	b.WriteString(`</select>`)
	return b.String()
}

// modelField is the picker over the names this soulstream has given the
// assistants an agent may think through. Saying nothing is a real answer
// and says what it means; an empty list offers the one act that fills it,
// in words, because that act is not this screen's to take.
func modelField(v declareView) string {
	if v.ModelsErr != nil {
		return fmt.Sprintf(`<p class="note">Reading the names failed: %s</p>`,
			esc(v.ModelsErr.Error()))
	}
	if len(v.Models) == 0 {
		return `<p class="note">No assistants are named here yet, so this agent will ` +
			`use the one this soulstream already thinks with. To name one, run ` +
			`<span class="mono">soulstream model set &lt;name&gt;</span> where this ` +
			`soulstream runs.</p>`
	}
	var b strings.Builder
	b.WriteString(`<div class="fields"><label class="field">Thinks with<select name="model">`)
	b.WriteString(`<option value="">the one this soulstream already uses</option>`)
	for _, n := range v.Models {
		fmt.Fprintf(&b, `<option value="%s">%s</option>`, esc(n), esc(n))
	}
	b.WriteString(`</select></label></div>`)
	return b.String()
}

// toolsField is the picker over what this soulstream can reach. Picking
// nothing is the ordinary case: an agent that needs no tools is declared
// without any.
func toolsField(v declareView) string {
	if len(v.Tools) == 0 {
		return `<p class="note">Nothing is connected here yet. Add a tool on the Tools ` +
			`screen and it can be offered to an agent.</p>`
	}
	var b strings.Builder
	b.WriteString(`<p class="note">Hold to pick more than one. An agent reaches exactly ` +
		`what you pick here and nothing else.</p>`)
	b.WriteString(`<div class="fields"><label class="field">May use` +
		`<select name="tools" multiple size="4">`)
	for _, n := range v.Tools {
		fmt.Fprintf(&b, `<option value="%s">%s</option>`, esc(n), esc(n))
	}
	b.WriteString(`</select></label></div>`)
	return b.String()
}

// declareNote is the declare form's own result line — what the packages
// that own the document said about it, in their words.
func declareNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, declareNoteID, esc(msg))
}

// declarationView is the document the form describes, served empty and
// filled on demand: the exact file the command line takes, so neither path
// has to be learned twice.
func declarationView(body string) string {
	if body == "" {
		return fmt.Sprintf(`<div id="%s"></div>`, declareJSONID)
	}
	return fmt.Sprintf(`<div id="%s"><details class="stow" open>`+
		`<summary>As JSON</summary>`+
		`<p class="note">The same file <span class="mono">soulstream agent submit</span> `+
		`takes.</p>`+
		`<textarea readonly rows="%d" data-declaration>%s</textarea></details></div>`,
		declareJSONID, jsonRows(body), esc(body))
}

// jsonRows sizes a read-only block to what is in it, between a glance and a
// screenful — a box that has to be scrolled to read one short document is a
// box that hides it.
func jsonRows(body string) int {
	n := strings.Count(body, "\n") + 1
	switch {
	case n < 4:
		return 4
	case n > 24:
		return 24
	default:
		return n
	}
}

// modelsList is the read-only list of the names an agent can be told to
// think through — folded side-matter under the tables, because it is
// background to what this screen is for.
//
// There is no act here on purpose. Naming a model is the command line's
// this slice, and a screen that offered a key it cannot honour would be
// worse than one that says where the act lives.
func modelsList(v declareView) string {
	var b strings.Builder
	b.WriteString(`<details class="stow"><summary>Models</summary>`)
	switch {
	case v.ModelsErr != nil:
		fmt.Fprintf(&b, `<p class="blank">Reading the names failed: %s</p>`,
			esc(v.ModelsErr.Error()))
	case len(v.Models) == 0:
		b.WriteString(`<p class="blank">None named yet. A name is what an agent is ` +
			`declared to think with, so the same declaration keeps working when what ` +
			`is behind the name changes. Name the first with ` +
			`<span class="mono">soulstream model set &lt;name&gt;</span> where this ` +
			`soulstream runs.</p>`)
	default:
		b.WriteString(`<div class="acts">`)
		for _, n := range v.Models {
			fmt.Fprintf(&b, `<span class="pill mono">%s</span>`, esc(n))
		}
		b.WriteString(`</div><p class="note">Named with ` +
			`<span class="mono">soulstream model set &lt;name&gt;</span> where this ` +
			`soulstream runs.</p>`)
	}
	b.WriteString(providerNote())
	b.WriteString(`</details>`)
	return b.String()
}

// providerNote is the one thing about models this screen deliberately
// cannot do. A provider's key is the deployment's own, kept where nothing
// this surface can reach ever sees it — so there is no field for it here
// and there never will be. What there is instead is the exact line that
// loads one, written to run unchanged under every shell a person might be
// in: no heredoc, no export.
func providerNote() string {
	return `<p class="note">A name needs an account with whoever provides it. The key ` +
		`for that account is loaded where this soulstream runs and is never typed into ` +
		`a screen:</p>` +
		`<label class="field">Paste block<textarea readonly rows="2" data-provider-command>` +
		esc("env SOULSTREAM_PROVIDER_KEY='<the key>' soulstream provider set anthropic") +
		`</textarea></label>`
}
