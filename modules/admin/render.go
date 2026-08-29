package admin

import (
	"fmt"
	"strings"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// The screen: the people who can sign in with the acts an administrator
// takes on them, the form that names a new one, and the applications that
// sign people in — with one result line under all of it. Plain words
// throughout — a person managing their colleagues' access reads about
// people and sign-ins, never about the components that hold them.

// The screen's patch targets. An act answers with a fragment for one or
// more; nothing else on the page moves. The add-form's result line is its
// own: an act's answer lands where the person acted, and the slide-over
// holding the form may be the only thing they are looking at.
const (
	resultID  = "people-result"
	tableID   = "people-table"
	clientsID = "people-clients"
	addNoteID = "people-add-note"
)

// view is one read of everything the screen shows.
type view struct {
	People     []soulstream.Person
	Err        error
	Groups     []string
	Clients    []soulstream.Client
	ClientsErr error
	// Who is the person somebody came here looking for — empty when they
	// simply opened the screen.
	Who string
}

// renderPeople is the whole screen's body: header, the roster the screen
// is for, the applications folded under it, and the add-form off to the
// side in the slide-over — the screen leads with who is here, and naming
// somebody new is a deliberate act with a surface of its own.
func renderPeople(v view) string {
	var b strings.Builder
	b.WriteString(`<div class="page-head"><div class="ph-words"><h1>People &amp; sign-in</h1>` +
		`<p class="lede">Everyone who can sign in here, read live from the ` +
		`sign-in service — the shell keeps none of it.</p></div>` + addKey() + `</div>`)
	b.WriteString(lookedUpNote(v.People, v.Err, v.Who))
	b.WriteString(`<div class="section">` + renderTable(v.People, v.Err, v.Who) + `</div>`)
	b.WriteString(`<div class="section">` + renderClientsSection(v.Clients, v.ClientsErr) + `</div>`)
	b.WriteString(resultNote(""))
	b.WriteString(shell.SlideOver("Add a person", addPanel(v.Groups)))
	return b.String()
}

// addKey pulls the slide-over out.
func addKey() string {
	return `<p class="act"><button type="button" class="btn" data-on:click="$panel = true">` +
		`Add person</button></p>`
}

// addPanel names a new person — the slide-over's whole body. Creation
// grants existence, never admission — the panel says what happens next (an
// invite), so nobody is left wondering why the person they added cannot
// sign in yet.
func addPanel(groups []string) string {
	hint := "space-separated — optional"
	if len(groups) > 0 {
		hint = "space-separated — here: " + strings.Join(groups, " ")
	}
	return `<p class="lede">They exist from here on; they sign in only after you create ` +
		`an invite for them and they enroll a passkey with it.</p>` +
		`<form id="person-add" data-on:submit="@post('/act/person-add', {contentType:'form'})">` +
		`<div class="fields">` +
		`<label class="field">Sign-in name` +
		`<input name="username" required autocomplete="off" spellcheck="false" ` +
		`placeholder="lowercase, no spaces"></label>` +
		`<label class="field">Shown as` +
		`<input name="shown" autocomplete="off" ` +
		`placeholder="what to call them on screen — optional"></label>` +
		`<label class="field">Groups` +
		`<input name="groups" autocomplete="off" spellcheck="false" ` +
		`placeholder="` + esc(hint) + `"></label>` +
		`</div>` +
		`<button class="btn" type="submit">Add person</button>` +
		addNote("") + `</form>`
}

// addNote is the slide-over form's own result line.
func addNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, addNoteID, esc(msg))
}

// lookedUpNote answers the question somebody arrived with. A name this list
// holds is pointed at; a name it does not hold is answered rather than left
// to a person hunting a row that was never there — plenty of voices on the
// record were never a sign-in, and this screen only ever knew about
// sign-ins.
func lookedUpNote(list []soulstream.Person, err error, who string) string {
	if who == "" || err != nil {
		return ""
	}
	for _, p := range list {
		if p.Username == who {
			return fmt.Sprintf(`<p class="lede">Looking up <span class="mono">%s</span>, `+
				`marked below.</p>`, esc(who))
		}
	}
	return fmt.Sprintf(`<p class="lede">Nobody who signs in here answers to `+
		`<span class="mono">%s</span> — everyone who does is below.</p>`, esc(who))
}

// renderTable is the list itself, and a patch target of its own so an act
// that changes what is true can hand back what is now true.
func renderTable(list []soulstream.Person, err error, who string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, tableID)
	switch {
	case err != nil:
		fmt.Fprintf(&b, `<p class="blank">%s</p>`,
			esc(refusalWords("Reading the list of people", err)))
	case len(list) == 0:
		b.WriteString(`<p class="blank">Nobody can sign in here yet. Add the first ` +
			`person above — they pick a passkey the moment they arrive.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Name</th><th>Sign-in name</th><th>Can sign in</th>` +
			`<th>Groups</th><th>Passkeys</th><th>Actions</th>` +
			`</tr></thead><tbody>`)
		for _, p := range list {
			b.WriteString(personRow(p, p.Username == who))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// personRow is one person, marked when they are the one somebody came here
// for. The acts are offered only where they mean something: an invite enrols
// a passkey for somebody who may sign in, somebody shut out is offered the
// way back in, and there is nothing to take away from the last person who
// can administer sign-ins here — whose row says so instead.
func personRow(p soulstream.Person, lookedUp bool) string {
	name := p.DisplayName
	if name == "" {
		name = p.Username
	}
	state := `<span class="pill warn">no</span>`
	if p.Active() {
		state = `<span class="pill ok"><span class="led ok"></span>yes</span>`
	}
	// The keys stack when the row has no width to spare for them, which is
	// the last column's own business rather than the table's.
	var acts string
	if p.Active() {
		// Taking this one person's sign-in away would leave nobody able to
		// administer sign-ins at all, so the sign-in service refuses it. The
		// screen does not offer a key that only ever earns a refusal — it
		// says the thing the refusal would have said. Creating an invite is
		// still offered: another passkey for the person holding the place
		// open is the opposite of a way to lose it. Disabling stands behind
		// its own question (askDisable), so one stray tap shuts nobody out.
		away := fmt.Sprintf(
			`<button class="btn ghost" title="Take their sign-in away" `+
				`data-on:click="@get('/people/disable-ask?who=%s')">`+
				`Disable</button>`, qesc(p.Username))
		if p.LastAdmin {
			away = `<span class="note">the last administrator stays</span>`
		}
		acts = fmt.Sprintf(
			`<div class="acts">`+
				`<button class="btn ghost" data-on:click="@post('/act/invite?who=%s')">`+
				`Create invite</button>%s</div>`,
			qesc(p.Username), away)
	} else {
		acts = fmt.Sprintf(
			`<div class="acts">`+
				`<button class="btn ghost" title="Let them sign in again" `+
				`data-on:click="@post('/act/enable?who=%s')">`+
				`Enable</button></div>`, qesc(p.Username))
	}
	row := "<tr>"
	if lookedUp {
		row = `<tr class="on">`
	}
	// A sign-in name has no word breaks of its own and is let wrap in a
	// narrow column, so the whole of it also rides in the hover: wrapped
	// across two lines, it is still one name.
	return fmt.Sprintf(`%s<td>%s</td><td class="mono" title="%s">%s</td><td>%s</td>`+
		`<td>%s</td><td class="mono">%d</td><td>%s</td></tr>`,
		row, esc(name), esc(p.Username), esc(p.Username), state,
		groupsCell(p), p.Credentials, acts)
}

// groupsCell is what a person's token would carry, editable in place: the
// memberships as the sign-in service spells them, in a small form whose
// save posts the whole new list. The service's rules — the last-admin
// refusal among them — answer in its words.
func groupsCell(p soulstream.Person) string {
	return fmt.Sprintf(
		`<form class="rowform" data-on:submit="@post('/act/groups?who=%s', {contentType:'form'})">`+
			`<input name="groups" value="%s" autocomplete="off" spellcheck="false" `+
			`aria-label="groups for %s">`+
			`<button class="btn ghost" type="submit">Save</button></form>`,
		qesc(p.Username), esc(strings.Join(p.Groups, " ")), esc(p.Username))
}

// renderClientsSection is the applications that sign people in: the
// registered ones, and the form that registers another. It is its own kind
// of thing — apps, not people, and a day-one task rather than a daily one —
// so the whole of it rests under a fold: one click away, never in the way
// of the roster the screen is for.
func renderClientsSection(clients []soulstream.Client, err error) string {
	var b strings.Builder
	b.WriteString(`<details class="stow"><summary>Apps that sign people in</summary>`)
	b.WriteString(`<p class="lede">Applications registered to sign people in here — the shell ` +
		`itself is one of them. Each may only return people to the exact addresses listed.</p>`)
	b.WriteString(renderClients(clients, err))
	b.WriteString(`<div class="card"><h2>Register an app</h2>` +
		`<form id="client-add" data-on:submit="@post('/act/client-add', {contentType:'form'})">` +
		`<div class="fields">` +
		`<label class="field">App id` +
		`<input name="id" required autocomplete="off" spellcheck="false" ` +
		`placeholder="lowercase, no spaces"></label>` +
		`<label class="field">Shown as` +
		`<input name="name" autocomplete="off" ` +
		`placeholder="what to call it on screen — optional"></label>` +
		`<label class="field">Return addresses` +
		`<input name="uris" required autocomplete="off" spellcheck="false" ` +
		`placeholder="space-separated, exact"></label>` +
		`</div>` +
		`<button class="btn" type="submit">Register app</button>` +
		`</form></div>`)
	b.WriteString(`</details>`)
	return b.String()
}

// renderClients is the registered applications, and a patch target of its
// own so an act that changes what is true can hand back what is now true.
func renderClients(clients []soulstream.Client, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, clientsID)
	switch {
	case err != nil:
		fmt.Fprintf(&b, `<p class="blank">%s</p>`,
			esc(refusalWords("Reading the list of apps", err)))
	case len(clients) == 0:
		b.WriteString(`<p class="blank">No apps registered yet. An app is somewhere ` +
			`else people sign in with their account here — register the first below.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Name</th><th>App id</th><th>Returns people to</th><th>Actions</th>` +
			`</tr></thead><tbody>`)
		for _, c := range clients {
			name := c.Name
			if name == "" {
				name = c.ClientID
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td class="mono" title="%s">%s</td>`+
				`<td class="mono">%s</td><td><div class="acts">`+
				`<button class="btn ghost" data-on:click="@get('/people/client-remove-ask?id=%s')">`+
				`Remove</button></div></td></tr>`,
				esc(name), esc(c.ClientID), esc(c.ClientID),
				esc(strings.Join(c.RedirectURIs, " ")), qesc(c.ClientID))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
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
// in plain words, and nothing at all when nothing has happened yet. The div
// is always served: it is the patch target every act answers to, and what
// replaces a shown-once invite the moment anything else happens.
func resultNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, resultID, esc(msg))
}

// disableConfirm is the question disabling stands behind: what it does and
// does not change, and the two ways out. Deciding takes two taps on
// purpose — the act shuts a colleague out, and one stray click must not.
func disableConfirm(who string) string {
	return fmt.Sprintf(`<div id="%s" class="note">`+
		`<p class="confirm">Disable sign-in for %s? They stay on every record and every `+
		`screen; they just cannot sign in here until you enable them again.</p>`+
		`<div class="acts">`+
		`<button class="btn" data-on:click="@post('/act/disable?who=%s')">Yes, disable</button>`+
		`<button class="btn ghost" data-on:click="@get('/people/disable-ask')">Keep it</button>`+
		`</div></div>`, resultID, esc(who), qesc(who))
}

// clientRemoveConfirm is the same question for an app.
func clientRemoveConfirm(id string) string {
	return fmt.Sprintf(`<div id="%s" class="note">`+
		`<p class="confirm">Remove %s? Sign-ins it completed are history and stay; `+
		`new ones stop the moment it goes.</p>`+
		`<div class="acts">`+
		`<button class="btn" data-on:click="@post('/act/client-delete?id=%s')">Yes, remove it</button>`+
		`<button class="btn ghost" data-on:click="@get('/people/client-remove-ask')">Keep it</button>`+
		`</div></div>`, resultID, esc(id), qesc(id))
}
