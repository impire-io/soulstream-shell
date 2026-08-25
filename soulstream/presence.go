package soulstream

import (
	"context"
	"time"

	"github.com/impire-io/soulstream-core/presence"
)

// LifeSign is one voice's standing on the realm's who-is-around face, in
// the words a row can carry. The face is a reading, never a store: each
// running thing renews its own small entry, a clean stop says goodbye,
// and silence tells the truth — so the sign is the reader's judgment of
// the entry's own age, taken fresh at every render. Advisory throughout:
// a sign may inform what a screen says, never what any surface does.
type LifeSign struct {
	// Present: the entry is fresh — the voice is around right now.
	Present bool
	// Left: the voice said goodbye; When is that moment.
	Left bool
	// When is the moment the sign is judged from — the entry's last
	// renewal, or the goodbye.
	When time.Time
	// Doing is the voice's own plain line about itself, "" when none.
	Doing string
}

// Presence reads the who-is-around face once, keyed by handle. It rides
// the shared read lane — the face is the realm's public shape, the
// catalog's own class. A realm with no face, and a face that cannot be
// read, are both an empty map: the sign is a courtesy, and a screen that
// cannot have it says nothing rather than guessing.
func (sp *Support) Presence(ctx context.Context) map[string]LifeSign {
	states, _, err := presence.All(ctx, sp.rc)
	if err != nil || len(states) == 0 {
		return nil
	}
	now := time.Now()
	out := make(map[string]LifeSign, len(states))
	for _, s := range states {
		r := s.Read(now)
		out[s.Persona] = LifeSign{
			Present: r.Word == presence.WordPresent,
			Left:    r.Word == presence.WordLeft,
			When:    r.When,
			Doing:   s.Entry.Doing,
		}
	}
	return out
}
