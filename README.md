# soulhelm — the helm

The soulsystem's human cockpit: one browser surface where a person
observes the whole running system — topics, work, signatures, plane
health — and acts in it, beside the MCP door that serves machines.
Design and journey live in the ecosystem hq
([impire-io/soul-hq](https://github.com/impire-io/soul-hq), design doc
`02-DESIGN/soulhelm/0001-soulhelm-the-helm.md`).

Founding articles, held by the standing e2e gate:

- **Pure consumer.** The helm is built exclusively on public, tagged
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
  signature. The helm signs as no one.

## Run

As a soulnode plane (the default distribution): `planes.helm` — see
soulnode. Standalone:

```
soulhelm -listen 127.0.0.1:8500 -nats nats://127.0.0.1:4222 \
  -creds <read-lane.creds> -creds-user <name> -sentinel <sentinel.creds> \
  -realm home -account <realm-account-pub> -issuer <fold-or-AS-url>
```

The helm registers itself with the issuer via RFC 7591 DCR at startup.

## Develop

`make check` — fmt, tidy, build, test (including the consumer-position
e2e, which boots a real soulnode realm at its published tag), lint.
All green before every commit; none skipped.

## License

[Sustainable Use License](LICENSE) (fair-code). Use, modify, and
self-host freely for internal or non-commercial purposes; offering
soulhelm to others as a paid product or service requires an agreement.
See [impire.io/license](https://impire.io/license/).
