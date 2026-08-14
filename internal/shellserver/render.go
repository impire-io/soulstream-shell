package shellserver

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/impire-io/soulstream-core/topic"
)

// The chat surface. The signed-in person sees the conversations they can
// reach on the left and one conversation in the middle, newest last, with
// the composer docked under it.
//
// Two patch targets belong to the live stream — #conversations and #dash —
// and the composer owns three of its own, so nothing the stream morphs can
// touch a half-written message.

// nameOf is the on-screen name for a persona: the realm's own persona
// directory when it publishes one, else the id the record itself carries.
// Never nothing — an unnamed voice is still a voice.
func nameOf(v view, persona string) string {
	if n := v.Names[persona]; n != "" {
		return n
	}
	if persona == "" {
		return "unattributed"
	}
	return persona
}

// mine reports whether a contribution is the signed-in person's own. The
// answer comes from the session's principal and the record's author —
// never from anything the browser said.
func mine(v view, author string) bool { return v.Me != "" && author == v.Me }

// sigMark is the earned signature verdict, kept quiet: present on every
// message, loud on none.
func sigMark(s topic.SigStatus) string {
	switch s {
	case topic.SigVerified:
		return `<span class="verdict ok">verified</span>`
	case topic.SigUnsigned:
		return `<span class="verdict">unsigned</span>`
	case topic.SigUnknownKey:
		return `<span class="verdict warn">unknown key</span>`
	default:
		return `<span class="verdict warn">` + esc(string(s)) + `</span>`
	}
}

// renderRail is the left column: the conversations this person can reach,
// most recently announced first, the open one marked.
func renderRail(v view) string {
	var b strings.Builder
	b.WriteString(`<nav id="conversations" class="rail-list">`)
	if len(v.Board) == 0 {
		b.WriteString(`<p class="rail-note">No conversations yet.</p>`)
	}
	for i := len(v.Board) - 1; i >= 0; i-- {
		e := v.Board[i]
		cls := "conv"
		if e.Path == v.TopicPath {
			cls = "conv on"
		}
		if v.Unread[e.Path] > 0 {
			cls += " unread"
		}
		name := e.Announcement.Name
		if name == "" {
			name = e.Path
		}
		fmt.Fprintf(&b, `<a class="%s" href="/?topic=%s"><span class="name">%s</span>`+
			`<span class="state">%s</span>%s</a>`,
			cls, qesc(e.Path), esc(name), esc(string(e.Lifecycle)), unreadMark(v.Unread[e.Path]))
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// threadNode is one message with the answers hanging off it.
type threadNode struct {
	Msg     *topic.Contribution
	Replies []*topic.Contribution
}

// threadItem is one thing on the conversation's timeline: a message with
// its answers, or something the room did rather than said.
type threadItem struct {
	At   time.Time
	Node *threadNode
	Work *topic.WorkItem
}

// rootOf walks an anchor chain up to the message it ultimately answers. An
// answer to an answer joins its root; an anchor the topic does not hold is
// its own root, so nothing is ever dropped for being unreachable.
func rootOf(byID map[string]*topic.Contribution, c *topic.Contribution) *topic.Contribution {
	for hops := 0; c.Anchor != "" && hops < 32; hops++ {
		p, ok := byID[c.Anchor]
		if !ok || p == c {
			break
		}
		c = p
	}
	return c
}

// timeline groups a materialised topic into what the chat shows: root
// messages in time order, every anchored answer under the message it
// answers, and work marks in their place.
func timeline(mt *topic.MaterializedTopic) []threadItem {
	if mt == nil {
		return nil
	}
	byID := make(map[string]*topic.Contribution, len(mt.Contributions))
	for i := range mt.Contributions {
		byID[mt.Contributions[i].OpID] = &mt.Contributions[i]
	}
	nodes := make(map[string]*threadNode, len(mt.Contributions))
	var items []threadItem
	for i := range mt.Contributions {
		c := &mt.Contributions[i]
		if rootOf(byID, c) != c {
			continue
		}
		n := &threadNode{Msg: c}
		nodes[c.OpID] = n
		items = append(items, threadItem{At: c.Timestamp, Node: n})
	}
	for i := range mt.Contributions {
		c := &mt.Contributions[i]
		root := rootOf(byID, c)
		if root == c {
			continue
		}
		if n := nodes[root.OpID]; n != nil {
			n.Replies = append(n.Replies, c)
		}
	}
	for i := range mt.WorkItems {
		items = append(items, threadItem{At: mt.WorkItems[i].Timestamp, Work: &mt.WorkItems[i]})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At.Before(items[j].At) })
	return items
}

// renderMsg is one bubble, with the answers to it (already rendered)
// hanging underneath. Side is attribution: the signed-in person's own
// messages sit right and carry no name — everyone else's sit left, named.
// A message with the reader's name in it is marked, quietly and for good:
// scrolling back, the ones that were about them are still the ones that
// were about them.
func renderMsg(v view, c *topic.Contribution, reply bool, answers string) string {
	cls := "msg"
	if mine(v, c.Author) {
		cls += " mine"
	}
	if reply {
		cls += " reply"
	}
	if mentionsMe(v, c) {
		cls += " mentions"
	}
	byline := fmt.Sprintf(`<div class="byline"><span class="name">%s</span>`+
		`<span class="at">%s</span></div>`,
		esc(nameOf(v, c.Author)), c.Timestamp.Format("15:04"))
	if mine(v, c.Author) {
		byline = fmt.Sprintf(`<div class="byline"><span class="at">%s</span></div>`,
			c.Timestamp.Format("15:04"))
	}
	return fmt.Sprintf(`<div class="%s" data-op="%s"><div class="bubble">%s`+
		`<p class="body">%s</p><div class="under">%s%s</div></div>%s</div>`,
		cls, esc(c.OpID), byline, esc(c.Body), sigMark(c.Sig),
		replyLink(v.TopicPath, c.OpID), answers)
}

// renderThread is the centre column: one conversation, oldest first.
func renderThread(v view) string {
	var b strings.Builder
	b.WriteString(`<div id="dash" class="thread-body">`)

	title, where := "No conversation open", ""
	if v.Topic != nil && v.Topic.Announcement != nil && v.Topic.Announcement.Name != "" {
		title, where = v.Topic.Announcement.Name, v.TopicPath
	} else if v.TopicPath != "" {
		title, where = v.TopicPath, v.TopicPath
	}
	fmt.Fprintf(&b, `<div class="thread-head centred"><h1>%s</h1>`+
		`<span class="where">%s</span></div>`, esc(title), esc(where))

	b.WriteString(`<div class="msgs centred">`)
	items := timeline(v.Topic)
	switch {
	case v.Err != "":
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
	case v.Topic == nil:
		b.WriteString(`<p class="blank">Pick a conversation on the left.</p>`)
	case len(items) == 0:
		b.WriteString(`<p class="blank">Nothing said here yet — write the first message.</p>`)
	}
	for _, it := range items {
		if it.Work != nil {
			fmt.Fprintf(&b, `<div class="sysline">%s opened work “%s” · %s</div>`,
				esc(nameOf(v, it.Work.Author)), esc(it.Work.Title), esc(string(it.Work.Status)))
			continue
		}
		answers := ""
		if len(it.Node.Replies) > 0 {
			var r strings.Builder
			r.WriteString(`<div class="replies">`)
			for _, rc := range it.Node.Replies {
				r.WriteString(renderMsg(v, rc, true, ""))
			}
			r.WriteString(`</div>`)
			answers = r.String()
		}
		b.WriteString(renderMsg(v, it.Node.Msg, false, answers))
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

// planeCard is one house readout — the same molded panel on the overview
// and on the system-status screen.
func planeCard(icon, heading, row string) string {
	return fmt.Sprintf(`<div class="card plane"><div class="head">%s<h2>%s</h2></div>`+
		`<div class="row">%s</div></div>`, Icon(icon), heading, row)
}

// storageRow and signInRow are the two readouts both screens carry.
func storageRow(v view) string {
	return fmt.Sprintf(`<span class="pill ok"><span class="led machine"></span>keeping</span>`+
		`<span class="mono">%d ops · %.1f MB</span>`, v.StreamMsg, v.StreamMB)
}

func signInRow(v view) string {
	fold := `<span class="pill warn">unreachable</span>`
	if v.FoldOK {
		fold = `<span class="pill ok"><span class="led"></span>serving</span>`
	}
	return fold + `<span class="mono">passkeys</span>`
}

// renderPlanes is the system-status screen's body — the house readouts
// that used to sit where the conversation now is. The work count is the
// open conversation's own; the details panel beside that conversation says
// the same thing in words.
func renderPlanes(v view) string {
	var b strings.Builder
	b.WriteString(`<div class="planes">`)
	b.WriteString(planeCard("cassette-tape", "Storage", storageRow(v)))
	b.WriteString(planeCard("key", "People &amp; sign-in", signInRow(v)))
	open, claimed := 0, 0
	if v.Topic != nil {
		for _, w := range v.Topic.WorkItems {
			switch w.Status {
			case topic.WorkOpen:
				open++
			case topic.WorkClaimed:
				claimed++
			}
		}
	}
	b.WriteString(planeCard("activity", "Work",
		fmt.Sprintf(`<span class="mono">open %d · claimed %d</span>`, open, claimed)))
	b.WriteString(`</div>`)
	return b.String()
}

// renderOverview is the Home screen's body: what the house is doing, and
// the way into every conversation from anywhere.
func renderOverview(v view) string {
	var b strings.Builder
	b.WriteString(`<h1>Your soulstream at a glance</h1>`)
	b.WriteString(`<p class="lede">Everything here is read live from your soulstream — ` +
		`the shell keeps none of it.</p>`)
	b.WriteString(`<div class="planes">`)
	b.WriteString(planeCard("cassette-tape", "Storage", storageRow(v)))
	b.WriteString(planeCard("key", "People &amp; sign-in", signInRow(v)))
	rooms := "conversation"
	if len(v.Board) != 1 {
		rooms = "conversations"
	}
	b.WriteString(planeCard("messages-square", "Talking",
		fmt.Sprintf(`<span class="mono">%d %s</span>`, len(v.Board), rooms)))
	b.WriteString(`</div>`)

	b.WriteString(`<h2 class="section">Conversations</h2>`)
	if v.Err != "" {
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
		return b.String()
	}
	if len(v.Board) == 0 {
		b.WriteString(`<p class="blank">No conversations yet.</p>`)
		return b.String()
	}
	b.WriteString(`<div class="rows">`)
	for i := len(v.Board) - 1; i >= 0; i-- {
		e := v.Board[i]
		name := e.Announcement.Name
		if name == "" {
			name = e.Path
		}
		fmt.Fprintf(&b, `<a class="row" href="/?topic=%s"><span class="name">%s</span>`+
			`<span class="what">%s</span>%s<span class="state">%s</span></a>`,
			qesc(e.Path), esc(name), esc(e.Announcement.SubjectMatter),
			unreadMark(v.Unread[e.Path]), esc(string(e.Lifecycle)))
	}
	b.WriteString(`</div>`)
	return b.String()
}
