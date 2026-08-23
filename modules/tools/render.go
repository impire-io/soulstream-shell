package tools

import (
	"fmt"
	"strings"

	"github.com/impire-io/soulstream-core/toolcatalog"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// The screen: what this soulstream can reach, this person's own standing
// on each remote tool, and the administrator's forms. Plain words — people
// read about tools and connected accounts; "grant", "resource" and "link"
// stay in the machine room.

// The screen's patch targets. The add-form's result line is its own,
// inside the slide-over holding the form.
const (
	resultID  = "tools-result"
	listID    = "tools-list"
	addNoteID = "tools-add-note"
)

// view is one read of everything the screen shows.
type view struct {
	Tools     []soulstream.Tool
	Connected map[string]bool
	Notes     []string
	Admin     bool
	// Msg is a message carried across the ceremony's redirects.
	Msg string
	Err string
}

// renderTools is the whole screen's body. The catalog leads; the admin's
// add-form waits in the slide-over behind its own key, because most visits
// are about the tools there are.
func renderTools(v view) string {
	var b strings.Builder
	b.WriteString(`<h1>Tools</h1>`)
	b.WriteString(`<p class="lede">What this soulstream can reach — services elsewhere you ` +
		`connect your own account to, and tools running here. Agents use them through ` +
		`their own door, always as the person they act for.</p>`)
	if v.Msg != "" {
		fmt.Fprintf(&b, `<p class="note">%s</p>`, esc(v.Msg))
	}
	key := ""
	if v.Admin {
		key = `<p class="act"><button type="button" class="btn" data-on:click="$panel = true">` +
			`Add tool</button></p>`
	}
	b.WriteString(`<div class="section">` + key + renderList(v) + `</div>`)
	b.WriteString(resultNote(""))
	if v.Admin {
		b.WriteString(shell.SlideOver("Add a tool", addPanel()))
	}
	return b.String()
}

// renderList is the merged catalog — and a patch target of its own so an
// act can hand back what is now true.
func renderList(v view) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, listID)
	for _, n := range v.Notes {
		fmt.Fprintf(&b, `<p class="note">%s</p>`, esc(n))
	}
	switch {
	case v.Err != "":
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
	case len(v.Tools) == 0:
		b.WriteString(`<p class="blank">No tools yet.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Tool</th><th>What it is</th><th>Where</th><th>You</th><th>Actions</th>` +
			`</tr></thead><tbody>`)
		for _, t := range v.Tools {
			b.WriteString(toolRow(t, v))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// toolRow is one tool: its kind in plain words, where a door reaches it,
// this person's own standing on it, and the acts each column honestly
// offers.
func toolRow(t soulstream.Tool, v view) string {
	kind := "runs here"
	if t.Kind == toolcatalog.KindRemote {
		kind = "connected service"
	} else if t.Kind != toolcatalog.KindWorkload {
		kind = string(t.Kind)
	}
	what := fmt.Sprintf(`<span class="label">%s</span> %s`, esc(kind), esc(t.Description))
	if t.Persona != "" {
		what += fmt.Sprintf(` <span class="mono">@%s</span>`, esc(t.Persona))
	}
	where := `<span class="note">not serving</span>`
	if t.Endpoint != "" {
		where = fmt.Sprintf(`<span class="mono">%s</span>`, esc(t.Endpoint))
	}

	// The person's own column: only a remote tool with its ceremony half
	// standing offers Connect; the honest states are said, not blanked.
	you := `<span class="note">—</span>`
	var acts []string
	if t.Kind == toolcatalog.KindRemote {
		switch {
		case !t.OnPlane:
			you = `<span class="pill warn">isn't serving</span>`
		case v.Connected != nil && v.Connected[t.Name]:
			you = `<span class="pill ok"><span class="led ok"></span>connected</span>`
			acts = append(acts, fmt.Sprintf(
				`<button class="btn ghost" data-on:click="@post('/act/tool-disconnect?name=%s')">`+
					`Disconnect</button>`, qesc(t.Name)))
		default:
			you = `<span class="note">not connected</span>`
			acts = append(acts, fmt.Sprintf(
				`<a class="btn ghost" href="/tools/connect?name=%s">Connect</a>`, qesc(t.Name)))
		}
	}
	if v.Admin {
		if t.Declared {
			acts = append(acts, `<span class="note">declared in configuration</span>`)
		} else {
			// Removing stands behind its own question (askRemove): one stray
			// tap takes nothing away from everyone.
			acts = append(acts, fmt.Sprintf(
				`<button class="btn ghost" data-on:click="@get('/tools/remove-ask?name=%s')">`+
					`Remove</button>`, qesc(t.Name)))
		}
	}
	return fmt.Sprintf(`<tr><td class="mono">%s</td><td>%s</td><td>%s</td><td>%s</td>`+
		`<td><div class="acts">%s</div></td></tr>`,
		esc(t.Name), what, where, you, strings.Join(acts, ""))
}

// addPanel is the administrator's one form, both kinds — the slide-over's
// whole body. The client secret goes to the identity plane and nowhere
// else — the form says so, and no page ever serves it back.
//
// The form shows the fields the chosen kind actually reads and no others:
// the Kind select drives a page-local signal, and each branch stands
// behind it. Half a screen of dead fields for whichever kind was picked
// was the old shape, and it overwhelmed for nothing. The provider's
// sign-in details fold further still — the everyday face is four fields.
func addPanel() string {
	return `<p class="lede">A connected service takes its provider's sign-in details — the ` +
		`secret goes to the identity plane and is never shown again. A tool running here ` +
		`takes its name and address.</p>` +
		`<form id="tool-add" data-signals="{kind:'remote'}" ` +
		`data-on:submit="@post('/act/tool-add', {contentType:'form'})">` +
		`<div class="fields">` +
		`<label class="field">Name<input name="name" required autocomplete="off" spellcheck="false" ` +
		`placeholder="lowercase, no spaces"></label>` +
		`<label class="field">Kind<select name="kind" data-bind:kind>` +
		`<option value="remote">connected service</option>` +
		`<option value="workload">runs here</option></select></label>` +
		`<label class="field">Address<input name="endpoint" autocomplete="off" spellcheck="false" ` +
		`placeholder="where its MCP server answers"></label>` +
		`<label class="field">Description<input name="description" autocomplete="off" ` +
		`placeholder="one line for screens and agents — optional"></label>` +
		`</div>` +
		`<div class="fields" data-show="$kind == 'workload'">` +
		`<label class="field">Runs as<input name="persona" autocomplete="off" ` +
		`spellcheck="false" placeholder="the name it takes part under"></label>` +
		`</div>` +
		`<div data-show="$kind == 'remote'">` +
		`<details class="stow"><summary>Provider sign-in</summary>` +
		`<p class="note">From the service's own developer settings. Without these the tool ` +
		`is listed, but nobody can connect an account to it yet.</p>` +
		`<div class="fields">` +
		`<label class="field">Authorize URL<input name="auth_url" autocomplete="off" spellcheck="false"></label>` +
		`<label class="field">Token URL<input name="token_url" autocomplete="off" spellcheck="false"></label>` +
		`<label class="field">Revoke URL<input name="revoke_url" autocomplete="off" spellcheck="false"></label>` +
		`<label class="field">Client id<input name="client_id" autocomplete="off" spellcheck="false"></label>` +
		`<label class="field">Client secret<input name="client_secret" type="password" ` +
		`autocomplete="off"></label>` +
		`<label class="field">Scopes<input name="scopes" autocomplete="off" spellcheck="false" ` +
		`placeholder="space-separated"></label>` +
		`<label class="field">Return address<input name="redirect_uri" autocomplete="off" ` +
		`spellcheck="false" placeholder="this shell's /tools/callback"></label>` +
		`</div></details></div>` +
		`<button class="btn" type="submit">Add tool</button>` +
		addNote("") + `</form>`
}

// addNote is the slide-over form's own result line.
func addNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, addNoteID, esc(msg))
}

// removeConfirm is the question removing stands behind: what goes, what
// keeps its custody, and the two ways out.
func removeConfirm(name string) string {
	return fmt.Sprintf(`<div id="%s" class="note">`+
		`<p class="confirm">Remove %s for everyone? Anyone&#39;s own connections to it `+
		`keep their custody until they disconnect.</p>`+
		`<div class="acts">`+
		`<button class="btn" data-on:click="@post('/act/tool-remove?name=%s')">Yes, remove it</button>`+
		`<button class="btn ghost" data-on:click="@get('/tools/remove-ask')">Keep it</button>`+
		`</div></div>`, resultID, esc(name), qesc(name))
}

// resultNote is the screen's result line — the patch target every act
// answers to.
func resultNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, resultID, esc(msg))
}
