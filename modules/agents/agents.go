// Package agents is the shell module for the machine voices this deployment
// answers for: naming one, handing it the credential it gets in with, seeing
// who vouched for which, and taking a credential away again.
//
// It is a module that is not always there. Issuing a credential in somebody
// else's name is authority a surface should only have where the deployment
// meant it to, so whether this screen exists is the deployment's declaration
// and not this module's opinion: it names the address it tells an agent to
// dial, or it names nothing and there is no such screen. Active reads
// exactly that — no probe, no guess. Absent, the module puts nothing on the
// rail and mounts nothing, and every path below answers like any other path
// nobody claimed.
//
// Two authorities meet on this screen and they are kept apart on purpose.
// Minting and taking away a credential ride the node-standing lane the
// deployment hands this surface, because the credential ops are refused to a
// person's own admission by design and no side-channel will be grown to get
// around that. Vouching for an agent rides the person: their key, their
// admission, their signature over the claim. The surface issues, the human
// vouches, and the surface signs nothing.
package agents

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

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
const sectionAgents = "agents"

// routeAgent is what this module answers to when somewhere else in the
// product has a machine voice on screen and wants to point at what it gets
// in with: the route name in the shell's cross-link facility, with one
// param, "who", spelled the way the record spells a voice.
const routeAgent = "agent"

// Module is the agents surface.
type Module struct {
	sh *shell.Shell
	sp *soulstream.Support
}

// New builds the module over a shell and the Soulstream support layer.
func New(sh *shell.Shell, sp *soulstream.Support) *Module {
	return &Module{sh: sh, sp: sp}
}

// Identity names the module. On screen it is called by what it holds, not by
// what holds it.
func (m *Module) Identity() shell.Identity {
	return shell.Identity{Slug: "agents", Name: "Agents"}
}

// Active is the whole of how this module learns which deployment it is in:
// one fact the deployment already declares about itself.
func (m *Module) Active(context.Context) bool { return m.sp.AgentsDial() != "" }

// Nav is this module's one key on the spine, carrying the open conversation
// the way every other entry does.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	return []shell.NavEntry{{
		Section: sectionAgents, Icon: "radio", Label: "Agents",
		Href: "/agents" + topicQuery(r.URL.Query().Get("topic")),
	}}
}

// Link lands on the list with one voice looked up on it — honestly,
// including when the voice turns out not to be an agent here, which is the
// ordinary case for every person on the record.
func (m *Module) Link(route string, params map[string]string) (shell.Link, bool) {
	who := params["who"]
	if route != routeAgent || who == "" {
		return shell.Link{}, false
	}
	href := "/agents?who=" + qesc(who)
	if topicPath := params["topic"]; topicPath != "" {
		href += "&amp;topic=" + qesc(topicPath)
	}
	// The label is left to the shell: what this place is called is the name
	// this module registered under, and saying it twice invites drift.
	return shell.Link{Href: href}, true
}

// Mount claims the screen, the question revoking stands behind, and the
// three acts offered from it.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /agents", m.agents)
	rt.HandleFunc("GET /agents/live", m.live)
	rt.HandleFunc("GET /agents/revoke-ask", m.askRevoke)
	rt.HandleFunc("POST /act/agent-add", m.actAdd)
	rt.HandleFunc("POST /act/agent-credential", m.actCredential)
	rt.HandleFunc("POST /act/agent-revoke", m.actRevoke)
}

// topicQuery carries the open conversation across screens.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// reach is this deployment's agent roster. A session with no roster cannot
// arrive here at all — an inactive module mounts no routes — so it is
// reported as the contradiction it would be rather than passed on as an
// empty screen.
func (m *Module) reach(r *http.Request) (*soulstream.Agents, error) {
	if m.sp.Session(r) == nil {
		return nil, errNoSession
	}
	ag := m.sp.Agents()
	if ag == nil {
		return nil, errNoRoster
	}
	return ag, nil
}

var (
	errNoSession = fmt.Errorf("no session — sign in first")
	errNoRoster  = fmt.Errorf("this deployment issues no agent credentials")
)

// agents is the screen: every voice somebody here answers for, what it can
// still get in with, and the form for adding another.
func (m *Module) agents(w http.ResponseWriter, r *http.Request) {
	ag, err := m.reach(r)
	if err != nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	list, err := ag.List(r.Context())
	m.sh.Render(w, r, shell.Page{
		Title: "agents", Section: sectionAgents, Live: true,
		Init: "@get('/agents/live')",
		Body: m.sh.Sheet(renderAgents(list, err, m.names(r.Context(), list),
			r.URL.Query().Get("who"), m.sp.Presence(r.Context()))),
	})
}

// live re-reads the roster every few seconds and morphs the table — the
// Around column is a judgment of the moment, so a screen left open must
// keep judging: an agent that just started shows in while the person
// still holds its paste block, and one that stopped goes to left or
// seen without a reload. The result line is never touched: it belongs
// to the acts, and a one-shot answer and the stream must never write
// the same element.
func (m *Module) live(w http.ResponseWriter, r *http.Request) {
	ag, err := m.reach(r)
	if err != nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	who := r.URL.Query().Get("who")
	shell.Stream(w, r, 5*time.Second, func(out io.Writer) {
		list, lerr := ag.List(r.Context())
		shell.WriteElements(out, renderTable(list, lerr, m.names(r.Context(), list), who,
			m.sp.Presence(r.Context())))
	})
}

// names is what to call each operator on screen. The record carries handles;
// a person reading this screen should see the colleague they know, with the
// handle beside it rather than instead of it.
func (m *Module) names(ctx context.Context, list []soulstream.Agent) map[string]string {
	out := map[string]string{}
	for _, a := range list {
		if _, done := out[a.OperatedBy]; !done {
			out[a.OperatedBy] = m.sp.Name(ctx, a.OperatedBy)
		}
	}
	return out
}

// actAdd names a new agent and hands back the one credential it will ever be
// shown. The person signed in is the one vouching, so their session does the
// signing and nothing here holds a key.
func (m *Module) actAdd(w http.ResponseWriter, r *http.Request) {
	ag, err := m.reach(r)
	if err != nil {
		shell.Patch(w, addNote(err.Error()))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, addNote("That form did not arrive whole: "+err.Error()))
		return
	}
	handle, shownAs := r.PostFormValue("handle"), r.PostFormValue("shown")
	if handle == "" {
		shell.Patch(w, addNote("An agent needs a handle."))
		return
	}
	cred, err := ag.Create(r.Context(), m.sp.Session(r), handle, shownAs)
	if err != nil {
		shell.Patch(w, addNote("Adding "+handle+" failed: "+err.Error()))
		return
	}
	// The panel goes away so the shown-once card is in front of the person,
	// not behind the form they just finished with.
	shell.PatchSignals(w, `{panel: false}`)
	shell.Patch(w, addNote(""))
	shell.Patch(w, renderCredential(cred, "is ready"))
	m.patchList(w, r, ag, handle)
}

// askRevoke patches the question revoking stands behind — or, asked about
// nobody, clears it, which is what "Keep it" does.
func (m *Module) askRevoke(w http.ResponseWriter, r *http.Request) {
	if m.sp.Session(r) == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	who := r.URL.Query().Get("who")
	if who == "" {
		shell.Patch(w, resultNote(""))
		return
	}
	shell.Patch(w, revokeConfirm(who))
}

// actCredential hands a standing agent a new credential. The new one is
// minted before the old one is taken away, so a transient failure leaves the
// agent working rather than locked out — and if the old one survives, the
// screen says so instead of implying it did not.
func (m *Module) actCredential(w http.ResponseWriter, r *http.Request) {
	ag, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	who := r.URL.Query().Get("who")
	cred, err := ag.Remint(r.Context(), who)
	switch {
	case err != nil && cred.Secret == "":
		shell.Patch(w, resultNote("Giving "+who+" a new credential failed: "+err.Error()))
		return
	case err != nil:
		// The new one exists and the old one outlived the act. Both facts go
		// on screen; hiding either would leave somebody believing the wrong
		// one.
		shell.Patch(w, renderCredential(cred, "has a new credential — and still has the old one"))
	default:
		shell.Patch(w, renderCredential(cred, "has a new credential"))
	}
	m.patchList(w, r, ag, who)
}

// actRevoke stops a credential being accepted, then re-reads the roster so
// the screen shows what is now true rather than what somebody clicked.
func (m *Module) actRevoke(w http.ResponseWriter, r *http.Request) {
	ag, err := m.reach(r)
	if err != nil {
		shell.Patch(w, resultNote(err.Error()))
		return
	}
	who := r.URL.Query().Get("who")
	if err := ag.TakeAway(who); err != nil {
		shell.Patch(w, resultNote("Taking "+who+"'s credential away failed: "+err.Error()))
		return
	}
	shell.Patch(w, resultNote(fmt.Sprintf(
		"%s cannot get in again. A connection it already has ends when the identity it was "+
			"admitted on runs out.", who)))
	m.patchList(w, r, ag, who)
}

// patchList hands back the roster as it now stands, with the row that just
// changed still marked.
func (m *Module) patchList(w http.ResponseWriter, r *http.Request, ag *soulstream.Agents, who string) {
	list, err := ag.List(r.Context())
	shell.Patch(w, renderTable(list, err, m.names(r.Context(), list), who,
		m.sp.Presence(r.Context())))
}
