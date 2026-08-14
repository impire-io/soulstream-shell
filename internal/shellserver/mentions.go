package shellserver

import (
	"context"
	"fmt"
	"time"

	"github.com/impire-io/soulstream-core/topic"
)

// Mentions: being tapped on the shoulder.
//
// Writing @somebody into a message is the record's own convention, not the
// shell's — the library parses the names out of a body as it is posted,
// writes them onto the message, and drops a slip in each named persona's
// inbox (topic.NotifySubject, one subject per persona). The shell's whole
// part is to notice: each signed-in session follows its own inbox over its
// own admitted connection, and what arrives becomes a mark on three
// surfaces — a count on the spine, a mark on the conversation in the rail,
// and the message itself, standing out where it was said.
//
// The tray is this session's and lives in memory only. Nothing about who
// has read what reaches the record: signing in again reads the tray back
// from the inbox, which is the thing that actually keeps.

// followMentions keeps one session's tray filled from that person's own
// inbox — their notify subject, over their own connection. The shell's read
// lane never reads anybody's mail.
//
// It runs for the life of the session and re-attaches when the inbox is not
// there to follow: a realm provisioned before the inbox stream existed grows
// one when it is next converged, and a session that signed in first should
// not stay deaf for the rest of its life.
func (s *Server) followMentions(ctx context.Context, sess *session) {
	for ctx.Err() == nil {
		// No keyring, on purpose: a slip is a pointer, never a claim. What it
		// points at is re-read from the conversation itself, where every
		// message carries the verdict it earned — so the worst a forged slip
		// can do is raise a mark over a message that says nothing.
		err := topic.FollowInbox(ctx, sess.rc, sess.Persona, nil, func(n topic.Notification) {
			sess.tap(n.Topic, n.OpID)
		})
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// tap records one slip. A slip already tallied is ignored, so an inbox
// replayed after a reconnect never resurrects a message already read.
func (sess *session) tap(path, opID string) {
	if path == "" || opID == "" {
		return
	}
	key := path + "\x00" + opID
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.seen[key] {
		return
	}
	sess.seen[key] = true
	if sess.unread[path] == nil {
		sess.unread[path] = map[string]bool{}
	}
	sess.unread[path][opID] = true
}

// read clears one conversation's marks — looking at a conversation is
// reading what is in it.
func (sess *session) read(path string) {
	if path == "" {
		return
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	delete(sess.unread, path)
}

// standing is how many unread messages each conversation on the board holds.
// A slip pointing somewhere the board does not reach is left out: anyone may
// drop a slip in anyone's inbox, and a mark that opens onto nothing is one
// the person would have no way to clear.
func (sess *session) standing(board []topic.BoardEntry) map[string]int {
	out := map[string]int{}
	if sess == nil {
		return out
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	for _, e := range board {
		if n := len(sess.unread[e.Path]); n > 0 {
			out[e.Path] = n
		}
	}
	return out
}

// unreadTotal is the whole tray as one number.
func unreadTotal(unread map[string]int) int {
	n := 0
	for _, c := range unread {
		n += c
	}
	return n
}

// mentionsMe reports whether a message tapped the signed-in person on the
// shoulder. The record says so itself — the library writes the names it
// parsed out of a body onto the message — so the mark is right whether or
// not the slip ever reached anyone's inbox.
func mentionsMe(v view, c *topic.Contribution) bool {
	if v.Me == "" {
		return false
	}
	for _, m := range c.Mentions {
		if m == v.Me {
			return true
		}
	}
	return false
}

// tallyWords is a count of waiting messages as a person reads it.
func tallyWords(n int) string {
	if n == 1 {
		return "1 message mentions you"
	}
	return fmt.Sprintf("%d messages mention you", n)
}

// mentionTally is the whole tray as one mark on the spine's Conversations
// key. It is its own patch target inside the spine, so the live stream keeps
// it current without morphing the spine around it — an expanded spine
// survives every tick, as it did before there was anything to count.
func mentionTally(n int) string {
	if n <= 0 {
		return `<span id="mentions" class="tally"></span>`
	}
	return fmt.Sprintf(`<span id="mentions" class="tally on" title="%s">%d</span>`,
		esc(tallyWords(n)), n)
}

// unreadMark is one conversation's share of the tray, shown on its row in a
// list of conversations.
func unreadMark(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(`<span class="tally on" title="%s">%d</span>`, esc(tallyWords(n)), n)
}
