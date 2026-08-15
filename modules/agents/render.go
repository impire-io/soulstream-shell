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
// here looking for — empty when they simply opened the screen.
func renderAgents(list []soulstream.Agent, err error, names map[string]string, who string) string {
	var b strings.Builder
	b.WriteString(`<h1>Agents</h1>`)
	b.WriteString(`<p class="lede">The machine voices somebody here answers for. ` +
		`Each one gets in with a credential of its own, so what it says arrives ` +
		`under its own name — and can be stopped without touching anybody else.</p>`)
	b.WriteString(lookedUpNote(list, err, who))
	b.WriteString(renderAddForm())
	b.WriteString(renderTable(list, err, names, who))
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
		b.WriteString(`<p class="blank">No agents yet.</p>`)
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
	operator := names[a.OperatedBy]
	if operator == "" {
		operator = a.OperatedBy
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
		`<td class="mono" title="%s">%s</td><td>%s <span class="mono">@%s</span></td>`+
		`<td>%s</td><td class="mono">%s</td><td>%s</td></tr>`,
		row, esc(operator), esc(name), esc(a.Handle), esc(a.Handle),
		esc(operator), esc(a.OperatedBy), state, esc(added), acts.String())
}

// renderCredential is the one answer on this screen carrying a secret. The
// credential store keeps a digest it cannot reverse, so this is the only
// time the credential itself will ever exist on a screen — which the screen
// says, rather than leaving somebody to find out by reloading.
//
// What it shows is the agent's whole configuration, in the shape the stdio
// server actually reads: the variable names below are that program's, not
// this screen's wording, and are spelled the way it spells them or the agent
// does not start.
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
	fmt.Fprintf(&b, `<label class="field">Configuration`+
		`<textarea readonly rows="14" data-credential>%s</textarea></label>`,
		esc(mcpConfig(c)))
	if c.Sentinel != "" {
		fmt.Fprintf(&b, `<label class="field">The shared file it points at — only needed if `+
			`this agent runs on another machine, and safe to copy: it admits nobody by itself`+
			`<textarea readonly rows="6" data-sentinel>%s</textarea></label>`, esc(c.Sentinel))
	}
	b.WriteString(renderSetupGuides(c))
	b.WriteString(`</div>`)
	return b.String()
}

// renderSetupGuides is where the configuration above meets the program that
// will use it: one fold per assistant people actually run, each saying — in
// that program's own terms — where the configuration goes. Every fold repeats
// only public halves or shapes; the one secret on this screen stays in the
// block above, already filled in where a fold carries a block of its own.
func renderSetupGuides(c soulstream.Credential) string {
	var b strings.Builder
	b.WriteString(`<div class="setup"><h3>Where this goes</h3>` +
		`<p>One machine needs the <span class="mono">soulstream-mcp</span> program on its path ` +
		`(<span class="mono">go install github.com/impire-io/soulstream-core/cmd/soulstream-mcp@latest</span>, ` +
		`or a release download). Then pick your assistant:</p>`)

	b.WriteString(`<details data-setup="claude-code"><summary>Claude Code</summary>` +
		`<p>Save the configuration above as <span class="mono">.mcp.json</span> in the folder ` +
		`the agent works from — it is already the exact file. If a <span class="mono">.mcp.json</span> ` +
		`exists there, add the <span class="mono">"soulstream"</span> entry under its ` +
		`<span class="mono">mcpServers</span>. The tools appear on the next start; ` +
		`headless runs want <span class="mono">--mcp-config .mcp.json</span>.</p></details>`)

	fmt.Fprintf(&b, `<details data-setup="codex"><summary>Codex</summary>`+
		`<p>Codex reads TOML, not JSON — the same values go in `+
		`<span class="mono">~/.codex/config.toml</span>:</p>`+
		`<textarea readonly rows="9" data-codex-config>%s</textarea></details>`,
		esc(codexConfig(c)))

	b.WriteString(`<details data-setup="other"><summary>Anything else that speaks MCP ` +
		`(pi.dev, opencode, …)</summary>` +
		`<p>Every such assistant has a place to declare an MCP server. Declare one named ` +
		`<span class="mono">soulstream</span> that runs the command ` +
		`<span class="mono">soulstream-mcp</span> with the five environment variables from the ` +
		`configuration above, spelled exactly that way. No arguments, no URL endpoint — ` +
		`it is a stdio server the assistant starts itself.</p></details>`)

	b.WriteString(`<details data-setup="wrap"><summary>Make it answer mentions on its own</summary>` +
		`<p>One command wraps the assistant on your machine — your sign-ins, your ` +
		`configuration — so mentions of this agent become answers even when nobody is ` +
		`sitting at it. Export the five values above in your shell ` +
		`(<span class="mono">export SOULSTREAM_URL=… SOULSTREAM_CREDS=… SOULSTREAM_TOKEN=… ` +
		`SOULSTREAM_REALM=… SOULSTREAM_PERSONA=…</span>), then run:</p>` +
		`<p><span class="mono">soulstream wrap --harness claude</span> — or ` +
		`<span class="mono">codex</span>, or <span class="mono">--template your-file.json</span> ` +
		`for anything else with a machine-readable finish.</p>` +
		`<p>It needs the wrapper beside the CLI ` +
		`(<span class="mono">go install github.com/impire-io/soulstream-workloads/cmd/soulstream-wrap@latest</span>). ` +
		`Mentions sent while it was off are answered when it starts; stop it any time — ` +
		`nothing is lost, and taking the credential away above stops it for good.</p></details>`)

	b.WriteString(`</div>`)
	return b.String()
}

// codexConfig is the same configuration in the shape codex reads: one TOML
// table in its config file, values identical to the JSON block above.
func codexConfig(c soulstream.Credential) string {
	return fmt.Sprintf(`[mcp_servers.soulstream]
command = "soulstream-mcp"

[mcp_servers.soulstream.env]
SOULSTREAM_URL = %q
SOULSTREAM_CREDS = %q
SOULSTREAM_TOKEN = %q
SOULSTREAM_REALM = %q
SOULSTREAM_PERSONA = %q`, c.Dial, c.SentinelPath, c.Secret, c.Realm, c.Handle)
}

// mcpConfig is the agent's configuration as its own program reads it: the
// server entry that launches the stdio door, with the address, the shared
// file, the credential and the two names it answers to.
func mcpConfig(c soulstream.Credential) string {
	return fmt.Sprintf(`{
  "mcpServers": {
    "soulstream": {
      "command": "soulstream-mcp",
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
// plain words. It is also what replaces a shown-once credential the moment
// anything else happens.
func resultNote(msg string) string {
	if msg == "" {
		msg = "—"
	}
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, resultID, esc(msg))
}
