// What the compiler says about who knows whom, asked from outside the
// component (Bar 4, both halves).
//
// Two claims carry the cross-module facility, and neither survives review
// alone. One: a module that points at another module's screen does not
// import it — otherwise the link is not a facility, it is a dependency with
// a lookup in front of it, and the module being pointed at could never be
// left out of a build again. Two: the outside module compiles against the
// exported frame and nothing else — otherwise "the exported contract is
// enough" is a claim about a package nobody has ever built from outside.
//
// Both are read off the graph the compiler actually builds, the way
// internal/purity reads the shell's, and both carry a positive control: a
// check that cannot fire proves nothing about what it did not find.
package e2e

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

const (
	// component is the shell component's own module path, and namespace the
	// namespace every Soulstream component publishes under.
	component = "github.com/impire-io/soulstream-shell"
	namespace = "github.com/impire-io/"
	// framePkg is the one package of the component an outside module may
	// know: the exported frame.
	framePkg = component + "/shell"
	// probe is the outside module, by the module path it was compiled from —
	// which is not in the namespace above, so the toolchain refuses it any
	// internal/ package of the component's before any of this runs.
	probe = "soulstream-shell.invalid/moduleprobe"
)

// modules is every human surface this build composes.
var modules = []string{
	component + "/modules/overview",
	component + "/modules/conversations",
	component + "/modules/admin",
	component + "/modules/agents",
}

// deps is the whole transitive import graph of a package pattern, as the
// compiler sees it. It runs in this module, so every replace and every pin
// the gate itself builds against is the one in force.
func deps(t *testing.T, pattern string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pattern)
	cmd.Dir = "."
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("go list -deps %s: %v\n%s", pattern, err, ee.Stderr)
		}
		t.Fatalf("go list -deps %s: %v", pattern, err)
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

// No module imports another. What one module knows about another is a slug
// and a route name it hands the frame (the cross-link facility) — never a
// package, so a build that leaves one of them out still compiles, and the
// module doing the pointing cannot tell the difference.
func TestNoModuleImportsAnother(t *testing.T) {
	for _, m := range modules {
		graph := deps(t, m)
		for _, p := range graph {
			for _, other := range modules {
				if other != m && p == other {
					t.Errorf("%s imports %s", m, other)
				}
			}
		}
	}
	// The control: the same walk over the layer where the modules do meet.
	// Composition imports all three by name — if this walk cannot see a
	// module in a graph that certainly holds three, it cannot have been
	// looking above either.
	seen := 0
	for _, p := range deps(t, component+"/embed") {
		for _, m := range modules {
			if p == m {
				seen++
			}
		}
	}
	if seen != len(modules) {
		t.Fatalf("the walk found %d of the %d modules in the composition layer — "+
			"it cannot fire, so its verdict on the modules means nothing", seen, len(modules))
	}
}

// The outside module compiles against the exported frame alone. Not the
// module-support layer, not a module beside it, not a component of the
// ecosystem, and not (by module path, which the compiler enforces) anything
// internal to the shell.
func TestTheOutsideModuleCompilesAgainstTheFrameAlone(t *testing.T) {
	var carried, bad []string
	for _, p := range deps(t, probe) {
		if !strings.HasPrefix(p, namespace) {
			continue
		}
		carried = append(carried, p)
		if p != framePkg {
			bad = append(bad, p)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("the outside module reaches past the exported frame:\n\t%s",
			strings.Join(bad, "\n\t"))
	}
	if len(carried) != 1 {
		t.Fatalf("the outside module's graph holds %d packages of this ecosystem's, "+
			"want the frame and nothing else: %v", len(carried), carried)
	}
	// The control: the same walk over the rig beside this gate, which
	// composes the product through its embed seam and reaches half the
	// ecosystem doing it. It must object loudly.
	var hits []string
	for _, p := range deps(t, "./rig") {
		if strings.HasPrefix(p, namespace) && p != framePkg {
			hits = append(hits, p)
		}
	}
	if len(hits) == 0 {
		t.Fatal("the walk found nothing to object to in the gate itself — it cannot " +
			"fire, so its verdict on the outside module means nothing")
	}
	t.Logf("the outside module carries %v; the control fired on %d packages, e.g. %s",
		carried, len(hits), hits[0])
}
