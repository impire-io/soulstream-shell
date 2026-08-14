package shellserver

import (
	"strings"
	"testing"
)

// A fragment that spans lines must reach the browser whole: an SSE field
// ends at the first newline, so each line needs its own data line.
func TestWriteElementsFramesEveryLine(t *testing.T) {
	var b strings.Builder
	writeElements(&b, "<div id=\"x\">\n  <svg />\n</div>", "mode replace")
	want := "event: datastar-patch-elements\n" +
		"data: mode replace\n" +
		"data: elements <div id=\"x\">\n" +
		"data: elements   <svg />\n" +
		"data: elements </div>\n\n"
	if b.String() != want {
		t.Fatalf("frame =\n%q\nwant\n%q", b.String(), want)
	}
}

// The icons ride those frames once a second; each is kept to one line.
func TestIconsAreOneLine(t *testing.T) {
	if len(icons) == 0 {
		t.Fatal("no icons embedded")
	}
	for name, svg := range icons {
		if strings.Contains(string(svg), "\n") {
			t.Errorf("icon %s spans lines: %q", name, svg)
		}
	}
}

// An icon that carries only a viewBox grows to the width of whatever
// holds it, and .btn svg is the only rule that would stop it.
func TestIconsCarryTheirSize(t *testing.T) {
	for name, svg := range icons {
		if !strings.Contains(string(svg), `width="24"`) ||
			!strings.Contains(string(svg), `height="24"`) {
			t.Errorf("icon %s has no intrinsic size: %q", name, svg)
		}
	}
}

// The composer's three pieces are three patch targets: a one-shot act
// response and the live stream must never write the same element.
func TestComposerTargetsAreDistinct(t *testing.T) {
	page := renderComposer("home/topic")
	for _, id := range []string{`id="composer"`, `id="composer-box"`, `id="composer-note"`, `id="reply-to"`} {
		if strings.Count(page, id) != 1 {
			t.Errorf("composer carries %s %d times, want 1", id, strings.Count(page, id))
		}
	}
	if strings.Contains(page, `id="dash"`) || strings.Contains(page, `id="result"`) {
		t.Error("the composer writes into a target the live stream owns")
	}
	if !strings.Contains(page, "contentType:'form'") {
		t.Error("the composer must post itself as form data — it holds no client state")
	}
}
