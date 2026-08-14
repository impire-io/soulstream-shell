# soulstream-shell — the shell

Soulstream's human cockpit: one browser surface where a person reads the
conversations of their realm and writes into them — a slim spine of
sections at the far left, the conversations beside it, one conversation
held to a reading measure in the middle with a composer docked under
it, and that conversation's details (who is in it, where it stands,
what anyone is waiting for) on the right — beside the MCP door that
serves machines. The house readouts (storage, sign-in, work) live on
the overview and the system-status screen, both one key away on the
spine.
Design and journey live in the ecosystem hq
([impire-io/soul-hq](https://github.com/impire-io/soul-hq), design doc
`02-DESIGN/soulstream-shell/0001-soulstream-shell-the-shell.md`).

Founding articles, held by the standing e2e gate:

- **Pure consumer.** The shell is built exclusively on public, tagged
  component surfaces — soulstream's realm and topic packages,
  soulidentity's client, any OIDC authorization server (the bundled
  soulfold by default). The e2e module's path sits outside this
  namespace, so an `internal/` import cannot compile.
- **Custodies nothing.** Sessions live in memory; credentials never
  reach the browser; the store of record stays the realm's. The e2e
  scans for credential-shaped leaks with a positive control.
- **Delegated authority, never borrowed identity.** Every act rides
  the signed-in person's own admission (sentinel + their fold-issued
  bearer through the OIDC callout lane) and their own persona
  signature. The shell signs as no one.
- **A frame anybody can build on.** The gate composes a module written
  in its own Go module, outside this namespace, importing the exported
  frame and nothing else ([`e2e/moduleprobe`](e2e/moduleprobe)): it
  registers, takes a key on the spine and renders a screen, on a frame
  carrying a product that is not this one — with no shell change to let
  it in.

## Shape

A pure shell, and modules beside it:

- [`shell/`](shell) — the frame, and nothing else: sign-in and sessions
  (generic OIDC, the bearer in memory and nowhere else), the page chrome
  (top bar, the spine, the design assets), the Datastar/SSE plumbing, and
  the module contract itself ([`shell/module.go`](shell/module.go) —
  identity, activation, navigation contribution, route mounting), with the
  one cross-module facility that lets separate modules feel like one
  product ([`shell/link.go`](shell/link.go): a module asks the frame, by
  slug and route, for a way into another module's screen; only the modules
  this deployment runs answer, and nothing else resolves). Its packages
  import no module and nothing Soulstream-specific, and the check is
  mechanical — the compiler's own import graph, riding `make test`
  ([`internal/purity`](internal/purity)).
- [`soulstream/`](soulstream) — the Soulstream module-support layer, where
  everything the shell refuses to hold lives: bearer → sentinel + callout →
  an admission that acts as the person, the clients, the persona directory,
  the mention tray.
- [`modules/overview`](modules/overview),
  [`modules/conversations`](modules/conversations) and
  [`modules/admin`](modules/admin) — the human surfaces, each registering
  through the one contract and importing none of the others (measured off
  the import graph in the gate). The third is the one that is not always
  there: a deployment whose people sign in against an authorization server
  it does not run has nobody here to administer, so that module is not part
  of the build at all. Where it is, a name in the conversation's People
  panel is a way into that person's sign-in — asked for through the frame,
  by slug and route, never by import; where it is not, the same name is
  plain text.
- [`embed/`](embed) — composition: the one place the pieces meet, and the
  only one that knows both that this is a shell and that this is
  Soulstream.

## Design

The canon is vendored at [`docs/design-canon.md`](docs/design-canon.md)
and implemented by `shell/assets/tokens.css`: the token
blocks and the shared component layer are verbatim from the design
system, and everything under `THE SHELL'S OWN COMPONENT LAYER` is this
repo's. Cassette futurism in a light key — molded panels, milled
lettering on coloured keys, printed scales, no gradients, no opacity as
a state, pills only for tags and badges.

**The two channels.** Amber is the human channel and teal the machine
channel, at deliberately equal weight. Every message carries the one it
belongs to, on the card's outer edge and in the lamp in its byline;
whose a message is is carried by which side it sits on and by nothing
else, so no colour anywhere says both.

Which channel a voice speaks on is read from the record, and the record
refuses the obvious question on purpose: soulstream removed the persona
`kind` field (human / agent / service) outright, because the protocol
cannot verify what controls a key. What it keeps instead is the
operator claim — `operated_by` plus the operator's countersignature —
so the shell reads accountability rather than species: a voice that
answers for itself is on the human channel, a voice somebody else
answers for is on the machine one. The seam and its limits are written
out in
[`modules/conversations/channel.go`](modules/conversations/channel.go);
the People panel always names the operator beside the voice, so the
claim is on the screen rather than implied by a shade of teal.

## Run

As a soulnode plane (the default distribution): `planes.shell` — see
soulnode. Standalone:

```
soulstream-shell -listen 127.0.0.1:8500 -nats nats://127.0.0.1:4222 \
  -creds <read-lane.creds> -creds-user <name> -sentinel <sentinel.creds> \
  -realm home -account <realm-account-pub> -issuer <fold-or-AS-url>
```

The shell registers itself with the issuer via RFC 7591 DCR at startup.

## Develop

`make check` — fmt, tidy, build, test (including the consumer-position
e2e, which boots a real soulnode realm at its published tag), lint.
All green before every commit; none skipped.

`make screens` stands the same rig up and leaves it running, with a
seeded conversation — three other voices, one of them a persona the
signed-in person operates and countersigns for, so both channels are on
the screen; answers on both sides; and work waiting, in hand and
finished, so the details panel has something real to say — and a
signed-in session, so the surface can be looked at rather than read
about. It prints the address and the session cookie and blocks until
`^C`; nothing outlives the temp dir it founds.

## License

[Sustainable Use License](LICENSE) (fair-code). Use, modify, and
self-host freely for internal or non-commercial purposes; offering
soulstream-shell to others as a paid product or service requires an agreement.
See [impire.io/license](https://impire.io/license/).
