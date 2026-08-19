// Package embed is soulstream-shell's public composition seam (the D29
// pattern): one assembly, value-only options, run and drain. Soulnode
// composes the shell plane through this; `soulstream-shell` standalone is
// the seam's other consumer.
//
// It is also the one place the pieces meet. The shell below it knows
// nothing of Soulstream; the modules beside it know nothing of each other
// or of how a deployment is put together. What a build runs, what the
// screens are called, and where every surface points is decided here.
package embed

import (
	"context"

	"github.com/impire-io/soulstream-shell/modules/admin"
	"github.com/impire-io/soulstream-shell/modules/agents"
	"github.com/impire-io/soulstream-shell/modules/conversations"
	"github.com/impire-io/soulstream-shell/modules/overview"
	"github.com/impire-io/soulstream-shell/modules/storage"
	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// Options is what the deployment supplies: every field points at a surface
// it already runs. The shell founds and owns nothing.
type Options struct {
	// Listen is the loopback HTTP address for the shell surface.
	Listen string
	// PublicURL is the origin browsers reach the surface on when the
	// deployment fronts the loopback listener — the OAuth callback is
	// built from it. Empty means the bound address is the reachable
	// origin (the bundle's default).
	PublicURL string
	// NATSURL reaches the realm's server.
	NATSURL string
	// CredsPath and CredsUser are the shell's own read lane.
	CredsPath string
	CredsUser string
	// SentinelPath is the public sentinel creds sessions present
	// beside their bearer for their own admission.
	SentinelPath string
	// Realm and Account name the realm and its account public key.
	Realm   string
	Account string
	// Issuer is the OIDC authorization server (the bundled fold by
	// default in soulnode).
	Issuer string
	// AdminBase is the base URL of the people-and-sign-in administration
	// surface this deployment runs — the sign-in plane's own when the
	// deployment runs one and signs its people in against it. A deployment
	// whose people live on an authorization server it does not run leaves
	// this empty, and the module that administers people is then not part
	// of that build at all: no key on the rail, no route, 404 like any path
	// nobody claimed.
	AdminBase string
	// AgentsDial is the address this deployment tells an agent to dial, and
	// the whole of how it says it issues agent credentials at all. A
	// deployment that leaves it empty runs no agents surface: no key on the
	// rail, no route, 404 like any path nobody claimed.
	//
	// It is deliberately not NATSURL. The address this surface reaches the
	// server on is the deployment's own business and may be one no agent
	// could use; what goes into somebody else's configuration file has to be
	// something the deployment is willing to stand behind.
	AgentsDial string
	// Ready, when set, receives the bound listen address.
	Ready func(addr string)
}

// sessionCookie is the name a browser holds a session by. It is composed in
// rather than named inside the shell: the frame has no product of its own,
// and the browsers of this deployment already hold this one.
const sessionCookie = "helm_session"

// Run starts the shell and blocks until ctx is canceled or the surface
// fails. Returned means drained: sessions closed, connections gone.
func Run(ctx context.Context, o Options) error {
	sh, err := shell.New(shell.Options{
		Listen:        o.Listen,
		PublicURL:     o.PublicURL,
		Issuer:        o.Issuer,
		ClientName:    "soulstream-shell",
		SessionCookie: sessionCookie,
		// The conversations are the front door: a person who signs in, or
		// signs out, or asks for a screen they cannot have, lands there.
		Home: "/",
		Brand: shell.Brand{
			Wordmark: "soulstream",
			Strip:    "shell",
			Where:    o.Realm,
			SignIn:   "This screen shows your soulstream — sign in with your passkey.",
			Action:   "Sign in with your passkey",
			Promise:  "your data lives in your soulstream, not here",
		},
		Ready: o.Ready,
	})
	if err != nil {
		return err
	}

	sp, err := soulstream.Open(ctx, sh, soulstream.Config{
		NATSURL:      o.NATSURL,
		CredsPath:    o.CredsPath,
		CredsUser:    o.CredsUser,
		SentinelPath: o.SentinelPath,
		Realm:        o.Realm,
		Account:      o.Account,
		Issuer:       o.Issuer,
		AdminBase:    o.AdminBase,
		AgentsDial:   o.AgentsDial,
	})
	if err != nil {
		return err
	}
	defer sp.Close()

	// The order is the order of the rail: the house first, then the room,
	// then the people who may come into it, then the machines they answer
	// for — and last, at the foot beside the readouts, the store itself.
	// Every one of them is registered the same way and on the same terms —
	// which of them this deployment actually runs is each module's own
	// answer, asked once at Run.
	sh.Register(overview.New(sh, sp), conversations.New(sh, sp),
		admin.New(sh, sp), agents.New(sh, sp), storage.New(sh, sp))
	return sh.Run(ctx)
}
