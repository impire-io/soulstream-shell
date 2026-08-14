package shell

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
)

//go:embed assets
var assetsFS embed.FS

// Assets is the embedded static tree served under /assets/ — the
// design-system token source, vendored fonts, the Datastar bundle, and
// the icon set. Fully self-contained: the shell makes no external
// fetches (the offline render gate).
func Assets() fs.FS {
	sub, _ := fs.Sub(assetsFS, "assets")
	return sub
}

var (
	iconLicense = regexp.MustCompile(`(?s)<!--.*?-->\s*`)
	iconBreaks  = regexp.MustCompile(`\s+`)
	icons       = map[string]template.HTML{}
)

func init() {
	entries, _ := assetsFS.ReadDir("assets/icons")
	for _, e := range entries {
		raw, err := assetsFS.ReadFile("assets/icons/" + e.Name())
		if err != nil {
			continue
		}
		// The vendored width and height stay on: an SVG carrying only a
		// viewBox grows to the width of whatever holds it, and .btn svg is
		// the only rule in the token source that would stop it.
		svg := iconLicense.ReplaceAllString(string(raw), "")
		// One line each: icons ride SSE frames a second at a time.
		svg = strings.TrimSpace(iconBreaks.ReplaceAllString(svg, " "))
		name := e.Name()[:len(e.Name())-len(".svg")]
		icons[name] = template.HTML(svg) // #nosec G203 -- vendored static SVGs, embedded at build
	}
}

// Icon returns the inlined SVG for a vendored icon name ("" if absent).
// Modules name an icon; the set itself is the shell's, so a screen cannot
// reach for a mark the design does not have.
func Icon(name string) template.HTML { return icons[name] }

// favicon answers the request every browser makes on its own. The bytes
// are vendored beside the rest of the assets — the shell fetches nothing.
func favicon(w http.ResponseWriter, r *http.Request) {
	raw, err := assetsFS.ReadFile("assets/favicon.ico")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(raw)
}
