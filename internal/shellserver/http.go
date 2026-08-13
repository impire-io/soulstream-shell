package shellserver

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

func listen(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

// page renders the shell. The whole surface is behind sign-in: an
// unauthenticated visitor gets the sign-in card, nothing of the realm
// (design 0001 §6 — the cockpit is not an open window).
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	sess := s.currentSession(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if sess == nil {
		fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>shell — soulsystem</title><link rel="stylesheet" href="/assets/tokens.css"></head>
<body><main style="max-width:420px;margin:10vh auto 0;padding:0 var(--space-6)">
<div style="display:flex;align-items:center;gap:var(--space-4);margin-bottom:var(--space-6)">
<span class="led"></span><span class="tbar-wordmark" style="font-family:var(--font-core);font-weight:var(--weight-bold);font-size:22px;letter-spacing:-.035em;font-variation-settings:'wdth' 88">soulsystem</span>
<span class="strip">shell</span></div>
<div class="card raised"><h1>Sign in</h1>
<p class="lede">The cockpit shows your realm — sign in with your passkey.</p>
<p style="margin-top:var(--space-6)"><a class="btn" style="border-bottom:none" href="/login">%sSign in with the fold</a></p>
</div><p class="foot">soulsystem · shell · realm %s</p></main></body></html>`,
			Icon("power"), esc(s.opts.Realm))
		return
	}
	topicPath := r.URL.Query().Get("topic")
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>shell — soulsystem</title><link rel="stylesheet" href="/assets/tokens.css">
<script type="module" src="/assets/datastar.js"></script></head>
<body data-init="@get('/live?topic=%s')">
<header class="tbar"><span class="wordmark">soulsystem</span><span class="strip">shell</span>
<span class="strip shell">realm · %s</span><span class="spacer"></span>
<span class="who">%s</span><span class="led"></span></header>
<main style="max-width:var(--content-max);margin:0 auto;padding:var(--space-8)">
<div id="dash">loading…</div>
<p style="margin-top:var(--space-7)">
<button data-on:click="@post('/act/work-open?topic=%s')">Open work item</button>
<form method="post" action="/logout" style="display:inline"><button class="btn ghost">Sign out</button></form>
</p>
<div id="result">—</div>
<p class="foot">soulsystem · shell · your data lives in the realm, not here</p>
</main></body></html>`,
		esc(topicPath), esc(s.opts.Realm), esc(sess.Display), esc(topicPath))
}

// live is the Datastar SSE channel: the observed state re-rendered and
// morphed into #dash. Session-gated like every realm-bearing surface.
func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	if s.currentSession(r) == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", http.StatusInternalServerError)
		return
	}
	topicPath := r.URL.Query().Get("topic")
	for {
		v := s.observe(r.Context(), topicPath, "")
		fmt.Fprintf(w, "event: datastar-patch-elements\ndata: elements %s\n\n", s.renderDash(v))
		fl.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func patch(w http.ResponseWriter, frag string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "event: datastar-patch-elements\ndata: elements %s\n\n", frag)
}

// actWorkOpen is a class-(a) mutation: an op on the record through the
// session's own admitted connection, attributed and (when the persona
// key materialized) signed as the signed-in principal.
func (s *Server) actWorkOpen(w http.ResponseWriter, r *http.Request) {
	sess := s.currentSession(r)
	if sess == nil {
		patch(w, `<div id="result">no session — sign in first</div>`)
		return
	}
	topicPath := r.URL.Query().Get("topic")
	if topicPath == "" {
		v := s.observe(r.Context(), "", "")
		topicPath = v.TopicPath
	}
	if topicPath == "" {
		patch(w, `<div id="result">no topic to open work on</div>`)
		return
	}
	id, err := topicOpenWork(r.Context(), sess, topicPath)
	if err != nil {
		patch(w, fmt.Sprintf(`<div id="result">work.open as %s refused: %s</div>`,
			esc(sess.Display), esc(err.Error())))
		return
	}
	patch(w, fmt.Sprintf(`<div id="result">work.open ok · %s · by %s (signed=%v)</div>`,
		esc(id), esc(sess.Display), sess.Signed))
}
