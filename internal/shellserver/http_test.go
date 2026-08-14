package shellserver

import (
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/topic"
)

// A fragment that spans lines must reach the browser whole: an SSE field
// ends at the first newline, so each line needs its own data line.
func TestWriteElementsFramesEveryLine(t *testing.T) {
	var b strings.Builder
	writeElements(&b, "<div id=\"x\">\n  <svg />\n</div>", "mode replace")
	want := "event: datastar-patch-elements\n" +
		"data: mode replace\n" +
		"data: elements <div id=\"x\">\n" +
		"data: elements   <svg />\n" +
		"data: elements </div>\n\n"
	if b.String() != want {
		t.Fatalf("frame =\n%q\nwant\n%q", b.String(), want)
	}
}

// The icons ride those frames once a second; each is kept to one line.
func TestIconsAreOneLine(t *testing.T) {
	if len(icons) == 0 {
		t.Fatal("no icons embedded")
	}
	for name, svg := range icons {
		if strings.Contains(string(svg), "\n") {
			t.Errorf("icon %s spans lines: %q", name, svg)
		}
	}
}

// An icon that carries only a viewBox grows to the width of whatever
// holds it, and .btn svg is the only rule that would stop it.
func TestIconsCarryTheirSize(t *testing.T) {
	for name, svg := range icons {
		if !strings.Contains(string(svg), `width="24"`) ||
			!strings.Contains(string(svg), `height="24"`) {
			t.Errorf("icon %s has no intrinsic size: %q", name, svg)
		}
	}
}

// The composer's three pieces are three patch targets, and none of them is
// a target the live stream owns: a one-shot act response and the stream
// must never write the same element, or a half-written message dies on the
// next tick.
func TestComposerTargetsAreDistinct(t *testing.T) {
	page := renderComposer("home/topic")
	for _, id := range []string{`id="composer"`, `id="composer-box"`, `id="composer-note"`, `id="reply-to"`} {
		if strings.Count(page, id) != 1 {
			t.Errorf("composer carries %s %d times, want 1", id, strings.Count(page, id))
		}
	}
	for _, id := range []string{`id="dash"`, `id="conversations"`, `id="result"`} {
		if strings.Contains(page, id) {
			t.Errorf("the composer writes into %s, a target the live stream owns", id)
		}
	}
	if !strings.Contains(page, "contentType:'form'") {
		t.Error("the composer must post itself as form data — it holds no client state")
	}
	if !strings.Contains(page, `class="dock"`) || !strings.Contains(page, composerPrompt) {
		t.Errorf("the composer does not dock under the conversation: %s", page)
	}
}

// convo builds a small conversation: a message from someone else, the
// signed-in person's answer to it, and their own message.
func convo() *topic.MaterializedTopic {
	t0 := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	return &topic.MaterializedTopic{
		Path:         "home/kitchen",
		Announcement: &topic.Announcement{Name: "kitchen table"},
		Contributions: []topic.Contribution{
			{OpID: "op-1", Author: "avery", Timestamp: t0, Type: topic.TypeTurnPost,
				Body: "is the kettle on", Sig: topic.SigVerified},
			{OpID: "op-2", Author: "u-me", Timestamp: t0.Add(time.Minute),
				Type: topic.TypeCommentAdd, Body: "just boiled", Anchor: "op-1",
				Sig: topic.SigVerified},
			{OpID: "op-3", Author: "u-me", Timestamp: t0.Add(2 * time.Minute),
				Type: topic.TypeTurnPost, Body: "mugs are in the cupboard",
				Sig: topic.SigUnsigned},
		},
	}
}

func meView() view {
	return view{
		Me:        "u-me",
		Names:     map[string]string{"u-me": "me", "avery": "Avery"},
		TopicPath: "home/kitchen",
		Topic:     convo(),
		Board: []topic.BoardEntry{
			{Path: "home/attic", Announcement: topic.Announcement{Name: "attic"},
				Lifecycle: topic.Lifecycle("active")},
			{Path: "home/kitchen", Announcement: topic.Announcement{Name: "kitchen table"},
				Lifecycle: topic.Lifecycle("active")},
		},
	}
}

// Whose message it is comes from the session's principal against the
// record's author — never from anything the browser said. The rendered
// side is the whole attribution a reader gets, so it has to be right.
func TestOwnMessagesAreDecidedByTheSessionPrincipal(t *testing.T) {
	got := renderThread(meView())
	for _, want := range []string{
		`<div class="msg" data-op="op-1">`,
		`<div class="msg mine reply" data-op="op-2">`,
		`<div class="msg mine" data-op="op-3">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("thread missing %s:\n%s", want, got)
		}
	}
	// Someone else's message is named; the reader's own is not — the side
	// says it.
	if !strings.Contains(got, `<span class="name">Avery</span>`) {
		t.Errorf("another person's message carries no name:\n%s", got)
	}
	if strings.Contains(got, `<span class="name">me</span>`) {
		t.Errorf("the reader's own message is labelled with their name:\n%s", got)
	}
	// Signed out, nothing is anyone's own.
	anon := meView()
	anon.Me = ""
	if strings.Contains(renderThread(anon), `class="msg mine`) {
		t.Error("a view with no session claims messages as someone's own")
	}
}

// An answer renders under the message it answers, inside it.
func TestAnswersHangOffTheMessageTheyAnswer(t *testing.T) {
	got := renderThread(meView())
	open := strings.Index(got, `<div class="msg" data-op="op-1">`)
	nested := strings.Index(got, `<div class="replies">`)
	answer := strings.Index(got, `data-op="op-2"`)
	sibling := strings.Index(got, `<div class="msg mine" data-op="op-3">`)
	if open < 0 || nested < open || answer < nested || sibling < answer {
		t.Fatalf("the answer does not hang off op-1 (%d/%d/%d/%d):\n%s",
			open, nested, answer, sibling, got)
	}
	// And an answer is not also a root: it appears exactly once.
	if n := strings.Count(got, `data-op="op-2"`); n != 1 {
		t.Errorf("the answer renders %d times, want 1", n)
	}
}

// An answer to an answer joins the message the chain started from, and an
// anchor the topic does not hold is still shown rather than dropped.
func TestTimelineFoldsChainsAndKeepsOrphans(t *testing.T) {
	t0 := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	mt := convo()
	mt.Contributions = append(mt.Contributions,
		topic.Contribution{OpID: "op-4", Author: "avery", Timestamp: t0.Add(3 * time.Minute),
			Type: topic.TypeCommentReply, Body: "thanks", Anchor: "op-2"},
		topic.Contribution{OpID: "op-5", Author: "avery", Timestamp: t0.Add(4 * time.Minute),
			Type: topic.TypeCommentAdd, Body: "about that other thing",
			Anchor: "op-gone", Dangling: true})
	items := timeline(mt)
	if len(items) != 3 {
		t.Fatalf("timeline has %d roots, want 3 (op-1, op-3, the orphan)", len(items))
	}
	if got := len(items[0].Node.Replies); got != 2 {
		t.Fatalf("op-1 carries %d answers, want 2 (the answer and the answer to it)", got)
	}
	if items[2].Node.Msg.OpID != "op-5" {
		t.Errorf("the orphaned answer is not a root of its own: %+v", items[2].Node.Msg)
	}
}

// Work marks sit in the conversation where they happened, in plain words.
func TestWorkMarksSitInTheConversation(t *testing.T) {
	mt := convo()
	mt.WorkItems = []topic.WorkItem{{
		ID: "w-1", Author: "avery", Title: "put the kettle on",
		Status: topic.WorkStatus("open"), Timestamp: mt.Contributions[0].Timestamp,
	}}
	got := renderThread(view{Me: "u-me", Names: map[string]string{"avery": "Avery"}, Topic: mt})
	if !strings.Contains(got, `<div class="sysline">Avery opened work “put the kettle on” · open</div>`) {
		t.Errorf("no work mark in the conversation:\n%s", got)
	}
}

// The rail names the conversations and marks the one that is open.
func TestRailMarksTheOpenConversation(t *testing.T) {
	got := renderRail(meView())
	if !strings.Contains(got, `<a class="conv on" href="/?topic=home%2Fkitchen">`) {
		t.Errorf("the open conversation is not marked:\n%s", got)
	}
	if !strings.Contains(got, `<a class="conv" href="/?topic=home%2Fattic">`) {
		t.Errorf("the other conversation is missing or marked open:\n%s", got)
	}
	if strings.Count(got, `id="conversations"`) != 1 {
		t.Errorf("the rail is not one patch target:\n%s", got)
	}
}

// The rail says so when there is nothing to open, in plain words.
func TestRailSaysWhenThereIsNothing(t *testing.T) {
	if got := renderRail(view{}); !strings.Contains(got, "No conversations yet") {
		t.Errorf("empty rail says nothing:\n%s", got)
	}
	if got := renderThread(view{}); !strings.Contains(got, "Pick a conversation") {
		t.Errorf("empty thread says nothing:\n%s", got)
	}
}

// The verdict on every message is the one the record earned, and it is
// there without shouting.
func TestSignatureVerdictIsShownOnEveryMessage(t *testing.T) {
	got := renderThread(meView())
	if n := strings.Count(got, `class="verdict`); n != 3 {
		t.Fatalf("%d messages carry a verdict, want 3:\n%s", n, got)
	}
	for _, want := range []string{`<span class="verdict ok">verified</span>`,
		`<span class="verdict">unsigned</span>`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing verdict %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, `class="pill ok">verified`) {
		t.Error("the verdict still shouts like a status pill")
	}
}

// The stream's two targets are whole elements the browser can morph by id,
// and they are not each other.
func TestLiveTargetsAreTwoWholeElements(t *testing.T) {
	v := meView()
	rail, thread := renderRail(v), renderThread(v)
	for _, c := range []struct{ frag, open, close string }{
		{rail, `<nav id="conversations" class="rail-list">`, `</nav>`},
		{thread, `<div id="dash" class="thread-body">`, `</div>`},
	} {
		if !strings.HasPrefix(c.frag, c.open) || !strings.HasSuffix(c.frag, c.close) {
			t.Errorf("fragment is not a whole element (%s … %s):\n%s", c.open, c.close, c.frag)
		}
	}
	if strings.Contains(rail, `id="dash"`) || strings.Contains(thread, `id="conversations"`) {
		t.Error("the stream's two targets write into each other")
	}
}

// Every message body reaches the browser escaped: the record carries what
// people typed, and what people type includes angle brackets.
func TestMessageBodiesAreEscaped(t *testing.T) {
	mt := convo()
	mt.Contributions[0].Body = `<script>alert("hi")</script>`
	got := renderThread(view{Me: "u-me", Topic: mt})
	if strings.Contains(got, "<script>") {
		t.Fatalf("a message body reached the page as markup:\n%s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("the body did not survive escaping:\n%s", got)
	}
}

// A persona the directory does not name keeps the id the record carries —
// a voice is never rendered as nothing.
func TestUnknownVoicesKeepTheirRecordedName(t *testing.T) {
	v := view{Names: map[string]string{"avery": "Avery"}}
	for _, c := range []struct{ persona, want string }{
		{"avery", "Avery"}, {"u-f468aecb", "u-f468aecb"}, {"", "unattributed"},
	} {
		if got := nameOf(v, c.persona); got != c.want {
			t.Errorf("nameOf(%q) = %q, want %q", c.persona, got, c.want)
		}
	}
}

// The system-status screen still carries the house readouts in plain words.
func TestStatusScreenKeepsThePlaneReadouts(t *testing.T) {
	got := renderPlanes(view{StreamMsg: 12, StreamMB: 3.25, FoldOK: true, Topic: convo()})
	for _, want := range []string{"Storage", "People &amp; sign-in", "Work", "12 ops"} {
		if !strings.Contains(got, want) {
			t.Errorf("status screen missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "is the kettle on") {
		t.Error("the status screen renders conversation content")
	}
}
