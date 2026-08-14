package admin

import (
	"fmt"
	"strings"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// The screen: one table of the people who can sign in, two acts per row,
// and one result line under it. Plain words throughout — a person managing
// their colleagues' access reads about people and sign-ins, never about the
// components that hold them.

// resultID and tableID are the screen's two patch targets. An act answers
// with a fragment for one or both; nothing else on the page moves.
const (
	resultID = "people-result"
	tableID  = "people-table"
)

// renderPeople is the whole screen's body.
func renderPeople(list []soulstream.Person, err error) string {
	var b strings.Builder
	b.WriteString(`<h1>People &amp; sign-in</h1>`)
	b.WriteString(`<p class="lede">Everyone who can sign in here, read live from the ` +
		`sign-in surface — the shell keeps none of it.</p>`)
	b.WriteString(renderTable(list, err))
	b.WriteString(resultNote(""))
	return b.String()
}

// renderTable is the list itself, and a patch target of its own so an act
// that changes what is true can hand back what is now true.
func renderTable(list []soulstream.Person, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, tableID)
	switch {
	case err != nil:
		fmt.Fprintf(&b, `<p class="blank">%s</p>`,
			esc(refusalWords("Reading the list of people", err)))
	case len(list) == 0:
		b.WriteString(`<p class="blank">Nobody can sign in here yet.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Name</th><th>Sign-in name</th><th>Can sign in</th>` +
			`<th>Groups</th><th>Passkeys</th><th>Actions</th>` +
			`</tr></thead><tbody>`)
		for _, p := range list {
			b.WriteString(personRow(p))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// personRow is one person. The acts are offered only where they mean
// something: an invite enrols a passkey for somebody who may sign in, and
// there is nothing to take away from somebody who already cannot.
func personRow(p soulstream.Person) string {
	name := p.DisplayName
	if name == "" {
		name = p.Username
	}
	state := `<span class="pill warn">no</span>`
	if p.Active() {
		state = `<span class="pill ok"><span class="led ok"></span>yes</span>`
	}
	var acts string
	if p.Active() {
		acts = fmt.Sprintf(
			`<button class="btn ghost" data-on:click="@post('/act/invite?who=%s')">`+
				`Create invite</button> `+
				`<button class="btn ghost" data-on:click="@post('/act/disable?who=%s')">`+
				`Take sign-in away</button>`,
			qesc(p.Username), qesc(p.Username))
	}
	return fmt.Sprintf(`<tr><td>%s</td><td class="mono">%s</td><td>%s</td>`+
		`<td>%s</td><td class="mono">%d</td><td>%s</td></tr>`,
		esc(name), esc(p.Username), state, groupTags(p.Groups), p.Credentials, acts)
}

// groupTags is what a person's token would carry, as the sign-in surface
// spells it. The names are the deployment's own, never rewritten here.
func groupTags(groups []string) string {
	if len(groups) == 0 {
		return `<span class="note">none</span>`
	}
	var b strings.Builder
	for _, g := range groups {
		fmt.Fprintf(&b, `<span class="pill">%s</span> `, esc(g))
	}
	return strings.TrimSpace(b.String())
}

// renderInvite is the one answer on this screen carrying a secret. The
// sign-in surface keeps only its digest, so this is the only time it will
// ever exist on a screen — which the screen says, rather than leaving
// somebody to find out by reloading.
func renderInvite(who string, inv soulstream.Invite) string {
	return fmt.Sprintf(`<div id="%s" class="card raised">`+
		`<h2>Invite for %s</h2>`+
		`<p class="lede">Shown once. Hand it over yourself — nothing keeps a copy, `+
		`and reloading this screen will not bring it back.</p>`+
		`<label class="field">Invite code`+
		`<input readonly value="%s" data-invite></label>`+
		`<label class="field">Enrolment link`+
		`<input readonly value="%s" data-enrol></label>`+
		`</div>`, resultID, esc(who), esc(inv.Token), esc(inv.URL))
}

// resultNote is the screen's result line — what happened to the last act,
// in plain words. It is also what replaces a shown-once invite the moment
// anything else happens.
func resultNote(msg string) string {
	if msg == "" {
		msg = "—"
	}
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, resultID, esc(msg))
}
