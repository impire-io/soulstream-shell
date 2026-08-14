package soulstream

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/registry"
)

// Session is one signed-in human's Soulstream side: their own admission,
// their own client, their own signer, and the tray of what has been said
// about them since they arrived. Memory only — nothing here touches disk.
type Session struct {
	// Persona is the admitted persona-shaped id every act is attributed to.
	Persona string
	// Signed says whether this person's own key materialised, so their acts
	// carry a signature rather than only an attribution.
	Signed bool

	sp *Support
	nc *nats.Conn
	rc *realm.Client
	// bearer is the token the issuer handed this person at sign-in — the
	// shell's own custody, copied here so a module can act as them against
	// a surface that speaks HTTP rather than NATS. It lives as long as the
	// session and no longer.
	bearer string

	// stop ends the inbox follower this session runs on its own connection.
	stop context.CancelFunc

	mu sync.Mutex
	// name is what to call this person on screen, and named says whether it
	// is theirs or the id standing in until somebody publishes one.
	name  string
	named bool
	// unread is this person's own tray: the conversations holding a message
	// with their name in it, and which messages those are. seen is every
	// slip already tallied, so a replayed inbox never resurrects a message
	// already read.
	unread map[string]map[string]bool
	seen   map[string]bool
}

// Client is this person's own admitted client: their connection, their
// persona, their signature. Every act rides it — the surface's own read
// lane never writes.
func (sess *Session) Client() *realm.Client { return sess.rc }

// Admin is this person's own reach into the deployment's people-and-sign-in
// administration surface, carrying their bearer and no standing of the
// surface's own. Nil when the deployment declares no such surface — the
// same absence the module above reads to know it is not part of this
// deployment.
func (sess *Session) Admin() *Admin {
	if sess.sp.cfg.AdminBase == "" {
		return nil
	}
	return &Admin{base: sess.sp.cfg.AdminBase, bearer: sess.bearer, cl: sess.sp.adminCl}
}

// ScreenName is what to call this person on screen: what the issuer said
// when it said anything, else what the realm's own persona directory
// publishes, else the id the record carries. A person should read their own
// name on their own screen, not the id a machine minted for them.
//
// It resolves on the session rather than through the shared name cache, and
// keeps asking until the directory answers: a profile published after
// somebody signed in reaches their screen without asking them to sign in
// again, and one person's unnamed session never leaves a miss in the cache
// every other reader then inherits for a minute.
func (sess *Session) ScreenName(ctx context.Context) string {
	sess.mu.Lock()
	name, named := sess.name, sess.named
	sess.mu.Unlock()
	if named {
		return name
	}
	if p, found, err := registry.Lookup(ctx, sess.sp.rc, sess.Persona); err == nil && found &&
		p.DisplayName != "" {
		return sess.nameIs(p.DisplayName)
	}
	return name
}

// nameIs settles what this person is called and returns it.
func (sess *Session) nameIs(name string) string {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	sess.name, sess.named = name, true
	return name
}

// close drains one session: the inbox follower first, so its consumer is
// gone before the connection under it is.
func (sess *Session) close() {
	if sess.stop != nil {
		sess.stop()
	}
	sess.nc.Close()
}
