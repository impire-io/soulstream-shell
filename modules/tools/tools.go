// Package tools is the shell module where a person sees the tools their
// soulstream can reach and connects their own accounts to the remote ones —
// and where an administrator adds a tool for everyone (hq design
// soulstream-shell 0005, the human end of the external-tools build).
//
// The lane rules are the design's: reading the catalog is the realm's
// public shape; connecting is the person's own act on their own prefix (the
// ceremony completes only on the session that started it — the plane holds
// the ceremony under their persona, so a callback landing anywhere else
// completes nothing); adding and removing ride the node-standing lane the
// deployment handed this surface, gated on the session's admin role the way
// the admin key is drawn — a display-plus-gate fact, with the guardrail at
// the plane's own op path as the deeper authority.
package tools

import (
	"context"
	"net/http"

	siclient "github.com/impire-io/soulstream-identity/client"

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
const sectionTools = "tools"

// Module is the tools surface.
type Module struct {
	sh *shell.Shell
	sp *soulstream.Support
}

// New builds the module over a shell and the Soulstream support layer.
func New(sh *shell.Shell, sp *soulstream.Support) *Module {
	return &Module{sh: sh, sp: sp}
}

// Identity names the module.
func (m *Module) Identity() shell.Identity {
	return shell.Identity{Slug: "tools", Name: "Tools"}
}

// Active reports that this deployment runs the module: the catalog and the
// plane's grants surface are always-on component facts now, so a
// deployment with a record to read has tools to list — even when the list
// is empty and the screen's whole answer is the way to add one.
func (m *Module) Active(context.Context) bool { return true }

// Nav is this module's one key, carrying the open conversation the way
// every key on the spine does.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	return []shell.NavEntry{{
		Section: sectionTools, Icon: "disc-3", Label: "Tools",
		Href: "/tools" + topicQuery(r.URL.Query().Get("topic")),
	}}
}

// Mount claims the screen, the linking ceremony's two legs, the question
// removing stands behind, and the acts.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /tools", m.tools)
	rt.HandleFunc("GET /tools/connect", m.connect)
	rt.HandleFunc("GET /tools/callback", m.callback)
	rt.HandleFunc("GET /tools/remove-ask", m.askRemove)
	rt.HandleFunc("POST /act/tool-disconnect", m.actDisconnect)
	rt.HandleFunc("POST /act/tool-add", m.actAdd)
	rt.HandleFunc("POST /act/tool-remove", m.actRemove)
}

// topicQuery carries the open conversation across screens.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// tools is the screen: the merged catalog with this person's own standing
// joined on, and the admin's forms where the session carries the role.
func (m *Module) tools(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	m.sh.Render(w, r, shell.Page{
		Title: "tools", Section: sectionTools, Live: true,
		Body: m.sh.Sheet(renderTools(m.view(r.Context(), r, sess))),
	})
}

// view is one read of everything the screen shows.
func (m *Module) view(ctx context.Context, r *http.Request, sess *soulstream.Session) view {
	v := view{
		Admin: sess.IsAdmin(),
		Msg:   r.URL.Query().Get("msg"),
	}
	tools, notes, err := m.sp.Tools(ctx)
	if err != nil {
		v.Err = err.Error()
		return v
	}
	v.Tools, v.Notes = tools, notes
	connections, err := sess.Connections()
	if err != nil {
		// The person's own standing being unreadable dims the Connect
		// column, never the catalog.
		v.Notes = append(v.Notes, "your own connections could not be read right now")
		return v
	}
	v.Connected = map[string]bool{}
	for _, g := range connections {
		v.Connected[g.Resource] = true
	}
	return v
}

// connect begins linking one remote tool as this person: their browser
// leaves for the provider's own sign-in and comes back to the callback
// below. Starting a ceremony changes no custody — the redirect is a plain
// navigation, which is why this is honestly a GET.
func (m *Module) connect(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	name := r.URL.Query().Get("name")
	link, err := sess.ConnectStart(name)
	if err != nil {
		http.Redirect(w, r, "/tools?msg="+qesc("Connecting "+name+" failed: "+err.Error()),
			http.StatusFound)
		return
	}
	http.Redirect(w, r, link.AuthorizeURL, http.StatusFound)
}

// callback is the ceremony's return leg: the provider hands back the code
// and the ceremony id (OAuth state — the plane's published contract), and
// completion runs as this session. The plane holds the ceremony under this
// persona's own prefix, so a callback landing on any other session
// completes nothing — said in the refusal, not papered over.
func (m *Module) callback(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	q := r.URL.Query()
	if err := sess.ConnectComplete(q.Get("state"), q.Get("code")); err != nil {
		http.Redirect(w, r, "/tools?msg="+qesc("Connecting failed: "+err.Error()),
			http.StatusFound)
		return
	}
	http.Redirect(w, r, "/tools?msg="+qesc("Connected."), http.StatusFound)
}

// actDisconnect revokes this person's own connection to one tool.
func (m *Module) actDisconnect(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		shell.Patch(w, resultNote("no session — sign in first"))
		return
	}
	name := r.URL.Query().Get("name")
	if err := sess.Disconnect(name); err != nil {
		shell.Patch(w, resultNote("Disconnecting "+name+" failed: "+err.Error()))
		return
	}
	shell.Patch(w, resultNote("Disconnected from "+name+"."))
	m.patchList(w, r, sess)
}

// actAdd writes a tool for everyone — both halves for a remote one, the
// catalog half for a run-here one. Admin-gated the way the acts of the
// admin module are: the shell checks the session's role, and the guardrail
// standing at the plane's op path decides regardless of what any surface
// believes.
func (m *Module) actAdd(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil || !sess.IsAdmin() {
		shell.Patch(w, addNote("adding tools needs an account that administers this deployment"))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, addNote("That form did not arrive whole: "+err.Error()))
		return
	}
	name := r.PostFormValue("name")
	if name == "" {
		shell.Patch(w, addNote("A tool needs a name."))
		return
	}
	var err error
	if r.PostFormValue("kind") == "workload" {
		err = m.sp.AddWorkloadTool(r.Context(), name,
			r.PostFormValue("persona"), r.PostFormValue("endpoint"), r.PostFormValue("description"))
	} else {
		err = m.sp.AddRemoteTool(r.Context(), siclient.ResourceConfig{
			Name:         name,
			AuthURL:      r.PostFormValue("auth_url"),
			TokenURL:     r.PostFormValue("token_url"),
			RevokeURL:    r.PostFormValue("revoke_url"),
			ClientID:     r.PostFormValue("client_id"),
			ClientSecret: r.PostFormValue("client_secret"),
			Scopes:       splitList(r.PostFormValue("scopes")),
			RedirectURI:  r.PostFormValue("redirect_uri"),
		}, r.PostFormValue("endpoint"), r.PostFormValue("description"))
	}
	if err != nil {
		shell.Patch(w, addNote("Adding "+name+" failed: "+err.Error()))
		return
	}
	// The panel goes away so the list holding the new row is in front of
	// the person, with the answer under it.
	shell.PatchSignals(w, `{panel: false}`)
	shell.Patch(w, addNote(""))
	shell.Patch(w, resultNote(name+" is available now."))
	m.patchList(w, r, sess)
}

// askRemove patches the question removing stands behind — or, asked about
// nothing, clears it, which is what "Keep it" does.
func (m *Module) askRemove(w http.ResponseWriter, r *http.Request) {
	if m.sp.Session(r) == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		shell.Patch(w, resultNote(""))
		return
	}
	shell.Patch(w, removeConfirm(name))
}

// actRemove reverses both halves. Standing connections keep their custody
// — the plane's own semantic, said here.
func (m *Module) actRemove(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil || !sess.IsAdmin() {
		shell.Patch(w, resultNote("removing tools needs an account that administers this deployment"))
		return
	}
	name := r.URL.Query().Get("name")
	if err := m.sp.RemoveTool(r.Context(), name); err != nil {
		shell.Patch(w, resultNote("Removing "+name+" failed: "+err.Error()))
		return
	}
	shell.Patch(w, resultNote(name+" is gone. Anyone's own connections to it keep their "+
		"custody until they disconnect."))
	m.patchList(w, r, sess)
}

// patchList hands back the list as it now stands.
func (m *Module) patchList(w http.ResponseWriter, r *http.Request, sess *soulstream.Session) {
	shell.Patch(w, renderList(m.view(r.Context(), r, sess)))
}

// splitList reads a space- or comma-separated list the way a person types
// one.
func splitList(raw string) []string {
	var out []string
	field := ""
	flush := func() {
		if field != "" {
			out = append(out, field)
			field = ""
		}
	}
	for _, r := range raw {
		if r == ' ' || r == ',' {
			flush()
			continue
		}
		field += string(r)
	}
	flush()
	return out
}
