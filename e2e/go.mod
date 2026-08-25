module soulstream-shell.invalid/e2e

go 1.26.2

require (
	github.com/impire-io/soulstream v0.13.0-rc.10
	github.com/impire-io/soulstream-core v0.13.0
	github.com/impire-io/soulstream-identity v0.11.0
	github.com/impire-io/soulstream-idp v0.8.0
	github.com/impire-io/soulstream-shell v0.10.0
	github.com/nats-io/nats.go v1.52.0
	soulstream-shell.invalid/moduleprobe v0.0.0
)

require (
	cel.dev/expr v0.25.1 // indirect
	github.com/antithesishq/antithesis-sdk-go v0.7.2-default-no-op // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-oidc/v3 v3.20.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/go-chi/chi/v5 v5.3.1 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/webauthn v0.17.4 // indirect
	github.com/go-webauthn/x v0.2.6 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/cel-go v0.31.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/gowebpki/jcs v1.0.1 // indirect
	github.com/impire-io/soulstream-archivist v0.4.1 // indirect
	github.com/impire-io/soulstream-mcp v0.1.0 // indirect
	github.com/impire-io/soulstream-workloads v0.7.0 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/modelcontextprotocol/go-sdk v1.6.1 // indirect
	github.com/muhlemmer/gu v0.3.1 // indirect
	github.com/muhlemmer/httpforwarded v0.1.0 // indirect
	github.com/nats-io/jwt/v2 v2.8.2 // indirect
	github.com/nats-io/nats-server/v2 v2.14.4 // indirect
	github.com/nats-io/nkeys v0.4.16 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/synadia-io/control-plane-sdk-go v0.9.0 // indirect
	github.com/synadia-io/orbit.go/natscontext v0.1.3 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/zitadel/oidc/v3 v3.48.1 // indirect
	github.com/zitadel/schema v1.3.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/exp v0.0.0-20240823005443-9b4947da3948 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240826202546-f6391c0de4c7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240826202546-f6391c0de4c7 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

replace github.com/impire-io/soulstream-shell => ../

// TEMPORARY — remove when the fold ships a tag carrying the
// last-administrator rule, and the node ships built on that tag.
//
// The lockout arm of the gate measures a refusal the published fold does
// not have yet: that the last enabled administrator cannot be disabled
// or demoted. Nothing in the shell imports the fold — this module boots
// one, bundled inside the node — so without this pin the gate would
// measure the old behaviour and pass on it.
replace github.com/impire-io/soulstream-idp => ../../soulstream-idp

// The outside module the probe arm composes. Its own module, in a namespace
// nobody here owns, never tagged and never published — it is only ever the
// working tree beside this one.
replace soulstream-shell.invalid/moduleprobe => ./moduleprobe
