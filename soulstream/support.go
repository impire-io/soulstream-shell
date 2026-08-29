// Package soulstream is the Soulstream module-support layer: what a shell
// module needs to read and write a Soulstream realm as the person looking
// at the screen.
//
// It exists because the shell refuses to hold any of this. The shell
// custodies an id and a bearer; turning those into an admission (the public
// sentinel plus that bearer through the OIDC callout lane), a client that
// signs as the person, and the directory reads that give an id a name is
// Soulstream's own work — so it lives here, on the modules' side of the
// contract, and the shell's packages import none of it.
//
// Nothing here is durable either. The connections and the trays live for as
// long as a session does and go when it does; the store of record stays the
// realm's.
package soulstream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/identity"
	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"
	siclient "github.com/impire-io/soulstream-identity/client"

	"github.com/impire-io/soulstream-shell/shell"
)

// Config points at the surfaces the deployment already runs. The support
// layer founds and owns nothing.
type Config struct {
	// NATSURL reaches the realm's server.
	NATSURL string
	// CredsPath is the surface's own read lane — an ordinary creds file the
	// deployment supplies (a soulnode plane hands its ops lane).
	CredsPath string
	// CredsUser is the principal name that creds file connects as; the
	// identity-plane directory reads ride its own prefix.
	CredsUser string
	// SentinelPath is the public sentinel creds file a session presents
	// beside its bearer for its own admission.
	SentinelPath string
	// Realm is the realm name; Account is the realm account public key (the
	// identity plane's account segment).
	Realm   string
	Account string
	// Issuer is the sign-in surface, read for the house readout only — the
	// shell does the signing in.
	Issuer string
	// AdminBase is the base URL of the people-and-sign-in administration
	// surface this deployment runs. Optional: a deployment whose people are
	// administered somewhere it does not run declares none, and every module
	// that needs one is then not part of that deployment.
	AdminBase string
	// AgentsDial is the address this deployment tells an agent to dial, and
	// its way of saying it issues agent credentials at all. Optional, on the
	// same terms as AdminBase.
	//
	// It is separate from NATSURL deliberately: the address this surface
	// reaches the server on is its own business, and is not always one it
	// could honestly print in somebody else's configuration file.
	AgentsDial string
	// GuardrailOn says this deployment's identity plane runs the guardrail
	// — the declared fact the approvals module activates by (design 0006
	// §4). Optional, on the same terms as the two above: no probe, no
	// shell-side configuration, just the deployment's own word.
	GuardrailOn bool
	// PlacementsTopic is the NAME of the topic this deployment places
	// declared agents on, and its way of saying it serves agents as
	// infrastructure at all. Optional, on the same terms as the facts
	// above: declared empty, the declare lane is not part of this build.
	PlacementsTopic string
	// CapabilityRole is the name of the signing role a declared agent's
	// tools resolve through — declared by the deployment's founding, never
	// derived here. Optional: declared empty, an agent is declared without
	// tools, which is the whole of what this surface can honestly offer
	// when nothing has told it what the role is called.
	CapabilityRole string
	// InferenceOn says this deployment serves models itself — the declared
	// fact the models screen words its empty states by (hq design
	// soulstream-shell 0010 §5). It shapes words only, never the reading:
	// what actually serves is discovered, not declared, so an instance
	// somebody runs beside the deployment still shows. Optional, on the
	// same no-probe terms as every fact above.
	InferenceOn bool
}

// Support is the layer itself: one read lane for the surface, and a session
// for every person signed into it.
type Support struct {
	cfg Config
	sh  *shell.Shell

	nc  *nats.Conn
	rc  *realm.Client
	dir *siclient.Client
	// adminCl is the lane to the administration surface: shared for its
	// connection pool, never for its authority — the bearer rides per call,
	// from the session that owns it.
	adminCl *http.Client

	mu        sync.Mutex
	keyCache  map[string]string // persona -> public key (directory reads)
	cardCache map[string]Card   // persona -> directory card (registry reads)
}

// sessionKey is this layer's own key on a shell session. It is unexported,
// so nothing else can reach what hangs off a person's session here.
type sessionKey struct{}

// Open connects the surface's own read lane and attaches to the shell's
// session lifecycle: from here on, every sign-in opens an admission for
// that person and every sign-out closes it.
func Open(ctx context.Context, sh *shell.Shell, cfg Config) (*Support, error) {
	if cfg.NATSURL == "" || cfg.CredsPath == "" || cfg.CredsUser == "" ||
		cfg.SentinelPath == "" || cfg.Realm == "" || cfg.Account == "" || cfg.Issuer == "" {
		return nil, errors.New("soulstream: every Config field is required")
	}
	sp := &Support{cfg: cfg, sh: sh,
		keyCache: map[string]string{}, cardCache: map[string]Card{},
		adminCl: &http.Client{Timeout: 10 * time.Second}}

	var err error
	sp.nc, err = nats.Connect(cfg.NATSURL, nats.UserCredentials(cfg.CredsPath),
		nats.MaxReconnects(-1), nats.ReconnectWait(300*time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("soulstream: read lane: %w", err)
	}
	sp.rc, err = realm.NewClient(ctx, sp.nc, realm.Config{Realm: cfg.Realm})
	if err != nil {
		sp.nc.Close()
		return nil, fmt.Errorf("soulstream: record client: %w", err)
	}
	sp.dir = siclient.New(sp.nc, cfg.Account, cfg.CredsUser)

	sh.Attach(sessionKey{}, sp)
	return sp, nil
}

// Close drains the read lane. The sessions go with the shell that holds
// them.
func (sp *Support) Close() {
	if sp.nc != nil {
		sp.nc.Close()
	}
}

// Reader is the surface's own read lane: no persona, no signer. It is what
// a module reads with when it is not acting for anybody, and it never
// writes.
func (sp *Support) Reader() *realm.Client { return sp.rc }

// AdminSurface is where this deployment administers the people who can sign
// in, "" when it administers them nowhere this surface reaches. It is a
// declared deployment fact and nothing else: a module asks it to learn
// whether it is part of this deployment at all, and asking is the whole of
// what that costs — no probe, no round-trip, no configuration of the
// shell's own.
func (sp *Support) AdminSurface() string { return sp.cfg.AdminBase }

// Session is the Soulstream side of the session this request carries, nil
// when it carries none.
func (sp *Support) Session(r *http.Request) *Session {
	sess := sp.sh.Session(r)
	if sess == nil {
		return nil
	}
	s, _ := sess.Attached(sessionKey{}).(*Session)
	return s
}

// SignedIn opens one person's own admission: the public sentinel plus the
// bearer the issuer gave them, through the callout lane, and a client that
// signs as their persona. Delegated authority, never borrowed identity —
// the surface signs as no one.
//
// The bearer rides a handler rather than a copy: the server bumps this
// connection whenever the callout-minted credential expires, and each
// reconnect must present the token the shell holds NOW — the one from
// sign-in dies within the hour, and re-presenting it is how a session
// used to rot live. A shell that cannot produce one hands up the empty
// string, which the callout refuses: fail closed, and the session's own
// eviction does the rest.
func (sp *Support) SignedIn(ctx context.Context, sh *shell.Session) (any, error) {
	bearerNow := func() string {
		b, err := sh.Bearer()
		if err != nil {
			return ""
		}
		return b
	}
	nc, err := nats.Connect(sp.cfg.NATSURL,
		nats.UserCredentials(sp.cfg.SentinelPath), nats.TokenHandler(bearerNow),
		nats.MaxReconnects(-1), nats.ReconnectWait(300*time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("admission (oidc lane): %w", err)
	}
	// The name to put on screen starts as whatever the issuer said. An
	// issuer that says nothing leaves the id standing in, and ScreenName
	// keeps asking the persona directory until somebody publishes one.
	sess := &Session{Persona: sh.Subject, sp: sp, nc: nc, name: sh.Subject,
		bearer: sh.Bearer,
		unread: map[string]map[string]bool{}, seen: map[string]bool{}}
	if sh.Name != "" {
		sess.name, sess.named = sh.Name, true
	}
	cfg := realm.Config{Realm: sp.cfg.Realm, Persona: sh.Subject}
	if signer, serr := siclient.New(nc, sp.cfg.Account, sh.Subject).
		PersonaSigner(sh.Subject); serr == nil {
		cfg.Signer = signer
		sess.Signed = true
	}
	rc, err := realm.NewClient(ctx, nc, cfg)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("soulstream client: %w", err)
	}
	sess.rc = rc
	// This person's own inbox, followed over this person's own connection,
	// for as long as they are signed in.
	fctx, stop := context.WithCancel(context.Background())
	sess.stop = stop
	go sess.followInbox(fctx)
	return sess, nil
}

// SignedOut drains one person's admission.
func (sp *Support) SignedOut(v any) {
	if sess, ok := v.(*Session); ok {
		sess.close()
	}
}

// SignInServing reports whether the sign-in surface answers — the house
// readout, nothing the surface depends on to work.
func (sp *Support) SignInServing() bool {
	cl := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := cl.Get(sp.cfg.Issuer + "/.well-known/openid-configuration")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}

// Keyring is the verification keyring for one materialised conversation:
// every voice in it, resolved against the identity plane's open directory
// and cached. A verdict is earned, not defaulted.
func (sp *Support) Keyring(mt *topic.MaterializedTopic) *identity.Keyring {
	if mt == nil {
		return &identity.Keyring{Keys: map[string][]string{}}
	}
	var authors []string
	for _, c := range mt.Contributions {
		authors = append(authors, c.Author)
	}
	for _, w := range mt.WorkItems {
		authors = append(authors, w.Author)
	}
	return sp.KeyringFor(authors...)
}

// KeyringFor is the same earned keyring for a set of personas named
// directly — for a reader holding ops rather than a materialised
// conversation. Duplicates and empty names cost nothing; a persona the
// directory cannot answer for is simply absent, which reads downstream as
// unknown-key rather than as a failure.
func (sp *Support) KeyringFor(personas ...string) *identity.Keyring {
	kr := &identity.Keyring{Keys: map[string][]string{}}
	for _, p := range personas {
		if p == "" || kr.Keys[p] != nil {
			continue
		}
		if k, ok := sp.personaKey(p); ok {
			kr.Keys[p] = []string{k}
		}
	}
	return kr
}

// personaKey resolves and caches a persona's public key from the identity
// plane's open directory.
func (sp *Support) personaKey(persona string) (string, bool) {
	sp.mu.Lock()
	if k, ok := sp.keyCache[persona]; ok {
		sp.mu.Unlock()
		return k, true
	}
	sp.mu.Unlock()
	k, err := sp.dir.PersonaPublicKey(persona)
	if err != nil {
		return "", false
	}
	sp.mu.Lock()
	sp.keyCache[persona] = k
	sp.mu.Unlock()
	return k, true
}
