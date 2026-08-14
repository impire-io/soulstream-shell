package conversations

import (
	"fmt"
	"strings"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
)

// The details panel: the column beside the conversation that answers the
// three questions a person asks about a room they are standing in — who is
// in here, where does this stand, and what is anyone waiting for.
//
// Every answer is derived server-side from the record itself. Nothing here
// is stored, nothing is taken on the browser's word, and nothing is a link
// unless it actually goes somewhere.
//
// It is the live stream's third patch target (#details), so it keeps up
// with the conversation without anyone reloading.

// participant is one voice in the conversation and what they did in it.
type participant struct {
	Persona string
	Name    string
	Me      bool
	Said    int
	// OperatedBy is the persona the directory says answers for this one, ""
	// when it answers for itself — the claim the channel colours are read
	// from, and the one this panel says out loud (see channel.go).
	OperatedBy string
}

// participants is who is in this conversation, in the order they first
// appear on the record: everyone who said something, plus anyone who opened
// or took on work without saying a word.
func participants(v view) []participant {
	if v.Topic == nil {
		return nil
	}
	var people []participant
	at := map[string]int{}
	add := func(persona string) int {
		if persona == "" {
			return -1
		}
		if i, ok := at[persona]; ok {
			return i
		}
		at[persona] = len(people)
		people = append(people, participant{
			Persona: persona, Name: nameOf(v, persona), Me: mine(v, persona),
			OperatedBy: v.Voices[persona].OperatedBy,
		})
		return at[persona]
	}
	for _, c := range v.Topic.Contributions {
		if i := add(c.Author); i >= 0 {
			people[i].Said++
		}
	}
	for _, w := range v.Topic.WorkItems {
		add(w.Author)
		add(w.Owner)
	}
	return people
}

// answersFor is who answers for a voice, said in front of what that voice
// has done here — or nothing at all for a voice that answers for itself,
// which is most of them and needs no line of its own.
func answersFor(v view, p participant) string {
	if p.OperatedBy == "" {
		return ""
	}
	return esc("operated by "+nameOf(v, p.OperatedBy)) + " · "
}

// saidWords is a participant's part in the conversation so far, counted
// rather than adjectival.
func saidWords(p participant) string {
	switch p.Said {
	case 0:
		return "nothing said yet"
	case 1:
		return "1 message"
	default:
		return fmt.Sprintf("%d messages", p.Said)
	}
}

// lifecycleWords says where the conversation stands in the words a person
// would use, not the vocabulary's own.
func lifecycleWords(l topic.Lifecycle) string {
	switch l {
	case topic.Proposed:
		return "Proposed — nobody has said anything yet."
	case topic.Active:
		return "Going on — people are talking here."
	case topic.Dormant:
		return "Quiet — nothing said here for a while."
	case topic.Closed:
		return "Closed — nothing more is being added."
	case topic.Archived:
		return "Archived — kept for reading, closed to writing."
	case "":
		return "Not known yet."
	default:
		return esc(string(l))
	}
}

// workWords is one work item in plain words: what is waiting for a person,
// and what somebody already has in hand.
func workWords(v view, w topic.WorkItem) string {
	title := w.Title
	if title == "" {
		title = "an unnamed piece of work"
	}
	switch w.Status {
	case topic.WorkOpen:
		return fmt.Sprintf(`<li class="w waiting">Waiting for someone to pick up `+
			`<span class="what">“%s”</span></li>`, esc(title))
	case topic.WorkClaimed:
		who, verb := "Someone", "is working on"
		if w.Owner != "" {
			who = nameOf(v, w.Owner)
			if mine(v, w.Owner) {
				who, verb = "You", "are working on"
			}
		}
		return fmt.Sprintf(`<li class="w doing"><span class="who">%s</span> %s `+
			`<span class="what">“%s”</span></li>`, esc(who), verb, esc(title))
	default:
		return ""
	}
}

// personName is a name in the People panel, and the one place this screen
// reaches into another screen.
//
// The name is what a person reads; the id behind it is the tooltip, written
// the way it would be typed. It is the only place the surface says how to
// tap somebody on the shoulder, because @-mentions only resolve against the
// id the record carries — never a display name.
//
// Where this deployment runs something that knows more about that person,
// the name is a link into it, and the tooltip on the link is whatever that
// place calls itself — words from a module this one cannot see. Where it
// does not, the name is exactly the text it always was: no dead link, no
// greyed-out control, nothing pretending there is a screen behind it.
func personName(v view, p participant) string {
	youMark := ""
	if p.Me {
		youMark = `<span class="you">you</span>`
	}
	name := fmt.Sprintf(`<span class="who" title="@%s">%s</span>%s`,
		esc(p.Persona), esc(p.Name), youMark)
	l, ok := v.Lookups[p.Persona]
	if !ok {
		return name
	}
	return fmt.Sprintf(`<a class="lookup" href="%s" title="%s">%s</a>`,
		l.Href, esc(l.Label), name)
}

// detSection is one titled block of the panel. The title is a mono label
// strip, the way the canon names the sections of an instrument.
func detSection(icon, heading, body string) string {
	return fmt.Sprintf(`<section class="det"><div class="det-head">%s`+
		`<h2 class="label">%s</h2></div>%s</section>`, shell.Icon(icon), esc(heading), body)
}

// renderDetails is the whole panel — the live stream's third target.
func renderDetails(v view) string {
	var b strings.Builder
	b.WriteString(`<aside id="details" class="details">`)
	if v.Topic == nil {
		b.WriteString(`<p class="det-note">Open a conversation to see who is in it.</p></aside>`)
		return b.String()
	}

	var people strings.Builder
	people.WriteString(`<ul class="people">`)
	who := participants(v)
	if len(who) == 0 {
		people.WriteString(`<li class="det-note">Nobody has spoken here yet.</li>`)
	}
	for _, p := range who {
		// The pip in front of the name is the channel, and the line under it
		// says what that channel was read from: a voice somebody else answers
		// for names them here, so the colour is never the only place the claim
		// appears. The name itself may be a way somewhere else (personName).
		fmt.Fprintf(&people, `<li>%s%s<span class="said">%s%s</span></li>`,
			channelPip(v, p.Persona), personName(v, p),
			answersFor(v, p), esc(saidWords(p)))
	}
	people.WriteString(`</ul>`)
	b.WriteString(detSection("users", "People", people.String()))

	b.WriteString(detSection("activity", "Status",
		`<p class="plain">`+lifecycleWords(v.Topic.Lifecycle)+`</p>`))

	var work strings.Builder
	work.WriteString(`<ul class="work">`)
	rows, done := 0, 0
	for _, w := range v.Topic.WorkItems {
		if w.Status == topic.WorkDone {
			done++
			continue
		}
		if line := workWords(v, w); line != "" {
			work.WriteString(line)
			rows++
		}
	}
	if rows == 0 {
		work.WriteString(`<li class="det-note">Nothing is waiting on anyone.</li>`)
	}
	work.WriteString(`</ul>`)
	if done > 0 {
		word := "1 thing finished"
		if done > 1 {
			word = fmt.Sprintf("%d things finished", done)
		}
		fmt.Fprintf(&work, `<p class="quiet">%s</p>`, word)
	}
	b.WriteString(detSection("clock", "Waiting on", work.String()))

	var files strings.Builder
	kept := 0
	files.WriteString(`<ul class="files">`)
	for _, a := range v.Topic.Attachments {
		if a.Removed {
			continue
		}
		kept++
		fmt.Fprintf(&files, `<li><span class="what">%s</span>`+
			`<span class="said">%s · %s</span></li>`,
			esc(a.Name), esc(nameOf(v, a.Author)), esc(shell.SizeWords(a.Size)))
	}
	files.WriteString(`</ul>`)
	if kept > 0 {
		b.WriteString(detSection("paperclip", "Attachments", files.String()))
	}

	b.WriteString(`</aside>`)
	return b.String()
}
