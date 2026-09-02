package agents

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

// The agent detail — the room behind the agent (hq design 0012 §4).
//
// An agent's home topic is not a conversation: it is the agent's
// operational room — its placement, its wake outcomes, its self-reports —
// so the conversations screen deliberately does not list it. This screen is
// where it lives instead: who the agent is, what its declaration says, and
// the room's thread with a composer whose message MENTIONS the agent —
// mention is the default-on wake, and a bare turn would reach an agent only
// through a topic wake it may not have. One outcome per turn is the
// engine's own idempotence (episode 0130); this surface just says the name.
//
// The thread renders through the support layer's one thread rendering
// (soulstream/threadview.go) — the same bubbles the conversations screen
// shows, defined once.

// roomPath is where the detail answers; the who rides the query.
const roomPath = "/agents/room"

// The room's own patch targets: the thread the live channel morphs, and the
// composer's note and box, which only the say act writes.
const (
	roomThreadID = "agent-room"
	roomNoteID   = "room-note"
	roomBoxID    = "room-box"
)

// roomView is one reading of everything the detail shows.
type roomView struct {
	Who   string
	Name  string
	Card  soulstream.Card
	Sign  soulstream.LifeSign
	Known bool
	// Roster is the credentialed row when this deployment issued one, nil
	// otherwise — a declared agent needs none.
	Roster *soulstream.Agent
	// Decls are this persona's placements, oldest first; Home is the room
	// its newest declaration names, "" when nothing here declares it.
	Decls []soulstream.Declared
	Home  string
	Topic *topic.MaterializedTopic
	// Me, Names, Voices are the shared thread rendering's ingredients.
	Me     string
	Names  map[string]string
	Voices map[string]soulstream.Voice
	Err    string
}

// tv is this reading as the shared thread rendering takes it. No reply
// control: the room's composer is one box that mentions the agent.
func (v roomView) tv() soulstream.ThreadView {
	return soulstream.ThreadView{
		Me: v.Me, Names: v.Names, Voices: v.Voices, TopicPath: v.Home,
	}
}

// room is the detail screen. Session-gated like every surface carrying the
// record; asked about nobody, it goes to the roster the way it always did.
func (m *Module) room(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		m.sh.SignIn(w, r)
		return
	}
	who := r.URL.Query().Get("who")
	if who == "" {
		http.Redirect(w, r, "/agents", http.StatusFound)
		return
	}
	v := m.roomRead(r.Context(), sess, who)
	page := shell.Page{Title: v.Name, Section: sectionAgents, Body: m.sh.Sheet(renderRoom(v))}
	if v.Home != "" {
		page.Live = true
		page.Init = fmt.Sprintf("@get('%s/live?who=%s')", roomPath, qesc(who))
	}
	m.sh.Render(w, r, page)
}

// roomRead gathers the detail: the directory's word on the voice, its life
// sign, its credential row when there is one, its placements, and — when a
// declaration names a home — the room itself, verdicts earned. Reading the
// room is reading it: the tray's marks for it clear here, exactly as they
// clear when a conversation is looked at.
func (m *Module) roomRead(ctx context.Context, sess *soulstream.Session, who string) roomView {
	v := roomView{Who: who, Me: sess.Persona,
		Names: map[string]string{}, Voices: map[string]soulstream.Voice{}}
	v.Card = m.sp.Card(ctx, who)
	v.Name = v.Card.Name
	if v.Name == "" {
		v.Name = who
	}
	v.Names[sess.Persona] = sess.ScreenName(ctx)
	v.Names[who] = v.Name
	v.Voices[who] = soulstream.Voice{OperatedBy: v.Card.OperatedBy}
	if v.Card.OperatedBy != "" && v.Names[v.Card.OperatedBy] == "" {
		v.Names[v.Card.OperatedBy] = m.sp.Name(ctx, v.Card.OperatedBy)
	}
	v.Sign, v.Known = m.sp.Presence(ctx)[who]
	if ag := m.sp.Agents(); ag != nil {
		if list, err := ag.List(ctx); err == nil {
			for i := range list {
				if list[i].Handle == who {
					v.Roster = &list[i]
					break
				}
			}
		}
	}
	if m.placing() {
		list, err := m.sp.Declared(ctx)
		if err != nil {
			v.Err = err.Error()
		}
		for _, d := range list {
			if d.Name == who {
				v.Decls = append(v.Decls, d)
				v.Home = d.Home
			}
		}
	}
	if v.Home != "" {
		th := topic.Open(m.sp.Reader(), v.Home)
		if mt, err := th.Materialise(ctx); err == nil {
			th.UseKeyring(m.sp.Keyring(mt))
			if mt2, err := th.Materialise(ctx); err == nil {
				mt = mt2
			}
			v.Topic = mt
			for _, c := range mt.Contributions {
				m.roomName(ctx, &v, c.Author)
			}
			for _, wi := range mt.WorkItems {
				m.roomName(ctx, &v, wi.Author)
				m.roomName(ctx, &v, wi.Owner)
			}
		} else {
			v.Err = fmt.Sprintf("its room %s: %v", v.Home, err)
		}
		sess.Read(v.Home)
	}
	return v
}

// roomName resolves one voice in the room, once.
func (m *Module) roomName(ctx context.Context, v *roomView, persona string) {
	if persona == "" || v.Names[persona] != "" {
		return
	}
	c := m.sp.Card(ctx, persona)
	v.Names[persona] = c.Name
	v.Voices[persona] = soulstream.Voice{OperatedBy: c.OperatedBy}
}

// roomLive keeps the room's thread current, the conversations screen's own
// cadence: the record is re-read and morphed into the thread's one target,
// and looking at the room keeps reading it.
func (m *Module) roomLive(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	who := r.URL.Query().Get("who")
	shell.Stream(w, r, time.Second, func(out io.Writer) {
		v := m.roomRead(r.Context(), sess, who)
		shell.WriteElements(out, roomThread(v))
	})
}

// actSay is the room's composer act: a message on the home topic through
// the session's own admitted connection, mentioning the agent so it wakes.
func (m *Module) actSay(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		shell.Patch(w, roomNote("Sign in first."))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, roomNote("That message could not be read."))
		return
	}
	who := r.URL.Query().Get("who")
	body := strings.TrimSpace(r.PostFormValue("body"))
	if body == "" {
		shell.Patch(w, roomNote("Write a message first."))
		return
	}
	// The room is resolved against the record at the act, never taken on
	// the browser's word: the newest declaration names it.
	home := m.sp.HomeOf(who)
	if home == "" {
		shell.Patch(w, roomNote("Nothing here declares "+who+" — there is no room to post in."))
		return
	}
	h := topic.Open(sess.Client(), home)
	if _, err := h.Materialise(r.Context()); err != nil {
		shell.Patch(w, roomNote("Its room could not be read: "+err.Error()))
		return
	}
	id, err := h.PostTurnMentioning(r.Context(), body, []string{who})
	if err != nil {
		if errors.Is(err, topic.ErrTopicArchived) {
			shell.Patch(w, roomNote("Its room is archived — kept for reading, closed to writing."))
			return
		}
		shell.Patch(w, roomNote("Not posted — "+err.Error()))
		return
	}
	shell.Patch(w, roomBox(), "mode replace")
	shell.Patch(w, roomNote("Said, with its name on it — a mention is what wakes it. · "+id))
}

// renderRoom is the whole detail: who this is, what its declaration says,
// and the room with the composer under it.
func renderRoom(v roomView) string {
	var b strings.Builder
	b.WriteString(`<div class="page-head"><div class="ph-words">`)
	fmt.Fprintf(&b, `<h1><span class="led machine" title="%s"></span> %s</h1>`,
		esc(soulstream.ChannelWords(v.tv(), v.Who)), esc(v.Name))
	fmt.Fprintf(&b, `<p class="lede"><span class="mono">@%s</span>%s%s</p>`,
		esc(v.Who), operatorWords(v), aroundWords(v))
	b.WriteString(`</div>`)
	if v.Home != "" {
		fmt.Fprintf(&b, `<p class="act"><a class="btn ghost" href="/?topic=%s">Read its whole room</a></p>`,
			qesc(v.Home))
	}
	b.WriteString(`</div>`)
	if v.Err != "" {
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
	}
	b.WriteString(roomFacts(v))
	b.WriteString(roomSection(v))
	return b.String()
}

// operatorWords names who answers for this voice, when the record says.
func operatorWords(v roomView) string {
	if v.Card.OperatedBy == "" {
		return ""
	}
	name := v.Names[v.Card.OperatedBy]
	if name == "" || name == v.Card.OperatedBy {
		return fmt.Sprintf(` · operated by <span class="mono">@%s</span>`, esc(v.Card.OperatedBy))
	}
	return fmt.Sprintf(` · operated by %s <span class="mono">@%s</span>`,
		esc(name), esc(v.Card.OperatedBy))
}

// aroundWords is the life sign in the head's own line, judged the way the
// roster judges it.
func aroundWords(v roomView) string {
	if !v.Known {
		return ""
	}
	switch {
	case v.Sign.Present:
		return ` · <span class="pill ok"><span class="led ok"></span>in</span>`
	case v.Sign.Left:
		return " · left " + agoWord(v.Sign.When)
	default:
		return " · seen " + agoWord(v.Sign.When)
	}
}

// roomFacts is what the declarations say, one line per placement — the
// wakes with their delivery classes, what it thinks with, where the
// placement stands — and the honest words for an agent nothing declares.
func roomFacts(v roomView) string {
	var b strings.Builder
	b.WriteString(`<div class="section"><h2>How it runs</h2>`)
	switch {
	case len(v.Decls) == 0 && v.Roster != nil:
		b.WriteString(`<p class="note">Runs from somebody&#39;s machine with its own ` +
			`credential — no room of its own here. Mention it in any conversation to reach it.</p>`)
	case len(v.Decls) == 0:
		b.WriteString(`<p class="note">Nothing on this deployment answers for how it runs.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Wakes on</th><th>Thinks with</th><th>State</th><th>Declared</th>` +
			`</tr></thead><tbody>`)
		for _, d := range v.Decls {
			model := `<span class="dim">the assistant already set up here</span>`
			if d.Model != "" {
				model = fmt.Sprintf(`<span class="mono">%s</span>`, esc(d.Model))
			}
			declared := "—"
			if !d.Opened.IsZero() {
				declared = d.Opened.Format("2006-01-02")
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td class="mono">%s</td></tr>`,
				wakeCell(d.Wakes), model, stateCell(d), esc(declared))
		}
		b.WriteString(`</tbody></table></div>`)
		b.WriteString(waitingNote(v.Decls))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// roomSection is the room itself: the thread the live channel keeps
// current, and the composer whose message says the agent's name.
func roomSection(v roomView) string {
	if v.Home == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="section"><h2>Talk to it</h2>`)
	b.WriteString(roomThread(v))
	fmt.Fprintf(&b, `<form id="room-say" class="dock room-dock"`+
		` data-on:submit="@post('/act/agent-say?who=%s', {contentType:'form'})">`,
		qesc(v.Who))
	b.WriteString(roomBox())
	fmt.Fprintf(&b, `<div class="dock-row"><button class="btn send" type="submit">%s`+
		`<span>Send</span></button></div>`, shell.Icon("send"))
	b.WriteString(roomNote(""))
	b.WriteString(`</form></div>`)
	return b.String()
}

// roomThread is the thread's one live target.
func roomThread(v roomView) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s" class="thread-body room"><div class="msgs">`, roomThreadID)
	msgs := soulstream.RenderMessages(v.tv(), v.Topic)
	if msgs == "" {
		b.WriteString(`<p class="blank">Nothing in its room yet — say something below; ` +
			`your message carries its name, which is what wakes it.</p>`)
	}
	b.WriteString(msgs)
	b.WriteString(`</div></div>`)
	return b.String()
}

// roomBox is the composer's box, its own patch target so the say act can
// hand back an empty one.
func roomBox() string {
	return fmt.Sprintf(`<textarea id="%s" name="body" rows="2"`+
		` placeholder="Say something — it wakes on its name"></textarea>`, roomBoxID)
}

// roomNote is the composer's own result line.
func roomNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, roomNoteID, esc(msg))
}
