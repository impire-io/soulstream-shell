package soulstream

import (
	"context"
	"slices"
	"time"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-workloads/fleet"
)

// The watch: the support layer's living projection of the board (hq design
// soulstream-shell 0012 §2). Two followers on the shared read lane, both
// memory only, both rebuilt from the log on connect — the record's own
// followed board (core spec 022) for every conversation's announcement,
// lifecycle and last activity, and one topic follow over the placements
// topic for the partition's declarations, because the board projection
// deliberately retains no op bodies and a declaration IS a work item's
// body. The render tick reads snapshots and performs no JetStream read.

// Room is one machinery topic's role on this deployment: the placements
// topic itself, or a declared agent's home. Home-ness is a role read from
// the declarations at every ask, never a property stored on the topic — a
// persona declared twice with two homes leaves both machinery, honestly,
// because submission is additive and nothing un-places.
type Room struct {
	// Agents are the personas whose declarations name this topic home.
	Agents []string
	// Placements says this is the placements topic itself.
	Placements bool
}

// watchState is what the two followers maintain, guarded by Support.mu's
// own sibling here to keep render reads off the directory cache's lock.
type watchState struct {
	follower       *topic.BoardFollower
	placementsPath string
	placements     *topic.MaterializedTopic
}

// runWatch keeps the projections alive for the life of the surface. The
// board follower survives reconnects on its own ordered consumer; a boot
// racing the realm's provisioning retries until the stream answers.
func (sp *Support) runWatch(ctx context.Context) {
	for ctx.Err() == nil {
		f, err := topic.FollowBoard(ctx, sp.rc, nil)
		if err == nil {
			sp.wmu.Lock()
			sp.watch.follower = f
			sp.wmu.Unlock()
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	if sp.cfg.PlacementsTopic == "" {
		return // this deployment places no agents: there is no machinery to watch
	}
	for ctx.Err() == nil {
		path := sp.resolvePlacements(ctx)
		if path == "" {
			// No placements topic yet — the ordinary state of a realm before
			// the first declare. Watch for its birth on the projection.
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		sp.wmu.Lock()
		sp.watch.placementsPath = path
		sp.wmu.Unlock()
		// Follow materialises then applies live ops until ctx ends; an error
		// falls back to re-resolving, because the honest reasons it stops —
		// a re-provisioned realm, a stream briefly gone — all read forward.
		_ = topic.Open(sp.rc, path).Follow(ctx, func(mt *topic.MaterializedTopic) {
			sp.wmu.Lock()
			sp.watch.placements = mt
			sp.wmu.Unlock()
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// resolvePlacements finds the placements topic's path by its declared NAME
// on the board, "" while nothing has started it. Reading never writes.
func (sp *Support) resolvePlacements(ctx context.Context) string {
	entries, err := sp.Board(ctx)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.Announcement.Name == sp.cfg.PlacementsTopic {
			return e.Path
		}
	}
	return ""
}

// Board is every conversation on the record: the projection's snapshot —
// zero round trips — once the follower is warm, and the one-shot read for
// the first moments of a boot, so a screen served while the projection is
// still cold answers rather than blanking.
func (sp *Support) Board(ctx context.Context) ([]topic.BoardEntry, error) {
	sp.wmu.Lock()
	f := sp.watch.follower
	sp.wmu.Unlock()
	if f != nil {
		return f.Entries(), nil
	}
	return topic.Board(ctx, sp.rc)
}

// Machinery is the partition: every topic that is a room of the record's
// own machinery rather than a person's conversation — the placements topic
// and every declared home — derived fresh from the watched declarations at
// every ask. Empty on a deployment that places no agents.
func (sp *Support) Machinery() map[string]Room {
	sp.wmu.Lock()
	mt, ppath := sp.watch.placements, sp.watch.placementsPath
	sp.wmu.Unlock()
	out := map[string]Room{}
	if ppath != "" {
		out[ppath] = Room{Placements: true}
	}
	if mt == nil {
		return out
	}
	for _, item := range mt.WorkItems {
		d, ok := fleet.DeclarationOf(item)
		if !ok {
			continue
		}
		r := out[d.Topic]
		if !slices.Contains(r.Agents, d.Persona) {
			r.Agents = append(r.Agents, d.Persona)
		}
		out[d.Topic] = r
	}
	return out
}

// HomeOf is one declared agent's current room: the home its newest
// placement names, "" for a persona nothing here declares. An agent
// declared twice answers with the later home — the earlier one stays
// machinery (Machinery says so), but the room a person talks to it in is
// the one its newest declaration lives by.
func (sp *Support) HomeOf(persona string) string {
	sp.wmu.Lock()
	mt := sp.watch.placements
	sp.wmu.Unlock()
	home := ""
	if mt == nil {
		return home
	}
	for _, item := range mt.WorkItems {
		if d, ok := fleet.DeclarationOf(item); ok && d.Persona == persona {
			home = d.Topic
		}
	}
	return home
}
