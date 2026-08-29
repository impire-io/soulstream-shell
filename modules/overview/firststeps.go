// The first-steps card — design 0008's §3, the first hour's spine.
// Guidance is a reading, never a store: every step derives from realm
// state read fresh at this render, done-marks and all, and the card is
// absent the moment nothing remains — with no flag anywhere to clear.
// A second person joining a furnished soulstream sees the furnished
// soulstream; the card is about the house filling up, not about them.

package overview

import (
	"context"
	"fmt"
	"strings"

	"github.com/impire-io/soulstream-core/toolcatalog"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// step is one item on the first-steps card: what to do in the person's
// words, where the act lives, and whether the house already shows it
// done. A step nobody on this session could take is never listed — a
// step that cannot be taken is a wall, not a door.
type step struct {
	Title, Detail, Href string
	Done                bool
}

// stepFacts is everything the derivation reads, gathered by the module
// and judged by deriveSteps — pure on purpose, so the no-store property
// is a unit test rather than a promise.
type stepFacts struct {
	// The agents surface: declared on, roster readable, how many named.
	AgentsOn, AgentsUnread bool
	AgentsNamed            int
	// How many conversations the board holds.
	Talks int
	// The tool catalog: readable, how many entries, how many of them are
	// services a person connects an account to.
	ToolsKnown    bool
	Tools, Remote int
	// This person: may they administer, and have they connected a
	// service of their own.
	Admin     bool
	Connected bool
	// The people roster: readable (admin only), and how many stand.
	PeopleKnown bool
	People      int
}

// deriveSteps turns one reading of the house into the card's steps.
// Unreadable facts contribute nothing — a step is offered on evidence,
// never on a guess.
func deriveSteps(f stepFacts) []step {
	var steps []step
	if f.AgentsOn && !f.AgentsUnread {
		steps = append(steps, step{
			Title:  "Set up your assistant",
			Detail: "it gets a name of its own and answers when you mention it",
			Href:   "/agents", Done: f.AgentsNamed > 0,
		})
	}
	steps = append(steps, step{
		Title:  "Start a conversation",
		Detail: "mention your assistant by name and it answers",
		Href:   "#convo-start", Done: f.Talks > 0,
	})
	if f.ToolsKnown {
		switch {
		case f.Admin:
			steps = append(steps, step{
				Title:  "Connect a tool",
				Detail: "services your assistant can use for you",
				Href:   "/tools", Done: f.Tools > 0,
			})
		case f.Remote > 0:
			// Not an administrator, but there are services to connect an
			// account to — that act is everyone's own.
			steps = append(steps, step{
				Title:  "Connect a tool",
				Detail: "services your assistant can use for you",
				Href:   "/tools", Done: f.Connected,
			})
		}
	}
	if f.PeopleKnown {
		steps = append(steps, step{
			Title:  "Invite someone",
			Detail: "they sign in with a passkey of their own",
			Href:   "/people", Done: f.People > 1,
		})
	}
	return steps
}

// firstSteps gathers the facts and derives the card's steps onto the
// view. The board and roster were already read for the screen; the
// catalog, this person's own connections, and (for administrators) the
// people roster are read here — each on the lane it already rides
// elsewhere, none kept.
func (m *Module) firstSteps(ctx context.Context, sess *soulstream.Session, v *view) {
	f := stepFacts{
		AgentsOn: v.AgentsOn, AgentsUnread: v.AgentsUnread,
		AgentsNamed: v.AgentsNamed,
		Talks:       len(v.Board),
		Admin:       sess.IsAdmin(),
	}
	if tools, _, err := m.sp.Tools(ctx); err == nil {
		f.ToolsKnown = true
		f.Tools = len(tools)
		for _, t := range tools {
			if t.Kind == toolcatalog.KindRemote {
				f.Remote++
			}
		}
	}
	if !f.Admin && f.Remote > 0 {
		if grants, err := sess.Connections(); err == nil {
			f.Connected = len(grants) > 0
		}
	}
	if f.Admin {
		if ad := sess.Admin(); ad != nil {
			if people, err := ad.People(ctx); err == nil {
				f.PeopleKnown = true
				f.People = len(people)
			}
		}
	}
	v.Steps = deriveSteps(f)
}

// firstStepsCard renders the steps — or nothing at all, which is the
// card's whole retirement plan: when every listed step is done, there is
// no card, and no state anywhere remembers there ever was one.
//
// The card counts itself quietly ("2 of 4 done") instead of explaining
// itself: a done step is a filled dot and a muted word, a pending one is
// a hollow dot and a way there, with its longer sentence riding the
// hover the way raw detail always does.
func firstStepsCard(steps []step) string {
	done := 0
	for _, s := range steps {
		if s.Done {
			done++
		}
	}
	if len(steps) == 0 || done == len(steps) {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="card firststeps"><div class="fs-head"><h2>First steps</h2>`)
	fmt.Fprintf(&b, `<span class="fs-count">%d of %d done</span></div>`, done, len(steps))
	b.WriteString(`<ol>`)
	for _, s := range steps {
		if s.Done {
			fmt.Fprintf(&b, `<li class="off"><span class="fs-dot" title="done">%s</span>%s</li>`,
				shell.Icon("check"), esc(s.Title))
			continue
		}
		// A step whose act lives on this screen points into the slide-over,
		// so the door has to open the panel it leads to.
		open := ""
		if strings.HasPrefix(s.Href, "#") {
			open = ` data-on:click="$panel = true"`
		}
		fmt.Fprintf(&b, `<li><span class="fs-dot"></span><a href="%s" title="%s"%s>%s</a></li>`,
			esc(s.Href), esc(s.Detail), open, esc(s.Title))
	}
	b.WriteString(`</ol></div>`)
	return b.String()
}
