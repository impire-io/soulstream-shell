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
	return row + vuMeter(level, label) + readout + storeWay(v)
}

// storeWay is the way from the number into the messages behind it, on a
// build that runs somewhere to go. A meter answers "how much"; the moment
// somebody stops believing it, the question is "which ones" — and this is
// the sentence that takes them there.
//
// Nothing is invented when there is nowhere to point: the readout is a
// readout, exactly as it was before anything could answer.
func storeWay(v view) string {
	if v.Store.Href == "" {
		return ""
	}
	return fmt.Sprintf(`<p class="hint"><a href="%s">Look inside</a> — every message `+
		`the store is keeping, exactly as it was written.</p>`, v.Store.Href)
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

// tile is one glance card: an icon and a label, one value, one quiet
// note. Every tile is the same shape on purpose — the calm of the glance
// is four instruments reading the same way. A tile with somewhere to go
// is the way there whole; the instruments themselves (the level ladder,
// the readout line) stay on the system-status screen.
func tile(href, icon, label, value, note string, warn bool) string {
	cls := "card tile"
	if warn {
		cls += " warn"
	}
	inner := fmt.Sprintf(`<div class="t-head">%s<span>%s</span></div><div class="t-value">%s</div>`,
		shell.Icon(icon), label, value)
	if note != "" {
		inner += fmt.Sprintf(`<div class="t-note">%s</div>`, note)
	}
	if href != "" {
		return fmt.Sprintf(`<a class="%s" href="%s">%s</a>`, cls, href, inner)
	}
	return fmt.Sprintf(`<div class="%s">%s</div>`, cls, inner)
}

// storageTile is the store at a glance. The honesty rules hold in the
// quiet register: an unroofed store is unmeasured and the note says so.
func storageTile(v view) string {
	value := fmt.Sprintf(`%d ops · %s`, v.StreamMsg, esc(shell.SizeWords(v.StreamBytes)))
	note := "keeping · no budget set"
	if v.StreamRoof > 0 {
		note = fmt.Sprintf("keeping · %.0f%% of %s",
			float64(v.StreamBytes)/float64(v.StreamRoof)*100,
			esc(shell.SizeWords(uint64(v.StreamRoof))))
	}
	return tile(v.Store.Href, "cassette-tape", "Storage", value, note, false)
}

// signInTile is the fold at a glance.
func signInTile(v view) string {
	if !v.FoldOK {
		return tile("", "key", "People &amp; sign-in", "unreachable", "passkeys", true)
	}
	return tile("", "key", "People &amp; sign-in", "serving", "passkeys", false)
}

// talkingTile counts the conversations, and how many are going on.
func talkingTile(v view) string {
	if len(v.Board) == 0 {
		return tile("", "messages-square", "Talking", "none yet", "", false)
	}
	rooms := "conversation"
	if len(v.Board) != 1 {
		rooms = "conversations"
	}
	on := 0
	for _, e := range v.Board {
		if e.Lifecycle == topic.Active {
			on++
		}
	}
	note := "all quiet"
	if on > 0 {
		note = fmt.Sprintf("%d going on", on)
	}
	return tile("", "messages-square", "Talking",
		fmt.Sprintf("%d %s", len(v.Board), rooms), note, false)
}

// agentsTile is the way from the front door to an agent of your own — on
// deployments that issue agent credentials at all, which is a declared fact
// read through the support layer: this module knows nothing of the agents
// screen it links to beyond the deployment's own word that it is there.
// Empty roster, the pointer; standing roster, the counts; unreadable, the
// honest word rather than a zero nobody measured.
func agentsTile(v view) string {
	if !v.AgentsOn {
		return ""
	}
	switch {
	case v.AgentsUnread:
		return tile("/agents", "radio", "Agents", "unreadable right now", "", true)
	case v.AgentsNamed == 0:
		return tile("/agents", "radio", "Agents", "none yet", "set one up — it answers for you", false)
	default:
		return tile("/agents", "radio", "Agents",
			fmt.Sprintf("%d named", v.AgentsNamed),
			fmt.Sprintf("%d can get in", v.AgentsIn), false)
	}
}

// renderOverview is the Home screen's body: what the house is doing, and
// the way into every conversation from anywhere. One thing leads per
// section, and the start form waits in the slide-over — creation is a
// deliberate act with a surface of its own, here as on every sheet.
func renderOverview(v view) string {
	var b strings.Builder
	b.WriteString(`<h1>Your soulstream at a glance</h1>`)
	b.WriteString(`<p class="lede">Everything here is read live from your soulstream — ` +
		`the shell keeps none of it.</p>`)
	b.WriteString(firstStepsCard(v.Steps))
	b.WriteString(`<div class="tiles">`)
	b.WriteString(storageTile(v))
	b.WriteString(signInTile(v))
	b.WriteString(talkingTile(v))
	b.WriteString(agentsTile(v))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="sec-head"><h2>Conversations</h2>` +
		`<button type="button" class="btn" data-on:click="$panel = true">` +
		string(shell.Icon("plus")) + `Start a conversation</button></div>`)
	if v.Err != "" {
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
		return b.String()
	}
	if len(v.Board) == 0 {
		b.WriteString(`<p class="blank">No conversations yet. Start one with the key above.</p>`)
		b.WriteString(startOver())
		return b.String()
	}
	var live, archived []int
	for i := len(v.Board) - 1; i >= 0; i-- {
		if v.Board[i].Lifecycle == topic.Archived {
			archived = append(archived, i)
			continue
		}
		live = append(live, i)
	}
	b.WriteString(boardTable(v, live))
	if len(archived) > 0 {
		fmt.Fprintf(&b, `<details class="archfold"><summary>Archived (%d)</summary>`,
			len(archived))
		b.WriteString(boardTable(v, archived))
		b.WriteString(`</details>`)
	}
	b.WriteString(startOver())
	return b.String()
}

// boardTable lists conversations the quiet way: a name that goes there,
// what it is about, and its standing as a lowercase word.
func boardTable(v view, idx []int) string {
	var rows strings.Builder
	for _, i := range idx {
		rows.WriteString(boardRow(v, v.Board[i]))
	}
	return `<div class="tablewrap convs"><table><thead><tr><th>Name</th><th>About</th>` +
		`<th>State</th></tr></thead><tbody>` + rows.String() + `</tbody></table></div>`
}

// boardRow is one conversation on the Home list.
func boardRow(v view, e topic.BoardEntry) string {
	name := e.Announcement.Name
	if name == "" {
		name = e.Path
	}
	return fmt.Sprintf(`<tr><td><a href="/?topic=%s">%s</a>%s</td>`+
		`<td class="what">%s</td><td>%s</td></tr>`,
		qesc(e.Path), esc(name), soulstream.UnreadMark(v.Unread[e.Path]),
		esc(e.Announcement.SubjectMatter), statePill(e.Lifecycle))
}

// statePill is a lifecycle word as a quiet chip — the going-on one is
// the live tone; everything else rests in ink.
func statePill(l topic.Lifecycle) string {
	cls := "pill"
	if l == topic.Active {
		cls = "pill live"
	}
	return fmt.Sprintf(`<span class="%s">%s</span>`, cls, stateWords(l))
}

// stateWords is a conversation's standing in a person's own word — the
// record's vocabulary stays on the record. The details panel says the same
// things in sentences; these are the one-word row forms of the same facts.
// An unknown word arrives as itself: newer records outrank this list.
func stateWords(l topic.Lifecycle) string {
	switch l {
	case topic.Proposed:
		return "new"
	case topic.Active:
		return "going on"
	case topic.Dormant:
		return "quiet"
	case topic.Closed:
		return "closed"
	case topic.Archived:
		return "archived"
	default:
		return esc(string(l))
	}
}

// startOver is where a conversation begins on Home — the same act the
// conversations screen offers, waiting in the sheet's slide-over the way
// creation does on every list screen: the table leads, and starting is a
// deliberate act with a surface of its own. Two surfaces, one act: the
// record does not care which door a conversation came in by.
func startOver() string {
	return shell.SlideOver("Start a conversation",
		`<p class="lede">It opens the moment you name it; the first message brings it to life.</p>`+
			`<form id="convo-start" data-on:submit="@post('/act/conversation-start', {contentType:'form'})">`+
			`<div class="fields">`+
			`<label class="field">Name`+
			`<input name="name" required autocomplete="off" placeholder="what to call it"></label>`+
			`<label class="field">What it’s about`+
			`<input name="about" autocomplete="off" placeholder="one line — optional"></label>`+
			`</div>`+
			`<button class="btn" type="submit">Start</button>`+
			`<div id="convo-start-note" class="note"></div>`+
			`</form>`)
}
