package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/record"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// Reading the stores: which ones there are, how a page of them is walked,
// and what one message turns back into.

const (
	// pageSize is how many messages one page of the list holds.
	pageSize = 50
	// tailSize is how many a followed screen carries — fewer, because a tail
	// is for watching what arrives rather than reading what is there.
	tailSize = 25
	// scanLimit is how many sequences one read may examine before it stops
	// and says so. A store compacts, so its live messages cluster near the
	// tail and a page of them is usually a page of sequences; a narrow filter
	// over a long history is the case this bounds. Whatever it stops short of
	// is said on the screen — a silent truncation would read as "that is
	// everything", which is the one thing a debugging surface must never say.
	scanLimit = 1000
	// payloadCap is how much of a payload is put on screen. Past it the
	// screen says the size and shows nothing: a debugging surface that hangs
	// a browser on one large message has stopped being one.
	payloadCap = 64 << 10
)

// store is one of the two stores a realm keeps. They are separate on
// purpose and the screen keeps them separate: the op-log is the realm's
// history, the inbox is one person's own slips, and a reader that merged
// them would be inventing a store nobody provisioned.
type store struct {
	// Key is this store's name in a URL, Label what a person reads, and
	// Stream the name the server answers to — all three on screen, because
	// this is the screen where the real name is the point.
	Key    string
	Label  string
	Stream string
	// Space is the subject space this store captures, shown as the default
	// the filter box is measured against.
	Space string
	// About says in one line what is in here.
	About string
}

// stores is every store this screen can read, in the order it offers them.
// The list is the protocol's, not a configuration: a realm holds exactly
// these ([`realm.StreamName`], [`realm.NotifyStreamName`]).
var stores = []store{
	{
		Key: "conversations", Label: "Conversations", Stream: realm.StreamName,
		Space: realm.StreamSubject,
		About: "Everything ever written in a conversation — what was said, what was " +
			"announced, every lifecycle step. Kept forever; compacted, never aged out.",
	},
	{
		Key: "notifications", Label: "Notifications", Stream: realm.NotifyStreamName,
		Space: realm.NotifyStreamSubject,
		About: "The slips telling somebody their name came up. One inbox per person, " +
			"holding the newest few — pointers, not the messages themselves.",
	},
}

// storeFor resolves a store key, settling anything unknown on the first —
// a link built by hand with a typo in it lands somewhere real rather than
// on an error about a name nobody typed on purpose.
func storeFor(key string) store {
	for _, s := range stores {
		if s.Key == key {
			return s
		}
	}
	return stores[0]
}

// serviceLane is the subject class no store captures. A person filtering for
// it is not looking at an empty store, they are looking at something kept
// nowhere on purpose, and the screen says which.
const serviceLane = topic.SvcSubjectPrefix

// op is one message as the screen reads it: what the store says about it,
// what the record says it is, and the verdict its signature earned.
type op struct {
	Seq uint64
	// Stored is the store's own timestamp — when the server took it, not
	// what the author claimed. The record's own claim rides in Rec and the
	// protocol is explicit that ordering authority is never a clock.
	Stored  time.Time
	Subject string
	Size    int
	// Rec is the parsed record, and Bad why it would not parse — a message
	// on these subjects that is not a record is exactly the kind of thing
	// this screen exists to make visible, so it is shown rather than skipped.
	Rec record.Record
	Bad string
	// Version is the wire version as the message itself spells it. The
	// parsed record does not keep it and this screen needs it: it is the
	// difference between a signature that could verify and one that was
	// written before the form it would have to verify under existed.
	Version string
	Sig     topic.SigStatus
	// Binding is the value the signature is bound to, derived from the
	// subject alone; "" for a subject shape the rule does not cover.
	Binding string
	// Canonical is the byte sequence the signature covers, and CanonErr why
	// there is none.
	Canonical []byte
	CanonErr  string
	Payload   []byte
}

// view is one read of the list.
type view struct {
	Store store
	Ops   []op
	// Err is why there is nothing to show, in the words of whoever refused.
	Err string
	// Empty says the store holds nothing at all, which is different from a
	// filter matching nothing and is said differently.
	Empty bool
	// Msgs and Bytes are what the store holds in total, for the line above
	// the list.
	Msgs, Bytes uint64
	// First and Last are the sequences the store still holds.
	First, Last uint64
	// Examined is how many sequences this read walked, and Capped whether it
	// stopped at the limit rather than at the end.
	Examined int
	Capped   bool
	// Oldest is the lowest sequence this read examined, so the key that
	// reads further back knows where to start.
	Oldest uint64
	// PatternErr is what is wrong with the subject pattern somebody typed.
	PatternErr string
	// ServiceLane says the pattern is aimed at the one subject class no
	// store keeps.
	ServiceLane bool
}

// opView is one read of a single message.
type opView struct {
	Store store
	Op    op
	Found bool
	Err   string
}

// read walks one page of a store, newest first.
//
// The walk is backwards by sequence, and every read rides the signed-in
// person's own client — never the surface's shared read lane. A sequence the
// store no longer holds is the ordinary case rather than an error: compaction
// deletes, so gaps are what a healthy op-log looks like from the outside.
func (m *Module) read(ctx context.Context, sess *soulstream.Session, a ask, limit int) view {
	s := storeFor(a.Store)
	v := view{Store: s}
	if strings.HasPrefix(a.Filter, serviceLane) {
		v.ServiceLane = true
	}
	filter := a.Filter
	if filter == "" {
		filter = s.Space
	} else if err := checkPattern(filter); err != nil {
		v.PatternErr = err.Error()
		return v
	}

	st, err := sess.Client().JetStream().Stream(ctx, s.Stream)
	if err != nil {
		v.Err = refusalWords(s, err)
		return v
	}
	info, err := st.Info(ctx)
	if err != nil {
		v.Err = refusalWords(s, err)
		return v
	}
	v.Msgs, v.Bytes = info.State.Msgs, info.State.Bytes
	v.First, v.Last = info.State.FirstSeq, info.State.LastSeq
	if v.Msgs == 0 {
		v.Empty = true
		return v
	}

	ceiling := a.Before
	if ceiling == 0 || ceiling > v.Last {
		ceiling = v.Last
	}
	v.Oldest = ceiling + 1
	for seq := ceiling; seq >= v.First && seq > 0; seq-- {
		if len(v.Ops) >= limit {
			break
		}
		if v.Examined >= scanLimit {
			v.Capped = true
			break
		}
		v.Examined++
		v.Oldest = seq
		msg, err := st.GetMsg(ctx, seq)
		if err != nil {
			// Deleted, compacted away, or never there. Not an error: it is
			// what a store that rolls its history up looks like.
			continue
		}
		if !subjectMatches(filter, msg.Subject) {
			continue
		}
		v.Ops = append(v.Ops, m.decode(sess, msg))
	}
	return v
}

// readOne is one message by sequence, read the same way and reported the
// same way when it is not there.
func (m *Module) readOne(ctx context.Context, sess *soulstream.Session,
	s store, seq uint64,
) opView {
	st, err := sess.Client().JetStream().Stream(ctx, s.Stream)
	if err != nil {
		return opView{Store: s, Err: refusalWords(s, err)}
	}
	msg, err := st.GetMsg(ctx, seq)
	if err != nil {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return opView{Store: s, Err: fmt.Sprintf(
				"%s holds no message %d any more — a rollup compacts a conversation's "+
					"history into one message and deletes what it replaced.", s.Label, seq)}
		}
		return opView{Store: s, Err: refusalWords(s, err)}
	}
	return opView{Store: s, Op: m.decode(sess, msg), Found: true}
}

// decode turns a stored message back into everything the screen can say
// about it: the record it parses to, the value its signature is bound to,
// the bytes that signature covers, and the verdict those earn against the
// author's own published key.
//
// The verdict is earned exactly as the conversation view earns it — the same
// core call, over a keyring built from the identity plane's open directory.
// There is no second verdict vocabulary on these screens.
func (m *Module) decode(sess *soulstream.Session, msg *jetstream.RawStreamMsg) op {
	o := op{
		Seq: msg.Sequence, Stored: msg.Time, Subject: msg.Subject,
		Size: len(msg.Data), Payload: msg.Data,
		Version: msg.Header.Get(record.HeaderVersion),
	}
	rec, err := record.Parse(msg.Header, msg.Data)
	if err != nil {
		o.Bad = err.Error()
		return o
	}
	o.Rec = rec
	o.Binding = binding(msg.Subject)
	kr := m.sp.KeyringFor(rec.Author)
	o.Sig = topic.VerifyRecord(rec, sess.Client().RealmKey(), o.Binding, kr)

	unsigned := rec
	unsigned.Signature = ""
	if o.Binding == "" {
		o.CanonErr = "no signing input: this subject is outside the shape the " +
			"binding rule covers, so there is nothing a signature could have been over"
		return o
	}
	canonical, cerr := unsigned.Canonical(sess.Client().RealmKey(), o.Binding)
	if cerr != nil {
		o.CanonErr = cerr.Error()
		return o
	}
	o.Canonical = canonical
	return o
}

// binding is the value an op's signature is bound to, derived from the
// subject it was consumed on: the conversation path for a conversation
// subject, the person for an inbox one.
//
// The rule is the record's own and is deliberately derivable from the
// subject alone, so any reader can recompute a signing input from the
// subject it read the message on — which is precisely what this screen is
// doing. A subject shape outside the rule returns "", and the screen says
// there is no signing input rather than inventing one.
func binding(subject string) string {
	for _, prefix := range []string{
		topic.OpsSubjectPrefix, topic.InfoSubjectPrefix,
		topic.NotifySubjectPrefix, topic.SvcSubjectPrefix,
	} {
		if strings.HasPrefix(subject, prefix) {
			return strings.TrimPrefix(subject, prefix)
		}
	}
	return ""
}

// refusalWords says what happened in the words of whoever refused, and names
// the one refusal a person can act on: their own sign-in not being permitted
// the read. Everything else is passed on as it was said.
func refusalWords(s store, err error) string {
	switch {
	case errors.Is(err, jetstream.ErrStreamNotFound):
		return fmt.Sprintf("This deployment holds no %s store (%s).", s.Label, s.Stream)
	case isPermission(err):
		return fmt.Sprintf("Your sign-in is not permitted to read %s (%s). "+
			"The server refused it: %v", s.Label, s.Stream, err)
	}
	return fmt.Sprintf("Reading %s (%s) failed: %v", s.Label, s.Stream, err)
}

// isPermission reports whether a refusal came from the server's own
// permissions rather than from anything this surface did. The server says so
// in words on a lane that carries no code for it, so the words are what
// there is to read.
func isPermission(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission") || strings.Contains(msg, "authorization")
}

// checkPattern refuses a subject pattern that is not one. The record's
// subjects are the way it is meant to be read, so the box takes a subject
// pattern rather than a search box — and a pattern that could never match
// is told so here rather than answering with an empty list that looks like
// a store with nothing in it.
func checkPattern(pattern string) error {
	if pattern == "" {
		return nil
	}
	if strings.ContainsAny(pattern, " \t\r\n") {
		return errors.New("a subject has no spaces in it")
	}
	tokens := strings.Split(pattern, ".")
	for i, tok := range tokens {
		switch {
		case tok == "":
			return errors.New("a subject has no empty parts — check the dots")
		case tok == ">" && i != len(tokens)-1:
			return errors.New("> matches the rest of a subject, so it can only be the last part")
		case tok != "*" && tok != ">" && strings.ContainsAny(tok, "*>"):
			return errors.New("* and > each stand for a whole part, never a piece of one")
		}
	}
	return nil
}

// subjectMatches is NATS subject matching: * stands for exactly one part, >
// for the rest of the subject, everything else matches itself.
//
// It is written here rather than borrowed because the client publishes no
// matcher, and the walk needs to match locally: the server-side filter
// reads forward from a sequence and this screen reads backward from the
// newest, which is the direction a person debugging actually wants.
func subjectMatches(pattern, subject string) bool {
	p := strings.Split(pattern, ".")
	s := strings.Split(subject, ".")
	for i, tok := range p {
		if tok == ">" {
			// > matches the rest, and there has to be a rest for it to match.
			return i < len(s)
		}
		if i >= len(s) {
			return false
		}
		if tok != "*" && tok != s[i] {
			return false
		}
	}
	return len(p) == len(s)
}
