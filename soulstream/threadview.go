package soulstream

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
)

// A conversation shown as messages: the one rendering of the record's
// thread this product has (hq design soulstream-shell 0012 §4). It lives in
// the support layer for the same reason UnreadMark does — two modules put
// the same thing on screen, and rendering it once here keeps it one thing:
// the conversations screen shows it as the room a person is in, the agent
// detail as the room behind an agent, and neither module may import the
// other (design 0002; the purity gates hold it).

// Voice is what the directory says about a persona beyond its name: who
// answers for it. A persona with no operator claim answers for itself —
// the human channel; one somebody else answers for speaks on the machine
// channel. Accountability, not species (the record refuses the species
// question on purpose; see the conversations module's channel notes).
type Voice struct {
	// OperatedBy is the persona the directory says answers for this one,
	// "" when the persona answers for itself.
	OperatedBy string
}

// Channel is the accent this voice speaks on: "human" or "machine" — class
// names as much as concepts.
func (vo Voice) Channel() string {
	if vo.OperatedBy != "" {
		return "machine"
	}
	return "human"
}

// ThreadView is what rendering one conversation's messages needs to know
// about the reader and the room: whose screen it is, what everyone is
// called, which channel each voice speaks on, and — optionally — how a
// per-message reply control renders. A nil ReplyLink is a thread read
// without a reply affordance, which is what a compact room shows.
type ThreadView struct {
	// Me is the signed-in person's own principal — what decides whose
	// messages sit right and which ones say the reader's name.
	Me string
	// Names maps a persona to the name shown for it on screen.
	Names map[string]string
	// Voices maps a persona to the directory's claim about who answers for
	// it. A persona missing here answers for itself.
	Voices map[string]Voice
	// TopicPath is the room these messages are in.
	TopicPath string
	// ReplyLink renders the per-message reply control, nil for none.
	ReplyLink func(opID string) string
}

// NameOf is the on-screen name for a persona: the directory's when it
// published one, else the id the record itself carries. Never nothing — an
// unnamed voice is still a voice.
func NameOf(tv ThreadView, persona string) string {
	if n := tv.Names[persona]; n != "" {
		return n
	}
	if persona == "" {
		return "unattributed"
	}
	return persona
}

// ChannelOf is the channel a persona speaks on.
func ChannelOf(tv ThreadView, persona string) string { return tv.Voices[persona].Channel() }

// ChannelWords is what the pip says when somebody hovers it: the record's
// own fact about the voice, in the words the record uses for it.
func ChannelWords(tv ThreadView, persona string) string {
	if op := tv.Voices[persona].OperatedBy; op != "" {
		return "operated by " + NameOf(tv, op)
	}
	return "answers for itself"
}

// PipFor is the lamp itself: the same 8px LED at the same weight on both
// channels, amber or teal, so neither outranks the other on a screen.
func PipFor(ch, words string) string {
	return fmt.Sprintf(`<span class="led %s" title="%s"></span>`, ch, shell.Esc(words))
}

// ChannelPip is the lamp beside a voice.
func ChannelPip(tv ThreadView, persona string) string {
	return PipFor(ChannelOf(tv, persona), ChannelWords(tv, persona))
}

// SigMark is the earned signature verdict, and silence is the earned
// normal (the calm pass): a verified message says nothing, because on a
// working realm that is every message and a word repeated under all of
// them is noise, not assurance. The exceptions are the news and they
// speak — unsigned, unknown key, anything else the record earned.
func SigMark(s topic.SigStatus) string {
	switch s {
	case topic.SigVerified:
		return ""
	case topic.SigUnsigned:
		return `<span class="verdict">unsigned</span>`
	case topic.SigUnknownKey:
		return `<span class="verdict warn">unknown key</span>`
	default:
		return `<span class="verdict warn">` + shell.Esc(string(s)) + `</span>`
	}
}

// MentionsMe reports whether a message tapped the reader on the shoulder.
// The record says so itself — the library writes the names it parsed out of
// a body onto the message.
func MentionsMe(tv ThreadView, c *topic.Contribution) bool {
	if tv.Me == "" {
		return false
	}
	for _, m := range c.Mentions {
		if m == tv.Me {
			return true
		}
	}
	return false
}

// ThreadNode is one message with the answers hanging off it.
type ThreadNode struct {
	Msg     *topic.Contribution
	Replies []*topic.Contribution
}

// ThreadItem is one thing on the conversation's timeline: a message with
// its answers, or something the room did rather than said.
type ThreadItem struct {
	At   time.Time
	Node *ThreadNode
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

// Timeline groups a materialised topic into what a thread shows: root
// messages in time order, every anchored answer under the message it
// answers, and work marks in their place.
func Timeline(mt *topic.MaterializedTopic) []ThreadItem {
	if mt == nil {
		return nil
	}
	byID := make(map[string]*topic.Contribution, len(mt.Contributions))
	for i := range mt.Contributions {
		byID[mt.Contributions[i].OpID] = &mt.Contributions[i]
	}
	nodes := make(map[string]*ThreadNode, len(mt.Contributions))
	var items []ThreadItem
	for i := range mt.Contributions {
		c := &mt.Contributions[i]
		if rootOf(byID, c) != c {
			continue
		}
		n := &ThreadNode{Msg: c}
		nodes[c.OpID] = n
		items = append(items, ThreadItem{At: c.Timestamp, Node: n})
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
		items = append(items, ThreadItem{At: mt.WorkItems[i].Timestamp, Work: &mt.WorkItems[i]})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At.Before(items[j].At) })
	return items
}

// WorkLine is one work mark on the timeline: the status stamped, the event
// in a sentence that matches it.
func WorkLine(tv ThreadView, w *topic.WorkItem) string {
	title := w.Title
	if title == "" {
		title = "an unnamed piece of work"
	}
	what := fmt.Sprintf("%s opened work “%s”", shell.Esc(NameOf(tv, w.Author)), shell.Esc(title))
	switch w.Status {
	case topic.WorkClaimed:
		who := "Someone"
		if w.Owner != "" {
			who = NameOf(tv, w.Owner)
		}
		what = fmt.Sprintf("%s took up “%s”", shell.Esc(who), shell.Esc(title))
	case topic.WorkDone:
		what = fmt.Sprintf("“%s” is finished", shell.Esc(title))
	}
	return fmt.Sprintf(`<div class="sysline"><span class="strip shell">%s</span>`+
		`<span class="what">%s</span></div>`, shell.Esc(string(w.Status)), what)
}

// RenderMessage is one bubble, with the answers to it (already rendered)
// hanging underneath.
//
// Two things are said about every message and they are kept apart. WHOSE it
// is is said by which side it sits on and by nothing else: the signed-in
// person's own sit right and carry no name, everyone else's sit left,
// named. WHICH CHANNEL it is in is said by the card's own edge and by the
// pip in its byline. A message with the reader's name in it is marked,
// quietly and for good, by hardening the card's outline rather than
// borrowing a channel colour.
func RenderMessage(tv ThreadView, c *topic.Contribution, reply bool, answers string) string {
	mine := tv.Me != "" && c.Author == tv.Me
	cls := "msg " + ChannelOf(tv, c.Author)
	if mine {
		cls += " mine"
	}
	if reply {
		cls += " reply"
	}
	if MentionsMe(tv, c) {
		cls += " mentions"
	}
	// The clock shows the time of day; the day itself rides the hover, for
	// a conversation read back across more days than one.
	at := fmt.Sprintf(`<span class="at" title="%s">%s</span>`,
		c.Timestamp.Format("2006-01-02 15:04"), c.Timestamp.Format("15:04"))
	byline := fmt.Sprintf(`<div class="byline">%s<span class="name">%s</span>%s</div>`,
		ChannelPip(tv, c.Author), shell.Esc(NameOf(tv, c.Author)), at)
	if mine {
		byline = fmt.Sprintf(`<div class="byline">%s%s</div>`, ChannelPip(tv, c.Author), at)
	}
	reach := ""
	if tv.ReplyLink != nil {
		reach = tv.ReplyLink(c.OpID)
	}
	return fmt.Sprintf(`<div class="%s" data-op="%s"><div class="bubble">%s`+
		`<p class="body">%s</p><div class="under">%s%s</div></div>%s</div>`,
		cls, shell.Esc(c.OpID), byline, RenderMessageBody(tv, c), SigMark(c.Sig),
		reach, answers)
}

// RenderMessages is a whole timeline as markup: work marks and message
// bubbles in time order, answers under the message they answer. The
// container is the caller's — a screen owns its blanks and its head.
func RenderMessages(tv ThreadView, mt *topic.MaterializedTopic) string {
	var b strings.Builder
	for _, it := range Timeline(mt) {
		if it.Work != nil {
			b.WriteString(WorkLine(tv, it.Work))
			continue
		}
		answers := ""
		if len(it.Node.Replies) > 0 {
			var r strings.Builder
			r.WriteString(`<div class="replies">`)
			for _, rc := range it.Node.Replies {
				r.WriteString(RenderMessage(tv, rc, true, ""))
			}
			r.WriteString(`</div>`)
			answers = r.String()
		}
		b.WriteString(RenderMessage(tv, it.Node.Msg, false, answers))
	}
	return b.String()
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
func mentionTokens(tv ThreadView, c *topic.Contribution) []mentionToken {
	var out []mentionToken
	seen := map[string]bool{}
	for _, m := range c.Mentions {
		for _, form := range []string{m, NameOf(tv, m)} {
			if form == "" {
				continue
			}
			t := "@" + strings.ToLower(form)
			if seen[t] {
				continue
			}
			seen[t] = true
			out = append(out, mentionToken{Text: t, Channel: ChannelOf(tv, m)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Text) > len(out[j].Text) })
	return out
}

// RenderMessageBody is a message as it was written — escaped, never
// rewritten — with the names that actually tapped somebody marked.
//
// What gets marked comes off the record's own mentions field, so the mark
// means a slip reached that person rather than that the text looks like an
// address. A name that resolved to nobody stays plain, which is the honest
// thing for it to be: nothing happened when it was posted.
func RenderMessageBody(tv ThreadView, c *topic.Contribution) string {
	tokens := mentionTokens(tv, c)
	if len(tokens) == 0 {
		return shell.Esc(c.Body)
	}
	body, lower := c.Body, strings.ToLower(c.Body)
	var b strings.Builder
	for i := 0; i < len(body); {
		if body[i] != '@' {
			next := strings.IndexByte(body[i:], '@')
			if next < 0 {
				b.WriteString(shell.Esc(body[i:]))
				break
			}
			b.WriteString(shell.Esc(body[i : i+next]))
			i += next
			continue
		}
		if n, ch := tokenAt(lower, i, tokens); n > 0 {
			fmt.Fprintf(&b, `<span class="mtoken %s">%s</span>`, ch, shell.Esc(body[i:i+n]))
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
		if strings.HasPrefix(lower[i:], t.Text) && tokenEnds(lower, i+len(t.Text)) {
			return len(t.Text), t.Channel
		}
	}
	return 0, ""
}

// tokenEnds reports whether a mention token may end at i: at the end of the
// text, or before something that is not part of a name. (The composer's
// picker keeps its own copy over its own grammar — the two may diverge.)
func tokenEnds(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
}
