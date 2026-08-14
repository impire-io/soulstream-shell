package shell

import (
	"fmt"
	"html"
	"net/url"
)

// The few words and numbers the frame renders the same way everywhere, so
// two modules never spell the same thing two ways.

// Esc renders a value safe as text on a page.
func Esc(s string) string { return html.EscapeString(s) }

// QueryEsc renders a value safe both as a query parameter and inside the
// HTML attribute that carries it.
func QueryEsc(s string) string { return Esc(url.QueryEscape(s)) }

// SizeWords is a byte count as a person reads it: never more digits than
// the number has any use for, and a scale that reaches as far as a store's
// own budget does.
func SizeWords(n uint64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	case n < 100<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n < 1<<30:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
}
