package shell

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fixture is a module made of nothing but what the contract asks for. The
// frame is tested against these rather than against the real surfaces on
// purpose: if the shell can seat a module it has never heard of, it is a
// shell — and if these ever have to grow a special case to be seated, that
// is the finding.
type fixture struct {
	slug    string
	on      bool
	entries []NavEntry
	paths   []string

	mounted []string
}

func (f *fixture) Identity() Identity          { return Identity{Slug: f.slug, Name: f.slug} }
func (f *fixture) Active(context.Context) bool { return f.on }
func (f *fixture) Nav(*http.Request) []NavEntry {
	return f.entries
}

func (f *fixture) Mount(rt Router) {
	for _, p := range f.paths {
		f.mounted = append(f.mounted, p)
		rt.HandleFunc(p, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(f.slug))
		})
	}
}

// brand is a product the shell has never heard of either: every word on
// screen has to come from here.
func brand() Brand {
	return Brand{
		Wordmark: "windmark", Strip: "strop", Where: "whereabouts",
		SignIn: "the lede that leads", Action: "press the key",
		Promise: "the promise made",
	}
}

func newTestShell(t *testing.T, mods ...Module) *Shell {
	t.Helper()
	s, err := New(Options{
		Listen: "127.0.0.1:0", Issuer: "http://127.0.0.1:1", ClientName: "test",
		SessionCookie: "test_session", Home: "/", Brand: brand(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Register(mods...)
	s.activate(context.Background(), http.NewServeMux())
	return s
}

// A fragment that spans lines must reach the browser whole: an SSE field
// ends at the first newline, so each line needs its own data line.
func TestWriteElementsFramesEveryLine(t *testing.T) {
	var b strings.Builder
	WriteElements(&b, "<div id=\"x\">\n  <svg />\n</div>", "mode replace")
	want := "event: datastar-patch-elements\n" +
		"data: mode replace\n" +
		"data: elements <div id=\"x\">\n" +
		"data: elements   <svg />\n" +
		"data: elements </div>\n\n"
	if b.String() != want {
		t.Fatalf("frame =\n%q\nwant\n%q", b.String(), want)
	}
}

// An act whose outcome is the browser going somewhere answers as a page
// script: the bundle runs a text/javascript response, and nothing on the
// screen is patched.
func TestScriptAnswersAsJavascript(t *testing.T) {
	rec := httptest.NewRecorder()
	Script(rec, `location.assign("/?topic=home.kitchen")`)
	if ct := rec.Header().Get("Content-Type"); ct != "text/javascript" {
		t.Errorf("a script answer is served as %q", ct)
	}
	if rec.Body.String() != `location.assign("/?topic=home.kitchen")` {
		t.Errorf("the script does not arrive as written: %q", rec.Body.String())
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

// The spine holds the few places a person can go, and every one of them was
// put there by a module: the shell keeps no list of its own. The section a
// person is on is marked, and only that one.
func TestTheSpineIsWhatTheModulesContribute(t *testing.T) {
	house := &fixture{slug: "house", on: true, entries: []NavEntry{
		{Section: "home", Icon: "home", Label: "Home", Href: "/home?topic=home%2Fkitchen"},
		{Section: "status", Icon: "gauge", Label: "System status",
			Href: "/status?topic=home%2Fkitchen", Foot: true},
	}}
	room := &fixture{slug: "room", on: true, entries: []NavEntry{
		{Section: "chat", Icon: "messages-square", Label: "Conversations",
			Href: "/?topic=home%2Fkitchen", Mark: `<span id="mark" class="tally"></span>`},
	}}
	s := newTestShell(t, house, room)
	r := httptest.NewRequest(http.MethodGet, "/?topic=home%2Fkitchen", nil)

	rail := s.rail(r, "chat")
	for _, want := range []string{
		`href="/home?topic=home%2Fkitchen"`, `<span class="lbl">Home</span>`,
		`href="/?topic=home%2Fkitchen"`, `<span class="lbl">Conversations</span>`,
		`href="/status?topic=home%2Fkitchen"`, `<span class="lbl">System status</span>`,
		`action="/logout"`, `<span class="lbl">Sign out</span>`,
	} {
		if !strings.Contains(rail, want) {
			t.Errorf("the spine is missing %s:\n%s", want, rail)
		}
	}
	if n := strings.Count(rail, `class="ir on"`); n != 1 {
		t.Errorf("%d spine entries are marked as where the person is, want 1:\n%s", n, rail)
	}
	if !strings.Contains(rail, `class="ir on" href="/?topic=home%2Fkitchen"`) {
		t.Errorf("the marked entry is not the one the page says it is on:\n%s", rail)
	}
	if !strings.Contains(s.rail(r, "home"), `class="ir on" href="/home?topic=home%2Fkitchen"`) {
		t.Error("another module's section does not mark itself")
	}
	// Contributed at the top, contributed at the foot: the group is the
	// module's to say, and the way out is the shell's own.
	top := strings.Index(rail, `class="ir-group"`)
	foot := strings.Index(rail, `class="ir-group ir-foot"`)
	if i := strings.Index(rail, ">Home<"); i < top || i > foot {
		t.Errorf("the top entry is not in the top group:\n%s", rail)
	}
	if i := strings.Index(rail, ">System status<"); i < foot {
		t.Errorf("the foot entry is not at the foot:\n%s", rail)
	}
	// Expanding is page-local: a signal and a class, no round-trip.
	if !strings.Contains(rail, `data-class:open="$rail"`) ||
		!strings.Contains(rail, `data-on:click="$rail = !$rail"`) {
		t.Errorf("the spine asks the server to expand itself:\n%s", rail)
	}
}

// A module may hang a live mark off its own entry — a count of what is
// waiting, a lamp. It rides in the module's own patch target, so a stream
// keeps it current without morphing the spine around it, and it hangs off
// the entry that contributed it rather than the first one on the rail.
func TestTheSpineCarriesAModulesMark(t *testing.T) {
	quiet := &fixture{slug: "room", on: true, entries: []NavEntry{
		{Section: "home", Icon: "home", Label: "Home", Href: "/home"},
		{Section: "chat", Icon: "messages-square", Label: "Conversations", Href: "/",
			Mark: `<span id="mark" class="tally on" title="3 waiting">3</span>`},
	}}
	s := newTestShell(t, quiet)
	rail := s.rail(httptest.NewRequest(http.MethodGet, "/", nil), "chat")
	if n := strings.Count(rail, `id="mark"`); n != 1 {
		t.Fatalf("the spine carries the mark %d times, want 1:\n%s", n, rail)
	}
	if strings.Index(rail, `id="mark"`) < strings.Index(rail, `>Conversations<`) {
		t.Errorf("the mark hangs off the wrong entry:\n%s", rail)
	}
	if strings.Contains(rail, `>Home<`) &&
		strings.Index(rail, `id="mark"`) < strings.Index(rail, `>Home<`) {
		t.Errorf("the mark landed on the entry before it:\n%s", rail)
	}
}

// A key on the spine may do something as well as go somewhere, and what it
// does is the module's to say. The frame renders the attributes it was
// handed onto that module's own key and reads none of them: it does not
// learn what was pulled out, only that something was.
func TestAModuleHangsItsOwnBehaviourOnItsOwnKey(t *testing.T) {
	room := &fixture{slug: "room", on: true, entries: []NavEntry{
		{Section: "home", Icon: "home", Label: "Home", Href: "/home"},
		{Section: "chat", Icon: "messages-square", Label: "Conversations", Href: "/",
			Attrs: `data-on:click="evt.preventDefault(); $panel = !$panel"`},
	}}
	s := newTestShell(t, room)
	rail := s.rail(httptest.NewRequest(http.MethodGet, "/", nil), "chat")
	want := `<a class="ir on" href="/" title="Conversations" aria-current="page" ` +
		`data-on:click="evt.preventDefault(); $panel = !$panel">`
	if !strings.Contains(rail, want) {
		t.Errorf("the key does not carry what the module hung on it (%s):\n%s", want, rail)
	}
	if n := strings.Count(rail, "$panel = !$panel"); n != 1 {
		t.Errorf("the module's own behaviour reached %d keys, want its own:\n%s", n, rail)
	}
	// And the frame declares the signal that behaviour is written against:
	// the spine's own, and the one a frame too narrow to seat a module's side
	// column beside the content needs.
	rec := httptest.NewRecorder()
	s.Render(rec, httptest.NewRequest(http.MethodGet, "/", nil),
		Page{Section: "chat", Body: "quiet"})
	if !strings.Contains(rec.Body.String(), `data-signals="{rail:false,panel:false}"`) {
		t.Errorf("the frame does not declare its two signals:\n%s", rec.Body.String())
	}
}

// Every screen tells the browser how wide it is. Without this one tag a
// phone renders the whole frame at 980px and scales it down, which is the
// difference between a narrow screen and a small photograph of a wide one —
// and it is the one responsive rule no stylesheet can carry.
func TestEveryScreenTellsTheBrowserHowWideItIs(t *testing.T) {
	const meta = `<meta name="viewport" content="width=device-width, initial-scale=1">`
	s := newTestShell(t, &fixture{slug: "room", on: true, entries: []NavEntry{
		{Section: "chat", Icon: "home", Label: "Conversations", Href: "/"},
	}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range []struct {
		what  string
		serve func(http.ResponseWriter)
	}{
		{"a live screen", func(w http.ResponseWriter) {
			s.Render(w, r, Page{Section: "chat", Live: true, Body: "live"})
		}},
		{"a still screen", func(w http.ResponseWriter) {
			s.Render(w, r, Page{Section: "chat", Body: "still"})
		}},
		{"the sign-in card", func(w http.ResponseWriter) { s.SignIn(w, r) }},
	} {
		rec := httptest.NewRecorder()
		c.serve(rec)
		if !strings.Contains(rec.Body.String(), meta) {
			t.Errorf("%s does not tell the browser how wide it is:\n%s", c.what, rec.Body.String())
		}
	}
}

// A module this deployment does not run is nowhere: no key on the spine,
// and no route — so its paths answer like any other path nobody claimed.
func TestAModuleThisDeploymentDoesNotRunIsNowhere(t *testing.T) {
	off := &fixture{slug: "absent", on: false, paths: []string{"GET /absent"},
		entries: []NavEntry{{Section: "absent", Label: "Absent", Href: "/absent"}}}
	on := &fixture{slug: "present", on: true, paths: []string{"GET /present"},
		entries: []NavEntry{{Section: "present", Label: "Present", Href: "/present"}}}
	s, err := New(Options{
		Listen: "127.0.0.1:0", Issuer: "http://127.0.0.1:1", ClientName: "test",
		SessionCookie: "test_session", Home: "/", Brand: brand(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Register(off, on)
	mux := http.NewServeMux()
	s.activate(context.Background(), mux)

	if len(off.mounted) != 0 {
		t.Errorf("an inactive module mounted %v", off.mounted)
	}
	if len(on.mounted) != 1 {
		t.Errorf("the active module mounted %v, want its one route", on.mounted)
	}
	if len(s.Modules()) != 1 || s.Modules()[0].Identity().Slug != "present" {
		t.Errorf("the shell is running %v", s.Modules())
	}
	rail := s.rail(httptest.NewRequest(http.MethodGet, "/", nil), "")
	if strings.Contains(rail, "Absent") {
		t.Errorf("an inactive module is still on the spine:\n%s", rail)
	}
	if !strings.Contains(rail, "Present") {
		t.Errorf("the active module is not on the spine:\n%s", rail)
	}
	for _, c := range []struct {
		path string
		want int
	}{{"/present", http.StatusOK}, {"/absent", http.StatusNotFound}} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.want {
			t.Errorf("%s = %d, want %d", c.path, rec.Code, c.want)
		}
	}
}

// owner is a fixture that also owns screens other modules may point at: the
// optional half of the contract. It is a separate type on purpose — a plain
// fixture must stay unlinkable, or the check that an unwilling module is
// never linked into would pass for the wrong reason.
type owner struct {
	fixture
	route string
	label string
}

func (o *owner) Link(route string, params map[string]string) (Link, bool) {
	if route != o.route || params["who"] == "" {
		return Link{}, false
	}
	return Link{Href: "/" + o.slug + "?who=" + params["who"], Label: o.label}, true
}

// One module points at another's screen through the frame and never through
// its package: it names who it wants, what kind of screen, and what the
// screen is about. The module that owns the screen builds the link, so the
// asking module spells none of its paths — and the place at the other end
// is named by the module that owns it, or by the name it registered under.
func TestOneModulePointsAtAnothersScreenThroughTheFrame(t *testing.T) {
	people := &owner{fixture: fixture{slug: "people", on: true}, route: "person"}
	s := newTestShell(t, people)

	l, ok := s.Link("people", "person", map[string]string{"who": "avery"})
	if !ok {
		t.Fatal("the deployment runs the module and it still did not answer")
	}
	if l.Href != "/people?who=avery" {
		t.Errorf("the link goes to %q", l.Href)
	}
	if l.Label != "people" {
		t.Errorf("the link is called %q, want the name the module registered under", l.Label)
	}
	named := &owner{fixture: fixture{slug: "people", on: true}, route: "person",
		label: "Who can sign in"}
	if l, _ := newTestShell(t, named).Link("people", "person",
		map[string]string{"who": "avery"}); l.Label != "Who can sign in" {
		t.Errorf("the module's own words for the place were overwritten with %q", l.Label)
	}
}

// hollow answers yes with nowhere to go. The shell hands that to nobody: a
// link is a place, and a module cannot make one out of an empty string by
// agreeing that it has.
type hollow struct{ fixture }

func (h *hollow) Link(string, map[string]string) (Link, bool) {
	return Link{Label: "nowhere in particular"}, true
}

// The five ways an ask comes back empty, all of them the same answer to the
// asking module: there is nowhere to point, render what you render for a
// stranger. A deployment that does not run the module is the one that
// matters — the module is not in the registry, so nothing about it resolves.
func TestALinkResolvesOnlyIntoAModuleThisDeploymentRuns(t *testing.T) {
	absent := &owner{fixture: fixture{slug: "people", on: false}, route: "person"}
	present := &owner{fixture: fixture{slug: "people", on: true}, route: "person"}
	unwilling := &fixture{slug: "quiet", on: true}
	nowhere := &hollow{fixture: fixture{slug: "hollow", on: true}}

	for _, c := range []struct {
		what             string
		mod              Module
		slug, route, who string
	}{
		{"a module this deployment does not run", absent, "people", "person", "avery"},
		{"a module nobody registered", present, "strangers", "person", "avery"},
		{"a module that accepts no links", unwilling, "quiet", "person", "avery"},
		{"a route the module does not offer", present, "people", "invoice", "avery"},
		{"params the module cannot use", present, "people", "person", ""},
		{"a module that answers with nowhere to go", nowhere, "hollow", "person", "avery"},
	} {
		s := newTestShell(t, c.mod)
		if l, ok := s.Link(c.slug, c.route, map[string]string{"who": c.who}); ok {
			t.Errorf("%s resolved to %q", c.what, l.Href)
		}
	}
}

// The frame says nothing about the product it frames that the product did
// not hand it. Every word on the sign-in card, the top bar and the foot of
// a sheet is composed in — which is what lets the same frame carry
// something else entirely.
func TestTheFrameSaysOnlyTheWordsItWasGiven(t *testing.T) {
	s := newTestShell(t)
	rec := httptest.NewRecorder()
	s.SignIn(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	gate := rec.Body.String()
	for _, want := range []string{
		"<title>strop — windmark</title>", `<span class="wordmark">windmark</span>`,
		`<span class="strip">strop</span>`, "the lede that leads", "press the key",
		`<p class="foot">windmark · strop · whereabouts</p>`, `href="/login"`,
	} {
		if !strings.Contains(gate, want) {
			t.Errorf("the sign-in card is missing %q:\n%s", want, gate)
		}
	}
	sess := &Session{Subject: "u-f468aecb", Name: "Daan"}
	bar := s.topbar(context.Background(), sess)
	if !strings.Contains(bar, `<span class="strip shell">whereabouts</span>`) {
		t.Errorf("the top bar does not say where this is:\n%s", bar)
	}
	if !strings.Contains(s.Sheet("body"), `<p class="foot">windmark · strop · the promise made</p>`) {
		t.Errorf("the sheet does not carry the promise: %s", s.Sheet("body"))
	}
	// Nothing of the frame's own vocabulary reaches the screen.
	for _, said := range []string{"module", "shell —", "Soulstream", "soulstream"} {
		if strings.Contains(gate+bar, said) {
			t.Errorf("the frame says %q, a word of its own:\n%s", said, gate+bar)
		}
	}
}

// The signed-in person reads their own name. The id behind it stays
// reachable — as a tooltip, never as the thing on screen.
func TestTheSignedInPersonIsNamedNotNumbered(t *testing.T) {
	s := newTestShell(t)
	bar := s.topbar(context.Background(), &Session{Subject: "u-f468aecb", Name: "Daan"})
	if !strings.Contains(bar, `<span class="who" title="u-f468aecb">Daan</span>`) {
		t.Errorf("the top bar does not say the person's name:\n%s", bar)
	}
}

// namer is an attachment that knows what to call somebody — the shape a
// support layer takes when the issuer mints an id and the name people know
// each other by lives somewhere else.
type namer struct{ name string }

func (n namer) ScreenName(context.Context) string { return n.name }
func (n namer) SignedIn(context.Context, *Session) (any, error) {
	return n, nil
}
func (n namer) SignedOut(any) {}

// What the issuer said wins; where the issuer said nothing, whatever hangs
// off the session may answer; and failing both, the id stands in. The shell
// never has to know where the answer came from.
func TestWhoTheFrameCallsSomebody(t *testing.T) {
	s := newTestShell(t)
	s.Attach(struct{}{}, namer{name: "from the directory"})
	for _, c := range []struct{ issuer, attached, want string }{
		{"from the issuer", "from the directory", "from the issuer"},
		{"", "from the directory", "from the directory"},
		{"", "", "u-f468aecb"},
	} {
		sess := &Session{Subject: "u-f468aecb", Name: c.issuer,
			attached: map[any]any{struct{}{}: namer{name: c.attached}}}
		if got := s.screenName(context.Background(), sess); got != c.want {
			t.Errorf("issuer %q, attachment %q → %q, want %q",
				c.issuer, c.attached, got, c.want)
		}
	}
}

// A screen is the module's body inside the shell's frame — and the module's
// markup reaches the browser untouched.
func TestAScreenIsAModulesBodyInTheFrame(t *testing.T) {
	room := &fixture{slug: "room", on: true, entries: []NavEntry{
		{Section: "chat", Icon: "home", Label: "Conversations", Href: "/"},
	}}
	s := newTestShell(t, room)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	s.Render(rec, r, Page{
		Title: "a screen", Section: "chat", Live: true,
		Init: "@get('/live')", Body: `<main id="body">what the module said</main>`,
		Tail: "\n<script>page.local()</script>\n",
	})
	page := rec.Body.String()
	for _, want := range []string{
		"<title>a screen — windmark</title>",
		`<script type="module" src="/assets/datastar.js"></script>`,
		`<body class="chat" data-signals="{rail:false,panel:false}" data-init="@get('/live')">`,
		`<header class="tbar slim">`, `<nav class="iconrail"`,
		`class="ir on" href="/"`, `<main id="body">what the module said</main>`,
		"</div>\n<script>page.local()</script>\n</body></html>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the screen is missing %q:\n%s", want, page)
		}
	}
	// The frame comes first and the body sits inside it, once.
	if strings.Index(page, `<nav class="iconrail"`) > strings.Index(page, `id="body"`) {
		t.Errorf("the body is rendered before the spine:\n%s", page)
	}
	if n := strings.Count(page, "what the module said"); n != 1 {
		t.Errorf("the module's markup appears %d times, want 1", n)
	}
	// A screen that does not stream loads no runtime and asks for nothing.
	rec = httptest.NewRecorder()
	s.Render(rec, r, Page{Section: "chat", Body: "quiet"})
	if quiet := rec.Body.String(); strings.Contains(quiet, "datastar.js") ||
		strings.Contains(quiet, "data-init") {
		t.Errorf("a still screen loads the streaming runtime:\n%s", quiet)
	}
}

// TestRedirectBase: the OAuth callback is built from the origin a
// browser can actually reach — the advertised PublicURL when the
// deployment fronts the listener, the bound address otherwise. A
// callback aimed at the bound address behind a front dead-ends in the
// visitor's own machine (found live: the first fronted deployment's
// sign-in bounced to the visitor's 127.0.0.1).
func TestRedirectBase(t *testing.T) {
	if got := redirectBase(Options{}, "127.0.0.1:8500"); got != "http://127.0.0.1:8500" {
		t.Fatalf("bundle default: %q", got)
	}
	if got := redirectBase(Options{PublicURL: "https://shell.example:8443"}, "127.0.0.1:8500"); got != "https://shell.example:8443" {
		t.Fatalf("fronted: %q", got)
	}
	if got := redirectBase(Options{PublicURL: "https://shell.example/"}, "127.0.0.1:8500"); got != "https://shell.example" {
		t.Fatalf("trailing slash not trimmed: %q", got)
	}
}
