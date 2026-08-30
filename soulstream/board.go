package soulstream

import (
	"sort"

	"github.com/impire-io/soulstream-core/topic"
)

// HumanConversations is what a list of conversations shows (hq design 0012
// §3): the machinery — agent homes and the placements topic — left out, and
// what remains ordered by last activity, newest first. Entries the reading
// carries no activity for (the one-shot board of a cold boot) keep the
// board's own order among themselves, which is what every list showed
// before there was an order to have.
func HumanConversations(entries []topic.BoardEntry, machinery map[string]Room) []topic.BoardEntry {
	out := make([]topic.BoardEntry, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		if _, isRoom := machinery[entries[i].Path]; isRoom {
			continue
		}
		out = append(out, entries[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastActivity.After(out[j].LastActivity)
	})
	return out
}

// LastLive is the conversation a screen opens onto when nobody named one:
// the most recently living human conversation — never archived, never the
// machinery's. "" when there is nothing to land in.
func LastLive(entries []topic.BoardEntry, machinery map[string]Room) string {
	for _, e := range HumanConversations(entries, machinery) {
		if e.Lifecycle != topic.Archived {
			return e.Path
		}
	}
	return ""
}
