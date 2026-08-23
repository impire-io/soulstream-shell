package storage

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/record"
	"github.com/impire-io/soulstream-core/topic"
)

// routes records what a module claims, without serving any of it.
type routes struct{ patterns []string }

func (rt *routes) Handle(pattern string, _ http.Handler) {
	rt.patterns = append(rt.patterns, pattern)
}

func (rt *routes) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	rt.patterns = append(rt.patterns, pattern)
}

// anOp is one well-formed, signed message as the screen would have read it.
func anOp() op {
	return op{
		Seq:     412,
		Stored:  time.Date(2026, 8, 19, 14, 3, 22, 0, time.UTC),
		Subject: topic.OpsSubjectPrefix + "kitchen-x7m2",
		Size:    31,
		Version: "2",
		Rec: record.Record{
			ID:        "8b5d5b2e-6a1f-4a71-9a4e-1f0b0c2d3e4f",
			Author:    "avery",
			Acting:    "u-3f2a",
			Type:      topic.TypeTurnPost,
			Timestamp: time.Date(2026, 8, 19, 14, 3, 21, 0, time.UTC),
			Signature: "c2ln",
			Payload:   []byte(`{"body":"is the kettle on"}`),
		},
		Sig:       topic.SigVerified,
		Binding:   "kitchen-x7m2",
		Canonical: []byte(`{"author":"avery","id":"8b5d5b2e"}`),
		Payload:   []byte(`{"body":"is the kettle on"}`),
	}
}

// The module claims its own screen, its live channel and the one message
// somebody opens — and nothing another surface could be serving.
func TestTheModuleClaimsItsOwnRoutes(t *testing.T) {
	m := New(nil, nil)
	if got := m.Identity(); got.Slug != "storage" || got.Name != "Storage" {
		t.Errorf("the module names itself %+v", got)
	}
	var rt routes
	m.Mount(&rt)
	want := []string{"GET /storage", "GET /storage/tail", "GET /storage/op"}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("the module mounts %v, want %v", rt.patterns, want)
	}
}

// One key, at the foot beside the other readout, carrying the open
// conversation the way every key on the spine does.
func TestTheModuleContributesOneFootKey(t *testing.T) {
	m := New(nil, nil)
	nav := m.Nav(httptest.NewRequest(http.MethodGet, "/storage?topic=home%2Fkitchen", nil))
	if len(nav) != 1 {
		t.Fatalf("the module contributes %d keys, want 1: %+v", len(nav), nav)
	}
	if nav[0].Href != "/storage?topic=home%2Fkitchen" || !nav[0].Foot {
		t.Errorf("the storage key is %+v", nav[0])
	}
	bare := m.Nav(httptest.NewRequest(http.MethodGet, "/storage", nil))
	if bare[0].Href != "/storage" {
		t.Errorf("the key carries a conversation nobody opened: %+v", bare[0])
	}
}

// The way in another screen asks the frame for: the plain one, one already
// looking at part of the subject space, and the two it declines — an unknown
// route, and a pattern that is not one. A declined ask leaves the asking
// screen with nowhere to point, which is what it must say in its own words
// rather than being handed a link that lands on a refusal.
func TestTheWayInResolvesOrDeclines(t *testing.T) {
	m := New(nil, nil)
	if l, ok := m.Link(routeStore, nil); !ok || l.Href != "/storage" {
		t.Errorf("the plain way in is %+v (ok=%v)", l, ok)
	}
	l, ok := m.Link(routeStore, map[string]string{
		"subject": topic.OpsSubjectPrefix + "kitchen-x7m2", "topic": "home/kitchen",
	})
	if !ok {
		t.Fatal("a way into one part of the subject space did not resolve")
	}
	for _, want := range []string{"filter=SOULSTREAM.TOPICS.OPS.kitchen-x7m2", "topic=home%2Fkitchen"} {
		if !strings.Contains(l.Href, want) {
			t.Errorf("the way in is missing %q: %s", want, l.Href)
		}
	}
	if _, ok := m.Link("something-else", nil); ok {
		t.Error("the module answered a route it does not offer")
	}
	if _, ok := m.Link(routeStore, map[string]string{"subject": "SOULSTREAM..OPS"}); ok {
		t.Error("the module built a link around a subject that is not one")
	}
}

// Subject matching is the whole of how a person narrows this screen, and it
// runs here rather than on the server, so it is measured here.
func TestSubjectMatching(t *testing.T) {
	for _, c := range []struct {
		pattern, subject string
		want             bool
	}{
		{"SOULSTREAM.TOPICS.>", "SOULSTREAM.TOPICS.OPS.kitchen", true},
		{"SOULSTREAM.TOPICS.>", "SOULSTREAM.TOPICS", false}, // > needs a rest to match
		{"SOULSTREAM.TOPICS.OPS.*", "SOULSTREAM.TOPICS.OPS.kitchen", true},
		{"SOULSTREAM.TOPICS.OPS.*", "SOULSTREAM.TOPICS.OPS.kitchen.reply", false},
		{"SOULSTREAM.TOPICS.OPS.kitchen", "SOULSTREAM.TOPICS.OPS.kitchen", true},
		{"SOULSTREAM.TOPICS.OPS.kitchen", "SOULSTREAM.TOPICS.OPS.attic", false},
		{"SOULSTREAM.*.OPS.>", "SOULSTREAM.TOPICS.OPS.a.b", true},
		{"SOULSTREAM.TOPICS.INFO.>", "SOULSTREAM.TOPICS.OPS.kitchen", false},
		{">", "SOULSTREAM.TOPICS.OPS.kitchen", true},
		{"SOULSTREAM.TOPICS.OPS.kitchen", "SOULSTREAM.TOPICS.OPS", false},
	} {
		if got := subjectMatches(c.pattern, c.subject); got != c.want {
			t.Errorf("%q against %q = %v, want %v", c.pattern, c.subject, got, c.want)
		}
	}
}

// A pattern that could never match is refused where it was typed, not
// answered with an empty list that reads like a store with nothing in it.
func TestAPatternThatIsNotOneIsRefused(t *testing.T) {
	for _, ok := range []string{"", "SOULSTREAM.>", "SOULSTREAM.TOPICS.OPS.*", "a.b.c"} {
		if err := checkPattern(ok); err != nil {
			t.Errorf("%q was refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"SOULSTREAM..OPS",       // an empty part
		"SOULSTREAM.>.OPS",      // > is only ever last
		"SOULSTREAM.TOP*.OPS",   // * stands for a whole part
		"SOULSTREAM. TOPICS.>",  // a subject has no spaces
		"SOULSTREAM.TOPICS.OP>", // > stands for a whole part
	} {
		if err := checkPattern(bad); err == nil {
			t.Errorf("%q was accepted as a subject pattern", bad)
		}
	}
}

// The value a signature is bound to is derived from the subject alone —
// that is the record's own rule, and this screen is the reader recomputing
// it. A subject outside the rule binds to nothing, and says so.
func TestTheSigningBindingComesOffTheSubject(t *testing.T) {
	for subject, want := range map[string]string{
		topic.OpsSubjectPrefix + "kitchen":   "kitchen",
		topic.InfoSubjectPrefix + "kitchen":  "kitchen",
		topic.NotifySubjectPrefix + "avery":  "avery",
		topic.SvcSubjectPrefix + "DISCOVER":  "DISCOVER",
		"SOMETHING.ELSE.ENTIRELY":            "",
		"SOULSTREAM.TOPICS.SOMETHING.NEW.ID": "",
	} {
		if got := binding(subject); got != want {
			t.Errorf("%s binds to %q, want %q", subject, got, want)
		}
	}
}

// The sentence this screen must never get wrong. Reading rides the person's
// own sign-in, and in this deployment that grants the whole subject space —
// so the screen says what a sign-in may read and never suggests it is
// narrowed to the person reading it.
func TestTheScreenNeverClaimsItIsNarrowedToYou(t *testing.T) {
	got := renderScreen(ask{}, view{Store: stores[0], Msgs: 12, Bytes: 1 << 20,
		First: 1, Last: 12, Ops: []op{anOp()}})
	for _, want := range []string{
		"your own sign-in", "the whole store", "not only the parts about you",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the scope note is missing %q:\n%s", want, got)
		}
	}
	for _, lie := range []string{"your messages", "only you", "only your", "just yours"} {
		if strings.Contains(strings.ToLower(got), lie) {
			t.Errorf("the screen claims a scoping it does not have (%q):\n%s", lie, got)
		}
	}
}

// The screen reads and does nothing else: no act, no delete, and no search
// box standing in for the query layer the protocol declines.
func TestTheScreenOffersNothingButReading(t *testing.T) {
	got := renderScreen(ask{}, view{Store: stores[0], Msgs: 3, First: 1, Last: 3,
		Ops: []op{anOp()}})
	for _, act := range []string{"@post(", "Delete", "delete", "Remove", "Purge"} {
		if strings.Contains(got, act) {
			t.Errorf("the screen offers %q, and it must offer nothing but reading:\n%s", act, got)
		}
	}
	if !strings.Contains(got, "There is no text search") {
		t.Errorf("the screen does not say why there is no search:\n%s", got)
	}
	if strings.Contains(strings.ToLower(got), `name="q"`) ||
		strings.Contains(strings.ToLower(got), "search</") {
		t.Errorf("the screen grew a search box:\n%s", got)
	}
}

// Every honest state the list has, each said differently — an empty store is
// not a filter that matched nothing, and neither is a read the server
// refused.
func TestTheListSaysWhichKindOfNothingItFound(t *testing.T) {
	for what, c := range map[string]struct {
		v    view
		want string
	}{
		"an empty store": {view{Store: stores[0], Empty: true},
			"is empty — nothing has been written here yet"},
		"a filter matching nothing": {view{Store: stores[0], Msgs: 40, First: 1, Last: 40},
			"No message in Conversations matches that subject"},
		"a refused read": {view{Store: stores[0], Err: "Your sign-in is not permitted to read"},
			"not permitted to read"},
		"a pattern that is not one": {view{Store: stores[0], PatternErr: "a subject has no spaces in it"},
			"fix the subject above"},
	} {
		if got := renderList(ask{}, c.v); !strings.Contains(got, c.want) {
			t.Errorf("%s does not say so (%q):\n%s", what, c.want, got)
		}
	}
}

// A read that stopped at its own limit says so. A silent stop would read as
// "that is everything", which on this screen costs somebody an afternoon.
func TestAShortenedReadSaysHowFarItLooked(t *testing.T) {
	got := renderList(ask{}, view{Store: stores[0], Msgs: 9000, First: 1, Last: 9000,
		Examined: scanLimit, Capped: true, Oldest: 8001, Ops: []op{anOp()}})
	for _, want := range []string{
		"Stopped after examining 1000 sequences back from 9000",
		"Nothing older than 8001 was looked at",
		"before=8000",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the shortened read is missing %q:\n%s", want, got)
		}
	}
	whole := renderList(ask{}, view{Store: stores[0], Msgs: 3, First: 1, Last: 3,
		Examined: 3, Oldest: 1, Ops: []op{anOp()}})
	if strings.Contains(whole, "Stopped after examining") {
		t.Errorf("a read that reached the end claims it stopped short:\n%s", whole)
	}
	if strings.Contains(whole, "Older messages") {
		t.Errorf("a read that reached the end offers to go further back:\n%s", whole)
	}
}

// The one subject class no store keeps is named as that, rather than shown
// as an empty store — the difference between "nothing happened" and "nothing
// is kept" is the whole answer somebody came for.
func TestTheServiceLaneIsNamedRatherThanShownEmpty(t *testing.T) {
	a := ask{Filter: topic.SvcSubjectPrefix + ">"}
	got := renderPicker(a, view{Store: stores[0], ServiceLane: true})
	for _, want := range []string{"Nothing is kept on", "answered and gone", "by design"} {
		if !strings.Contains(got, want) {
			t.Errorf("the service lane note is missing %q:\n%s", want, got)
		}
	}
}

// Both stores are offered, each under a name a person reads and the name the
// server answers to — this is the screen where the real name is the point.
func TestBothStoresAreOfferedByBothNames(t *testing.T) {
	got := renderPicker(ask{}, view{Store: stores[0]})
	for _, want := range []string{
		"Conversations", realm.StreamName, "Notifications", realm.NotifyStreamName,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the picker is missing %q:\n%s", want, got)
		}
	}
}

// One message whole: the headers as the wire spells them, the payload as it
// is, the canonical form beside it, and the verdict with the value it was
// measured against.
func TestOneMessageIsShownWhole(t *testing.T) {
	got := renderOp(opView{Store: stores[0], Op: anOp(), Found: true})
	for _, want := range []string{
		"Conversations · sequence 412",
		record.HeaderVersion, record.HeaderAuthor, record.HeaderActing,
		record.HeaderSig, record.HeaderTs,
		"avery", "u-3f2a", "2026-08-19 14:03:22Z",
		"is the kettle on",                         // the payload, as it is
		`{&#34;author&#34;:&#34;avery&#34;`,        // the canonical form, escaped
		`<span class="verdict ok">verified</span>`, // the earned verdict
		"bound to kitchen-x7m2",                    // and what it was measured against
		"not the payload",                          // said out loud, because people get this wrong
		"Unknown key is not a failure",             // and this
		// The wire forms are all still here, resting under folds; the
		// payload and the verdict lead the panel, open.
		`<details class="stow"><summary>Headers</summary>`,
		`<details class="stow"><summary>Signed bytes</summary>`,
		`<h3 class="label">Payload</h3>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the message is missing %q:\n%s", want, got)
		}
	}
	// A header the record does not carry is named as unset rather than left
	// blank, so nobody reads an empty cell as a value.
	bare := anOp()
	bare.Rec.Signature = ""
	bare.Sig, bare.Canonical = topic.SigUnsigned, nil
	if !strings.Contains(renderOp(opView{Store: stores[0], Op: bare, Found: true}), "not set") {
		t.Error("a header the record does not carry is shown as blank")
	}
}

// A subject outside the binding rule has no signing input, and the panel
// says that rather than showing bytes it made up.
func TestASubjectOutsideTheRuleHasNoSigningInput(t *testing.T) {
	o := anOp()
	o.Subject, o.Binding, o.Canonical = "SOMETHING.ELSE", "", nil
	o.CanonErr = "no signing input: this subject is outside the shape the " +
		"binding rule covers, so there is nothing a signature could have been over"
	got := renderOp(opView{Store: stores[0], Op: o, Found: true})
	for _, want := range []string{"no signing input", "outside the binding rule"} {
		if !strings.Contains(got, want) {
			t.Errorf("the panel is missing %q:\n%s", want, got)
		}
	}
}

// A message on a record subject that is not a record is exactly what this
// screen exists to make visible, so it is shown — with no verdict invented
// for it.
func TestSomethingThatIsNotARecordIsShownAsThat(t *testing.T) {
	o := op{Seq: 7, Subject: topic.OpsSubjectPrefix + "kitchen",
		Bad: "Soulstream-Author: missing", Payload: []byte("{}"), Size: 2}
	list := renderList(ask{}, view{Store: stores[0], Msgs: 7, First: 1, Last: 7,
		Ops: []op{o}})
	if !strings.Contains(list, `<span class="pill warn">not a record</span>`) {
		t.Errorf("the list does not mark a message that is not a record:\n%s", list)
	}
	panel := renderOp(opView{Store: stores[0], Op: o, Found: true})
	if !strings.Contains(panel, "Soulstream-Author: missing") {
		t.Errorf("the panel does not say why it would not parse:\n%s", panel)
	}
	for _, invented := range []string{"verdict ok", "verdict warn", "Signed bytes"} {
		if strings.Contains(panel, invented) {
			t.Errorf("a message that is not a record was given %q anyway:\n%s", invented, panel)
		}
	}
}

// A payload cannot break the page. Bytes that are not text, and more bytes
// than a screen should hold, are each described rather than rendered.
func TestAHostilePayloadCannotBreakThePage(t *testing.T) {
	for what, c := range map[string]struct {
		data []byte
		want string
		gone string
	}{
		"bytes that are not text": {[]byte{0xff, 0xfe, 0x00, 0x01},
			"bytes that are not text", "\xff"},
		"more than a screen should hold": {bytes.Repeat([]byte("x"), payloadCap+1),
			"too much to put on a screen", strings.Repeat("x", 200)},
	} {
		o := anOp()
		o.Payload, o.Size = c.data, len(c.data)
		got := renderOp(opView{Store: stores[0], Op: o, Found: true})
		if !strings.Contains(got, c.want) {
			t.Errorf("%s is not described (%q):\n%s", what, c.want, got)
		}
		if strings.Contains(got, c.gone) {
			t.Errorf("%s was rendered anyway:\n%s", what, got[:min(len(got), 400)])
		}
	}
	// And markup in a payload is text on a screen, never markup in a page.
	o := anOp()
	o.Payload = []byte(`{"body":"<script>alert(1)</script>"}`)
	got := renderOp(opView{Store: stores[0], Op: o, Found: true})
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Errorf("a payload's markup reached the page as markup:\n%s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("a payload's markup is not shown as the text it is:\n%s", got)
	}
}

// A sequence the store no longer holds is a rollup, not a fault, and is said
// as one.
func TestAMissingSequenceIsExplainedAsCompaction(t *testing.T) {
	got := renderOp(opView{Store: stores[0],
		Err: refusalWords(stores[0], errMsgNotFound{})})
	if !strings.Contains(got, "Reading Conversations") {
		t.Errorf("an unreadable message does not name the store:\n%s", got)
	}
}

// errMsgNotFound stands in for the client's own not-found error in the one
// place a test needs an error that is not a permission refusal.
type errMsgNotFound struct{}

func (errMsgNotFound) Error() string { return "message not found" }

// A store nobody typed the name of lands somewhere real: a link built by
// hand with a typo in it is not an error about a name nobody meant.
func TestAnUnknownStoreSettlesOnTheFirst(t *testing.T) {
	if got := storeFor("nonsense"); got.Key != stores[0].Key {
		t.Errorf("an unknown store resolved to %+v", got)
	}
	if got := storeFor("notifications"); got.Stream != realm.NotifyStreamName {
		t.Errorf("the inbox store resolved to %+v", got)
	}
}

// Where a person is survives every key they press: the store, the subject
// and the open conversation ride every link on the screen.
func TestWhereYouAreRidesEveryKey(t *testing.T) {
	a := ask{Store: "notifications", Filter: topic.NotifySubjectPrefix + ">",
		Topic: "home/kitchen"}
	q := a.query("follow=1")
	for _, want := range []string{
		"store=notifications", "filter=SOULSTREAM.PERSONA.NOTIFY.%3E",
		"topic=home%2Fkitchen", "follow=1",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("the query is missing %q: %s", want, q)
		}
	}
}
