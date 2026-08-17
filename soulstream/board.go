package soulstream

import "github.com/impire-io/soulstream-core/topic"

// LastLive is the conversation a screen opens onto when nobody named one:
// the newest board entry that is not archived. An archived conversation is
// kept for reading, not for landing in — a person still reaches it by
// asking for it. "" when every conversation is archived, or there are none.
func LastLive(entries []topic.BoardEntry) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Lifecycle != topic.Archived {
			return entries[i].Path
		}
	}
	return ""
}
