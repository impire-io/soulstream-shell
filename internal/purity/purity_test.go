// Package purity holds the standing check that the shell is what it claims
// to be: a frame that imports no module, and nothing of the component its
// modules read — and, beside it, the check that what the whole component
// costs is only ever what was named openly.
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

// The workloads packages this component may reach, and only these: the
// declaration's own parser and validator, and the placement plane's submit
// and read (hq design soulstream-shell 0009 §5, the one new dependency
// named openly). What those two pull in behind them is their business; what
// this component reaches for is this component's, and it is this.
var workloadsAllowed = map[string]bool{
	"github.com/impire-io/soulstream-workloads/declaration": true,
	"github.com/impire-io/soulstream-workloads/fleet":       true,
}

// The inference packages this component may reach, and only these: the
// catalogue's published contract — the one codec, so the sheet's entry and
// the deployment's own are byte-identical — and the resolve-and-collect
// client whose Resolve is the Serving now reading (hq design
// soulstream-shell 0010 §5, the one new dependency named openly).
var inferenceAllowed = map[string]bool{
	"github.com/impire-io/soulstream-inference/catalogue": true,
	"github.com/impire-io/soulstream-inference/client":    true,
}

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

// The other bar this component keeps: a dependency is a thing somebody
// argued for, so the packages it reaches for are the ones that were argued
// for and no others. Measured on the import edges this repo's own packages
// write — what those reach in turn is theirs to decide, and pinning it here
// would be pinning somebody else's design.
func TestTheComponentReachesOnlyTheWorkloadsPackagesItNamed(t *testing.T) {
	cmd := exec.Command("go", "list", "-f",
		"{{.ImportPath}}{{range .Imports}} {{.}}{{end}}", "./...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./...: %v\n%s", err, exitOutput(err))
	}
	var bad []string
	reached := map[string]bool{}
	pkgs := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkgs++
		for _, imported := range fields[1:] {
			if !strings.HasPrefix(imported, "github.com/impire-io/soulstream-workloads") {
				continue
			}
			reached[imported] = true
			if !workloadsAllowed[imported] {
				bad = append(bad, fields[0]+" imports "+imported)
			}
		}
	}
	if pkgs == 0 {
		t.Fatal("go list ./... returned nothing — the check would pass vacuously")
	}
	if len(bad) > 0 {
		t.Fatalf("this component reaches past the dependency it named:\n\t%s",
			strings.Join(bad, "\n\t"))
	}
	// The control: a check on a dependency nobody uses proves nothing, so
	// both named packages must actually be reached.
	for want := range workloadsAllowed {
		if !reached[want] {
			t.Fatalf("nothing here imports %s — the check cannot fire, so its verdict "+
				"on the rest means nothing", want)
		}
	}
	t.Logf("the component reaches %d workloads package(s) across %d packages", len(reached), pkgs)
}

// The same bar for the other named dependency: the inference packages this
// component reaches are the ones design 0010 argued for and no others,
// with the same reached-ness control — a check on a dependency nobody uses
// proves nothing.
func TestTheComponentReachesOnlyTheInferencePackagesItNamed(t *testing.T) {
	cmd := exec.Command("go", "list", "-f",
		"{{.ImportPath}}{{range .Imports}} {{.}}{{end}}", "./...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./...: %v\n%s", err, exitOutput(err))
	}
	var bad []string
	reached := map[string]bool{}
	pkgs := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkgs++
		for _, imported := range fields[1:] {
			if !strings.HasPrefix(imported, "github.com/impire-io/soulstream-inference") {
				continue
			}
			reached[imported] = true
			if !inferenceAllowed[imported] {
				bad = append(bad, fields[0]+" imports "+imported)
			}
		}
	}
	if pkgs == 0 {
		t.Fatal("go list ./... returned nothing — the check would pass vacuously")
	}
	if len(bad) > 0 {
		t.Fatalf("this component reaches past the dependency it named:\n\t%s",
			strings.Join(bad, "\n\t"))
	}
	for want := range inferenceAllowed {
		if !reached[want] {
			t.Fatalf("nothing here imports %s — the check cannot fire, so its verdict "+
				"on the rest means nothing", want)
		}
	}
	t.Logf("the component reaches %d inference package(s) across %d packages", len(reached), pkgs)
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
