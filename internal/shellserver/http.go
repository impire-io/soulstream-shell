package shellserver

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/impire-io/soulstream-core/topic"
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
%s
<p style="margin-top:var(--space-7)">
<button data-on:click="@post('/act/work-open?topic=%s')">Open work item</button>
<form method="post" action="/logout" style="display:inline"><button class="btn ghost">Sign out</button></form>
</p>
<div id="result">—</div>
<p class="foot">soulsystem · shell · your data lives in the realm, not here</p>
</main></body></html>`,
		qesc(topicPath), esc(s.opts.Realm), esc(sess.Display),
		renderComposer(topicPath), qesc(topicPath))
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
		writeElements(w, s.renderDash(v))
		fl.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

// writeElements frames one datastar-patch-elements event. Every line of
// the fragment gets its own data line: a raw newline ends an SSE field,
// so a fragment written as one line would reach the browser truncated at
// its first line break.
func writeElements(w io.Writer, frag string, opts ...string) {
	fmt.Fprint(w, "event: datastar-patch-elements\n")
	for _, o := range opts {
		fmt.Fprintf(w, "data: %s\n", o)
	}
	for line := range strings.SplitSeq(frag, "\n") {
		fmt.Fprintf(w, "data: elements %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

// patch writes one patch frame, optionally with patch options ("mode
// replace"). Several frames may ride one response; the first write
// settles the content type.
func patch(w http.ResponseWriter, frag string, opts ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	writeElements(w, frag, opts...)
}

// resolveTopic falls back to the conversation the view itself defaults
// to when the caller names none.
func (s *Server) resolveTopic(ctx context.Context, want string) string {
	if want != "" {
		return want
	}
	return s.observe(ctx, "", "").TopicPath
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
	topicPath := s.resolveTopic(r.Context(), r.URL.Query().Get("topic"))
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

// actPostTurn is the composer's act: a message on the record through the
// session's own admitted connection, attributed and signed as the
// signed-in principal. The message itself reaches the view the ordinary
// way — the live stream's next morph — so only the composer's own
// targets are patched here.
func (s *Server) actPostTurn(w http.ResponseWriter, r *http.Request) {
	sess := s.currentSession(r)
	if sess == nil {
		patch(w, composerNote("Sign in first."))
		return
	}
	if err := r.ParseForm(); err != nil {
		patch(w, composerNote("That message could not be read."))
		return
	}
	body := strings.TrimSpace(r.PostFormValue("body"))
	if body == "" {
		patch(w, composerNote("Write a message first."))
		return
	}
	topicPath := s.resolveTopic(r.Context(), r.URL.Query().Get("topic"))
	if topicPath == "" {
		patch(w, composerNote("There is no conversation to post to."))
		return
	}
	id, err := topicSay(r.Context(), sess, topicPath, body, r.PostFormValue("reply-to"))
	if err != nil {
		patch(w, composerNote("Not posted — "+err.Error()))
		return
	}
	patch(w, composerBox(), "mode replace")
	patch(w, composerReplyTo("", ""))
	patch(w, composerNote("Posted as "+sess.Display+" · "+id))
}

// composerReply sets — or, with no op, clears — the message the composer
// answers. Only the anchor line is patched, so a half-written message
// stays where it is.
func (s *Server) composerReply(w http.ResponseWriter, r *http.Request) {
	if s.currentSession(r) == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	opID := r.URL.Query().Get("op")
	if opID == "" {
		patch(w, composerReplyTo("", ""))
		return
	}
	// The anchor is resolved against the record, never taken on the
	// browser's word: an op that is not in this conversation is no anchor.
	topicPath := s.resolveTopic(r.Context(), r.URL.Query().Get("topic"))
	mt, err := topic.Open(s.rc, topicPath).Materialise(r.Context())
	if err != nil {
		patch(w, composerNote("That conversation could not be read."))
		return
	}
	author, ok := contributionAuthor(mt, opID)
	if !ok {
		patch(w, composerReplyTo("", ""))
		return
	}
	patch(w, composerReplyTo(opID, author))
}
