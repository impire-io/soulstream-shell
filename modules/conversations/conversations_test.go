package conversations

import (
	"fmt"
	"io/fs"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
)

// tokens is the design token source, read from where the shell serves it.
func tokens(t *testing.T) string {
	t.Helper()
	css, err := fs.ReadFile(shell.Assets(), "tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(css)
}

// routes records what a module claims, without serving any of it.
type routes struct{ patterns []string }

func (rt *routes) Handle(pattern string, _ http.Handler) {
	rt.patterns = append(rt.patterns, pattern)
}

func (rt *routes) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	rt.patterns = append(rt.patterns, pattern)
}

// The module claims its own screen, its own channel and its own acts — and
// nothing another surface could be serving.
func TestTheModuleClaimsItsOwnRoutes(t *testing.T) {
	m := New(nil, nil)
	if got := m.Identity(); got.Slug != "conversations" || got.Name != "Conversations" {
		t.Errorf("the module names itself %+v", got)
	}
	var rt routes
	m.Mount(&rt)
	want := []string{"GET /{$}", "GET /live", "POST /act/post-turn",
		"GET /composer/reply", "GET /composer/suggest"}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("the module mounts %v, want %v", rt.patterns, want)
	}
	for _, p := range rt.patterns {
		if strings.Contains(p, "/home") || strings.Contains(p, "/status") ||
			strings.Contains(p, "/login") || strings.Contains(p, "/assets") {
			t.Errorf("the module claims %q, which is not its own", p)
		}
	}
}

// The composer's three pieces are three patch targets, and none of them is
// a target the live stream owns: a one-shot act response and the stream
// must never write the same element, or a half-written message dies on the
// next tick.
func TestComposerTargetsAreDistinct(t *testing.T) {
	page := renderComposer("home/topic")
	for _, id := range []string{`id="composer"`, `id="composer-box"`, `id="composer-note"`,
		`id="reply-to"`, `id="mention-suggest"`, `id="mention-picks"`} {
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
	if !strings.Contains(page, `class="dock centred"`) || !strings.Contains(page, composerPrompt) {
		t.Errorf("the composer does not dock under the conversation: %s", page)
	}
}

// The conversation is held to one reading measure, and the composer is held
// to the same one: a person writes where they read.
func TestTheConversationColumnIsCentred(t *testing.T) {
	thread := renderThread(meView())
	for _, want := range []string{`class="thread-head centred"`, `class="msgs centred"`} {
		if !strings.Contains(thread, want) {
			t.Errorf("the conversation is not held to the measure (%s):\n%s", want, thread)
		}
	}
	if !strings.Contains(renderComposer("home/kitchen"), "centred") {
		t.Error("the composer is not held to the conversation's measure")
	}
	css := tokens(t)
	if !strings.Contains(css, "--chat-max:") ||
		!strings.Contains(css, ".centred{padding-inline:max(") {
		t.Error("the token source does not cap and centre the conversation")
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
		`<div class="msg human" data-op="op-1">`,
		`<div class="msg human mine reply" data-op="op-2">`,
		`<div class="msg human mine" data-op="op-3">`,
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
	if strings.Contains(renderThread(anon), `class="msg human mine`) {
		t.Error("a view with no session claims messages as someone's own")
	}
}

// An answer renders under the message it answers, inside it.
func TestAnswersHangOffTheMessageTheyAnswer(t *testing.T) {
	got := renderThread(meView())
	open := strings.Index(got, `<div class="msg human" data-op="op-1">`)
	nested := strings.Index(got, `<div class="replies">`)
	answer := strings.Index(got, `data-op="op-2"`)
	sibling := strings.Index(got, `<div class="msg human mine" data-op="op-3">`)
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
	if !strings.Contains(got, `<div class="sysline"><span class="strip shell">open</span>`+
		`<span class="what">Avery opened work “put the kettle on”</span></div>`) {
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

// operated is the view with a voice somebody else answers for in it: the
// record's own way of saying a message was not written by a person answering
// for themselves.
func operated() view {
	v := meView()
	mt := v.Topic
	mt.Contributions = append(mt.Contributions, topic.Contribution{
		OpID: "op-4", Author: "scribe", Type: topic.TypeTurnPost,
		Timestamp: mt.Contributions[0].Timestamp.Add(3 * time.Minute),
		Body:      "kettle logged", Sig: topic.SigVerified,
	})
	v.Names["scribe"] = "Scribe"
	v.Voices = map[string]voice{"scribe": {OperatedBy: "u-me"}}
	return v
}

// Every message says which of the two channels it is in, and it says it with
// the card's own edge and the lamp in its byline — never by tinting the
// message. The channel comes off the record's operator claim: a voice that
// answers for itself is on the human channel, a voice somebody else answers
// for is on the machine one.
func TestEveryMessageCarriesItsChannel(t *testing.T) {
	got := renderThread(operated())
	for _, want := range []string{
		`<div class="msg human" data-op="op-1">`,
		`<div class="msg machine" data-op="op-4">`,
		`<span class="led human" title="answers for itself"></span>`,
		`<span class="led machine" title="operated by me"></span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the conversation is missing %q:\n%s", want, got)
		}
	}
	// Every message carries one, including the reader's own — which has no
	// name on it, so the lamp is the only thing that could say.
	if n := strings.Count(got, `class="led `); n != 4 {
		t.Errorf("%d of 4 messages carry a channel lamp:\n%s", n, got)
	}
}

// Whose a message is is carried by which side it sits on and by nothing
// else. No colour anywhere may say it, or the two channels stop being the
// only thing colour means on this surface.
func TestOwnMessagesAreNotColoured(t *testing.T) {
	v := operated()
	if got := renderThread(v); !strings.Contains(got, `<div class="msg human mine" data-op="op-3">`) {
		t.Fatalf("no message is rendered as the reader's own:\n%s", got)
	}
	// A machine-channel message the reader wrote is teal and right: the two
	// say different things and neither implies the other.
	v.Voices["u-me"] = voice{OperatedBy: "avery"}
	both := renderThread(v)
	if !strings.Contains(both, `<div class="msg machine mine" data-op="op-3">`) {
		t.Errorf("channel and side do not compose:\n%s", both)
	}
	if !strings.Contains(both, `<div class="msg human" data-op="op-1">`) {
		t.Errorf("somebody else's channel moved with the reader's:\n%s", both)
	}
	if strings.Contains(tokens(t), ".msg.mine>.bubble{background") {
		t.Error("the reader's own messages are tinted — side is the whole attribution")
	}
}

// A message with the reader's name in it is marked without borrowing a
// channel colour: the mark hardens the card's outline, which is the canon's
// other kind of edge, and the channel edge stays exactly where it was.
func TestTheMentionMarkTakesNoChannelColour(t *testing.T) {
	v := operated()
	v.Topic.Contributions[3].Mentions = []string{"u-me"}
	got := renderThread(v)
	if !strings.Contains(got, `<div class="msg machine mentions" data-op="op-4">`) {
		t.Errorf("a machine-channel message that says your name lost its channel:\n%s", got)
	}
	// The channel moves the card's edge; the mark moves the card's outline.
	// Two custom properties, on purpose, so neither can be read as the other.
	css := tokens(t)
	for _, want := range []string{
		"--chan:var(--channel-human)", ".msg.machine{--chan:var(--channel-machine)}",
		".msg.mentions{--edge:var(--border-strong)}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("the token source does not hold the channel edge (%q)", want)
		}
	}
}

// A name that tapped somebody carries that person's channel, so a word in a
// sentence reads the way the messages that person writes do.
func TestMentionTokensCarryTheChannelOfWhoTheyTapped(t *testing.T) {
	c := &topic.Contribution{OpID: "op-9", Author: "u-me",
		Body: "@Scribe and @Avery, both of you", Mentions: []string{"scribe", "avery"}}
	got := renderBody(operated(), c)
	for _, want := range []string{
		`<span class="mtoken machine">@Scribe</span>`,
		`<span class="mtoken human">@Avery</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the body is missing %q:\n%s", want, got)
		}
	}
}

// The People panel says what the colour was read from, so the claim is on
// the screen rather than implied by a shade of teal.
func TestThePeoplePanelNamesWhoAnswersForAVoice(t *testing.T) {
	got := renderDetails(operated())
	for _, want := range []string{
		`<span class="led machine" title="operated by me"></span>` +
			`<span class="who" title="@scribe">Scribe</span>`,
		"operated by me · 1 message",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the People list is missing %q:\n%s", want, got)
		}
	}
	// A voice that answers for itself says nothing extra — most of them do,
	// and a line saying so on every one would be noise.
	if strings.Contains(got, "operated by me · 3 messages") {
		t.Errorf("a voice that answers for itself is described as operated:\n%s", got)
	}
}

// The picker says which channel a name belongs to before it is written into
// the message: picking a voice somebody else answers for is a different act
// from picking the person themselves.
func TestThePickerSaysWhichChannelANameIs(t *testing.T) {
	got := renderSuggest([]participant{
		{Persona: "avery", Name: "Avery Blake"},
		{Persona: "scribe", Name: "Scribe", OperatedBy: "u-me"},
	})
	for _, want := range []string{
		`<span class="led human" title="answers for itself"></span>` +
			`<span class="who">Avery Blake</span>`,
		`<span class="led machine" title="operated by @u-me"></span>` +
			`<span class="who">Scribe</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the picker is missing %q:\n%s", want, got)
		}
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

// The mark this module hangs on its own key on the spine: its own patch
// target, so the live stream keeps it current without morphing the spine
// around it — an expanded spine survives every tick, as it did before there
// was anything to count.
func TestTheModulesMarkOnTheSpine(t *testing.T) {
	if quiet := spineTally(0); quiet != `<span id="mentions" class="tally"></span>` {
		t.Errorf("nothing is waiting and the mark still counts: %s", quiet)
	}
	loud := spineTally(3)
	if loud != `<span id="mentions" class="tally on" title="3 messages mention you">3</span>` {
		t.Errorf("the mark does not carry the count: %s", loud)
	}
	if !strings.Contains(spineTally(1), "1 message mentions you") {
		t.Errorf("one waiting message reads as many: %s", spineTally(1))
	}
	// It is a target of its own: the stream's other three write the rail,
	// the conversation and the details, and none of them is this.
	for _, id := range []string{`id="dash"`, `id="conversations"`, `id="details"`} {
		if strings.Contains(loud, id) {
			t.Errorf("the mark writes into %s, a target of its own:\n%s", id, loud)
		}
	}
}

// mentioned is the conversation with the reader's name in someone's message.
func mentioned() *topic.MaterializedTopic {
	mt := convo()
	mt.Contributions[0].Mentions = []string{"u-me"}
	return mt
}

// A message that says your name is marked where it was said — and the mark
// comes off the record itself, so it holds whether or not the slip in your
// inbox ever arrived.
func TestAMessageThatSaysYourNameIsMarked(t *testing.T) {
	v := meView()
	v.Topic = mentioned()
	got := renderThread(v)
	if !strings.Contains(got, `<div class="msg human mentions" data-op="op-1">`) {
		t.Errorf("the message that mentions the reader is not marked:\n%s", got)
	}
	if n := strings.Count(got, "mentions"); n != 1 {
		t.Errorf("%d messages are marked, want 1:\n%s", n, got)
	}
	// Somebody else's name is not the reader's.
	other := meView()
	other.Topic = mentioned()
	other.Me = "avery"
	if strings.Contains(renderThread(other), `class="msg human mentions`) {
		t.Error("a mention of someone else is marked as the reader's")
	}
	// Signed out, nobody's name is in anything.
	anon := meView()
	anon.Topic, anon.Me = mentioned(), ""
	if strings.Contains(renderThread(anon), "mentions") {
		t.Error("a view with no session claims a mention as somebody's")
	}
}

// A conversation holding messages with your name in it is marked in the
// list, with its own share of the count.
func TestTheRailMarksConversationsThatWantYou(t *testing.T) {
	v := meView()
	v.Unread = map[string]int{"home/attic": 2}
	rail := renderRail(v)
	if !strings.Contains(rail, `<a class="conv unread" href="/?topic=home%2Fattic">`) {
		t.Errorf("the conversation holding the mentions is not marked:\n%s", rail)
	}
	if !strings.Contains(rail, `<span class="tally on" title="2 messages mention you">2</span>`) {
		t.Errorf("the rail does not say how many are waiting:\n%s", rail)
	}
	if strings.Contains(rail, `class="conv on unread"`) {
		t.Errorf("the open conversation is marked unread:\n%s", rail)
	}
	// Nothing waiting, nothing marked.
	if strings.Contains(renderRail(meView()), "tally") {
		t.Error("an empty tray still marks the rail")
	}
}

// working is the conversation with work on it: one thing waiting, one in
// somebody's hands, one finished.
func working() *topic.MaterializedTopic {
	mt := convo()
	mt.Lifecycle = topic.Active
	t0 := mt.Contributions[0].Timestamp
	mt.WorkItems = []topic.WorkItem{
		{ID: "w-1", Author: "avery", Title: "restock the coffee",
			Status: topic.WorkOpen, Timestamp: t0},
		{ID: "w-2", Author: "u-me", Owner: "avery", Title: "fix the tap",
			Status: topic.WorkClaimed, Timestamp: t0.Add(time.Minute)},
		{ID: "w-3", Author: "avery", Owner: "avery", Title: "wipe the counter",
			Status: topic.WorkDone, Timestamp: t0.Add(2 * time.Minute)},
	}
	mt.Attachments = []topic.Attachment{
		{OpID: "a-1", Author: "avery", Name: "receipt.txt", Size: 2048},
		{OpID: "a-2", Author: "avery", Name: "gone.txt", Size: 10, Removed: true},
	}
	return mt
}

// The word the operator retired. The Go keeps it — realm.Client, the realm
// package, the flag a deployment sets — but nothing a person reads says it.
func TestNothingServedSaysTheRetiredWord(t *testing.T) {
	v := meView()
	v.Topic, v.Unread = working(), map[string]int{"home/attic": 1}
	for what, served := range map[string]string{
		"the rail":         renderRail(v),
		"the conversation": renderThread(v),
		"the details":      renderDetails(v),
		"the composer":     renderComposer("home/kitchen"),
		"the spine mark":   spineTally(1),
	} {
		if strings.Contains(strings.ToLower(served), "realm") {
			t.Errorf("%s says the retired word:\n%s", what, served)
		}
	}
}

// The signed-in person reads their own name, everywhere they appear. The id
// behind it stays reachable — as a tooltip, never as the thing on screen.
func TestTheSignedInPersonIsNamedNotNumbered(t *testing.T) {
	v := meView()
	v.Names["u-me"] = "Daan"
	det := renderDetails(v)
	if !strings.Contains(det, `<span class="who" title="@u-me">Daan</span><span class="you">you</span>`) {
		t.Errorf("the People list does not put the pill on the name:\n%s", det)
	}
	// Everyone else keeps the handle behind their name too — it is the only
	// place the surface says what to type to tap somebody.
	if !strings.Contains(det, `<span class="who" title="@avery">Avery</span>`) {
		t.Errorf("the other person carries no handle:\n%s", det)
	}
}

// The details panel answers who is in the conversation, where it stands and
// what is waiting — all of it read off the record, in plain words.
func TestDetailsPanelReadsTheConversation(t *testing.T) {
	v := meView()
	v.Topic = working()
	got := renderDetails(v)
	for _, want := range []string{
		`<span class="who">Avery</span>`, "1 message",
		`<span class="you">you</span>`, "2 messages",
		"Going on — people are talking here.",
		`Waiting for someone to pick up <span class="what">“restock the coffee”</span>`,
		`<span class="who">Avery</span> is working on <span class="what">“fix the tap”</span>`,
		"1 thing finished", "receipt.txt",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the details panel is missing %q:\n%s", want, got)
		}
	}
	// A finished item is counted, not listed; a withdrawn attachment is gone.
	if strings.Contains(got, "wipe the counter") {
		t.Errorf("a finished item is still listed as waiting:\n%s", got)
	}
	if strings.Contains(got, "gone.txt") {
		t.Errorf("a withdrawn attachment is still listed:\n%s", got)
	}
	// Nothing in here pretends to be a way somewhere. This view resolved no
	// links, which is what the panel is handed by a deployment running
	// nothing else that knows about people — and then a name is text.
	if strings.Contains(got, "<a ") || strings.Contains(got, "<button") {
		t.Errorf("the details panel offers something that does not navigate:\n%s", got)
	}
}

// A name in the panel is a way into whatever else this deployment runs that
// knows about that person — and the same name, as text, where it runs
// nothing. Both arms come off one field: the links the shell resolved for
// this render. This module builds no part of the href and none of the words
// on it; it does not know what answered, or whether anything could.
func TestAPersonInThePanelLeadsWhereThereIsSomewhereToLead(t *testing.T) {
	v := meView()
	v.Names["u-me"] = "Daan"
	v.Lookups = map[string]shell.Link{
		"avery": {Href: "/people?who=avery&amp;topic=home%2Fkitchen", Label: "People & sign-in"},
	}
	got := renderDetails(v)
	if !strings.Contains(got, `<a class="lookup" href="/people?who=avery&amp;topic=home%2Fkitchen"`+
		` title="People &amp; sign-in"><span class="who" title="@avery">Avery</span></a>`) {
		t.Errorf("the name does not lead where the shell said it could:\n%s", got)
	}
	// Nobody else resolved, so nobody else is a link — including the person
	// reading, whose own pill stays where it was.
	if n := strings.Count(got, "<a "); n != 1 {
		t.Errorf("%d names lead somewhere, want the one that resolved:\n%s", n, got)
	}
	if !strings.Contains(got,
		`<span class="who" title="@u-me">Daan</span><span class="you">you</span>`) {
		t.Errorf("the reader's own name did not stay plain:\n%s", got)
	}
	// And when it is the reader who resolves, the pill rides inside the link
	// rather than being left outside it.
	v.Lookups = map[string]shell.Link{"u-me": {Href: "/people?who=u-me", Label: "Who signs in"}}
	if mine := renderDetails(v); !strings.Contains(mine,
		`<a class="lookup" href="/people?who=u-me" title="Who signs in">`+
			`<span class="who" title="@u-me">Daan</span><span class="you">you</span></a>`) {
		t.Errorf("the reader's own name leads somewhere but leaves the pill behind:\n%s", mine)
	}
}

// The person's own claim reads as theirs, and an unowned claim names nobody
// it cannot name.
func TestDetailsPanelNamesWhoHasTheWork(t *testing.T) {
	v := meView()
	for _, c := range []struct{ owner, want string }{
		{"u-me", `<span class="who">You</span> are working on`},
		{"", `<span class="who">Someone</span> is working on`},
	} {
		got := workWords(v, topic.WorkItem{
			ID: "w", Title: "the thing", Status: topic.WorkClaimed, Owner: c.owner,
		})
		if !strings.Contains(got, c.want) {
			t.Errorf("owner %q renders %q, want %q", c.owner, got, c.want)
		}
	}
}

// The panel is the live stream's third target: one whole element, and not
// one of the other two.
func TestDetailsPanelIsOneWholeTarget(t *testing.T) {
	v := meView()
	v.Topic = working()
	got := renderDetails(v)
	if !strings.HasPrefix(got, `<aside id="details" class="details">`) ||
		!strings.HasSuffix(got, "</aside>") {
		t.Errorf("the details panel is not a whole element:\n%s", got)
	}
	if strings.Contains(got, `id="dash"`) || strings.Contains(got, `id="conversations"`) ||
		strings.Contains(got, `id="composer`) {
		t.Errorf("the details panel writes into another target:\n%s", got)
	}
	// With no conversation open it still fills its target, and says so.
	blank := renderDetails(view{})
	if !strings.Contains(blank, `id="details"`) ||
		!strings.Contains(blank, "Open a conversation") {
		t.Errorf("the empty details panel says nothing:\n%s", blank)
	}
}

// room is the people a picker would offer: the reader, one other voice,
// and two who answer to the same name.
func room() []participant {
	return []participant{
		{Persona: "u-me", Name: "Daan", Me: true},
		{Persona: "avery", Name: "Avery Blake"},
		{Persona: "u-19f2", Name: "Sam"},
		{Persona: "u-77c1", Name: "Sam"},
	}
}

// Typing @ offers the room; typing more of a name narrows to it. A person
// is not offered themselves, and any word of a name is a way in.
func TestThePickerOffersTheRoomAndNarrows(t *testing.T) {
	names := func(got []participant) string {
		var out []string
		for _, p := range got {
			out = append(out, p.Persona)
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct{ q, want string }{
		{"", "avery,u-19f2,u-77c1"}, // the room, without the reader in it
		{"av", "avery"},             // the start of a name
		{"bl", "avery"},             // …or of any word of it
		{"AVER", "avery"},           // case is not a way to miss somebody
		{"u-", "u-19f2,u-77c1"},     // the handle is offered too
		{"sam", "u-19f2,u-77c1"},    // two people answer to the same name
		{"da", ""},                  // the reader is never offered
		{"nobody-by-that-name", ""},
	} {
		if got := names(suggestions(room(), c.q)); got != c.want {
			t.Errorf("suggestions(%q) = %q, want %q", c.q, got, c.want)
		}
	}
	// The list is a glance, not a directory.
	var crowd []participant
	for i := range suggestLimit + 4 {
		crowd = append(crowd, participant{Persona: fmt.Sprintf("p-%d", i), Name: "Person"})
	}
	if got := len(suggestions(crowd, "")); got != suggestLimit {
		t.Errorf("the picker offers %d people at once, want %d", got, suggestLimit)
	}
}

// The list is one patch target of its own, empty when it is closed, and it
// never writes into anything the live stream or the composer owns.
func TestTheSuggestionListIsItsOwnClosedTarget(t *testing.T) {
	if got := renderSuggest(nil); got != `<div id="mention-suggest" class="suggest"></div>` {
		t.Errorf("the closed list is not an empty target: %s", got)
	}
	got := renderSuggest(suggestions(room(), "av"))
	for _, want := range []string{
		`<div id="mention-suggest" class="suggest" role="listbox"`,
		`class="sug on" role="option" aria-selected="true"`,
		`data-mention="avery" data-name="Avery Blake"`,
		`data-on:click="mentionPick(el)"`,
		`<span class="who">Avery Blake</span><span class="handle">@avery</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the suggestion list is missing %q:\n%s", want, got)
		}
	}
	// Every row is a button that does not submit the message being written.
	if n := strings.Count(got, `<button type="button"`); n != strings.Count(got, "<button") {
		t.Errorf("a suggestion row would submit the composer:\n%s", got)
	}
	// Exactly one row is the one Enter would take.
	if n := strings.Count(renderSuggest(suggestions(room(), "")), `aria-selected="true"`); n != 1 {
		t.Errorf("%d rows are marked as the obvious pick, want 1", n)
	}
	for _, id := range []string{`id="dash"`, `id="conversations"`, `id="details"`,
		`id="composer-box"`, `id="mention-picks"`} {
		if strings.Contains(got, id) {
			t.Errorf("the suggestion list writes into %s:\n%s", id, got)
		}
	}
}

// Who a message is about is decided against the record, never on the
// browser's word: a pick counts while the body still names them, a name
// typed by hand counts when it can only mean one person, and everything
// else is left to the library's own grammar.
func TestWhoAMessageIsAboutIsDecidedAgainstTheRecord(t *testing.T) {
	for _, c := range []struct {
		name  string
		body  string
		picks []string
		want  []string
	}{
		{"a pick, and the body still names them",
			"@Avery Blake could you look?", []string{"avery"}, []string{"avery"}},
		{"a pick whose name was deleted taps nobody",
			"never mind", []string{"avery"}, nil},
		{"a name typed by hand, unmistakable",
			"@Avery Blake are you about?", nil, []string{"avery"}},
		{"two people answer to it, so it stays as typed",
			"@Sam have you seen this?", nil, nil},
		{"…unless one of them was picked",
			"@Sam have you seen this?", []string{"u-77c1"}, []string{"u-77c1"}},
		{"a name nobody in the room answers to",
			"@Nobody are you there?", nil, nil},
		{"a pick for somebody who is not in the room",
			"@Ghost hello", []string{"u-ghost"}, nil},
		{"the reader may tap themselves",
			"note to @Daan", nil, []string{"u-me"}},
		{"a name is not read out of a longer word",
			"@Sams are plural", []string{"u-77c1"}, nil},
		{"nobody at all", "kettle is on", nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := resolveMentions(c.body, c.picks, room())
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("resolveMentions(%q, %v) = %v, want %v", c.body, c.picks, got, c.want)
			}
		})
	}
}

// The body reaches the page exactly as it was typed, with the names that
// actually tapped somebody marked — and nothing else marked, however much
// it looks like an address.
func TestTheBodyKeepsWhatWasTypedAndMarksWhatTapped(t *testing.T) {
	v := meView()
	v.Names["avery"] = "Avery Blake"
	c := &topic.Contribution{
		OpID: "op-9", Author: "u-me", Body: "@Avery Blake and @Nobody — see <this>",
		Mentions: []string{"avery"},
	}
	got := renderBody(v, c)
	if !strings.Contains(got, `<span class="mtoken human">@Avery Blake</span>`) {
		t.Errorf("the name that tapped somebody is not marked:\n%s", got)
	}
	if strings.Contains(got, `mtoken human">@Nobody`) {
		t.Errorf("a name that tapped nobody is marked as if it had:\n%s", got)
	}
	if !strings.Contains(got, "@Nobody") {
		t.Errorf("the unresolved name was not kept as typed:\n%s", got)
	}
	if strings.Contains(got, "<this>") || !strings.Contains(got, "&lt;this&gt;") {
		t.Errorf("the body escaped its own markup:\n%s", got)
	}
	// The handle is marked too, when that is what was written.
	c.Body = "@avery, then"
	if !strings.Contains(renderBody(v, c), `<span class="mtoken human">@avery</span>`) {
		t.Errorf("the handle form is not marked:\n%s", renderBody(v, c))
	}
	// A message that tapped nobody is untouched text.
	plain := &topic.Contribution{OpID: "op-8", Body: "e@mail.example and @nobody"}
	if got := renderBody(v, plain); got != "e@mail.example and @nobody" {
		t.Errorf("a message with no mentions was rewritten: %q", got)
	}
}

// Typing asks the server, debounced, and only once there is an @ to answer
// — the caret is the browser's business and the list is the server's.
func TestTheComposerAsksTheServerForTheList(t *testing.T) {
	box := composerBox("home/kitchen")
	for _, want := range []string{
		`data-on:input__debounce.150ms=`,
		`mentionQuery(el) === null ? mentionClose() :`,
		`@get('/composer/suggest?topic=home%2Fkitchen&amp;q=' + encodeURIComponent(mentionQuery(el)))`,
	} {
		if !strings.Contains(box, want) {
			t.Errorf("the message box is missing %q:\n%s", want, box)
		}
	}
	// The page-local half is served with the page, and it is the only half.
	for _, want := range []string{"window.mentionQuery", "window.mentionPick",
		"window.mentionClose", `case "ArrowDown"`, `case "Escape"`} {
		if !strings.Contains(mentionScript, want) {
			t.Errorf("the picker's page-local script is missing %q", want)
		}
	}
	if strings.Contains(mentionScript, "fetch(") || strings.Contains(mentionScript, "EventSource") {
		t.Error("the picker reaches the server on its own instead of through Datastar")
	}
}
