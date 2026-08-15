package agents

import (
	"fmt"
	"strings"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// The screen: one form for adding an agent, one table of the ones there
// are, and one result line under both. Every row is a machine voice, so
// every row carries the machine channel's lamp — the same teal the
// conversation screen puts beside the same voice, read from the same fact.

// The screen's two patch targets. An act answers with a fragment for one or
// both; nothing else on the page moves.
const (
	resultID = "agents-result"
	tableID  = "agents-table"
)

// renderAgents is the whole screen's body. who is the voice somebody came
// here looking for — empty when they simply opened the screen. The form and
// the roster each sit in a section of their own: the screen is read top to
// bottom, and the canon's rhythm (dense, never cramped) is the section gap.
func renderAgents(list []soulstream.Agent, err error, names map[string]string, who string) string {
	var b strings.Builder
	b.WriteString(`<h1>Agents</h1>`)
	b.WriteString(`<p class="lede">The AI assistants that take part here under their ` +
		`own names. Each gets in with a credential of its own, so what it says ` +
		`carries its name — and each can be stopped without touching anything else.</p>`)
	b.WriteString(lookedUpNote(list, err, who))
	b.WriteString(`<div class="section">` + renderAddForm() + `</div>`)
	b.WriteString(`<div class="section">` + renderTable(list, err, names, who) + `</div>`)
	b.WriteString(resultNote(""))
	return b.String()
}

// lookedUpNote answers the question somebody arrived with. Most voices on
// the record are not agents, so a name this list does not hold is answered
// rather than left to somebody hunting a row that was never there.
func lookedUpNote(list []soulstream.Agent, err error, who string) string {
	if who == "" || err != nil {
		return ""
	}
	for _, a := range list {
		if a.Handle == who {
			return fmt.Sprintf(`<p class="lede">Looking up <span class="mono">%s</span>, `+
				`marked below.</p>`, esc(who))
		}
	}
	return fmt.Sprintf(`<p class="lede">No agent here answers to `+
		`<span class="mono">%s</span> — every one that does is below.</p>`, esc(who))
}

// renderAddForm is how an agent comes into being. Two words from a person:
// what it is called on the record, and what to call it on a screen. Adding
// it is also vouching for it, which the form says out loud — the claim is
// signed with the key of whoever is reading this, and their name is what
// ends up beside the agent's for good.
func renderAddForm() string {
	return `<div class="card"><h2>Add an agent</h2>` +
		`<p class="lede">You vouch for what you add: your name goes on it, signed ` +
		`with your own key, and stays there.</p>` +
		`<form id="agent-add" data-on:submit="@post('/act/agent-add', {contentType:'form'})">` +
		`<div class="fields">` +
		`<label class="field">Handle` +
		`<input name="handle" autocomplete="off" spellcheck="false" ` +
		`placeholder="lowercase, no spaces"></label>` +
		`<label class="field">Shown as` +
		`<input name="shown" autocomplete="off" placeholder="what to call it on screen"></label>` +
		`</div>` +
		`<button class="btn" type="submit">Add agent</button>` +
		`</form></div>`
}

// renderTable is the roster itself, and a patch target of its own so an act
// that changes what is true can hand back what is now true.
func renderTable(list []soulstream.Agent, err error, names map[string]string, who string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, tableID)
	switch {
	case err != nil:
		fmt.Fprintf(&b, `<p class="blank">Reading the list of agents failed: %s</p>`,
			esc(err.Error()))
	case len(list) == 0:
		b.WriteString(`<p class="blank">No agents yet. Name one above — the screen ` +
			`answers with a credential and the command that brings it to life.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Name</th><th>Handle</th><th>Vouched for by</th>` +
			`<th>Can get in</th><th>Added</th><th>Actions</th>` +
			`</tr></thead><tbody>`)
		for _, a := range list {
			b.WriteString(agentRow(a, names, a.Handle == who))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// agentRow is one agent, marked when it is the one somebody came here for.
// The lamp is the machine channel's, and what it says when somebody hovers
// it is the record's own fact about the voice, in the record's words.
func agentRow(a soulstream.Agent, names map[string]string, lookedUp bool) string {
	name := a.ShownAs
	if name == "" {
		name = a.Handle
	}
	// The operator cell says who a person knows, with the handle beside it.
	// When the record offers no name beyond the handle, the handle stands
	// alone rather than beside a copy of itself.
	operator := names[a.OperatedBy]
	opCell := fmt.Sprintf(`%s <span class="mono">@%s</span>`, esc(operator), esc(a.OperatedBy))
	if operator == "" || operator == a.OperatedBy {
		operator = a.OperatedBy
		opCell = fmt.Sprintf(`<span class="mono">@%s</span>`, esc(a.OperatedBy))
	}
	state := `<span class="pill warn">no</span>`
	if a.Admitted() {
		state = `<span class="pill ok"><span class="led ok"></span>yes</span>`
	}
	added := "—"
	if !a.Added.IsZero() {
		added = a.Added.Format("2006-01-02")
	}
	// The keys stack when the row has no width to spare for them, which is
	// the last column's own business rather than the table's.
	var acts strings.Builder
	acts.WriteString(`<div class="acts">`)
	fmt.Fprintf(&acts, `<button class="btn ghost" data-on:click="@post('/act/agent-credential?who=%s')">`+
		`New credential</button>`, qesc(a.Handle))
	if a.Admitted() {
		fmt.Fprintf(&acts, `<button class="btn ghost" data-on:click="@post('/act/agent-revoke?who=%s')">`+
			`Take the credential away</button>`, qesc(a.Handle))
	}
	acts.WriteString(`</div>`)
	row := "<tr>"
	if lookedUp {
		row = `<tr class="on">`
	}
	// A handle has no word breaks of its own and is let wrap in a narrow
	// column, so the whole of it also rides in the hover: wrapped across two
	// lines, it is still one name.
	return fmt.Sprintf(`%s<td><span class="led machine" title="operated by %s"></span> %s</td>`+
		`<td class="mono" title="%s">%s</td><td>%s</td>`+
		`<td>%s</td><td class="mono">%s</td><td>%s</td></tr>`,
		row, esc(operator), esc(name), esc(a.Handle), esc(a.Handle),
		opCell, state, esc(added), acts.String())
}

// renderCredential is the one answer on this screen carrying a secret. The
// credential store keeps a digest it cannot reverse, so this is the only
// time the credential itself will ever exist on a screen — which the screen
// says, rather than leaving somebody to find out by reloading.
//
// The card leads with the paste block — the product's promise is one
// binary, one paste (design soulstream/0002 §4) — and folds the hard paths
// under it: easy is easy, hard stays possible.
func renderCredential(c soulstream.Credential, what string) string {
	name := c.ShownAs
	if name == "" {
		name = c.Handle
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s" class="card raised"><h2>%s %s</h2>`, resultID, esc(name), esc(what))
	b.WriteString(`<p class="lede">Shown once. Hand it over yourself — nothing keeps a copy, ` +
		`and reloading this screen will not bring it back. It does not run out on its own; ` +
		`take it away when this agent should stop.</p>`)
	b.WriteString(renderRunIt(c, name))
	b.WriteString(renderOtherWays(c))
	b.WriteString(`</div>`)
	return b.String()
}

// renderRunIt is the primary path, first and above every fold: paste one
// block, the agent runs. The block is written to run unchanged under POSIX
// shells and fish — no heredoc, no export — because the person pasting it
// chose their shell and this screen did not.
func renderRunIt(c soulstream.Credential, name string) string {
	var b strings.Builder
	b.WriteString(`<div class="setup" data-setup="wrap"><h3>Run your agent</h3>` +
		`<p>On the machine where your assistant is signed in — this one or any other:</p>`)
	b.WriteString(`<ol class="steps">` +
		`<li>Have the <span class="mono">soulstream</span> program there — the same one that runs ` +
		`this realm, from the <a href="https://github.com/impire-io/soulstream/releases">releases ` +
		`page</a>. Nothing else to install.</li>` +
		`<li>Paste this whole block into a terminal. It saves the agent&#39;s credentials ` +
		`file and starts answering mentions:</li></ol>`)
	fmt.Fprintf(&b, `<label class="field">Paste block`+
		`<textarea readonly id="wrap-cmd" rows="12" data-wrap-command>%s</textarea></label>`,
		esc(wrapBlock(c)))
	b.WriteString(`<p class="act"><button type="button" class="btn ghost" ` +
		`data-on:click="navigator.clipboard.writeText(document.getElementById('wrap-cmd').value); el.textContent = 'Copied'">` +
		`Copy the block</button></p>`)
	fmt.Fprintf(&b, `<p>Mentions of %s become answers even when nobody is at the keyboard; `+
		`mentions sent while it was off are answered when it starts. Stop it any time with `+
		`Ctrl-C — nothing is lost — and taking the credential away above stops it for good. `+
		`<span class="mono">--harness codex</span> wraps codex instead; `+
		`<span class="mono">--template your-file.json</span> wraps anything else with a `+
		`machine-readable finish.</p>`, esc(name))
	b.WriteString(`</div>`)
	return b.String()
}

// renderOtherWays keeps the hard paths possible, folded below the paste
// block: the same lane, spelled for each program that reads it. Every fold
// repeats only public halves or shapes beside the blocks that carry the one
// secret — and "shown once" above covers all of it.
func renderOtherWays(c soulstream.Credential) string {
	var b strings.Builder
	b.WriteString(`<div class="setup"><h3>Other ways to connect it</h3>` +
		`<p>For driving this agent&#39;s tools from an assistant yourself — ` +
		`without the always-on answering above:</p>`)

	fmt.Fprintf(&b, `<details data-setup="claude-code"><summary>Claude Code</summary><div class="fold">`+
		`<p>Save this as <span class="mono">.mcp.json</span> in the folder the agent works from — `+
		`it is already the exact file. If one exists there, add the <span class="mono">"soulstream"</span> `+
		`entry under its <span class="mono">mcpServers</span>. The tools appear on the next start; `+
		`headless runs want <span class="mono">--mcp-config .mcp.json</span>. On a machine that is `+
		`not this one, save the credentials file (last fold) and put its path in `+
		`<span class="mono">SOULSTREAM_CREDS</span>.</p>`+
		`<label class="field">.mcp.json<textarea readonly rows="14" data-credential>%s</textarea></label>`+
		`</div></details>`, esc(mcpConfig(c)))

	fmt.Fprintf(&b, `<details data-setup="codex"><summary>Codex</summary><div class="fold">`+
		`<p>Codex reads TOML, not JSON — the same values go in `+
		`<span class="mono">~/.codex/config.toml</span>:</p>`+
		`<label class="field">config.toml<textarea readonly rows="11" data-codex-config>%s</textarea></label>`+
		`</div></details>`, esc(codexConfig(c)))

	b.WriteString(`<details data-setup="other"><summary>Anything else that speaks MCP ` +
		`(pi.dev, opencode, …)</summary><div class="fold">` +
		`<p>Every such assistant has a place to declare an MCP server. Declare one named ` +
		`<span class="mono">soulstream</span> that runs the command <span class="mono">soulstream</span> ` +
		`with the one argument <span class="mono">mcp</span> and the five environment variables ` +
		`from the blocks above, spelled exactly that way. No URL endpoint — it is a stdio ` +
		`server the assistant starts itself.</p></div></details>`)

	if c.Sentinel != "" {
		fmt.Fprintf(&b, `<details data-setup="creds"><summary>The credentials file by itself</summary>`+
			`<div class="fold">`+
			`<p>The paste block already writes this file. By hand it is only needed on a machine `+
			`that is not this one — and it is safe to copy: it admits nobody by itself.</p>`+
			`<label class="field">Credentials file`+
			`<textarea readonly rows="6" data-sentinel>%s</textarea></label></div></details>`,
			esc(c.Sentinel))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// wrapBlock is the paste block: the whole of running this agent as one
// portable script. It writes the credentials file itself so the same block
// works on any machine — the deployment's own path is true only here — and
// it spells the five names exactly the way every door reads them.
func wrapBlock(c soulstream.Credential) string {
	var b strings.Builder
	creds := "'" + c.SentinelPath + "'"
	if c.Sentinel != "" {
		creds = `"$HOME/.soulstream/` + c.Handle + `.creds"`
		b.WriteString("mkdir -p \"$HOME/.soulstream\"\n")
		fmt.Fprintf(&b, "printf '%%s' '%s' > %s\n\n", c.Sentinel, creds)
	}
	fmt.Fprintf(&b, "env SOULSTREAM_URL='%s' \\\n", c.Dial)
	fmt.Fprintf(&b, "    SOULSTREAM_CREDS=%s \\\n", creds)
	fmt.Fprintf(&b, "    SOULSTREAM_TOKEN='%s' \\\n", c.Secret)
	fmt.Fprintf(&b, "    SOULSTREAM_REALM='%s' \\\n", c.Realm)
	fmt.Fprintf(&b, "    SOULSTREAM_PERSONA='%s' \\\n", c.Handle)
	b.WriteString("    soulstream wrap --harness claude\n")
	return b.String()
}

// codexConfig is the same configuration in the shape codex reads: one TOML
// table in its config file, values identical to the JSON block above.
func codexConfig(c soulstream.Credential) string {
	return fmt.Sprintf(`[mcp_servers.soulstream]
command = "soulstream"
args = ["mcp"]

[mcp_servers.soulstream.env]
SOULSTREAM_URL = %q
SOULSTREAM_CREDS = %q
SOULSTREAM_TOKEN = %q
SOULSTREAM_REALM = %q
SOULSTREAM_PERSONA = %q`, c.Dial, c.SentinelPath, c.Secret, c.Realm, c.Handle)
}

// mcpConfig is the agent's configuration as an MCP client reads it: the
// server entry that launches the tool door out of the one product binary
// (`soulstream mcp`, design soulstream/0002), with the address, the shared
// file, the credential and the two names it answers to.
func mcpConfig(c soulstream.Credential) string {
	return fmt.Sprintf(`{
  "mcpServers": {
    "soulstream": {
      "command": "soulstream",
      "args": ["mcp"],
      "env": {
        "SOULSTREAM_URL": %q,
        "SOULSTREAM_CREDS": %q,
        "SOULSTREAM_TOKEN": %q,
        "SOULSTREAM_REALM": %q,
        "SOULSTREAM_PERSONA": %q
      }
    }
  }
}`, c.Dial, c.SentinelPath, c.Secret, c.Realm, c.Handle)
}

// resultNote is the screen's result line — what happened to the last act, in
// plain words, and nothing at all when nothing has happened yet. The div is
// always served: it is the patch target every act answers to, and what
// replaces a shown-once credential the moment anything else happens.
func resultNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, resultID, esc(msg))
}
