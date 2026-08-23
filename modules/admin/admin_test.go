package admin

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// routes records what a module claims, without serving any of it.
type routes struct{ patterns []string }

func (rt *routes) Handle(pattern string, _ http.Handler) {
	rt.patterns = append(rt.patterns, pattern)
}

func (rt *routes) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	rt.patterns = append(rt.patterns, pattern)
}

// people is a list as the sign-in surface hands one over: somebody who can
// sign in and has a passkey, and somebody who cannot.
func people() []soulstream.Person {
	return []soulstream.Person{
		{ID: "u-1", Username: "owner", DisplayName: "Daan", Status: "active",
			Groups: []string{"admin", "keeper"}, Credentials: 1},
		{ID: "u-2", Username: "avery", Status: "disabled", Credentials: 0},
	}
}

// The module claims its screen and the two acts offered from it — and
// nothing another surface could be serving.
func TestTheModuleClaimsItsOwnRoutes(t *testing.T) {
	m := New(nil, nil)
	if got := m.Identity(); got.Slug != "admin" || got.Name != "People & sign-in" {
		t.Errorf("the module names itself %+v", got)
	}
	var rt routes
	m.Mount(&rt)
	want := []string{
		"GET /people",
		"GET /people/disable-ask",
		"GET /people/client-remove-ask",
		"POST /act/person-add",
		"POST /act/invite",
		"POST /act/disable",
		"POST /act/enable",
		"POST /act/groups",
		"POST /act/client-add",
		"POST /act/client-delete",
	}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("the module mounts %v, want %v", rt.patterns, want)
	}
}

// Its one key on the spine carries the open conversation, so the way back
// from here lands where the person left. The key's shape is tested here;
// whether it is drawn at all is the session gate's business, proven in the
// e2e — an administrator sees it, anybody else does not.
func TestTheKeyOnTheSpineCarriesTheOpenConversation(t *testing.T) {
	entry := navEntry("home/kitchen")
	if entry.Icon != "users" || entry.Label != "People & sign-in" {
		t.Errorf("the entry reads %+v", entry)
	}
	if !strings.HasSuffix(entry.Href, "?topic=home%2Fkitchen") {
		t.Errorf("the entry drops the open conversation: %q", entry.Href)
	}
	if bare := navEntry(""); bare.Href != "/people" {
		t.Errorf("with no conversation open the entry reads %q", bare.Href)
	}
}

// The screen says what a person needs in plain words, and offers each act
// only where it means something: nothing to take away from somebody who
// already cannot sign in.
func TestTheScreenOffersActsOnlyWhereTheyMeanSomething(t *testing.T) {
	body := renderPeople(view{People: people()})
	for _, want := range []string{
		"People &amp; sign-in", "Sign-in name", "Passkeys",
		"Daan", "avery", `class="pill ok"`, `class="pill warn"`,
		`@post('/act/invite?who=owner')`, `@get('/people/disable-ask?who=owner')`,
		`id="people-table"`, `id="people-result"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen is missing %q", want)
		}
	}
	// Disabling stands behind a question: the row offers the question, never
	// the act itself.
	if strings.Contains(body, "/act/disable") {
		t.Error("the screen offers the disable act without its question")
	}
	if strings.Contains(body, "/people/disable-ask?who=avery") {
		t.Error("the screen offers to take a sign-in away from somebody who has none")
	}
	if !strings.Contains(body, "/act/enable?who=avery") {
		t.Error("somebody shut out is offered no way back in")
	}
	// The plain-language rule: the screen names no component and no byname.
	for _, banned := range []string{"realm", "fold", "idp", "soulstream-idp", "OIDC"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(banned)) {
			t.Errorf("the screen says %q at a person", banned)
		}
	}
}

// Six columns of other people's names and ids do not narrow past a point,
// so the table scrolls inside its own box rather than pushing the screen
// sideways and clipping its last column off the edge of the frame.
func TestTheTableScrollsInsideItsOwnBox(t *testing.T) {
	body := renderPeople(view{People: people()})
	if n := strings.Count(body, "<table"); n != 1 {
		t.Fatalf("the screen serves %d tables, want 1", n)
	}
	if !strings.Contains(body, `<div class="tablewrap"><table>`) {
		t.Errorf("the table is not inside the container that scrolls it:\n%s", body)
	}
	// Before it scrolls, it gives: the keys in the last column stack instead
	// of widening every row, and a sign-in name that has to wrap keeps the
	// whole of itself in the hover.
	if n := strings.Count(body, `<div class="acts">`); n != 2 {
		t.Errorf("the acts stack on %d rows, want one per person (2)", n)
	}
	if !strings.Contains(body, `<td class="mono" title="owner">owner</td>`) {
		t.Errorf("a sign-in name that wraps cannot be read whole:\n%s", body)
	}
}

// The one answer carrying a secret says out loud that it is the only time
// it will ever be shown.
func TestTheInviteIsShownOnceAndSaysSo(t *testing.T) {
	got := renderInvite("avery", soulstream.Invite{
		Token: "sfi_deadbeef", URL: "http://as/enroll?invite=sfi_deadbeef",
	})
	for _, want := range []string{"sfi_deadbeef", "Shown once", "avery",
		`id="people-result"`, "Enrolment link"} {
		if !strings.Contains(got, want) {
			t.Errorf("the invite answer is missing %q:\n%s", want, got)
		}
	}
}

// The one row the screen offers no way to break: the last person who can
// administer sign-ins here. The key is not drawn at all — a key that only
// ever earns a refusal is worse than no key — and the row says what the
// refusal would have said, in the same words. Everything else about that
// person is unchanged, invites included.
func TestTheLastAdministratorIsNotOfferedTheLethalKey(t *testing.T) {
	list := people()
	list[0].LastAdmin = true
	body := renderPeople(view{People: list})
	if strings.Contains(body, "/people/disable-ask?who=owner") {
		t.Errorf("the screen offers to lock the deployment out of itself:\n%s", body)
	}
	if !strings.Contains(body, `<span class="note">the last administrator stays</span>`) {
		t.Errorf("the screen withholds the key without saying why:\n%s", body)
	}
	if !strings.Contains(body, `@post('/act/invite?who=owner')`) {
		t.Errorf("the screen withheld an invite from the person holding the place open:\n%s", body)
	}
	// It is one person's row, not a mood the whole table is in.
	if n := strings.Count(body, "the last administrator stays"); n != 1 {
		t.Errorf("%d rows say the last administrator stays, want 1:\n%s", n, body)
	}
	// And with a second administrator standing, the surface says so and the
	// key comes back: the rule is about the last one, never a particular one.
	if again := renderPeople(view{People: people()}); !strings.Contains(again,
		"/people/disable-ask?who=owner") {
		t.Errorf("the key never comes back:\n%s", again)
	}
}

// A destructive act stands behind a question that says what it changes and
// offers both ways out; answering "keep it" clears the question the same
// way it came in. The add-form lives in the slide-over so the roster leads
// the screen, and its own result line rides inside the panel.
func TestDestructiveActsStandBehindAQuestion(t *testing.T) {
	q := disableConfirm("avery")
	for _, want := range []string{
		"Disable sign-in for avery?", "until you enable them again",
		`@post('/act/disable?who=avery')`, "Yes, disable",
		`@get('/people/disable-ask')`, "Keep it",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("the disable question is missing %q:\n%s", want, q)
		}
	}
	c := clientRemoveConfirm("shell")
	for _, want := range []string{
		"Remove shell?", `@post('/act/client-delete?id=shell')`,
		`@get('/people/client-remove-ask')`,
	} {
		if !strings.Contains(c, want) {
			t.Errorf("the remove question is missing %q:\n%s", want, c)
		}
	}
}

// The screen leads with the roster; the add-form waits in the slide-over
// behind its own key, with a result line of its own beside the fields.
func TestTheAddFormWaitsInTheSlideOver(t *testing.T) {
	body := renderPeople(view{People: people()})
	table := strings.Index(body, `id="people-table"`)
	panel := strings.Index(body, `class="slideover"`)
	if table < 0 || panel < 0 || panel < table {
		t.Fatalf("the roster does not lead the screen (table at %d, panel at %d)", table, panel)
	}
	for _, want := range []string{
		`data-on:click="$panel = true"`, "Add person",
		`id="person-add"`, `id="people-add-note"`, "Add a person",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen is missing %q", want)
		}
	}
}

// A person without the standing is told so, in the surface's own words —
// never shown a fault, and never left guessing.
func TestAStandingRefusalReadsAsOne(t *testing.T) {
	denied := &soulstream.Refusal{Status: http.StatusForbidden,
		Msg: "adminapi: the token carries no admin role"}
	words := refusalWords("Creating an invite for avery", denied)
	if !strings.Contains(words, "administers sign-ins") ||
		!strings.Contains(words, "no admin role") {
		t.Errorf("a standing refusal reads as %q", words)
	}
	broken := &soulstream.Refusal{Status: http.StatusInternalServerError,
		Msg: "store: unreachable"}
	if w := refusalWords("Reading the list of people", broken); strings.Contains(w, "yours does not") {
		t.Errorf("a fault reads as a standing refusal: %q", w)
	}
}

// A rule the sign-in surface holds — the refusal that still arrives when
// somebody else got there first, or another client asked — is passed on in
// the surface's own words. Not "failed", which it did not, and not a
// standing problem, which it is not: the sentence the surface wrote.
func TestARuleRefusalArrivesInTheSurfacesOwnWords(t *testing.T) {
	held := &soulstream.Refusal{Status: http.StatusConflict,
		Msg: "the last administrator stays — add another administrator first"}
	words := refusalWords("Disabling owner", held)
	if !strings.Contains(words, "the last administrator stays — add another administrator first") {
		t.Errorf("the surface's own words did not survive: %q", words)
	}
	for _, invented := range []string{"failed", "yours does not"} {
		if strings.Contains(words, invented) {
			t.Errorf("the screen added %q to a refusal that explained itself: %q", invented, words)
		}
	}
}

// The module builds its own way in. Somewhere else in the product a person
// is named on a screen; what that screen gets back is a path this package
// spelled, carrying the conversation the person was reading, and no words of
// its own — what this place is called is the name it registered under, and
// the frame fills that in.
func TestTheModuleBuildsTheWayIntoItsOwnScreen(t *testing.T) {
	m := New(nil, nil)
	l, ok := m.Link("person", map[string]string{"who": "avery", "topic": "home/kitchen"})
	if !ok {
		t.Fatal("the module refused a link to a person it administers")
	}
	if l.Href != "/people?who=avery&amp;topic=home%2Fkitchen" {
		t.Errorf("the way in reads %q", l.Href)
	}
	if l.Label != "" {
		t.Errorf("the module named the place itself: %q", l.Label)
	}
	if l, ok := m.Link("person", map[string]string{"who": "avery"}); !ok ||
		l.Href != "/people?who=avery" {
		t.Errorf("with no conversation open the way in reads %q", l.Href)
	}
	// What it will not build: a screen it does not have, and a person it was
	// not told the name of. Both come back as nothing to point at.
	for _, c := range []struct {
		what  string
		route string
		who   string
	}{
		{"a kind of screen this module does not have", "invoice", "avery"},
		{"a link to nobody in particular", "person", ""},
	} {
		if l, ok := m.Link(c.route, map[string]string{"who": c.who}); ok {
			t.Errorf("%s resolved to %q", c.what, l.Href)
		}
	}
}

// Somebody who arrived here looking for one person is answered about that
// person: the row marked where the list holds them, and said in words where
// it does not — plenty of voices on the record were never a sign-in, and
// this screen only ever knew about sign-ins.
func TestTheScreenAnswersAboutThePersonSomebodyCameFor(t *testing.T) {
	held := renderPeople(view{People: people(), Who: "avery"})
	if !strings.Contains(held, `Looking up <span class="mono">avery</span>`) {
		t.Errorf("the screen does not say who it was asked about:\n%s", held)
	}
	if n := strings.Count(held, `<tr class="on">`); n != 1 {
		t.Errorf("%d rows are marked, want the one:\n%s", n, held)
	}
	if !strings.Contains(held, `<tr class="on"><td>avery</td>`) {
		t.Errorf("the marked row is not the person's:\n%s", held)
	}
	stranger := renderPeople(view{People: people(), Who: "u-f468aecb"})
	if !strings.Contains(stranger,
		`Nobody who signs in here answers to <span class="mono">u-f468aecb</span>`) {
		t.Errorf("the screen leaves somebody hunting a row that was never there:\n%s", stranger)
	}
	if strings.Contains(stranger, `<tr class="on">`) {
		t.Errorf("a row is marked for somebody who is not in the list:\n%s", stranger)
	}
	// Opened from the spine rather than followed into, the screen is what it
	// always was: no lookup line, no marked row.
	plain := renderPeople(view{People: people()})
	if strings.Contains(plain, "Looking up") || strings.Contains(plain, `<tr class="on">`) {
		t.Errorf("the screen answers a question nobody asked:\n%s", plain)
	}
}

// An empty list is a sentence, not an empty table.
func TestAnEmptyListSaysSo(t *testing.T) {
	if got := renderTable(nil, nil, ""); !strings.Contains(got, "Nobody can sign in here yet") {
		t.Errorf("an empty list renders as %q", got)
	}
}
