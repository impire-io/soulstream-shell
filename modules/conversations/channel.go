package conversations

import "github.com/impire-io/soulstream-shell/soulstream"

// Channels: which of the two voices something was said in.
//
// The canon gives this surface two accents at deliberately equal weight —
// amber is the human channel, teal the machine channel — and asks every
// message to carry the one it belongs to. Carrying that needs the record to
// say which channel a voice speaks on, and the record refuses the question
// in those words, on purpose: the persona `kind` field (human / agent /
// service) was removed outright in soulstream-core 014, because the protocol
// cannot verify what controls a key and will not keep a claim it cannot
// check. There is no field to branch on, and there is not meant to be one.
//
// What the persona directory does carry — and what an operator
// countersigns — is the one question about a voice that can be answered
// honestly: who answers for it. That reading, and the accent it colours,
// is defined once in the support layer (soulstream.Voice, threadview.go) —
// the same fact reaches this screen and the agent detail identically.
//
// THE SEAM, NAMED. Operated-by is accountability, not species. A person
// could publish a card naming an operator and a program could publish none;
// the record would say so and this surface would believe it, because
// believing the record is the only honest thing a reader can do. The claim
// is self-published, which is safe to honour directly — naming an operator
// claims less for a voice, never more — and registry.AttestationStatus is
// already there for the day this surface wants to separate an attested claim
// from an unverified one. Until then the People panel names the operator
// beside the voice, so what the colour is read from is on the screen rather
// than implied.

// The two channels, as this package's own words for its lists.
const (
	channelHuman   = "human"
	channelMachine = "machine"
)

// voice is the support layer's reading of what the directory says about a
// persona beyond its name — one definition, both screens.
type voice = soulstream.Voice

// pipFor is the lamp itself: the same 8px LED at the same weight on both
// channels, amber or teal, so neither outranks the other on a screen.
func pipFor(ch, words string) string { return soulstream.PipFor(ch, words) }

// channelPip is the lamp beside a voice, from a whole view.
func channelPip(v view, persona string) string {
	return soulstream.ChannelPip(v.tv(), persona)
}

// pip is the lamp for one participant, for the lists that carry participants
// rather than a whole view. It names an operator by handle: those lists show
// handles anyway, and the name behind one is a read they have no reason to
// make.
func (p participant) pip() string {
	if p.OperatedBy != "" {
		return pipFor(channelMachine, "operated by @"+p.OperatedBy)
	}
	return pipFor(channelHuman, "answers for itself")
}
