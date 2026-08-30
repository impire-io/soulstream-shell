package conversations

import (
	"fmt"
	"strings"

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
//
// The messages themselves render through the support layer's one thread
// rendering (soulstream/threadview.go) — the same bubbles the agent detail
// shows for an agent's room, defined once (hq design 0012 §4).

// tv is this view as the shared thread rendering reads it, with this
// screen's own reply control riding along.
func (v view) tv() soulstream.ThreadView {
	path := v.TopicPath
	return soulstream.ThreadView{
		Me: v.Me, Names: v.Names, Voices: v.Voices, TopicPath: path,
		ReplyLink: func(opID string) string { return replyLink(path, opID) },
	}
}

// nameOf is the on-screen name for a persona — the shared rendering's rule.
func nameOf(v view, persona string) string { return soulstream.NameOf(v.tv(), persona) }

// mine reports whether a contribution is the signed-in person's own. The
// answer comes from the session's principal and the record's author —
// never from anything the browser said.
func mine(v view, author string) bool { return v.Me != "" && author == v.Me }

// timeline and renderBody are this package's names for the shared thread
// rendering — kept so the standing tests pin the one behavior from here.
func timeline(mt *topic.MaterializedTopic) []soulstream.ThreadItem {
	return soulstream.Timeline(mt)
}

func renderBody(v view, c *topic.Contribution) string {
	return soulstream.RenderMessageBody(v.tv(), c)
}

// renderRail is the left column: the conversations this person can reach,
// the most recently active first (hq design 0012 §3), the open one marked.
// The machinery — agent homes, the placements topic — is not listed here at
// all: those are rooms of the record's own, reached from the Agents screen.
// The archived ones rest under a fold at the foot — still one click away,
// never gone. The fold's toggle is the person's own: the morph is told to
// leave the open attribute alone, so the stream re-writing this list once a
// second never snaps the fold shut. The server serves it open in one case
// only — when the conversation the person is looking at is itself archived.
func renderRail(v view) string {
	var b strings.Builder
	b.WriteString(`<nav id="conversations" class="rail-list">`)
	human := soulstream.HumanConversations(v.Board, v.Machinery)
	if len(human) == 0 {
		b.WriteString(`<p class="rail-note">No conversations yet. Start one above.</p>`)
	}
	var archived []topic.BoardEntry
	for _, e := range human {
		if e.Lifecycle == topic.Archived {
			archived = append(archived, e)
			continue
		}
		b.WriteString(railRow(v, e, "conv"))
	}
	if len(archived) > 0 {
		open := ""
		for _, e := range archived {
			if e.Path == v.TopicPath {
				open = " open"
			}
		}
		fmt.Fprintf(&b, `<details id="rail-archived" class="archfold" data-preserve-attr="open"%s>`+
			`<summary>Archived (%d)</summary>`, open, len(archived))
		for _, e := range archived {
			b.WriteString(railRow(v, e, "conv archived"))
		}
		b.WriteString(`</details>`)
	}
	b.WriteString(roomsWaiting(v))
	b.WriteString(`</nav>`)
	return b.String()
}

// roomsWaiting is the rail's honest word when a message with this person's
// name in it landed in a room the rail deliberately does not list (an
// agent's home): the count still stands on the spine, and this line is
// where the click lands somewhere real — the agent's own detail. Without a
// resolvable place to point at, the words stand alone; nothing is hidden
// silently either way (hq design 0012 §4, bar 4).
func roomsWaiting(v view) string {
	n := 0
	for path := range v.Machinery {
		n += v.Unread[path]
	}
	if n == 0 {
		return ""
	}
	words := "1 message for you in an agent’s room"
	if n > 1 {
		words = fmt.Sprintf("%d messages for you in agent rooms", n)
	}
	if v.RoomsLink.Href != "" {
		return fmt.Sprintf(`<a class="conv roomswait unread" href="%s">`+
			`<span class="name">%s</span></a>`, v.RoomsLink.Href, esc(words))
	}
	return fmt.Sprintf(`<p class="rail-note">%s</p>`, esc(words))
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

// roomNote is the honest word over a deep-opened machinery topic: the rail
// does not list it, but a person who was given its path still reads it —
// with whose room it is said plainly, and the way to the agent beside it
// (hq design 0012 §3).
func roomNote(v view) string {
	room, ok := v.Machinery[v.TopicPath]
	if !ok {
		return ""
	}
	if len(room.Agents) == 0 {
		return `<p class="room-note">This is where this deployment places its agents — ` +
			`the record’s own room, not listed with the conversations.</p>`
	}
	name := nameOf(v, room.Agents[0])
	link := ""
	if v.RoomLink.Href != "" {
		link = fmt.Sprintf(` <a href="%s">About %s</a>`, v.RoomLink.Href, esc(name))
	}
	return fmt.Sprintf(`<p class="room-note">This is %s’s own room — its wakes and answers `+
		`land here, so it is not listed with the conversations.%s</p>`, esc(name), link)
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
	b.WriteString(roomNote(v))

	b.WriteString(`<div class="msgs centred">`)
	msgs := soulstream.RenderMessages(v.tv(), v.Topic)
	switch {
	case v.Err != "":
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
	case v.Topic == nil:
		b.WriteString(`<p class="blank">Pick a conversation on the left.</p>`)
	case msgs == "":
		b.WriteString(`<p class="blank">Nothing said here yet — write the first message.</p>`)
	}
	b.WriteString(msgs)
	b.WriteString(`</div></div>`)
	return b.String()
}
