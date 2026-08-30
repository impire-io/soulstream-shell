package soulstream

import (
	"context"
	"fmt"
	"time"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
)

// Mentions: being tapped on the shoulder.
//
// Writing @somebody into a message is the record's own convention — the
// library parses the names out of a body as it is posted, writes them onto
// the message, and drops a slip in each named persona's inbox (one subject
// per persona). Noticing is the surface's part, and it rides the person's
// own admission, which is why it lives here rather than in any one module:
// the conversations a person reads and the overview they glance at both
// show what is waiting for them, and neither owns the other.
//
// The tray is one session's and lives in memory only. Nothing about who has
// read what reaches the record: signing in again reads the tray back from
// the inbox, which is the thing that actually keeps.

// followInbox keeps one session's tray filled from that person's own inbox
// — their notify subject, over their own connection. The surface's read
// lane never reads anybody's mail.
//
// It runs for the life of the session and re-attaches when the inbox is not
// there to follow: a realm provisioned before the inbox stream existed
// grows one when it is next converged, and a session that signed in first
// should not stay deaf for the rest of its life.
func (sess *Session) followInbox(ctx context.Context) {
	for ctx.Err() == nil {
		// No keyring, on purpose: a slip is a pointer, never a claim. What
		// it points at is re-read from the conversation itself, where every
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
func (sess *Session) tap(path, opID string) {
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

// Read clears one conversation's marks — looking at a conversation is
// reading what is in it.
func (sess *Session) Read(path string) {
	if path == "" {
		return
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	delete(sess.unread, path)
}

// Waiting is one conversation's share of the tray by itself — for a row
// that is not a conversation row, like a declared agent's line saying its
// hidden room holds a message for the reader (hq design 0012 §4, bar 4).
// Nil-safe: no session, nothing waiting.
func (sess *Session) Waiting(path string) int {
	if sess == nil || path == "" {
		return 0
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return len(sess.unread[path])
}

// Standing is how many unread messages each conversation on the board
// holds. A slip pointing somewhere the board does not reach is left out:
// anyone may drop a slip in anyone's inbox, and a mark that opens onto
// nothing is one the person would have no way to clear.
func (sess *Session) Standing(board []topic.BoardEntry) map[string]int {
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

// Total is a whole tray as one number.
func Total(unread map[string]int) int {
	n := 0
	for _, c := range unread {
		n += c
	}
	return n
}

// TallyWords is a count of waiting messages as a person reads it.
func TallyWords(n int) string {
	if n == 1 {
		return "1 message mentions you"
	}
	return fmt.Sprintf("%d messages mention you", n)
}

// UnreadMark is one conversation's share of the tray, shown on its row in
// any list of conversations. Two modules put it on their rows, so it is
// rendered once here rather than twice in the same words.
func UnreadMark(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(`<span class="tally on" title="%s">%d</span>`, shell.Esc(TallyWords(n)), n)
}
