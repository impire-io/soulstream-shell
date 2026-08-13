package helmserver

import (
	"context"

	"github.com/impire-io/soulstream/topic"
)

// topicOpenWork posts work.open through the session's realm client —
// the session's connection, the session's persona, the session's
// signature. The helm's own lane never writes.
func topicOpenWork(ctx context.Context, sess *session, path string) (string, error) {
	return topic.Open(sess.rc, path).
		OpenWork(ctx, "opened by "+sess.Display, "opened from the helm")
}
