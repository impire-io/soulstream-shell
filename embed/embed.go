// Package embed is soulhelm's public composition seam (the D29
// pattern): one assembly, value-only options, run and drain. Soulnode
// composes the helm plane through this; `soulhelm serve` is the seam's
// other consumer. Nothing else in the module is importable — the
// implementation lives under internal/.
package embed

import (
	"context"

	"github.com/impire-io/soulhelm/internal/helmserver"
)

// Options mirrors helmserver.Options — the deployment supplies every
// surface the helm consumes; the helm founds and owns nothing.
type Options struct {
	// Listen is the loopback HTTP address for the helm surface.
	Listen string
	// NATSURL reaches the realm's server.
	NATSURL string
	// CredsPath and CredsUser are the helm's own read lane.
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
	// Ready, when set, receives the bound listen address.
	Ready func(addr string)
}

// Run starts the helm and blocks until ctx is canceled or the surface
// fails. Returned means drained: sessions closed, connections gone.
func Run(ctx context.Context, o Options) error {
	return helmserver.Run(ctx, helmserver.Options{
		Listen: o.Listen, NATSURL: o.NATSURL,
		CredsPath: o.CredsPath, CredsUser: o.CredsUser,
		SentinelPath: o.SentinelPath,
		Realm:        o.Realm, Account: o.Account,
		Issuer: o.Issuer, Ready: o.Ready,
	})
}
