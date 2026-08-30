// Package overview is the shell module that watches the house: the
// overview a person lands on — what the store holds, whether sign-in is
// serving, and the way into every conversation — and the system-status
// screen behind it, with the one act taken from there.
//
// It is the observe core the surface began as, re-homed as a module: it
// plugs into the shell through the module contract alone (shell.Module) and
// reads everything it shows through the Soulstream support layer. The shell
// does not know this package exists.
package overview

import (
	"context"
	"fmt"
	"net/http"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// esc and qesc are the frame's own escaping, named short because most of
// this package is markup.
var (
	esc  = shell.Esc
	qesc = shell.QueryEsc
)

// This module's keys on the spine: where a person is, when they are here.
const (
	sectionHome   = "home"
	sectionStatus = "status"
)

// storeName is the stream the house readout measures. The deployment's
// store of record answers to it; nothing here founds it.
const storeName = "SOULSTREAM"

// Where a person goes when the readout is the thing they have stopped
// believing: the module that shows the store's own messages, and the kind of
// screen wanted from it. Two words handed to the frame — no import, and no
// way for this package to learn whether such a module exists. A build
// without one comes back with nothing, and the readout is a readout, which
// is what it always was.
const (
	storageModule = "storage"
	routeStore    = "store"
)

// Module is the house-watching surface.
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
	return shell.Identity{Slug: "overview", Name: "Overview"}
}

// Active reports that this deployment runs the module: a deployment with a
// record to read is a deployment with a house to watch, and the support
// layer would not have opened without one.
func (m *Module) Active(context.Context) bool { return true }

// Nav is this module's two keys: the overview at the top, where a person
// starts, and the readouts at the foot, beside the way out. Both carry the
// open conversation, so coming back from here lands where the person left.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	q := topicQuery(r.URL.Query().Get("topic"))
	return []shell.NavEntry{
		{Section: sectionHome, Icon: "home", Label: "Home", Href: "/home" + q},
		{Section: sectionStatus, Icon: "gauge", Label: "System status",
			Href: "/status" + q, Foot: true},
	}
}

// Mount claims the two screens and the act offered from the second.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /home", m.home)
	rt.HandleFunc("GET /status", m.status)
	rt.HandleFunc("POST /act/work-open", m.actWorkOpen)
}

// topicQuery carries the open conversation across screens.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// withTopic hands the frame a request that says which conversation this
// screen settled on. Every module builds its rail entry from the request
// itself, so a screen that defaulted to the newest conversation has to say
// so out loud — or the way onward from here would land somewhere else.
func withTopic(r *http.Request, topicPath string) *http.Request {
	if topicPath == "" || r.URL.Query().Get("topic") == topicPath {
		return r
	}
	q := r.URL.Query()
	q.Set("topic", topicPath)
	out := r.Clone(r.Context())
	out.URL.RawQuery = q.Encode()
	return out
}

// home is the overview: the house at a glance and the way into every
// conversation. It is what the Home key on the spine reaches from anywhere.
func (m *Module) home(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	v := m.observe(r.Context(), r.URL.Query().Get("topic"), sess)
	m.firstSteps(r.Context(), sess, &v)
	m.sh.Render(w, withTopic(r, v.TopicPath), shell.Page{
		Title: "home", Section: sectionHome, Live: true,
		Body: m.sh.Sheet(renderOverview(v)),
	})
}

// status is the house readouts — storage, sign-in, work. They are no longer
// the centre of the surface, so they render on request rather than once a
// second.
func (m *Module) status(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	v := m.observe(r.Context(), r.URL.Query().Get("topic"), sess)
	body := fmt.Sprintf(`<h1>System status</h1>
<p class="lede">What the house itself is doing — read live, kept nowhere.</p>
%s
<p class="act">
<button class="btn ghost" data-on:click="@post('/act/work-open?topic=%s')">Open work item</button></p>
<div id="result" class="note">—</div>`, renderPlanes(v), qesc(v.TopicPath))
	m.sh.Render(w, withTopic(r, v.TopicPath), shell.Page{
		Title: "system status", Section: sectionStatus, Live: true,
		Body: m.sh.Sheet(body),
	})
}

// actWorkOpen is the act this screen offers: an op on the record through
// the session's own admitted connection, attributed and (when the persona
// key materialized) signed as the signed-in principal. It patches the
// result line on the screen that offered it and nothing else.
func (m *Module) actWorkOpen(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		shell.Patch(w, `<div id="result" class="note">no session — sign in first</div>`)
		return
	}
	topicPath := r.URL.Query().Get("topic")
	if topicPath == "" {
		topicPath = m.defaultTopic(r.Context())
	}
	if topicPath == "" {
		shell.Patch(w, `<div id="result" class="note">There is no conversation to open a work item in.</div>`)
		return
	}
	who := sess.ScreenName(r.Context())
	id, err := topic.Open(sess.Client(), topicPath).
		OpenWork(r.Context(), "opened by "+who, "opened from the shell")
	if err != nil {
		shell.Patch(w, fmt.Sprintf(`<div id="result" class="note">Opening a work item as %s was refused: %s</div>`,
			esc(who), esc(err.Error())))
		return
	}
	// A plain sentence for the person, honest about the signature either
	// way; the op id rides in mono with the whole of it in the hover — the
	// once a year somebody needs it.
	sig := "signed and on the record"
	if !sess.Signed {
		sig = "attributed, not signed"
	}
	shell.Patch(w, fmt.Sprintf(`<div id="result" class="note">Work item opened by %s — %s. `+
		`<span class="mono" title="%s">%s</span></div>`,
		esc(who), sig, esc(id), esc(shortID(id))))
}

// shortID is an op id a result line can hold; the whole rides the hover.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12] + "…"
	}
	return id
}

// view is one read of the house.
type view struct {
	// Unread is how many messages in each conversation have this person's
	// name in them and have not been looked at yet — the session's own tray,
	// which this screen shows on the rows it lists.
	Unread map[string]int
	Board  []topic.BoardEntry
	// Machinery says which board entries are the record's own rooms — agent
	// homes, the placements topic — left off the Home list the way the rail
	// leaves them off (hq design 0012 §3).
	Machinery map[string]soulstream.Room
	// Topic is the open conversation, read for the work it carries.
	Topic     *topic.MaterializedTopic
	TopicPath string
	// StreamBytes is what the store holds; StreamRoof is the byte roof it
	// declares for itself, 0 when it declares none. The house readout needs
	// both: a level means nothing without the scale it is read against, and
	// a store provisioned with no roof has no scale to invent one from.
	StreamMsg   uint64
	StreamBytes uint64
	StreamRoof  int64
	// Store is the way from the readout into the store's own messages, when
	// this build runs a module that shows them. Resolved through the frame,
	// so this module never learns which one answered — and never learns that
	// none did beyond the answer being empty.
	Store  shell.Link
	FoldOK bool
	// AgentsOn says this deployment issues agent credentials — the same
	// declared fact the agents screen exists by, read through the support
	// layer so this module needs no knowledge of that one. Named counts how
	// many agents the record holds; In, how many can still get in. Unread
	// is true when the roster could not be read, so the card can say so
	// instead of showing a zero it did not measure.
	AgentsOn              bool
	AgentsNamed, AgentsIn int
	AgentsUnread          bool
	// Steps is the first-steps card's derivation — filled for Home only,
	// recomputed from the realm at every render, kept nowhere (design
	// 0008 §2: guidance is a reading, never a store).
	Steps []step
	Err   string
}

// observe reads everything both screens show: the conversations, the open
// one's work, and the house readouts themselves.
func (m *Module) observe(ctx context.Context, topicPath string,
	sess *soulstream.Session,
) view {
	v := view{TopicPath: topicPath, Unread: map[string]int{}}
	entries, err := m.sp.Board(ctx)
	if err != nil {
		v.Err = fmt.Sprintf("board: %v", err)
		return v
	}
	v.Board = entries
	v.Machinery = m.sp.Machinery()
	v.Unread = sess.Standing(entries)
	if v.TopicPath == "" {
		v.TopicPath = soulstream.LastLive(entries, v.Machinery)
	}
	if v.TopicPath != "" {
		if mt, err := topic.Open(m.sp.Reader(), v.TopicPath).Materialise(ctx); err == nil {
			v.Topic = mt
		} else {
			v.Err = fmt.Sprintf("topic %s: %v", v.TopicPath, err)
		}
	}
	m.health(ctx, &v)
	return v
}

// health fills in the house readouts themselves.
func (m *Module) health(ctx context.Context, v *view) {
	if info, err := m.sp.Reader().JetStream().Stream(ctx, storeName); err == nil {
		if si, err := info.Info(ctx); err == nil {
			v.StreamMsg = si.State.Msgs
			v.StreamBytes = si.State.Bytes
			// The server stores an unlimited roof as -1; the readout wants one
			// word for "no scale", so both spellings become zero here.
			if si.Config.MaxBytes > 0 {
				v.StreamRoof = si.Config.MaxBytes
			}
		}
	}
	// Where the readout leads, asked once per render. Everything about the
	// answer belongs to whoever answers; this module supplies nothing and
	// renders whatever href it is handed, or the readout alone.
	v.Store, _ = m.sh.Link(storageModule, routeStore, nil)
	v.FoldOK = m.sp.SignInServing()
	if ag := m.sp.Agents(); ag != nil {
		v.AgentsOn = true
		list, err := ag.List(ctx)
		if err != nil {
			v.AgentsUnread = true
			return
		}
		v.AgentsNamed = len(list)
		for _, a := range list {
			if a.Admitted() {
				v.AgentsIn++
			}
		}
	}
}

// defaultTopic is the conversation an act falls back to when the screen
// naming one has gone stale.
func (m *Module) defaultTopic(ctx context.Context) string {
	entries, err := m.sp.Board(ctx)
	if err != nil {
		return ""
	}
	return soulstream.LastLive(entries, m.sp.Machinery())
}
