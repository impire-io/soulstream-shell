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
seeded conversation — two other voices, answers on both sides, and work
waiting, in hand and finished, so the details panel has something real
to say — and a signed-in session, so the surface can be looked at
rather than read about. It prints the address and the session cookie
and blocks until `^C`; nothing outlives the temp dir it founds.

## License

[Sustainable Use License](LICENSE) (fair-code). Use, modify, and
self-host freely for internal or non-commercial purposes; offering
soulstream-shell to others as a paid product or service requires an agreement.
See [impire.io/license](https://impire.io/license/).
