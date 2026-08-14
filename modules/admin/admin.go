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

// Nav is this module's one key on the spine. It carries the open
// conversation the way every other entry does, so coming back from here
// lands where the person left.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	return []shell.NavEntry{{
		Section: sectionPeople, Icon: "users", Label: "People & sign-in",
		Href: "/people" + topicQuery(r.URL.Query().Get("topic")),
	}}
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

// Mount claims the screen and the two acts offered from it.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /people", m.people)
	rt.HandleFunc("POST /act/invite", m.actInvite)
	rt.HandleFunc("POST /act/disable", m.actDisable)
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

// people is the screen: everyone who can sign in, and the two acts a
// person with the standing may take on them. Somebody who arrived here from
// elsewhere in the product came looking for one of them (see Link), so that
// one is marked and said out loud — and said out loud too when the list
// turns out not to hold them.
func (m *Module) people(w http.ResponseWriter, r *http.Request) {
	a, err := m.reach(r)
	if err != nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	list, err := a.People(r.Context())
	m.sh.Render(w, r, shell.Page{
		Title: "people and sign-in", Section: sectionPeople, Live: true,
		Body: m.sh.Sheet(renderPeople(list, err, r.URL.Query().Get("who"))),
	})
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
