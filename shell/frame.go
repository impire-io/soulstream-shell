package shell

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// The frame: everything on a screen that is not a module's own.
//
// The head and the token source, the ink housing at the top, the spine of
// sections at the far left, and the sheet a screen that does not stream is
// laid out on. A module renders what goes beside the spine and nothing
// else, so the way to every other screen is in the same place on all of
// them — and so a module never has to know what the others are.

// pageHead is the head every screen shares: the token source, the icon the
// browser tab shows, and (for the screens that stream) the runtime itself.
func (s *Shell) head(title string, live bool) string {
	script := ""
	if live {
		script = `<script type="module" src="/assets/datastar.js"></script>`
	}
	// A screen that names itself is named; the one that does not is the
	// frame's own front door, and carries the frame's name.
	if title == "" {
		title = s.opts.Brand.Strip
	}
	return fmt.Sprintf(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title><link rel="stylesheet" href="/assets/tokens.css">
<link rel="icon" href="/favicon.ico" sizes="32x32">
<link rel="icon" href="/assets/favicon.svg" type="image/svg+xml">%s</head>`,
		Esc(title+" — "+s.opts.Brand.Wordmark), script)
}

// topbar is the ink housing every signed-in screen hangs from. It says the
// person's own name; the id behind it is the tooltip, for the once a year
// somebody needs it.
func (s *Shell) topbar(ctx context.Context, sess *Session) string {
	b := s.opts.Brand
	id := ""
	if sess != nil {
		id = sess.Subject
	}
	return fmt.Sprintf(`<header class="tbar slim"><span class="wordmark">%s</span>`+
		`<span class="strip">%s</span><span class="strip shell">%s</span>`+
		`<span class="spacer"></span><span class="who" title="%s">%s</span>`+
		`<span class="led"></span></header>`,
		Esc(b.Wordmark), Esc(b.Strip), Esc(b.Where),
		Esc(id), Esc(s.screenName(ctx, sess)))
}

// navLink is one entry on the spine. Collapsed, the label is the hover
// title and the accessible name both — the icon alone names nothing.
func navLink(e NavEntry, active string) string {
	cls, current := "ir", ""
	if e.Section == active {
		cls, current = "ir on", ` aria-current="page"`
	}
	return fmt.Sprintf(`<a class="%s" href="%s" title="%s"%s>%s<span class="lbl">%s</span>%s</a>`,
		cls, e.Href, Esc(e.Label), current, Icon(e.Icon), Esc(e.Label), e.Mark)
}

// rail is the spine itself: a slim column of icons at the far left of every
// signed-in screen, marked for the screen it is on.
//
// Every entry on it is contributed — the shell holds no list of places a
// person can go, only the order the modules registered in. The column
// expands to icons-and-labels on a page-local signal: no round-trip, no
// stored preference, and nothing a live stream morphs, so an expanded rail
// survives every tick.
func (s *Shell) rail(r *http.Request, active string) string {
	var top, foot []NavEntry
	for _, m := range s.live {
		for _, e := range m.Nav(r) {
			if e.Foot {
				foot = append(foot, e)
			} else {
				top = append(top, e)
			}
		}
	}
	var b strings.Builder
	b.WriteString(`<nav class="iconrail" aria-label="Sections" data-class:open="$rail">`)
	fmt.Fprintf(&b, `<button type="button" class="ir toggle" title="Show the labels"`+
		` aria-label="Show or hide the labels" data-on:click="$rail = !$rail">`+
		`%s<span class="lbl">Collapse</span></button>`, Icon("chevrons-right"))
	b.WriteString(`<div class="ir-group">`)
	for _, e := range top {
		b.WriteString(navLink(e, active))
	}
	b.WriteString(`</div><div class="ir-group ir-foot">`)
	for _, e := range foot {
		b.WriteString(navLink(e, active))
	}
	// Sign out is the shell's own, and it is a form rather than a link: it
	// is the one thing on the spine that changes something.
	fmt.Fprintf(&b, `<form method="post" action="/logout"><button type="submit" class="ir"`+
		` title="Sign out">%s<span class="lbl">Sign out</span></button></form>`, Icon("log-out"))
	b.WriteString(`</div></nav>`)
	return b.String()
}

// Page is one screen: what the tab says, where on the spine the person is,
// and the module's own markup for the middle of it.
type Page struct {
	// Title is what the tab says, before the product's own name. Empty
	// leaves the frame's own name standing — for the screen a deployment
	// opens on.
	Title string
	// Section is the module's own key for this screen; the rail entry
	// carrying it is the one marked.
	Section string
	// Live loads the streaming runtime, for a screen the module patches
	// over SSE.
	Live bool
	// Init is what the screen asks for the moment it loads — a Datastar
	// expression, "" for a screen that asks for nothing.
	Init string
	// Body is the module's own markup: inside the frame, beside the spine.
	Body string
	// Tail rides after the frame, before the page ends — the page-local
	// script an interaction needs and no server can supply.
	Tail string
}

// Render writes one whole screen: the shell's chrome around a module's
// body.
func (s *Shell) Render(w http.ResponseWriter, r *http.Request, p Page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	init := ""
	if p.Init != "" {
		init = ` data-init="` + p.Init + `"`
	}
	fmt.Fprintf(w, `%s
<body class="chat" data-signals="{rail:false}"%s>
%s
<div class="frame">
%s
%s
</div>%s</body></html>`,
		s.head(p.Title, p.Live), init, s.topbar(r.Context(), s.Session(r)),
		s.rail(r, p.Section), p.Body, p.Tail)
}

// Sheet is the shape every screen that is not a live surface takes: one
// scrolling column beside the spine, with the product's own promise under
// it. A module hands it a body and puts the result in Page.Body.
func (s *Shell) Sheet(body string) string {
	b := s.opts.Brand
	return fmt.Sprintf(`<main class="sheet"><div class="sheet-in">%s
<p class="foot">%s · %s · %s</p>
</div></main>`, body, Esc(b.Wordmark), Esc(b.Strip), Esc(b.Promise))
}

// SignIn is what somebody who is not signed in is shown. It is the shell's
// own screen: sign-in is the shell's own business, and no module is
// reachable before it.
func (s *Shell) SignIn(w http.ResponseWriter, _ *http.Request) {
	b := s.opts.Brand
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `%s
<body><main class="gate">
<div class="gate-head"><span class="led human"></span>
<span class="wordmark">%s</span><span class="strip">%s</span></div>
<div class="card raised"><h1>Sign in</h1>
<p class="lede">%s</p>
<p class="act"><a class="btn" href="/login">%s%s</a></p>
</div><p class="foot">%s · %s · %s</p></main></body></html>`,
		s.head("", false), Esc(b.Wordmark), Esc(b.Strip), Esc(b.SignIn),
		Icon("power"), Esc(b.Action), Esc(b.Wordmark), Esc(b.Strip), Esc(b.Where))
}
