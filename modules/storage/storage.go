// Package storage is the shell module that shows what the store actually
// holds: the messages themselves, one at a time and whole, rather than a
// count of them.
//
// It exists because the house readout is a fuel gauge. "412 ops · 3.1 MB"
// is the right thing on an overview and the wrong thing entirely when a
// message did not arrive, a signature reads in a way nobody expected, or an
// agent is writing a shape no reader folds. Until this screen, the answer to
// all three was a terminal, an operator's credentials, and a tool outside
// the surface that reported the problem.
//
// Two rules shape everything below.
//
// **It reads as the person, never as the surface.** Every read here rides
// the signed-in person's own admission — not the shared read lane the board
// and the conversation view are built on. That lane exists so the surface
// can render a realm's public shape while acting as nobody; raw subject-level
// access to the stores is not that. What it does NOT mean is per-person
// scoping: the product's own ceremony grants every admitted persona the
// whole subject space and the JetStream API with it, so this screen shows
// the entire store to anyone signed in — a fact about the deployment, not
// something this module introduces. So the screen says what a person's
// sign-in can read and never implies a narrowing it does not have. The day a
// deployment narrows that grant, or sealed payloads land, this screen
// follows with no change here.
//
// **It reads and nothing else.** No act, no delete (the record is
// append-only and offering one would be a lie about the protocol), no
// persistent index, and no search: a query layer is the one thing the
// protocol names as deliberately absent, and a debugging screen is not the
// place to smuggle one in. What it offers instead is the record's own way of
// being read — a subject pattern, which is exactly what the taxonomy was
// shaped for.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
const sectionStorage = "storage"

// routeStore is what this module answers to when another screen has the
// store on it and wants to offer a way in: the route name in the shell's
// cross-link facility, with one optional param, "subject", spelled the way
// the record spells a subject pattern.
//
// It is a word two packages agree on rather than a symbol either imports.
// Getting it wrong resolves to no link at all — the same answer a deployment
// that does not run this module gives, which is the failure to have.
const routeStore = "store"

// Module is the storage surface.
type Module struct {
	sh *shell.Shell
	sp *soulstream.Support
}

// New builds the module over a shell and the Soulstream support layer.
func New(sh *shell.Shell, sp *soulstream.Support) *Module {
	return &Module{sh: sh, sp: sp}
}

// Identity names the module. On screen it is called by what it holds, the
// same word the house readout already uses for it.
func (m *Module) Identity() shell.Identity {
	return shell.Identity{Slug: "storage", Name: "Storage"}
}

// Active reports that this deployment runs the module: a deployment with a
// record to read has a store to look into, and the support layer would not
// have opened without one.
//
// It is deliberately not gated on being an administrator. Authority here
// comes from the transport — what a person's own admission is permitted to
// read — and a gate drawn over a grant that already permits the read would
// be decoration standing in for a permission. If the deployment narrows the
// grant, the gate arrives for free and in the place that can enforce it.
func (m *Module) Active(context.Context) bool { return true }

// Nav is this module's one key, at the foot beside the other readout: this
// is where a person goes when a number on a screen is the thing they have
// stopped believing.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	return []shell.NavEntry{{
		Section: sectionStorage, Icon: "database", Label: "Storage",
		Href: "/storage" + topicQuery(r.URL.Query().Get("topic")), Foot: true,
	}}
}

// Link lands on the list, optionally already looking at one part of the
// subject space. A pattern that is not one is declined here rather than
// rendered into a screen that would refuse it — the asking module then has
// nowhere to point and says so in its own words.
func (m *Module) Link(route string, params map[string]string) (shell.Link, bool) {
	if route != routeStore {
		return shell.Link{}, false
	}
	href := "/storage"
	if subject := params["subject"]; subject != "" {
		if err := checkPattern(subject); err != nil {
			return shell.Link{}, false
		}
		href += "?filter=" + qesc(subject)
	}
	if topicPath := params["topic"]; topicPath != "" {
		sep := "?"
		if len(href) > len("/storage") {
			sep = "&amp;"
		}
		href += sep + "topic=" + qesc(topicPath)
	}
	// The label is left to the shell: what this place is called is the name
	// this module registered under, and saying it twice invites drift.
	return shell.Link{Href: href}, true
}

// Mount claims the screen, the live tail it can be put into, and the one
// message a person asks to see whole.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /storage", m.storage)
	rt.HandleFunc("GET /storage/tail", m.tail)
	rt.HandleFunc("GET /storage/op", m.op)
}

// topicQuery carries the open conversation across screens.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// ask is what a request is asking of the store: which one, what part of its
// subject space, where to start reading back from, and whether the screen is
// following the tail.
type ask struct {
	// Store is the store key ("conversations" | "notifications"); anything
	// else settles to the first.
	Store string
	// Filter is the subject pattern, "" for the store's own whole space.
	Filter string
	// Before is the sequence to read back from, 0 for the newest.
	Before uint64
	// Follow says the screen is watching the tail rather than sitting still.
	Follow bool
	// Topic is the open conversation, carried across screens like everywhere.
	Topic string
}

// asked reads what the request is asking for. Nothing here refuses: a
// malformed pattern is carried through to the read, which reports it in
// words, because a person mid-typing should be told what is wrong rather
// than silently given something else.
func asked(r *http.Request) ask {
	q := r.URL.Query()
	before, _ := strconv.ParseUint(q.Get("before"), 10, 64)
	return ask{
		Store:  q.Get("store"),
		Filter: q.Get("filter"),
		Before: before,
		Follow: q.Get("follow") == "1",
		Topic:  q.Get("topic"),
	}
}

// query renders an ask back into the query string every key on this screen
// carries, so where a person is survives every key they press.
func (a ask) query(extra ...string) string {
	q := "?store=" + qesc(a.Store)
	if a.Filter != "" {
		q += "&amp;filter=" + qesc(a.Filter)
	}
	if a.Topic != "" {
		q += "&amp;topic=" + qesc(a.Topic)
	}
	for _, e := range extra {
		q += "&amp;" + e
	}
	return q
}

// storage is the screen: the filter above, the ops below, and the panel one
// of them opens into.
func (m *Module) storage(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	a := asked(r)
	a.Store = storeFor(a.Store).Key
	init := ""
	if a.Follow {
		init = fmt.Sprintf("@get('/storage/tail%s')", a.query())
	}
	v := m.read(r.Context(), sess, a, pageSize)
	m.sh.Render(w, r, shell.Page{
		Title: "storage", Section: sectionStorage, Live: true, Init: init,
		Body: m.sh.Sheet(renderScreen(a, v)),
	})
}

// tail is the live channel the screen opens when it is following: the same
// list, re-read on a slow tick and patched in place. It carries fewer ops
// than the page does, because a tail is for watching what arrives and a
// page is for reading what is there.
//
// The tick is deliberately unhurried. Every re-read walks the store's own
// sequences, which is a real cost the conversation stream does not have, and
// nobody watching a store needs it faster than this.
func (m *Module) tail(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	a := asked(r)
	a.Store, a.Before = storeFor(a.Store).Key, 0
	shell.Stream(w, r, tailEvery, func(out io.Writer) {
		shell.WriteElements(out, renderList(a, m.read(r.Context(), sess, a, tailSize)))
	})
}

// op is one message whole: everything it is, and nothing summarised away.
// This is the screen a person reaches when the summary is the thing they
// have stopped trusting.
func (m *Module) op(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		shell.Patch(w, renderOp(opView{Err: "no session — sign in first"}))
		return
	}
	a := asked(r)
	seq, err := strconv.ParseUint(r.URL.Query().Get("seq"), 10, 64)
	if err != nil || seq == 0 {
		shell.Patch(w, renderOp(opView{Err: "that is not a sequence number"}))
		return
	}
	shell.Patch(w, renderOp(m.readOne(r.Context(), sess, storeFor(a.Store), seq)))
}

// tailEvery is how often a followed screen re-reads the store.
const tailEvery = 2 * time.Second
