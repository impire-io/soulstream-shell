// A shell module written from outside the component (the research topic's
// Bar 4, second half: outside pluggability). Its module path is NOT under
// github.com/impire-io/soulstream-shell — it is not even in the impire-io
// namespace — so the Go toolchain itself refuses this package any internal/
// import of the shell's, and the only surface it can compile against is the
// exported one. The .invalid TLD never resolves: this module is never
// tagged, never published, and exists to be composed by the gate beside it.
module soulstream-shell.invalid/moduleprobe

go 1.26.2

require github.com/impire-io/soulstream-shell v0.2.0

require (
	github.com/coreos/go-oidc/v3 v3.20.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
)

// Only in force when this module is built on its own; composed by the gate,
// the gate's own replace of the shell is what applies.
replace github.com/impire-io/soulstream-shell => ../..
