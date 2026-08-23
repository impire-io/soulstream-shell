// Package approvals is the shell module where a human answers the
// guardrail: a deferred act appears as a ticket, and one tap closes the
// loop the plane keeps open for it (hq design soulstream-shell 0006, the
// human end of the approvals build).
//
// The one line this module must never blur (design §3): the YES IS THE
// PERSON'S — minted on their session, signed by their own key, verified by
// the plane against the directory and the rule's approvers clause — and
// the DELIVERY IS THE SURFACE'S: the originator is an agent or service
// that is not in this browser, so the surface carries the signed artifact
// to its tail on the node-standing lane. Carrying a sealed answer is not
// authority: the artifact converts only the invocation its signer named,
// for the actor it names, and the plane verifies both.
package approvals

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// esc and qesc are the frame's own escaping.
var (
	esc  = shell.Esc
	qesc = shell.QueryEsc
)

// This module's key on the spine.
const sectionApprovals = "approvals"

// Module is the approvals surface.
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
	return shell.Identity{Slug: "approvals", Name: "Approvals"}
}

// Active is the whole of how this module learns which deployment it is in:
// the deployment's own word that its identity plane runs the guardrail —
// no probe, no shell-side configuration (design 0006 §4).
func (m *Module) Active(context.Context) bool { return m.sp.ApprovalsOn() }

// Nav draws the key only for a session that could act — the admin role, or
// a persona named in some standing rule's approvers clause — with the
// pending count as its mark: the spine is where a person learns something
// waits for them. A display fact, as everywhere: the plane refuses an
// outside approver by name regardless of what any surface draws.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	sess := m.sp.Session(r)
	if sess == nil || !m.mayAct(sess) {
		return nil
	}
	mark := spineTally(0)
	if pending, err := m.sp.PendingApprovals(); err == nil {
		mark = spineTally(len(pending))
	}
	return []shell.NavEntry{{
		Section: sectionApprovals, Icon: "circle-alert", Label: "Approvals",
		Href: "/approvals" + topicQuery(r.URL.Query().Get("topic")), Mark: mark,
	}}
}

// mayAct says whether this session could answer anything: administrators
// always, and any persona a standing rule names as an approver.
func (m *Module) mayAct(sess *soulstream.Session) bool {
	if sess.IsAdmin() {
		return true
	}
	rules, err := m.sp.GuardrailRules()
	if err != nil {
		return false
	}
	for _, rule := range rules {
		if slices.Contains(rule.Approvers, sess.Persona) {
			return true
		}
	}
	return false
}

// Mount claims the screen, its live channel, and its two acts.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /approvals", m.approvals)
	rt.HandleFunc("GET /approvals/live", m.live)
	rt.HandleFunc("POST /act/approval-approve", m.actAnswer(true))
	rt.HandleFunc("POST /act/approval-deny", m.actAnswer(false))
}

// topicQuery carries the open conversation across screens.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// approvals is the screen: everything awaiting a decision. It opens its
// live channel the moment it loads — this is the one sheet where standing
// still lies: windows run out, new tickets arrive, and a screen left open
// must keep saying what is actually waiting.
func (m *Module) approvals(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	m.sh.Render(w, r, shell.Page{
		Title: "approvals", Section: sectionApprovals, Live: true,
		Init: "@get('/approvals/live')",
		Body: m.sh.Sheet(renderApprovals(m.view(r.Context(), sess))),
	})
}

// live re-reads the pending tickets every few seconds and morphs the list
// and the mark on the spine — the countdowns recompute, arrivals appear,
// expiries go. The result line is never touched: it belongs to the acts,
// and a one-shot answer and the stream must never write the same element.
func (m *Module) live(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	shell.Stream(w, r, 5*time.Second, func(out io.Writer) {
		v := m.view(r.Context(), sess)
		shell.WriteElements(out, renderList(v))
		shell.WriteElements(out, spineTally(len(v.Tickets)))
	})
}

// view is one read of the pending tickets, each principal resolved to the
// name a person reads.
func (m *Module) view(ctx context.Context, sess *soulstream.Session) view {
	v := view{MayAct: m.mayAct(sess)}
	pending, err := m.sp.PendingApprovals()
	if err != nil {
		v.Err = "Reading what is waiting failed: " + err.Error()
		return v
	}
	for _, t := range pending {
		row := ticket{
			InvocationID: t.InvocationID, Principal: t.Principal,
			Action: t.Action, Rule: t.Rule, ExpiresAt: t.ExpiresAt,
		}
		if user := principalUser(t.Principal); user != "" {
			row.Who = m.sp.Name(ctx, user)
		}
		v.Tickets = append(v.Tickets, row)
	}
	return v
}

// actAnswer is one tap: the person's signed yes or no, minted on their
// session and delivered to the originator's tail. Refusals come back in
// the plane's words — an approver outside the rule's clause reads the
// by-name refusal, not a shell invention.
func (m *Module) actAnswer(approve bool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := m.sp.Session(r)
		if sess == nil {
			shell.Patch(w, resultNote("no session — sign in first"))
			return
		}
		invocation := r.URL.Query().Get("invocation")
		principal := r.URL.Query().Get("principal")
		actor := principalUser(principal)
		if invocation == "" || actor == "" {
			shell.Patch(w, resultNote("the ticket did not arrive whole"))
			return
		}
		answer, err := sess.MintApproval(invocation, actor)
		if err != nil {
			shell.Patch(w, resultNote("Signing your answer failed: "+err.Error()))
			return
		}
		verb := "Approved"
		if !approve {
			verb = "Denied"
		}
		if err := m.sp.Deliver(principal, approve, invocation, answer); err != nil {
			shell.Patch(w, resultNote(verb+" was refused: "+err.Error()))
			m.patchList(w, r, sess)
			return
		}
		shell.Patch(w, resultNote(verb+"."))
		m.patchList(w, r, sess)
	}
}

// patchList hands back the list as it now stands.
func (m *Module) patchList(w http.ResponseWriter, r *http.Request, sess *soulstream.Session) {
	shell.Patch(w, renderList(m.view(r.Context(), sess)))
}

// principalUser is the persona half of a ticket's account/user principal.
func principalUser(principal string) string {
	if i := strings.LastIndex(principal, "/"); i >= 0 {
		return principal[i+1:]
	}
	return principal
}
