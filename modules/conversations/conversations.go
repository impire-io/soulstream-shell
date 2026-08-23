// Package conversations is the shell module people talk in: the
// conversations they can reach on the left, one of them open in the middle
// with a composer docked under it, who is in it and what it is waiting on
// beside that — and a mark on every message that said their name.
//
// It is built beside the shell and plugs into it through the module
// contract alone (shell.Module): identity, activation, what it puts on the
// rail, and the routes it claims. Everything it reads and writes rides the
// Soulstream support layer, never the shell — the shell does not know this
// package exists.
package conversations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

// chat is the path this module's own screen answers on — the deployment's
// front door, which is why it is the shell's Home.
const chatPath = "/"

// section is this module's key for its one screen, marked on the rail when
// a person is on it.
const section = "chat"

// Where else in this product a person on screen here can be looked up: the
// module that administers who may sign in, and the kind of screen wanted
// from it. Two words and a param name — no import, no build tag, and no way
// for this package to learn whether such a module exists. The shell asks the
// modules this deployment actually runs, and the People panel renders a
// plain name whenever the answer is nothing (see details.go).
//
// This is the whole of the coupling, and it is deliberately the weak kind: a
// deployment that runs no sign-in administration and a word misspelled here
// come back identically, as no link, which is the only failure a person
// should ever be shown.
const (
	adminModule = "admin"
	routePerson = "person"
)

// Module is the conversations surface.
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
	return shell.Identity{Slug: "conversations", Name: "Conversations"}
}

// Active reports that this deployment runs the module: a deployment with a
// record to read is a deployment people talk in, and the support layer
// would not have opened without one.
func (m *Module) Active(context.Context) bool { return true }

// Nav is this module's one key on the spine, carrying the open
// conversation so coming back from another screen lands where the person
// left, and the count of what is waiting for them.
//
// On this module's own screen the key does a second thing: it pulls out the
// list of conversations. In a frame too narrow to seat that list beside the
// conversation it is a drawer, and this is what opens it — the way to the
// other conversations stays exactly where it is on every screen, at every
// width. Nothing is taken away by claiming the click here: going to the
// screen a person is already on is all it would otherwise do.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	return []shell.NavEntry{{
		Section: section, Icon: "messages-square", Label: "Conversations",
		Href:  chatPath + topicQuery(r.URL.Query().Get("topic")),
		Mark:  m.tally(r),
		Attrs: panelToggle(r.URL.Path),
	}}
}

// Mount claims the conversation screen, its live channel, the acts a
// person takes from it — posting, starting, closing, archiving — and the
// composer's own two lanes.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /{$}", m.chat)
	rt.HandleFunc("GET /live", m.live)
	rt.HandleFunc("POST /act/post-turn", m.actPostTurn)
	rt.HandleFunc("GET /composer/reply", m.composerReply)
	rt.HandleFunc("GET /composer/suggest", m.composerSuggest)
	rt.HandleFunc("POST /act/conversation-start", m.actStart)
	rt.HandleFunc("POST /act/conversation-close", m.actClose)
	rt.HandleFunc("POST /act/conversation-archive", m.actArchive)
	rt.HandleFunc("GET /lifecycle/archive-ask", m.archiveAsk)
}

// topicQuery carries the open conversation across screens.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// panelToggle is what the key does beyond going somewhere, on the one screen
// where it has something else to do: pull out the list of conversations.
//
// Everywhere else it is a plain way here and nothing is claimed. Here, going
// to the screen a person is already on is all the click would otherwise do,
// so the click is spent on the thing the narrow frame actually needs — and
// the list stays reachable from the same place at every width.
func panelToggle(path string) string {
	if path != chatPath {
		return ""
	}
	return `data-on:click="evt.preventDefault(); $panel = !$panel"`
}

// tally is the mark on this module's key: how many messages are waiting
// with this person's name in them.
//
// On the conversation screen itself it is served empty and the live stream
// fills it a moment later, the way it fills every other target there —
// counting it here would mean reading the board before serving a page that
// is about to read it anyway. Every other screen is served once and never
// morphed, so there it is counted now.
func (m *Module) tally(r *http.Request) string {
	sess := m.sp.Session(r)
	if sess == nil || r.URL.Path == chatPath {
		return spineTally(0)
	}
	board, err := topic.Board(r.Context(), m.sp.Reader())
	if err != nil {
		return spineTally(0)
	}
	return spineTally(soulstream.Total(sess.Standing(board)))
}

// chat is the conversation screen. The whole surface is behind sign-in: an
// unauthenticated visitor gets the shell's sign-in card, nothing of the
// record — this is not an open window.
func (m *Module) chat(w http.ResponseWriter, r *http.Request) {
	if m.sp.Session(r) == nil {
		m.sh.SignIn(w, r)
		return
	}
	topicPath := r.URL.Query().Get("topic")
	// Whether this conversation is archived is read once, at the page, so
	// the composer is never offered where the record would refuse it. The
	// default conversation needs no read: the default skips archived ones.
	archived := false
	if topicPath != "" {
		if mt, err := topic.Open(m.sp.Reader(), topicPath).Materialise(r.Context()); err == nil {
			archived = mt.Lifecycle == topic.Archived
		}
	}
	m.sh.Render(w, r, shell.Page{
		Section: section,
		Live:    true,
		Init:    fmt.Sprintf("@get('/live?topic=%s')", qesc(topicPath)),
		Body:    chatBody(topicPath, archived),
		Tail:    "\n" + stickScript + "\n" + mentionScript + "\n",
	})
}

// chatBody is the screen's three columns as the page first serves them, each
// one waiting for the live stream's first tick.
//
// The list of conversations is a column of its own where there is room for
// one and a drawer over the conversation where there is not. Which of the
// two it is, is the frame's to decide by width — this markup is the same
// either way, and the only thing held here is whether the drawer is out.
// That is the frame's own panel signal, so the key on the spine, the way out
// of the drawer and the scrim behind it are all saying the same word.
//
// It survives no reload, which is right: picking a conversation is a page
// load, and a drawer that outlived one would be a drawer standing over the
// very thing it was asked for. It survives every morph, which is also right:
// the stream writes the list inside this markup, never this markup.
//
// The fold where a conversation begins sits between the head and the list —
// served once, morphed never, so a half-written name survives every tick.
// On an archived conversation the composer's place is a quiet note: the
// record would refuse the write, so the surface does not offer it. And the
// last element is the lifecycle acts' own answer dock, empty until an act
// speaks — the one target beside the details panel that the stream never
// writes.
//
// The details column holds a signal of its own, info, declared on the
// thread (served once, morphed never): where the frame is too narrow to
// seat the column beside the conversation, it is a drawer over it, out
// from the Details key in the conversation's own head — everything the
// panel says stays one tap away at every width.
func chatBody(topicPath string, archived bool) string {
	dock := renderComposer(topicPath)
	if archived {
		dock = archivedDock()
	}
	return fmt.Sprintf(`<aside class="rail" data-class:open="$panel">
<div class="rail-head">%s<h2 class="label">Conversations</h2>
<button type="button" class="rail-shut" title="Hide the conversations"
 aria-label="Hide the conversations" data-on:click="$panel = false">%s</button></div>
%s
<nav id="conversations" class="rail-list"><p class="rail-note">loading…</p></nav>
</aside>
<div class="rail-scrim" data-on:click="$panel = false"></div>
<section class="thread" data-signals="{info:false}">
<div id="dash" class="thread-body"><p class="blank">loading…</p></div>
%s
</section>
<aside id="details" class="details" data-class:open="$info"><p class="det-note">loading…</p></aside>
%s`,
		shell.Icon("messages-square"), shell.Icon("chevrons-right"),
		startFold(), dock, lifeNote(""))
}

// stickScript keeps the newest message in view.
const stickScript = `<script>
// Keep the newest message in view. The stream morphs the conversation once
// a second and #dash survives every morph, so one observer holds — and it
// only follows for someone already at the foot: a person reading back is
// left where they are.
(() => {
  const el = document.getElementById("dash");
  let stick = true;
  el.addEventListener("scroll", () => {
    stick = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  });
  new MutationObserver(() => { if (stick) el.scrollTop = el.scrollHeight; })
    .observe(el, {childList: true, subtree: true, characterData: true});
})();
</script>`

// live is the module's SSE channel: the observed state re-rendered and
// morphed into this screen's own four targets — the rail of conversations,
// the conversation itself, the details beside it, and the mentions tally on
// the spine. Session-gated like every surface that carries the record, and
// the session is also what tells the render whose messages are theirs and
// which of them said their name.
func (m *Module) live(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	topicPath := r.URL.Query().Get("topic")
	shell.Stream(w, r, time.Second, func(out io.Writer) {
		v := m.observe(r.Context(), topicPath, sess)
		// Looking at a conversation is reading what is in it: its marks go
		// before the tick that would have shown them, so the count never
		// stands over the very messages in front of the person.
		sess.Read(v.TopicPath)
		v.Unread = sess.Standing(v.Board)
		shell.WriteElements(out, renderRail(v))
		shell.WriteElements(out, renderThread(v))
		shell.WriteElements(out, renderDetails(v))
		shell.WriteElements(out, spineTally(soulstream.Total(v.Unread)))
	})
}

// view is one render of the whole observed conversation state.
type view struct {
	// Me is the signed-in person's own principal, taken from their session
	// and never from the request: it is what decides whose messages are
	// theirs. "" when signed out.
	Me string
	// Names maps a persona to the name shown for it on screen.
	Names map[string]string
	// Voices maps a persona to what the directory says about it beyond its
	// name — the operator claim the channel colours are read from. A persona
	// missing here answers for itself (see channel.go).
	Voices map[string]voice
	// Lookups is where else in this deployment each of these people can be
	// read about — one resolved link per persona, and no entry at all for a
	// persona nothing else in the build can say anything about. The shell
	// resolves them; this module never learns what answered.
	Lookups map[string]shell.Link
	// Unread is how many messages in each conversation have this person's
	// name in them and have not been looked at yet — this session's own
	// tray, kept in memory and never on the record.
	Unread    map[string]int
	Board     []topic.BoardEntry
	Topic     *topic.MaterializedTopic
	TopicPath string
	Err       string
}

// observe reads what this screen shows: the board, the open conversation
// with every verdict earned, and everyone either of them names.
func (m *Module) observe(ctx context.Context, topicPath string, sess *soulstream.Session) view {
	v := view{TopicPath: topicPath, Names: map[string]string{},
		Voices: map[string]voice{}, Lookups: map[string]shell.Link{},
		Unread: map[string]int{}}
	if sess != nil {
		v.Me = sess.Persona
		v.Names[sess.Persona] = sess.ScreenName(ctx)
	}
	entries, err := topic.Board(ctx, m.sp.Reader())
	if err != nil {
		v.Err = fmt.Sprintf("board: %v", err)
		return v
	}
	v.Board = entries
	v.Unread = sess.Standing(entries)
	if v.TopicPath == "" {
		v.TopicPath = soulstream.LastLive(entries)
	}
	if v.TopicPath != "" {
		th := topic.Open(m.sp.Reader(), v.TopicPath)
		if mt, err := th.Materialise(ctx); err == nil {
			th.UseKeyring(m.sp.Keyring(mt))
			if mt2, err := th.Materialise(ctx); err == nil {
				mt = mt2
			}
			v.Topic = mt
		} else {
			v.Err = fmt.Sprintf("topic %s: %v", v.TopicPath, err)
		}
	}
	names, voices := m.directory(ctx, v.Topic)
	for p, n := range names {
		if v.Names[p] == "" {
			v.Names[p] = n
		}
	}
	v.Voices = voices
	v.Lookups = m.lookups(v.Names, v.TopicPath)
	return v
}

// lookups asks the frame, once per render, where else each person in this
// conversation can be read about. Everything about the answer belongs to
// whoever answers: this module supplies a persona and the conversation to
// come back to, and puts whatever href it is handed on the screen.
//
// In a deployment that administers nobody, every ask comes back empty and
// the panel says the same names in plain text. Nothing here has to know
// which deployment it is in — that is the shell's to know and this module's
// to render either way.
func (m *Module) lookups(names map[string]string, topicPath string) map[string]shell.Link {
	out := map[string]shell.Link{}
	for persona := range names {
		if l, ok := m.sh.Link(adminModule, routePerson, map[string]string{
			"who": persona, "topic": topicPath,
		}); ok {
			out[persona] = l
		}
	}
	return out
}

// directory resolves everyone a conversation mentions by name once per
// render: what each persona is called on screen, and the operator claim
// their channel is read from. Everyone the panel beside the conversation
// can name is here — whoever spoke, whoever opened or took on work,
// whoever attached something.
func (m *Module) directory(ctx context.Context, mt *topic.MaterializedTopic,
) (map[string]string, map[string]voice) {
	names, voices := map[string]string{}, map[string]voice{}
	add := func(p string) {
		if p == "" || names[p] != "" {
			return
		}
		c := m.sp.Card(ctx, p)
		names[p], voices[p] = c.Name, voice{OperatedBy: c.OperatedBy}
	}
	if mt != nil {
		for _, c := range mt.Contributions {
			add(c.Author)
		}
		for _, w := range mt.WorkItems {
			add(w.Author)
			add(w.Owner)
		}
		for _, a := range mt.Attachments {
			add(a.Author)
		}
	}
	return names, voices
}

// resolveTopic falls back to the conversation the view itself defaults to
// when the caller names none.
func (m *Module) resolveTopic(ctx context.Context, want string) string {
	if want != "" {
		return want
	}
	return m.observe(ctx, "", nil).TopicPath
}

// actPostTurn is the composer's act: a message on the record through the
// session's own admitted connection, attributed and signed as the signed-in
// principal. The message itself reaches the view the ordinary way — the
// live stream's next morph — so only the composer's own targets are patched
// here.
func (m *Module) actPostTurn(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		shell.Patch(w, composerNote("Sign in first."))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, composerNote("That message could not be read."))
		return
	}
	body := strings.TrimSpace(r.PostFormValue("body"))
	if body == "" {
		shell.Patch(w, composerNote("Write a message first."))
		return
	}
	topicPath := m.resolveTopic(r.Context(), r.URL.Query().Get("topic"))
	if topicPath == "" {
		shell.Patch(w, composerNote("There is no conversation to post to."))
		return
	}
	// Who the message is about, decided here against the record: the picks
	// the composer sent, kept only where the body still names them, and any
	// name typed by hand that can only mean one person in the room. The body
	// itself is posted exactly as written — nothing rewrites a word of it.
	mentions := resolveMentions(body, r.PostForm["mention"],
		m.peopleIn(r.Context(), sess, topicPath))
	id, err := say(r.Context(), sess, topicPath, body, r.PostFormValue("reply-to"), mentions)
	if err != nil {
		// A page served before somebody archived this conversation still
		// carries a composer; the record's refusal is answered in the
		// composer's own words, not as a raw error.
		if errors.Is(err, topic.ErrTopicArchived) {
			shell.Patch(w, composerNote("This conversation is archived — kept for reading, closed to writing."))
			return
		}
		shell.Patch(w, composerNote("Not posted — "+err.Error()))
		return
	}
	shell.Patch(w, composerBox(topicPath), "mode replace")
	shell.Patch(w, composerPicks(), "mode replace")
	shell.Patch(w, renderSuggest(nil))
	shell.Patch(w, composerReplyTo("", ""))
	shell.Patch(w, composerNote("Posted as "+sess.ScreenName(r.Context())+" · "+id))
}

// composerSuggest offers the people in the conversation for the @ somebody
// is typing. It reads the record over the surface's own read lane, morphs
// one element and keeps nothing: a half-written message never leaves the
// browser except as the fragment this filters on.
func (m *Module) composerSuggest(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	topicPath := m.resolveTopic(r.Context(), r.URL.Query().Get("topic"))
	people := m.peopleIn(r.Context(), sess, topicPath)
	shell.Patch(w, renderSuggest(suggestions(people, r.URL.Query().Get("q"))))
}

// composerReply sets — or, with no op, clears — the message the composer
// answers. Only the anchor line is patched, so a half-written message stays
// where it is.
func (m *Module) composerReply(w http.ResponseWriter, r *http.Request) {
	if m.sp.Session(r) == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	opID := r.URL.Query().Get("op")
	if opID == "" {
		shell.Patch(w, composerReplyTo("", ""))
		return
	}
	// The anchor is resolved against the record, never taken on the
	// browser's word: an op that is not in this conversation is no anchor.
	topicPath := m.resolveTopic(r.Context(), r.URL.Query().Get("topic"))
	mt, err := topic.Open(m.sp.Reader(), topicPath).Materialise(r.Context())
	if err != nil {
		shell.Patch(w, composerNote("That conversation could not be read."))
		return
	}
	author, ok := contributionAuthor(mt, opID)
	if !ok {
		shell.Patch(w, composerReplyTo("", ""))
		return
	}
	shell.Patch(w, composerReplyTo(opID, m.sp.Name(r.Context(), author)))
}
