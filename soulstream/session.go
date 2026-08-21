package soulstream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/registry"
	siclient "github.com/impire-io/soulstream-identity/client"
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
	// bearer produces the issuer's token of the moment, straight from the
	// shell's custody — asked per use, never copied, so a session that has
	// outlived its first access token still acts with a living one. The
	// error is the shell saying the session is over.
	bearer func() (string, error)

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

// identity is this person's own identity-plane client, built on their own
// admission: linking, revoking, and approval-minting are each the
// person's own act on their own prefix, never the surface's.
func (sess *Session) identity() *siclient.Client {
	return siclient.New(sess.nc, sess.sp.cfg.Account, sess.Persona)
}

// Connections is this person's own linked services.
func (sess *Session) Connections() ([]siclient.GrantInfo, error) {
	return sess.identity().Grants()
}

// ConnectStart begins linking one service as this person: the URL their
// own browser opens, and the ceremony id that rides back as OAuth state.
func (sess *Session) ConnectStart(resource string) (siclient.GrantLink, error) {
	return sess.identity().GrantLinkStart(resource)
}

// ConnectComplete redeems the ceremony the callback carried. The plane
// holds the ceremony under this persona's own prefix, so a callback
// landing on any other session completes nothing.
func (sess *Session) ConnectComplete(linkID, code string) error {
	return sess.identity().GrantLinkComplete(linkID, code)
}

// Disconnect revokes this person's own grant for one service.
func (sess *Session) Disconnect(resource string) error {
	return sess.identity().GrantRevoke(resource)
}

// approvalTTL bounds a minted answer: minutes — the retry's window, the
// plane's own scale for it.
const approvalTTL = 5 * time.Minute

// MintApproval signs this person's answer to one deferred invocation, as
// their own persona key: exactly a delegation naming the invocation, for
// the originator to be named its actor. Minting is the person's; the
// support layer's Deliver carries it.
func (sess *Session) MintApproval(invocationID, actor string) (siclient.Delegation, error) {
	return sess.identity().MintApproval(invocationID, actor, approvalTTL)
}

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

// adminRole is the group name whose members administer sign-ins — part of
// the sign-in surface's published contract (its tokens carry group names as
// the roles claim, and its admin API admits exactly this one), consumed
// here the way the rest of that contract is: as the wire spells it.
const adminRole = "admin"

// IsAdmin reports whether this person's own token carries the admin role.
// It is read locally from the bearer's claims for exactly one purpose:
// whether to draw the administration key on the spine. A display fact, not
// an authority — every administrative act still carries the bearer to the
// sign-in surface, whose verified refusal is the answer that counts.
func (sess *Session) IsAdmin() bool {
	bearer, err := sess.bearer()
	if err != nil {
		return false
	}
	parts := strings.Split(bearer, ".")
	if len(parts) != 3 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Roles []string `json:"roles"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return false
	}
	return slices.Contains(claims.Roles, adminRole)
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
