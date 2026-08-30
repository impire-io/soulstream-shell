package soulstream

import (
	"testing"

	"github.com/impire-io/soulstream-core/topic"
)

// The conversation a screen opens onto is the newest one still live: an
// archived conversation is kept for reading, never landed in unasked.
func TestLastLiveSkipsTheArchived(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []topic.BoardEntry
		want    string
	}{
		{"the newest wins", []topic.BoardEntry{
			{Path: "home/attic", Lifecycle: topic.Active},
			{Path: "home/kitchen", Lifecycle: topic.Active},
		}, "home/kitchen"},
		{"an archived newest is skipped", []topic.BoardEntry{
			{Path: "home/attic", Lifecycle: topic.Dormant},
			{Path: "home/kitchen", Lifecycle: topic.Archived},
		}, "home/attic"},
		{"closed still lands", []topic.BoardEntry{
			{Path: "home/attic", Lifecycle: topic.Closed},
		}, "home/attic"},
		{"all archived is nowhere", []topic.BoardEntry{
			{Path: "home/attic", Lifecycle: topic.Archived},
			{Path: "home/kitchen", Lifecycle: topic.Archived},
		}, ""},
		{"no conversations is nowhere", nil, ""},
	} {
		if got := LastLive(tc.entries, nil); got != tc.want {
			t.Errorf("%s: LastLive = %q, want %q", tc.name, got, tc.want)
		}
	}
}
