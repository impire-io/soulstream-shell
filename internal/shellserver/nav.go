package shellserver

import (
	"fmt"
	"strings"
)

// The shell's spine: a slim column of icons at the far left of every
// signed-in screen, before the conversations. It holds the few places a
// person can go — the overview and the conversations at the top, the house
// readouts and the way out at the foot — and marks where they already are.
//
// The column expands to icons-and-labels on a page-local signal: no server
// round-trip, no stored preference, and nothing the live stream morphs, so
// an expanded rail survives every tick.
//
// Entries are rows in a table rather than markup written out by hand. A
// navigation entry contributed from somewhere else later is one more row —
// not a new shape to invent.

// The sections a rail entry can point at. The current one renders marked.
const (
	sectionHome   = "home"
	sectionChat   = "chat"
	sectionStatus = "status"
)

// navEntry is one place the rail can take a person.
type navEntry struct {
	Section string
	Icon    string
	Label   string
	Href    string
	// Mark is what the entry carries after its label — the mentions tally
	// on Conversations, nothing on the rest. It is its own patch target, so
	// the live stream keeps it current without morphing the spine around it.
	Mark string
}

// topicQuery carries the open conversation across screens, so coming back
// from the overview lands where the person left.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// navEntries is the rail's content: what sits at the top, what sits at the
// foot. Sign out is not here — it is a form, not a link (see renderIconRail).
func navEntries(topicPath, tally string) (top, foot []navEntry) {
	q := topicQuery(topicPath)
	top = []navEntry{
		{Section: sectionHome, Icon: "home", Label: "Home", Href: "/home" + q},
		{Section: sectionChat, Icon: "messages-square", Label: "Conversations",
			Href: "/" + q, Mark: tally},
	}
	foot = []navEntry{
		{Section: sectionStatus, Icon: "gauge", Label: "System status", Href: "/status" + q},
	}
	return top, foot
}

// navLink is one entry. Collapsed, the label is the hover title and the
// accessible name both — the icon alone names nothing.
func navLink(e navEntry, active string) string {
	cls, current := "ir", ""
	if e.Section == active {
		cls, current = "ir on", ` aria-current="page"`
	}
	return fmt.Sprintf(`<a class="%s" href="%s" title="%s"%s>%s<span class="lbl">%s</span>%s</a>`,
		cls, e.Href, esc(e.Label), current, Icon(e.Icon), esc(e.Label), e.Mark)
}

// renderIconRail is the spine itself, marked for the screen it is on. The
// tally is passed in already rendered: the conversation page serves an empty
// one and lets the live stream fill it a moment later, the way it fills
// every other target on that page.
func renderIconRail(active, topicPath, tally string) string {
	top, foot := navEntries(topicPath, tally)
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
	fmt.Fprintf(&b, `<form method="post" action="/logout"><button type="submit" class="ir"`+
		` title="Sign out">%s<span class="lbl">Sign out</span></button></form>`, Icon("log-out"))
	b.WriteString(`</div></nav>`)
	return b.String()
}

// pageHead is the head every screen shares: the token source, the icon the
// browser tab shows, and (for the screens that stream) Datastar itself.
func pageHead(title string, live bool) string {
	script := ""
	if live {
		script = `<script type="module" src="/assets/datastar.js"></script>`
	}
	return fmt.Sprintf(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title><link rel="stylesheet" href="/assets/tokens.css">
<link rel="icon" href="/favicon.ico" sizes="32x32">
<link rel="icon" href="/assets/favicon.svg" type="image/svg+xml">%s</head>`, esc(title), script)
}
