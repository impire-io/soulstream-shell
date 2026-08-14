package soulstream

import (
	"context"
	"time"

	"github.com/impire-io/soulstream-core/registry"
)

// The persona directory: what an id is called, and who answers for it.
//
// Both come off one read, so a module that wants a name gets the operator
// claim beside it for nothing. A persona the directory does not name yet is
// remembered too, briefly, so a missing profile costs one read a minute
// rather than one a second — and still appears when it is published.

// Card is what the directory says about a persona.
type Card struct {
	// Name is what to call this persona on screen. Never empty: a persona
	// with no published profile keeps the id the record carries.
	Name string
	// OperatedBy is the persona the directory says answers for this one, ""
	// when it answers for itself.
	OperatedBy string

	found bool
	at    time.Time
}

// Card resolves a persona's directory entry. A directory that does not
// answer is not an error here — the id answers for itself until it does.
func (sp *Support) Card(ctx context.Context, persona string) Card {
	sp.mu.Lock()
	e, ok := sp.cardCache[persona]
	sp.mu.Unlock()
	if ok && (e.found || time.Since(e.at) < time.Minute) {
		return e
	}
	e = Card{Name: persona, at: time.Now()}
	if p, found, err := registry.Lookup(ctx, sp.rc, persona); err == nil && found {
		e.OperatedBy = p.OperatedBy
		if p.DisplayName != "" {
			e.Name, e.found = p.DisplayName, true
		}
	}
	sp.mu.Lock()
	sp.cardCache[persona] = e
	sp.mu.Unlock()
	return e
}

// Name is the on-screen name for a persona.
func (sp *Support) Name(ctx context.Context, persona string) string {
	return sp.Card(ctx, persona).Name
}
