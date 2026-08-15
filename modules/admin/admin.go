// Package admin is the shell module for the people who can sign in: who
// they are, how somebody gets their first passkey, and taking a sign-in
// away.
//
// It is the module that is not always there, and that is the point of it.
// Whether a deployment runs this surface is neither the module's opinion
// nor the shell's: a deployment that signs its people in against an
// authorization server it does not run has nobody to administer from here,
// and says so by declaring no administration surface. Active reads exactly
// that declaration — no probe, no reachability guess, and not one line of
// configuration invented for the shell's sake. Absent, the module puts
// nothing on the rail and mounts nothing, so every path below answers like
// any other path nobody claimed.
//
// Authority is delegated, never borrowed: every call rides the signed-in
// person's own bearer through the Soulstream support layer, and what comes
// back when they lack the standing is the sign-in surface's own refusal,
// put on the screen in the words it used.
package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// esc and qesc are the frame's own escaping, named short because most of
// this module is markup.
var (
	esc  = shell.Esc
	qesc = shell.QueryEsc
)

// This module's key on the spine.
const sectionPeople = "people"

// routePerson is what this module answers to when somewhere else in the
// product has a person on screen and wants to point at their sign-in: the
// route name in the shell's cross-link facility, with one param, "who",
// spelled the way the sign-in surface spells a person.
//
// It is a word two packages agree on rather than a symbol either of them
// imports — the asking module does not know this one exists, and this one
// never learns who asked. Getting the word wrong resolves to no link at
// all, which is exactly what a deployment that does not run this module
// gets, and is the failure to have.
const routePerson = "person"

// Module is the people-and-sign-in surface.
type Module struct {
	sh *shell.Shell
	sp *soulstream.Support
}

// New builds the module over a shell and the Soulstream support layer.
func New(sh *shell.Shell, sp *soulstream.Support) *Module {
	return &Module{sh: sh, sp: sp}
}

// Identity names the module. On screen it is called by what it does, not
// by what it is built on: a person managing their colleagues' sign-ins
// should never have to know which component answers.
func (m *Module) Identity() shell.Identity {
	return shell.Identity{Slug: "admin", Name: "People & sign-in"}
}

// Active is the whole of how this module learns which deployment it is in:
// one fact the deployment already declares about itself.
func (m *Module) Active(context.Context) bool { return m.sp.AdminSurface() != "" }

// Nav is this module's one key on the spine — drawn only for the people
// who could use the screen, read from their own token's roles. A display
// fact, not an authority: the routes stay mounted, and the sign-in
// surface's verified refusal remains the answer behind every act.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	sess := m.sp.Session(r)
	if sess == nil || !sess.IsAdmin() {
		return nil
	}
	return []shell.NavEntry{navEntry(r.URL.Query().Get("topic"))}
}

// navEntry is the one key's shape, built apart from the gate above so the
// shape stays checkable without a session.
func navEntry(topicPath string) shell.NavEntry {
	return shell.NavEntry{
		Section: sectionPeople, Icon: "users", Label: "People & sign-in",
		Href: "/people" + topicQuery(topicPath),
	}
}

// Link is the other half of this module's place in the product: elsewhere a
// person is named on a screen, and this is where what they may sign in with
// lives. The link lands on the list with that person looked up on it —
// honestly, including when nobody here answers to the name, which is the
// ordinary case for a voice on the record that was never a sign-in.
//
// Only this module builds this module's paths. The open conversation rides
// along the way it does on every key of the spine, so the way back from
// here lands where the person left.
func (m *Module) Link(route string, params map[string]string) (shell.Link, bool) {
	who := params["who"]
	if route != routePerson || who == "" {
		return shell.Link{}, false
	}
	href := "/people?who=" + qesc(who)
	if topicPath := params["topic"]; topicPath != "" {
		href += "&amp;topic=" + qesc(topicPath)
	}
	// The label is left to the shell: what this place is called is the name
	// this module registered under, and saying it twice invites the two to
	// drift.
	return shell.Link{Href: href}, true
}

// Mount claims the screen and the acts offered from it.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /people", m.people)
	rt.HandleFunc("POST /act/person-add", m.actAdd)
	rt.HandleFunc("POST /act/invite", m.actInvite)
	rt.HandleFunc("POST /act/disable", m.actDisable)
	rt.HandleFunc("POST /act/enable", m.actEnable)
	rt.HandleFunc("POST /act/groups", m.actGroups)
	rt.HandleFunc("POST /act/client-add", m.actClientAdd)
	rt.HandleFunc("POST /act/client-delete", m.actClientDelete)
}

// topicQuery carries the open conversation across screens.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// reach is this request's own way into the surface: the signed-in person's,
// or nothing. A request carrying no session belongs at the front door. A
// session with no reach cannot arrive here at all — an inactive module
// mounts no routes — so it is reported as the contradiction it would be
// rather than passed on as an empty screen.
func (m *Module) reach(r *http.Request) (*soulstream.Admin, error) {
	sess := m.sp.Session(r)
	if sess == nil {
		return nil, errNoSession
	}
	a := sess.Admin()
	if a == nil {
		return nil, errors.New("this deployment administers its sign-ins elsewhere")
	}
	return a, nil
}

// errNoSession is the one refusal that is not the surface's: nobody is
// signed in, so there is nobody to act as.
var errNoSession = errors.New("no session — sign in first")

// people is the screen: everyone who can sign in, the acts a person with
// the standing may take on them, and the applications that sign people in.
// Somebody who arrived here from elsewhere in the product came looking for
// one of them (see Link), so that one is marked and said out loud — and
// said out loud too when the list turns out not to hold them.
func (m *Module) people(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	list, err := a.People(r.Context())
	// The groups are a convenience for the add form; a list that cannot be
	// read leaves the form no poorer than the deployment's own knowledge.
	groups, _ := a.Groups(r.Context())
	clients, cerr := a.Clients(r.Context())
	m.sh.Render(w, r, shell.Page{
		Title: "people and sign-in", Section: sectionPeople, Live: true,
		Body: m.sh.Sheet(renderPeople(view{
			People: list, Err: err, Groups: groups,
			Clients: clients, ClientsErr: cerr,
			Who: r.URL.Query().Get("who"),
		})),
	})
}

// actAdd names a new person. They exist from there on but cannot sign in
// until an invite enrolls their passkey — creation grants existence, never
// admission, which is the sign-in surface's own rule.
func (m *Module) actAdd(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, resultNote("That form did not arrive whole: "+err.Error()))
		return
	}
	username := r.PostFormValue("username")
	if username == "" {
		shell.Patch(w, resultNote("A person needs a sign-in name."))
		return
	}
	if err := a.Create(r.Context(), username, r.PostFormValue("shown"),
		splitGroups(r.PostFormValue("groups"))); err != nil {
		shell.Patch(w, resultNote(refusalWords("Adding "+username, err)))
		return
	}
	shell.Patch(w, resultNote(username+" exists now — create an invite below so they can enroll a passkey."))
	m.patchList(w, r, a, username)
}

// actEnable is actDisable undone: the surface accepts them again.
func (m *Module) actEnable(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	who := r.URL.Query().Get("who")
	if err := a.Enable(r.Context(), who); err != nil {
		shell.Patch(w, resultNote(refusalWords("Enabling "+who, err)))
		return
	}
	shell.Patch(w, resultNote(fmt.Sprintf("%s can sign in again.", who)))
	m.patchList(w, r, a, who)
}

// actGroups replaces somebody's group memberships — the names their next
// token carries. The surface's own rules apply, the last-admin refusal
// among them, and come back in its words.
func (m *Module) actGroups(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, resultNote("That form did not arrive whole: "+err.Error()))
		return
	}
	who := r.URL.Query().Get("who")
	groups := splitGroups(r.PostFormValue("groups"))
	if err := a.SetGroups(r.Context(), who, groups); err != nil {
		shell.Patch(w, resultNote(refusalWords("Changing "+who+"'s groups", err)))
		return
	}
	shell.Patch(w, resultNote(fmt.Sprintf("%s's groups are now: %s.", who, joinOrNone(groups))))
	m.patchList(w, r, a, who)
}

// actClientAdd registers an application that signs people in: an id, a
// name to show people, and the exact addresses it may return them to.
func (m *Module) actClientAdd(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, resultNote("That form did not arrive whole: "+err.Error()))
		return
	}
	id, name := r.PostFormValue("id"), r.PostFormValue("name")
	uris := splitGroups(r.PostFormValue("uris"))
	if id == "" || len(uris) == 0 {
		shell.Patch(w, resultNote("An app needs an id and at least one return address."))
		return
	}
	if err := a.CreateClient(r.Context(), id, name, uris); err != nil {
		shell.Patch(w, resultNote(refusalWords("Registering "+id, err)))
		return
	}
	shell.Patch(w, resultNote(id+" can sign people in now."))
	m.patchClients(w, r, a)
}

// actClientDelete unregisters an application. Sign-ins it completed are
// history and stay; new ones stop.
func (m *Module) actClientDelete(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	id := r.URL.Query().Get("id")
	if err := a.DeleteClient(r.Context(), id); err != nil {
		shell.Patch(w, resultNote(refusalWords("Removing "+id, err)))
		return
	}
	shell.Patch(w, resultNote(id+" cannot sign people in any more."))
	m.patchClients(w, r, a)
}

// patchList hands back the people as they now stand, with the row that
// just changed still marked.
func (m *Module) patchList(w http.ResponseWriter, r *http.Request, a *soulstream.Admin, who string) {
	list, err := a.People(r.Context())
	shell.Patch(w, renderTable(list, err, who))
}

// patchClients hands back the applications as they now stand.
func (m *Module) patchClients(w http.ResponseWriter, r *http.Request, a *soulstream.Admin) {
	clients, err := a.Clients(r.Context())
	shell.Patch(w, renderClients(clients, err))
}

// splitGroups reads a space- or comma-separated list the way a person
// types one.
func splitGroups(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ' ' || r == ',' })
	out := make([]string, 0, len(fields))
	out = append(out, fields...)
	return out
}

// joinOrNone says a list out loud, including the empty one.
func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

// actInvite mints a single-use enrolment invite for somebody who already
// exists. The token comes back once and is kept nowhere, so the screen
// shows it whole and says out loud that this is the only time.
func (m *Module) actInvite(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	who := r.URL.Query().Get("who")
	inv, err := a.MintInvite(r.Context(), who)
	if err != nil {
		shell.Patch(w, resultNote(refusalWords("Creating an invite for "+who, err)))
		return
	}
	shell.Patch(w, renderInvite(who, inv))
}

// actDisable takes somebody's sign-in away, then re-reads the list so the
// screen shows what is now true rather than what the person clicked.
func (m *Module) actDisable(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	who := r.URL.Query().Get("who")
	if err := a.Disable(r.Context(), who); err != nil {
		shell.Patch(w, resultNote(refusalWords("Disabling "+who, err)))
		return
	}
	shell.Patch(w, resultNote(fmt.Sprintf("%s can no longer sign in.", who)))
	list, err := a.People(r.Context())
	// The row that just changed is the row to keep an eye on, so the re-read
	// list comes back with it still marked.
	shell.Patch(w, renderTable(list, err, who))
}

// refusalWords says what happened in the surface's own words. A refusal a
// person is meant to read as "not yours to do" is named as that; a rule the
// surface holds is passed on as it was said, because it was written to be
// read and a second sentence about it would only get in the way; anything
// else is reported as the fault it is, never dressed up.
func refusalWords(what string, err error) string {
	var ref *soulstream.Refusal
	if errors.As(err, &ref) {
		switch {
		case ref.Denied():
			return what + " needs an account that administers sign-ins — yours does not. " +
				"The sign-in surface said: " + ref.Msg
		case ref.Rule():
			return what + ": " + ref.Msg
		}
	}
	return what + " failed: " + err.Error()
}
