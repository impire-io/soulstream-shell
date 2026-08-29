package soulstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-core/identity"
	infercat "github.com/impire-io/soulstream-inference/catalogue"
	inferclient "github.com/impire-io/soulstream-inference/client"
)

// The models facility: what the surface reads and writes to manage the
// virtual model names agents think through (hq design soulstream-shell
// 0010).
//
// The same two rules as the declare facility. The entry is upstream's —
// the codec, the bucket and its shape have exactly one definition
// (soulstream-inference's catalogue package) and nothing here repeats
// any of it. And the lanes are the design's: reading rides the shared
// read lane (the catalog class — the names are the realm's own
// configuration, and this surface is the party managing it); writing is
// the person's own act on their own admission, because the canonical
// persona scope carries the substrate permission itself and the surface
// acts as nobody.
//
// Nothing is kept: every list is reconstructed at the render that asks.

// InferenceOn says this deployment serves models itself — a declared
// fact shaping the screen's words only. The reading below is discovery,
// so an instance run beside the deployment still shows.
func (sp *Support) InferenceOn() bool { return sp.cfg.InferenceOn }

// Model is one virtual name as the screen reads it: the entry whole, and
// its stored form for the row's fold — the same document the deployment's
// own model verb writes, so the screen never carries a second schema.
type Model struct {
	Name  string
	Entry infercat.Entry
	JSON  string
}

// Models lists every virtual name, sorted, entries whole. An absent
// bucket or an empty one is an ordinary answer: a realm that has named
// no models is not an error.
func (sp *Support) Models(ctx context.Context) ([]Model, error) {
	kv, err := sp.rc.JetStream().KeyValue(ctx, infercat.Bucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the model names: %w", err)
	}
	named, err := infercat.List(ctx, kv)
	if err != nil {
		return nil, fmt.Errorf("reading the model names: %w", err)
	}
	out := make([]Model, 0, len(named))
	for _, n := range named {
		m := Model{Name: n.Name, Entry: n.Entry}
		if body, jerr := json.MarshalIndent(n.Entry, "", "  "); jerr == nil {
			m.JSON = string(body)
		}
		out = append(out, m)
	}
	return out, nil
}

// resolveWindow bounds the discovery scatter — the deployment's own
// window, one scatter, the answers of the moment.
const resolveWindow = 150 * time.Millisecond

// Serving is one serving instance as the screen reads it.
type Serving struct {
	ID         string
	Model      string
	Capability string
	Tags       map[string]string
	Formats    []string
}

// Serving lists what actually serves right now, from one discovery
// scatter on the shared read lane. Resolving is infrastructure's act by
// the thinking plane's own design — no person's admission carries it —
// and the shared lane is the infrastructure standing this surface
// already holds. An empty answer is the moment's truth, never an error.
// The scatter is bounded by its own window rather than a context — the
// window closing is its normal end.
func (sp *Support) Serving() ([]Serving, error) {
	instances, err := inferclient.Resolve(sp.nc, resolveWindow)
	if err != nil {
		return nil, fmt.Errorf("asking what serves: %w", err)
	}
	out := make([]Serving, 0, len(instances))
	for _, in := range instances {
		out = append(out, Serving{
			ID: in.ID, Model: in.Model, Capability: in.Metadata["capability"],
			Tags: in.Tags, Formats: in.Formats,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// SetModel points one virtual name at an entry as the person signed in —
// add and re-point are the same act, the way the deployment's own model
// verb spells it. The name's grammar is the record's and the entry's
// refusals are the codec's, each surfaced in its own words; the bucket
// is brought into existence create-or-report on the first name a realm
// is ever given, the seeding posture the model verb keeps.
func (sess *Session) SetModel(ctx context.Context, name string, e infercat.Entry) error {
	if err := identity.CheckName(name); err != nil {
		return fmt.Errorf("%q is not a model name: %w", name, err)
	}
	js, err := jetstream.New(sess.nc)
	if err != nil {
		return fmt.Errorf("reaching the model names: %w", err)
	}
	kv, err := js.KeyValue(ctx, infercat.Bucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(ctx, infercat.Config())
	}
	if err != nil {
		return fmt.Errorf("reaching the model names: %w", err)
	}
	return infercat.Set(ctx, kv, name, e)
}

// RemoveModel takes one virtual name out of the catalogue as the person
// signed in. A name already absent — bucket and all — is an ordinary
// success: the catalogue ends in the same state either way.
func (sess *Session) RemoveModel(ctx context.Context, name string) error {
	js, err := jetstream.New(sess.nc)
	if err != nil {
		return fmt.Errorf("reaching the model names: %w", err)
	}
	kv, err := js.KeyValue(ctx, infercat.Bucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reaching the model names: %w", err)
	}
	return infercat.Delete(ctx, kv, name)
}
