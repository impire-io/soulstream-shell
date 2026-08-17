package conversations

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/impire-io/soulstream-core/topic"
)

// The create fold and the lifecycle dock are the acts' own targets: served
// once by the page, written only by act responses, and never by the live
// stream — the same one-writer rule the composer holds.
func TestLifecycleTargetsAreTheActsOwn(t *testing.T) {
	body := chatBody("home/kitchen", false)
	for _, id := range []string{`id="convo-start"`, `id="convo-start-note"`, `id="convo-life"`} {
		if n := strings.Count(body, id); n != 1 {
			t.Errorf("the page carries %s %d times, want 1", id, n)
		}
	}
	v := meView()
	for what, frag := range map[string]string{
		"the rail":         renderRail(v),
		"the conversation": renderThread(v),
		"the details":      renderDetails(v),
	} {
		for _, id := range []string{`id="convo-start"`, `id="convo-start-note"`, `id="convo-life"`} {
			if strings.Contains(frag, id) {
				t.Errorf("%s writes %s, a target the acts own", what, id)
			}
		}
	}
	for _, frag := range []string{startNote("x"), lifeNote("x"), archiveConfirm("home/kitchen")} {
		for _, id := range []string{`id="dash"`, `id="conversations"`, `id="details"`, `id="mentions"`} {
			if strings.Contains(frag, id) {
				t.Errorf("an act response writes %s, a target the stream owns:\n%s", id, frag)
			}
		}
	}
	if !strings.Contains(startFold(), "contentType:'form'") {
		t.Error("the create form does not post itself as form data")
	}
}

// The archived conversations rest under a fold at the foot of the rail —
// still one click away, never gone, and never in the way. The fold's
// toggle survives the stream's morphs by its preserve mark; the server
// serves it open only when the person is looking at an archived
// conversation.
func TestTheRailFoldsTheArchivedAway(t *testing.T) {
	v := meView()
	v.Board = append(v.Board,
		topic.BoardEntry{Path: "home/cellar",
			Announcement: topic.Announcement{Name: "cellar"}, Lifecycle: topic.Archived},
		topic.BoardEntry{Path: "home/shed",
			Announcement: topic.Announcement{Name: "shed"}, Lifecycle: topic.Archived})
	rail := renderRail(v)
	for _, want := range []string{
		`<details id="rail-archived" class="archfold" data-preserve-attr="open">`,
		`<summary>Archived (2)</summary>`,
		`<a class="conv archived" href="/?topic=home%2Fcellar">`,
		`<a class="conv archived" href="/?topic=home%2Fshed">`,
	} {
		if !strings.Contains(rail, want) {
			t.Errorf("the rail is missing %q:\n%s", want, rail)
		}
	}
	// The fold sits inside the rail's one patch target, after the live rows.
	if strings.Index(rail, `id="rail-archived"`) < strings.Index(rail, "home%2Fattic") {
		t.Errorf("the archived fold stands before the live conversations:\n%s", rail)
	}
	if !strings.HasSuffix(rail, "</details></nav>") {
		t.Errorf("the fold is not held inside the rail's own element:\n%s", rail)
	}
	// Looking at an archived conversation, the fold holding it is served
	// open — a person is never shown a closed drawer with themselves in it.
	v.TopicPath = "home/cellar"
	if !strings.Contains(renderRail(v), `data-preserve-attr="open" open>`) {
		t.Errorf("the fold hides the conversation the person is in:\n%s", renderRail(v))
	}
	// Nothing archived, no fold.
	if strings.Contains(renderRail(meView()), "rail-archived") {
		t.Error("an empty fold is served")
	}
}

// The panel offers the next honest act and only that: a live conversation
// can be closed; a closed one can be archived, behind its own ask; an
// archived one is done — and no reopen is offered anywhere, because the
// record has none.
func TestTheNextHonestActPerLifecycle(t *testing.T) {
	at := func(l topic.Lifecycle) string {
		v := meView()
		v.Topic.Lifecycle = l
		return lifecycleActs(v)
	}
	closeAct := `@post('/act/conversation-close?topic=home%2Fkitchen')`
	askAct := `@get('/lifecycle/archive-ask?topic=home%2Fkitchen')`
	for _, l := range []topic.Lifecycle{topic.Proposed, topic.Active, topic.Dormant} {
		got := at(l)
		if !strings.Contains(got, closeAct) || !strings.Contains(got, "Close this conversation") {
			t.Errorf("a %s conversation offers no close:\n%s", l, got)
		}
		if strings.Contains(got, "Archive") {
			t.Errorf("a %s conversation offers the terminal act directly:\n%s", l, got)
		}
	}
	closed := at(topic.Closed)
	if !strings.Contains(closed, askAct) || !strings.Contains(closed, "Archive for good") {
		t.Errorf("a closed conversation offers no archive:\n%s", closed)
	}
	if strings.Contains(closed, "conversation-close") {
		t.Errorf("a closed conversation still offers close:\n%s", closed)
	}
	if got := at(topic.Archived); got != "" {
		t.Errorf("an archived conversation still offers an act:\n%s", got)
	}
	if got := lifecycleActs(view{}); got != "" {
		t.Errorf("no conversation, yet an act is offered:\n%s", got)
	}
	for _, l := range []topic.Lifecycle{topic.Proposed, topic.Active,
		topic.Dormant, topic.Closed, topic.Archived} {
		if strings.Contains(strings.ToLower(at(l)), "reopen") {
			t.Errorf("a %s conversation offers a reopen the record does not have", l)
		}
	}
}

// Archiving stands behind a second step that says what it means — kept
// for reading, closed to writing, no way back — with both ways out of
// the question.
func TestArchiveStandsBehindItsConfirm(t *testing.T) {
	got := archiveConfirm("home/kitchen")
	for _, want := range []string{
		"There is no way back.",
		`@post('/act/conversation-archive?topic=home%2Fkitchen')`, "Yes, archive it",
		`@get('/lifecycle/archive-ask')`, "Keep it as it is",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the ask is missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, `id="convo-life"`); n != 1 {
		t.Errorf("the ask writes %d targets, want its own one", n)
	}
}

// The record can close a conversation and still hand back an error — the
// tidy-up behind the close is best-effort. The words never call a
// standing close a failure.
func TestCloseWordsNeverCallAStandingCloseAFailure(t *testing.T) {
	for _, tc := range []struct {
		name, opID string
		err        error
		want, not  string
	}{
		{"clean", "op-1", nil,
			"Closed — people can still read it, and it can be archived from here.", "Could not"},
		{"standing with a lost tidy-up", "op-1", errors.New("compaction failed"),
			"Closed. The tidy-up behind it did not finish; nothing is lost.", "Could not"},
		{"already archived", "", fmt.Errorf("topic: is archived — %w", topic.ErrTopicArchived),
			"This conversation is already archived.", "Could not"},
		{"refused", "", errors.New("no admission"),
			"Could not close — no admission", "Closed —"},
	} {
		got := closeWords(tc.opID, tc.err)
		if got != tc.want {
			t.Errorf("%s: closeWords = %q, want %q", tc.name, got, tc.want)
		}
		if strings.Contains(got, tc.not) {
			t.Errorf("%s: the words lie: %q", tc.name, got)
		}
	}
}

// Archive's answers stay honest through its half-successes: already done
// is done, a lost final compaction leaves the archive standing and says
// how to finish it.
func TestArchiveWordsAnswerHonestly(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"clean", nil, "Archived — kept for reading, closed to writing."},
		{"already", fmt.Errorf("topic: is already archived — %w", topic.ErrTopicArchived),
			"Already archived — kept for reading."},
		{"lost races", fmt.Errorf("lost 3 races: %w (archive again)", topic.ErrRollupLost),
			"Archived, but the final tidy-up lost a race — archive again to finish it."},
		{"refused", errors.New("no admission"), "Could not finish archiving — no admission"},
	} {
		if got := archiveWords(tc.err); got != tc.want {
			t.Errorf("%s: archiveWords = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// On an archived conversation the composer's place is a quiet note: the
// record would refuse the write, so the surface does not offer it. The
// fold where a conversation begins stays — an archive ends one
// conversation, never the starting of another.
func TestTheComposerYieldsOnAnArchivedConversation(t *testing.T) {
	gone := chatBody("home/kitchen", true)
	if strings.Contains(gone, `id="composer-box"`) {
		t.Errorf("an archived conversation still offers the composer:\n%s", gone)
	}
	if !strings.Contains(gone, "kept for reading. Nothing new can be added.") {
		t.Errorf("the archived dock does not say why there is no composer:\n%s", gone)
	}
	for _, keep := range []string{`id="convo-start"`, `id="convo-life"`} {
		if !strings.Contains(gone, keep) {
			t.Errorf("an archived conversation lost %s:\n%s", keep, gone)
		}
	}
	if !strings.Contains(chatBody("home/kitchen", false), `id="composer-box"`) {
		t.Error("a live conversation has no composer")
	}
}
