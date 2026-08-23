package shell

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// The morph plumbing: how a module gets a fragment onto a screen that is
// already open. The shell owns the wire format so every module patches the
// same way, and owns none of the fragments.

// WriteElements frames one datastar-patch-elements event. Every line of the
// fragment gets its own data line: a raw newline ends an SSE field, so a
// fragment written as one line would reach the browser truncated at its
// first line break.
func WriteElements(w io.Writer, frag string, opts ...string) {
	fmt.Fprint(w, "event: datastar-patch-elements\n")
	for _, o := range opts {
		fmt.Fprintf(w, "data: %s\n", o)
	}
	for line := range strings.SplitSeq(frag, "\n") {
		fmt.Fprintf(w, "data: elements %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

// Patch writes one patch frame as a whole response, optionally with patch
// options ("mode replace"). Several frames may ride one response; the first
// write settles the content type.
func Patch(w http.ResponseWriter, frag string, opts ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	WriteElements(w, frag, opts...)
}

// PatchSignals writes one datastar-patch-signals frame — for an act whose
// answer includes the page putting something away, a slide-over mostly. It
// rides the same response as the element patches beside it; the first
// write settles the content type.
func PatchSignals(w http.ResponseWriter, signals string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, "event: datastar-patch-signals\n")
	fmt.Fprintf(w, "data: signals %s\n", signals)
	fmt.Fprint(w, "\n")
}

// Script answers an act whose outcome is the browser doing something —
// navigating, mostly — rather than an element changing. The bundle runs
// a text/javascript response as a page script; nothing on screen is
// patched, so an act that answers this way patches nothing else.
func Script(w http.ResponseWriter, js string) {
	w.Header().Set("Content-Type", "text/javascript")
	fmt.Fprint(w, js)
}

// Stream turns a response into a patch channel and calls tick every
// interval until the person navigates away. The module writes whatever
// fragments its screen is made of; the shell flushes the tick and holds the
// channel open.
func Stream(w http.ResponseWriter, r *http.Request, every time.Duration, tick func(io.Writer)) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", http.StatusInternalServerError)
		return
	}
	for {
		tick(w)
		fl.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(every):
		}
	}
}
