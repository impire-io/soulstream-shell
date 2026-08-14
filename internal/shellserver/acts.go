package shellserver

import (
	"context"

	"github.com/impire-io/soulstream-core/topic"
)

// topicOpenWork posts work.open through the session's realm client —
// the session's connection, the session's persona, the session's
// signature. The shell's own lane never writes.
func topicOpenWork(ctx context.Context, sess *session, path, who string) (string, error) {
	return topic.Open(sess.rc, path).
		OpenWork(ctx, "opened by "+who, "opened from the shell")
}

// topicSay adds a message to a conversation through the session's own
// admitted connection — same rule as every act: the person's persona,
// the person's key. With an anchor it is a reply on that message.
//
// The handle materialises first so the new op parents onto the state the
// person was looking at, and so a reply's anchor is resolvable.
func topicSay(ctx context.Context, sess *session, path, body, anchor string) (string, error) {
	h := topic.Open(sess.rc, path)
	if _, err := h.Materialise(ctx); err != nil {
		return "", err
	}
	if anchor != "" {
		return h.AddComment(ctx, body, anchor)
	}
	return h.PostTurn(ctx, body)
}

// contributionAuthor returns the author of one contribution in a topic,
// and whether the op is there at all — the check that keeps a reply
// anchored to something real.
func contributionAuthor(mt *topic.MaterializedTopic, opID string) (string, bool) {
	if mt == nil {
		return "", false
	}
	for _, c := range mt.Contributions {
		if c.OpID == opID {
			return c.Author, true
		}
	}
	return "", false
}
