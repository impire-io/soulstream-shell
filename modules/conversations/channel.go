package conversations

import "fmt"

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
// honestly: who answers for it. A persona with no `operated_by` line answers
// for itself; the directory calls that a principal, and this surface reads
// it as the human channel. A persona that names an operator is a voice
// somebody else answers for — an assistant, a scheduled job, a tool session
// with a name of its own — and this surface reads that as the machine
// channel.
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

// The two channels. They are class names as much as they are concepts: a
// message card, a mention token and an LED pip all carry the same word.
const (
	channelHuman   = "human"
	channelMachine = "machine"
)

// voice is what the directory says about a persona beyond its name.
type voice struct {
	// OperatedBy is the persona the directory says answers for this one, ""
	// when the persona answers for itself.
	OperatedBy string
}

// channel is the accent this voice speaks on. The zero voice — a persona
// with no directory card, or none read yet — is the human channel: no
// operator claim exists to read, and "answers for itself" is the record's
// own answer to that, not a guess of ours.
func (vo voice) channel() string {
	if vo.OperatedBy != "" {
		return channelMachine
	}
	return channelHuman
}

// channelOf is the channel a persona speaks on.
func channelOf(v view, persona string) string { return v.Voices[persona].channel() }

// channelWords is what the pip says when somebody hovers it: the record's
// own fact about the voice, in the words the record uses for it. Never
// "human" or "machine" — those are the design's names for the two accents,
// and the record does not make that claim.
func channelWords(v view, persona string) string {
	if op := v.Voices[persona].OperatedBy; op != "" {
		return "operated by " + nameOf(v, op)
	}
	return "answers for itself"
}

// pipFor is the lamp itself: the same 8px LED at the same weight on both
// channels, amber or teal, so neither outranks the other on a screen.
func pipFor(ch, words string) string {
	return fmt.Sprintf(`<span class="led %s" title="%s"></span>`, ch, esc(words))
}

// channelPip is the lamp beside a voice, from a whole view.
func channelPip(v view, persona string) string {
	return pipFor(channelOf(v, persona), channelWords(v, persona))
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
