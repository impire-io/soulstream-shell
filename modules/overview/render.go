package overview

import (
	"fmt"
	"math"
	"strings"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// The house readouts and the overview they sit on: the molded panels, the
// segmented ladder the canon puts in place of every progress bar, and the
// list of conversations this screen opens onto.

// planeCard is one house readout — the same molded panel on the overview
// and on the system-status screen. Everything after the heading is a block
// of the card's body, in the order it is given.
func planeCard(icon, heading string, body ...string) string {
	return fmt.Sprintf(`<div class="card plane"><div class="head">%s<h2>%s</h2></div>%s</div>`,
		shell.Icon(icon), heading, strings.Join(body, ""))
}

// vuSegments is how many lamps the house readout has. Enough that a single
// segment is a fine enough step to watch, few enough that the ladder still
// reads as a ladder rather than as a bar.
const vuSegments = 24

// vuMeter is the segmented ladder the canon puts in place of every progress
// bar. level is a fraction of full scale; a level below zero means the
// instrument has no scale to read against and every lamp stays dark, which
// is the honest thing for an instrument with nothing to measure to do.
//
// The far end of the ladder is the amber and then the red zone. The zone is
// carried on the segment either way and coloured only where the lamps reach
// it, so a ladder at rest is uniformly dark rather than looking lit at the
// top.
func vuMeter(level float64, label string) string {
	lit := 0
	if level > 0 {
		lit = int(math.Round(level * vuSegments))
		if lit < 1 {
			lit = 1 // anything at all in the store lights the first lamp
		}
		if lit > vuSegments {
			lit = vuSegments
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="vu" role="img" aria-label="%s">`, esc(label))
	for i := range vuSegments {
		cls := "seg"
		switch {
		case i >= vuSegments-2:
			cls += " red"
		case i >= vuSegments-6:
			cls += " amber"
		}
		if i < lit {
			cls += " lit"
		}
		fmt.Fprintf(&b, `<span class="%s"></span>`, cls)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// storageCard is the house's own readout: what the store holds, and how far
// up its own scale that reaches.
//
// The scale is the byte roof the store declares for itself. A store
// provisioned without one has no scale, and the meter says so rather than
// inventing a ceiling to look full against — an unroofed store is not 0%
// full, it is unmeasured.
func storageCard(v view) string {
	row := fmt.Sprintf(`<div class="row"><span class="pill ok"><span class="led ok"></span>`+
		`keeping</span><span class="mono">%d ops · %s</span></div>`,
		v.StreamMsg, esc(shell.SizeWords(v.StreamBytes)))
	level, scale, pct := -1.0, "no budget set", ""
	if v.StreamRoof > 0 {
		level = float64(v.StreamBytes) / float64(v.StreamRoof)
		scale = "of " + shell.SizeWords(uint64(v.StreamRoof))
		pct = fmt.Sprintf("%.0f%%", level*100)
	}
	label := "the store declares no budget to measure against"
	if pct != "" {
		label = pct + " of the store's budget used"
	}
	readout := fmt.Sprintf(`<p class="readout"><span class="cap">%s</span>`+
		`<span class="mono">%s</span></p>`, esc(scale), esc(pct))
	return row + vuMeter(level, label) + readout
}

// signInRow is the other readout both screens carry.
func signInRow(v view) string {
	fold := `<span class="pill warn">unreachable</span>`
	if v.FoldOK {
		fold = `<span class="pill ok"><span class="led ok"></span>serving</span>`
	}
	return `<div class="row">` + fold + `<span class="mono">passkeys</span></div>`
}

// renderPlanes is the system-status screen's body — the house readouts
// that used to sit where the conversation now is. The work count is the
// open conversation's own; the details panel beside that conversation says
// the same thing in words.
func renderPlanes(v view) string {
	var b strings.Builder
	b.WriteString(`<div class="planes">`)
	b.WriteString(planeCard("cassette-tape", "Storage", storageCard(v)))
	b.WriteString(planeCard("key", "People &amp; sign-in", signInRow(v)))
	open, claimed := 0, 0
	if v.Topic != nil {
		for _, w := range v.Topic.WorkItems {
			switch w.Status {
			case topic.WorkOpen:
				open++
			case topic.WorkClaimed:
				claimed++
			}
		}
	}
	b.WriteString(planeCard("activity", "Work",
		fmt.Sprintf(`<div class="row"><span class="mono">open %d · claimed %d</span></div>`,
			open, claimed)))
	b.WriteString(`</div>`)
	return b.String()
}

// renderOverview is the Home screen's body: what the house is doing, and
// the way into every conversation from anywhere.
func renderOverview(v view) string {
	var b strings.Builder
	b.WriteString(`<h1>Your soulstream at a glance</h1>`)
	b.WriteString(`<p class="lede">Everything here is read live from your soulstream — ` +
		`the shell keeps none of it.</p>`)
	b.WriteString(`<div class="planes">`)
	b.WriteString(planeCard("cassette-tape", "Storage", storageCard(v)))
	b.WriteString(planeCard("key", "People &amp; sign-in", signInRow(v)))
	rooms := "conversation"
	if len(v.Board) != 1 {
		rooms = "conversations"
	}
	b.WriteString(planeCard("messages-square", "Talking",
		fmt.Sprintf(`<div class="row"><span class="mono">%d %s</span></div>`,
			len(v.Board), rooms)))
	b.WriteString(`</div>`)

	b.WriteString(`<h2 class="section label">Conversations</h2>`)
	if v.Err != "" {
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
		return b.String()
	}
	if len(v.Board) == 0 {
		b.WriteString(`<p class="blank">No conversations yet.</p>`)
		return b.String()
	}
	b.WriteString(`<div class="rows">`)
	for i := len(v.Board) - 1; i >= 0; i-- {
		e := v.Board[i]
		name := e.Announcement.Name
		if name == "" {
			name = e.Path
		}
		fmt.Fprintf(&b, `<a class="row" href="/?topic=%s"><span class="name">%s</span>`+
			`<span class="what">%s</span>%s<span class="state">%s</span></a>`,
			qesc(e.Path), esc(name), esc(e.Announcement.SubjectMatter),
			soulstream.UnreadMark(v.Unread[e.Path]), esc(string(e.Lifecycle)))
	}
	b.WriteString(`</div>`)
	return b.String()
}
