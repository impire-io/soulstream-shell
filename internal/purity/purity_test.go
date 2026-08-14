// Package purity holds the standing check that the shell is what it claims
// to be: a frame that imports no module, and nothing of the component its
// modules read.
//
// It is mechanical on purpose. "The shell is agnostic" is exactly the kind
// of claim that rots by eye — one convenient import, and the frame quietly
// belongs to one product forever. So it is measured against the import
// graph the compiler actually builds (go list -deps over every package
// under shell/), it runs with the rest of the tests, and it fails the build
// the day it stops being true.
package purity

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

const (
	// module is this repo's own module path.
	module = "github.com/impire-io/soulstream-shell"
	// namespace is the component namespace the shell must stay clear of —
	// every Soulstream component publishes under it.
	namespace = "github.com/impire-io/"
)

// deps is the whole transitive import graph of a package pattern, as the
// compiler sees it: not what the files say, what the build pulls in.
func deps(t *testing.T, pattern string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pattern)
	cmd.Dir = "../.." // the module root, whatever the test's own directory is
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pattern, err, exitOutput(err))
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	if len(paths) == 0 {
		t.Fatalf("go list -deps %s returned nothing — the check would pass vacuously", pattern)
	}
	return paths
}

func exitOutput(err error) []byte {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.Stderr
	}
	return nil
}

// forbidden says why a dependency has no business in the shell, or "" when
// it is fair game. Two rules: no module and no module-support layer of this
// repo's own, and nothing at all from the component namespace.
func forbidden(path string) string {
	switch {
	case strings.HasPrefix(path, module+"/modules/"):
		return "a module"
	case path == module+"/soulstream" || strings.HasPrefix(path, module+"/soulstream/"):
		return "the Soulstream module-support layer"
	case path == module+"/embed" || strings.HasPrefix(path, module+"/embed/"):
		return "the composition layer"
	case strings.HasPrefix(path, module):
		return "" // the shell's own packages
	case strings.HasPrefix(path, namespace):
		return "a Soulstream component"
	}
	return ""
}

// external drops the standard library: a path whose first element carries a
// dot is somebody's domain, everything else is Go's own.
func external(path string) bool {
	head, _, _ := strings.Cut(path, "/")
	return strings.Contains(head, ".")
}

// The bar: the shell's packages import no module and no component. Measured
// from the graph, not from the import blocks — a package three hops down
// that reaches for a component would be caught here and nowhere else.
func TestTheShellImportsNoModuleAndNoComponent(t *testing.T) {
	var carried, bad []string
	for _, p := range deps(t, "./shell/...") {
		if external(p) {
			carried = append(carried, p)
		}
		if why := forbidden(p); why != "" {
			bad = append(bad, p+" — "+why)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("the shell is not agnostic; its import graph reaches:\n\t%s",
			strings.Join(bad, "\n\t"))
	}
	// The whole of what a pure shell costs, printed so the proof is
	// readable rather than merely green.
	t.Logf("the shell's non-stdlib import graph (%d):\n\t%s",
		len(carried), strings.Join(carried, "\n\t"))
}

// The positive control. A check that cannot fail proves nothing, so the
// same walk runs over the layer built to hold exactly what the shell may
// not touch — the Soulstream module-support layer — and is required to
// report violations there.
func TestTheCheckFiresOnSomethingImpure(t *testing.T) {
	var hits []string
	for _, p := range deps(t, "./soulstream/...") {
		if why := forbidden(p); why != "" {
			hits = append(hits, p+" — "+why)
		}
	}
	if len(hits) == 0 {
		t.Fatal("the check found nothing to object to in the Soulstream module-support " +
			"layer — it cannot fire, so its verdict on the shell means nothing")
	}
	t.Logf("control fired on %d dependencies, e.g. %s", len(hits), hits[0])
}
