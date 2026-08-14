// Package fits holds the standing check that nothing the shell serves is
// pinned wider than the narrowest screen it promises to render on.
//
// The promise is 360px. The measurement that actually settles it is taken in
// a browser at two viewports — document scrollWidth against innerWidth at
// 1000px and at 390px — and no test in this module can take it: there is no
// layout engine in `go test`, and standing one up would trade a hermetic
// gate for a flaky one.
//
// So this package asserts the part a hermetic test can honestly reach, which
// is the part that broke: a width written as a number. A column pinned at
// 420px, an inline style carrying a fixed width, a table with nowhere to
// scroll — each of those is a promise that the screen is at least that wide,
// and each is visible in the bytes the shell serves. What is left over
// (whether the whole composition adds up at 390px) is the browser's to say,
// and the review's.
//
// Test files are left out of the corpus on purpose: a test serves nothing,
// and the positive controls below are made of exactly the strings these
// checks look for.
package fits

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/impire-io/soulstream-shell/shell"
)

// narrowest is the screen the shell promises to render on whole. Below it,
// nothing is promised; at it and above, nothing may be cut off.
const narrowest = 360

// fixedWidth finds a width written as a number of pixels — the kind that
// cannot give. max-width is deliberately not matched: a cap is a promise
// about how wide something may grow, never about how much room it needs, and
// `@media (max-width:…)` is how the steps below are written in the first
// place.
var fixedWidth = regexp.MustCompile(`(^|[;{\s"'])(?:min-)?width\s*:\s*(\d+)px`)

// tooWide is every fixed width in a piece of CSS that is wider than the
// narrowest screen, in the order they appear.
func tooWide(css string) []string {
	var out []string
	for _, m := range fixedWidth.FindAllStringSubmatch(css, -1) {
		if px, err := strconv.Atoi(m[2]); err == nil && px > narrowest {
			out = append(out, strings.TrimLeft(m[0], " \t\n;{\"'"))
		}
	}
	return out
}

// The token source is the only stylesheet the shell serves, so it is the
// only place a column can be pinned. Read from where the shell serves it
// rather than from disk: what is asserted is what a browser gets.
func TestTheTokenSourcePinsNothingWiderThanTheNarrowestScreen(t *testing.T) {
	css, err := fs.ReadFile(shell.Assets(), "tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	if bad := tooWide(string(css)); len(bad) > 0 {
		t.Errorf("the token source pins %d width(s) past %dpx:\n\t%s",
			len(bad), narrowest, strings.Join(bad, "\n\t"))
	}
	// The positive control. A check that cannot fail proves nothing, so the
	// same matcher runs over a rule written the way the failing one would be
	// and is required to object to it — and to leave the cap and the
	// breakpoint beside it alone, which are the two shapes it must not
	// mistake for a floor.
	control := ".column{width:420px;max-width:900px}@media (max-width:1180px){.x{width:8px}}"
	if got := tooWide(control); len(got) != 1 || !strings.HasPrefix(got[0], "width:420px") {
		t.Fatalf("the control fired on %v — the check cannot tell a floor from a cap", got)
	}
}

// inlineStyle finds a style attribute in served markup.
var inlineStyle = regexp.MustCompile(`style="[^"]*"`)

// tag finds the opening of a table.
var tableTag = regexp.MustCompile(`<table[\s>]`)

// scrollsInsideItself is how far back from a table the container that scrolls
// it may be written. Every one of them is on the same line as the table it
// wraps, so this is generous.
const scrollsInsideItself = 160

// sources is every Go file in the repo that could serve markup — both
// modules of it, minus the tests.
func sources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	root := "../.." // the module root, whatever the test's own directory is
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("no sources walked — the checks below would pass vacuously")
	}
	return out
}

// Markup carries no widths of its own. Every width on every screen comes
// from the one stylesheet checked above, which is what lets the steps there
// actually govern: a number written into a tag answers to no breakpoint.
func TestNoServedMarkupCarriesAFixedWidth(t *testing.T) {
	var bad []string
	for path, src := range sources(t) {
		for _, style := range inlineStyle.FindAllString(src, -1) {
			if len(tooWide(style)) > 0 {
				bad = append(bad, path+": "+style)
			}
		}
	}
	if len(bad) > 0 {
		t.Errorf("served markup pins a width past %dpx:\n\t%s",
			narrowest, strings.Join(bad, "\n\t"))
	}
	control := `<div class="x" style="width:900px;color:red">`
	if got := inlineStyle.FindAllString(control, -1); len(got) != 1 || len(tooWide(got[0])) != 1 {
		t.Fatalf("the control did not fire on %q", control)
	}
}

// A table is the one thing the shell serves that cannot always be made to
// fit: six columns of somebody else's names and ids do not narrow past a
// point. So every one of them scrolls inside its own box — which is the
// canon's answer, and the difference between a column a person can reach and
// a column clipped off the edge of the frame.
func TestEveryTableTheShellServesScrollsInsideItself(t *testing.T) {
	var found, bad []string
	for path, src := range sources(t) {
		for _, at := range tableTag.FindAllStringIndex(src, -1) {
			found = append(found, path)
			from := max(0, at[0]-scrollsInsideItself)
			if !strings.Contains(src[from:at[0]], "tablewrap") {
				bad = append(bad, path+": "+src[from:at[1]])
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no table found in anything served — the check passed vacuously")
	}
	if len(bad) > 0 {
		t.Errorf("%d table(s) with nowhere to scroll:\n\t%s", len(bad), strings.Join(bad, "\n\t"))
	}
	t.Logf("%d table(s) served, every one inside the container that scrolls it", len(found))
	// And the container has to actually scroll, or the class is decoration:
	// the shared layer clips it for the corner radius and the shell's own
	// layer hands the horizontal axis back.
	css, err := fs.ReadFile(shell.Assets(), "tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".tablewrap{overflow-x:auto") {
		t.Error("the container every table sits in does not scroll")
	}
}
