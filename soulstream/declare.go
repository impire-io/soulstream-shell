package soulstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/fleet"
)

// The declare facility: what the surface reads and writes to place an agent
// on the deployment rather than on somebody's laptop.
//
// Two rules shape all of it. The declaration is upstream's — parsing,
// validation and the wire format have exactly one definition
// (soulstream-workloads' declaration and fleet packages), and nothing here
// repeats any of it. And the lanes are the ones already decided (hq
// soulstream-shell 0005 §3): the placement topic is realm-public record,
// read on the shared read lane like the board; submitting is the person's
// own act on their own admission, because a placement is an ordinary work
// item any persona may open and the surface acts as nobody.
//
// Nothing is kept. Every list below is reconstructed from the log at the
// render that asks for it — there is no store here to drift.

// PlacementsTopic is the NAME of the topic this deployment's declared
// agents are placed on, "" when the deployment declares none. It is a
// declared fact and nothing else: the surface asks it to learn whether
// this lane is part of this build at all, with no probe and no round trip.
func (sp *Support) PlacementsTopic() string { return sp.cfg.PlacementsTopic }

// CapabilityRole is the name of the signing role a declared agent's tools
// resolve through — a name the deployment's founding declared, "" when the
// deployment names none. The declaration carries names, never grants
// (workloads design 0005 §5), and this is the one name the shell cannot
// derive from anything it reads.
func (sp *Support) CapabilityRole() string { return sp.cfg.CapabilityRole }

// Wake is one declared way of waking an agent, in the shape a screen shows
// it: the kind, the delivery class upstream fixed as a normative fact a
// shell MUST surface, and whatever that kind was pointed at.
type Wake struct {
	Kind     string
	Delivery string
	Detail   string
}

// Declared is one placement as the screen reads it: who was declared, what
// wakes them, what they think through, and where the placement itself
// stands on the record.
type Declared struct {
	ItemID string
	Name   string
	Home   string
	Wakes  []Wake
	Model  string
	Tools  []string
	// State is the placement's own status, carried as the record spells it
	// — translated where it reaches a screen and nowhere else. Owner is the
	// node whose claim the log settled on, "" while nobody has claimed it.
	State topic.WorkStatus
	Owner string
	// Opened is when the placement was submitted.
	Opened time.Time
	// JSON is the declaration exactly as it was submitted, indented — the
	// same document `soulstream agent submit` takes, so the screen never
	// carries a second schema.
	JSON string
}

// Declared lists every agent placed on this deployment, newest last, read
// from the placement topic's own log. A deployment that declares no
// placement topic has no such list; a topic nobody has started yet lists
// nothing, which is the ordinary state of a realm before the first declare.
func (sp *Support) Declared(ctx context.Context) ([]Declared, error) {
	name := sp.cfg.PlacementsTopic
	if name == "" {
		return nil, ErrNoPlacements
	}
	path, found, err := placementsPath(ctx, sp, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	mt, err := topic.Open(sp.rc, path).Materialise(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading where agents are placed: %w", err)
	}
	out := make([]Declared, 0, len(mt.WorkItems))
	for _, item := range mt.WorkItems {
		d, ok := fleet.DeclarationOf(item)
		if !ok {
			// An ordinary work item on the same topic is somebody else's
			// business, not a half-read placement.
			continue
		}
		out = append(out, declaredFrom(item, d))
	}
	return out, nil
}

// ErrNoPlacements is what a deployment that places no agents answers with.
// It is a contradiction rather than a failure — the lane asking for this
// list is not drawn at all in such a deployment — so it is reported by
// name instead of read as an unreadable realm.
var ErrNoPlacements = errors.New("this deployment places no agents")

// declaredFrom turns one placement work item into what a row says.
func declaredFrom(item topic.WorkItem, d declaration.Declaration) Declared {
	dec := Declared{
		ItemID: item.ID, Name: d.Persona, Home: d.Topic,
		State: item.Status, Owner: item.Owner, Opened: item.Timestamp,
	}
	if d.Inference != nil {
		dec.Model = d.Inference.Model
	}
	if d.Capabilities != nil {
		dec.Tools = d.Capabilities.Tools
	}
	for _, w := range d.Wake {
		dec.Wakes = append(dec.Wakes, Wake{
			Kind: string(w.Kind), Delivery: w.DeliveryClass(), Detail: wakeDetail(w),
		})
	}
	if body, err := json.MarshalIndent(d, "", "  "); err == nil {
		dec.JSON = string(body)
	}
	return dec
}

// wakeDetail is what a wake entry was pointed at, in one short phrase.
func wakeDetail(w declaration.WakeEntry) string {
	switch w.Kind {
	case declaration.WakeTopic:
		return w.Path
	case declaration.WakeSchedule:
		if w.TTL != "" {
			return w.Name + " · " + w.Pattern + " · " + w.TTL
		}
		return w.Name + " · " + w.Pattern
	case declaration.WakeSubject:
		return w.Subject
	default:
		return ""
	}
}

// placementsPath resolves the declared placement-topic NAME to the path the
// board holds it under. It never starts the topic: reading must not write,
// and a realm where nothing has been declared yet is a realm with nothing
// to read.
func placementsPath(ctx context.Context, sp *Support, name string) (string, bool, error) {
	entries, err := topic.Board(ctx, sp.rc)
	if err != nil {
		return "", false, fmt.Errorf("reading the conversations: %w", err)
	}
	for _, e := range entries {
		if e.Announcement.Name == name {
			return e.Path, true, nil
		}
	}
	return "", false, nil
}

// Declare places one agent on the deployment as the person signed in: the
// placement is an ordinary work item carrying the declaration, opened on
// their own admission and signed with their own key. The surface performs
// no act here it could not perform through the published packages — the
// wire format has one definition and it is fleet's.
//
// The placement topic is started on first use, the way the product's own
// submit path starts it: a placement nobody could open is not an honest
// absence, it is a lane with nowhere to land.
func (sess *Session) Declare(ctx context.Context, d declaration.Declaration) (string, error) {
	name := sess.sp.cfg.PlacementsTopic
	if name == "" {
		return "", ErrNoPlacements
	}
	path, found, err := placementsPath(ctx, sess.sp, name)
	if err != nil {
		return "", err
	}
	if !found {
		h, serr := topic.StartTopic(ctx, sess.rc, topic.StartTopicInput{
			Name:          name,
			SubjectMatter: "where declared agents are placed on this deployment",
		})
		if serr != nil {
			return "", fmt.Errorf("making room for placed agents: %w", serr)
		}
		path = h.Path()
	}
	id, err := fleet.Submit(ctx, topic.Open(sess.rc, path), d)
	if err != nil {
		return "", err
	}
	return id, nil
}

// CatalogueBucket is where this realm's model names live — one key per
// name. The shell reads the NAMES and nothing else: what a name resolves
// to is the thinking plane's business, and reading a route the surface
// cannot use would be reading somebody else's configuration.
const CatalogueBucket = "soulstream-inference-catalogue"

// ModelNames lists the names agents may be declared to think through,
// sorted. A realm that has named none lists none — an empty catalogue is
// an ordinary answer and never an error.
func (sp *Support) ModelNames(ctx context.Context) ([]string, error) {
	kv, err := sp.rc.JetStream().KeyValue(ctx, CatalogueBucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the model names: %w", err)
	}
	keys, err := kv.Keys(ctx)
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("listing the model names: %w", err)
	}
	sort.Strings(keys)
	return keys, nil
}
