package conversations

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
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
// most recently announced first, the open one marked. The archived ones
// rest under a fold at the foot — still one click away, never gone. The
// fold's toggle is the person's own: the morph is told to leave the open
// attribute alone, so the stream re-writing this list once a second never
// snaps the fold shut. The server serves it open in one case only —
// when the conversation the person is looking at is itself archived.
func renderRail(v view) string {
	var b strings.Builder
	b.WriteString(`<nav id="conversations" class="rail-list">`)
	if len(v.Board) == 0 {
		b.WriteString(`<p class="rail-note">No conversations yet. Start one above.</p>`)
	}
	var archived []int
	for i := len(v.Board) - 1; i >= 0; i-- {
		if v.Board[i].Lifecycle == topic.Archived {
			archived = append(archived, i)
			continue
		}
		b.WriteString(railRow(v, v.Board[i], "conv"))
	}
	if len(archived) > 0 {
		open := ""
		for _, i := range archived {
			if v.Board[i].Path == v.TopicPath {
				open = " open"
			}
		}
		fmt.Fprintf(&b, `<details id="rail-archived" class="archfold" data-preserve-attr="open"%s>`+
			`<summary>Archived (%d)</summary>`, open, len(archived))
		for _, i := range archived {
			b.WriteString(railRow(v, v.Board[i], "conv archived"))
		}
		b.WriteString(`</details>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// railRow is one conversation in the rail.
func railRow(v view, e topic.BoardEntry, cls string) string {
	if e.Path == v.TopicPath {
		cls += " on"
	}
	if v.Unread[e.Path] > 0 {
		cls += " unread"
	}
	name := e.Announcement.Name
	if name == "" {
		name = e.Path
	}
	return fmt.Sprintf(`<a class="%s" href="/?topic=%s"><span class="name">%s</span>`+
		`<span class="state">%s</span>%s</a>`,
		cls, qesc(e.Path), esc(name), stateWords(e.Lifecycle), soulstream.UnreadMark(v.Unread[e.Path]))
}

// stateWords is a conversation's standing in a person's own word — the
// record's vocabulary stays on the record. The details panel says the same
// things in sentences (lifecycleWords); these are the one-word row forms.
// An unknown word arrives as itself: newer records outrank this list.
func stateWords(l topic.Lifecycle) string {
	switch l {
	case topic.Proposed:
		return "new"
	case topic.Active:
		return "going on"
	case topic.Dormant:
		return "quiet"
	case topic.Closed:
		return "closed"
	case topic.Archived:
		return "archived"
	default:
		return esc(string(l))
	}
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

// workLine is one work mark on the timeline: the status stamped, the event
// in a sentence that matches it.
func workLine(v view, w *topic.WorkItem) string {
	title := w.Title
	if title == "" {
		title = "an unnamed piece of work"
	}
	what := fmt.Sprintf("%s opened work “%s”", esc(nameOf(v, w.Author)), esc(title))
	switch w.Status {
	case topic.WorkClaimed:
		who := "Someone"
		if w.Owner != "" {
			who = nameOf(v, w.Owner)
		}
		what = fmt.Sprintf("%s took up “%s”", esc(who), esc(title))
	case topic.WorkDone:
		what = fmt.Sprintf("“%s” is finished", esc(title))
	}
	return fmt.Sprintf(`<div class="sysline"><span class="strip shell">%s</span>`+
		`<span class="what">%s</span></div>`, esc(string(w.Status)), what)
}

// renderMsg is one bubble, with the answers to it (already rendered)
// hanging underneath.
//
// Two things are said about every message and they are kept apart. WHOSE it
// is is said by which side it sits on and by nothing else: the signed-in
// person's own sit right and carry no name, everyone else's sit left, named.
// WHICH CHANNEL it is in is said by the card's own edge and by the pip in
// its byline — amber for a voice that answers for itself, teal for one
// somebody else answers for (channel.go). Neither says the other's thing.
//
// A message with the reader's name in it is marked, quietly and for good:
// scrolling back, the ones that were about them are still the ones that were
// about them. The mark hardens the card's outline rather than borrowing a
// channel colour, so it can never be mistaken for who spoke.
func renderMsg(v view, c *topic.Contribution, reply bool, answers string) string {
	cls := "msg " + channelOf(v, c.Author)
	if mine(v, c.Author) {
		cls += " mine"
	}
	if reply {
		cls += " reply"
	}
	if mentionsMe(v, c) {
		cls += " mentions"
	}
	// The clock shows the time of day; the day itself rides the hover, for
	// a conversation read back across more days than one.
	at := fmt.Sprintf(`<span class="at" title="%s">%s</span>`,
		c.Timestamp.Format("2006-01-02 15:04"), c.Timestamp.Format("15:04"))
	byline := fmt.Sprintf(`<div class="byline">%s<span class="name">%s</span>%s</div>`,
		channelPip(v, c.Author), esc(nameOf(v, c.Author)), at)
	if mine(v, c.Author) {
		byline = fmt.Sprintf(`<div class="byline">%s%s</div>`, channelPip(v, c.Author), at)
	}
	return fmt.Sprintf(`<div class="%s" data-op="%s"><div class="bubble">%s`+
		`<p class="body">%s</p><div class="under">%s%s</div></div>%s</div>`,
		cls, esc(c.OpID), byline, renderBody(v, c), sigMark(c.Sig),
		replyLink(v.TopicPath, c.OpID), answers)
}

// mentionToken is one way a tapped persona may have been written, and the
// channel that persona speaks on — so a name in a sentence carries the same
// accent as the messages the person it names writes.
type mentionToken struct {
	Text    string // lowercased "@…", for matching
	Channel string
}

// mentionTokens is what to look for in a body: for every persona the record
// says the message tapped, the handle it may have been written as and the
// name this reader is shown for them. Lowercased for matching, longest
// first, so "@Avery Blake" wins where "@Avery" would also fit.
func mentionTokens(v view, c *topic.Contribution) []mentionToken {
	var out []mentionToken
	seen := map[string]bool{}
	for _, m := range c.Mentions {
		for _, form := range []string{m, nameOf(v, m)} {
			if form == "" {
				continue
			}
			t := "@" + strings.ToLower(form)
			if seen[t] {
				continue
			}
			seen[t] = true
			out = append(out, mentionToken{Text: t, Channel: channelOf(v, m)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Text) > len(out[j].Text) })
	return out
}

// renderBody is a message as it was written — escaped, never rewritten —
// with the names that actually tapped somebody marked.
//
// What gets marked comes off the record's own mentions field, so the mark
// means a slip reached that person rather than that the text looks like an
// address. A name that resolved to nobody stays plain, which is the honest
// thing for it to be: nothing happened when it was posted.
func renderBody(v view, c *topic.Contribution) string {
	tokens := mentionTokens(v, c)
	if len(tokens) == 0 {
		return esc(c.Body)
	}
	body, lower := c.Body, strings.ToLower(c.Body)
	var b strings.Builder
	for i := 0; i < len(body); {
		if body[i] != '@' {
			next := strings.IndexByte(body[i:], '@')
			if next < 0 {
				b.WriteString(esc(body[i:]))
				break
			}
			b.WriteString(esc(body[i : i+next]))
			i += next
			continue
		}
		if n, ch := tokenAt(lower, i, tokens); n > 0 {
			fmt.Fprintf(&b, `<span class="mtoken %s">%s</span>`, ch, esc(body[i:i+n]))
			i += n
			continue
		}
		b.WriteByte('@')
		i++
	}
	return b.String()
}

// tokenAt is the length of the mention token starting at i and the channel
// of the voice it names, or 0 for none.
func tokenAt(lower string, i int, tokens []mentionToken) (int, string) {
	for _, t := range tokens {
		if strings.HasPrefix(lower[i:], t.Text) && endsToken(lower, i+len(t.Text)) {
			return len(t.Text), t.Channel
		}
	}
	return 0, ""
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
	// The Details key exists for the widths where the details column has no
	// place of its own — the CSS keeps it off every other screen. It rides
	// the head the stream morphs, which is fine: it is the same markup every
	// tick, and the signal it flips lives outside the stream's targets.
	fmt.Fprintf(&b, `<div class="thread-head centred"><h1>%s</h1>`+
		`<span class="where">%s</span>`+
		`<button type="button" class="det-open" title="Who is here and where this stands"`+
		` data-on:click="$info = !$info">%s<span>Details</span></button></div>`,
		esc(title), esc(where), shell.Icon("users"))

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
			// A stamped strip for the state, plain words for the event: the
			// strip is the thing that shouts, and a work title in capitals is a
			// title nobody wrote. The sentence agrees with the strip — a
			// claimed item is in somebody's hands, a done one is finished, and
			// saying "opened" over either would be the strip calling the
			// sentence a liar.
			b.WriteString(workLine(v, it.Work))
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
