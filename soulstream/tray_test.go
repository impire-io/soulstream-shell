package soulstream

import (
	"strings"
	"testing"

	"github.com/impire-io/soulstream-core/topic"
)

// The tray is the session's own: slips fill it, opening a conversation
// empties that conversation's share, and an inbox replayed after a
// reconnect never resurrects a message already read.
func TestTheTrayFillsAndEmptiesOnReading(t *testing.T) {
	sess := &Session{unread: map[string]map[string]bool{}, seen: map[string]bool{}}
	board := []topic.BoardEntry{{Path: "home/attic"}, {Path: "home/kitchen"}}
	sess.tap("home/attic", "op-1")
	sess.tap("home/attic", "op-1") // the same slip twice is one message
	sess.tap("home/attic", "op-2")
	sess.tap("home/kitchen", "op-3")
	if got := sess.Standing(board); got["home/attic"] != 2 || got["home/kitchen"] != 1 {
		t.Fatalf("the tray holds %v, want 2 in the attic and 1 in the kitchen", got)
	}
	if n := Total(sess.Standing(board)); n != 3 {
		t.Errorf("the whole tray counts %d, want 3", n)
	}
	sess.Read("home/attic")
	if got := sess.Standing(board); got["home/attic"] != 0 || got["home/kitchen"] != 1 {
		t.Fatalf("reading the attic left %v", got)
	}
	sess.tap("home/attic", "op-1") // the inbox replays
	if got := sess.Standing(board); got["home/attic"] != 0 {
		t.Errorf("a replayed inbox resurrected a message already read: %v", got)
	}
	// A slip pointing somewhere the board does not reach is not a mark: it
	// would open onto nothing, and nothing would ever clear it.
	sess.tap("home/cellar", "op-9")
	if got := sess.Standing(board); len(got) != 1 {
		t.Errorf("a slip for a conversation off the board became a mark: %v", got)
	}
}

// The mark two screens both put on a conversation row, said once here so it
// cannot come out two different ways.
func TestTheMarkOnAConversation(t *testing.T) {
	if got := UnreadMark(0); got != "" {
		t.Errorf("nothing is waiting and the row is still marked: %s", got)
	}
	if got := UnreadMark(2); got != `<span class="tally on" title="2 messages mention you">2</span>` {
		t.Errorf("the mark does not say how many are waiting: %s", got)
	}
	if !strings.Contains(UnreadMark(1), "1 message mentions you") {
		t.Errorf("one waiting message reads as many: %s", UnreadMark(1))
	}
	// The words are a person's, and never the retired one.
	for _, n := range []int{0, 1, 7} {
		if strings.Contains(strings.ToLower(TallyWords(n)), "realm") {
			t.Errorf("the count says the retired word: %s", TallyWords(n))
		}
	}
}
