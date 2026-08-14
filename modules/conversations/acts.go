package conversations

import (
	"context"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// say adds a message to a conversation through the session's own admitted
// connection — same rule as every act: the person's persona, the person's
// key. With an anchor it is a reply on that message.
//
// The handle materialises first so the new op parents onto the state the
// person was looking at, and so a reply's anchor is resolvable.
//
// mentions is who the picker resolved. The library records the union of
// those and whatever its own grammar reads out of the body, and taps each
// one; the body goes as written either way.
func say(ctx context.Context, sess *soulstream.Session, path, body, anchor string,
	mentions []string,
) (string, error) {
	h := topic.Open(sess.Client(), path)
	if _, err := h.Materialise(ctx); err != nil {
		return "", err
	}
	if anchor != "" {
		return h.AddCommentMentioning(ctx, body, anchor, mentions)
	}
	return h.PostTurnMentioning(ctx, body, mentions)
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
