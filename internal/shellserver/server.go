// Package shellserver is the shell's engine: the soulsystem's human
// cockpit, rendered server-side over a backend-held realm client and
// pushed to the browser as Datastar SSE patches (design 0001 §5). The
// shell is a pure consumer of public component surfaces and custodies
// nothing durable — sessions live in memory, credentials never reach
// the browser, and the store of record stays the realm's.
package shellserver

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/identity"
	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"
	siclient "github.com/impire-io/soulstream-identity/client"
)

// Options configures a shell. The shell never founds or owns anything:
// every field points at surfaces the deployment already runs.
type Options struct {
	// Listen is the loopback HTTP address for the shell surface.
	Listen string
	// NATSURL reaches the realm's server.
	NATSURL string
	// CredsPath is the shell's own read lane — an ordinary creds file
	// the deployment supplies (a soulnode plane hands its ops lane).
	CredsPath string
	// CredsUser is the principal name that creds file connects as; the
	// identity-plane directory reads ride its own prefix.
	CredsUser string
	// SentinelPath is the public sentinel creds file sessions present
	// beside their fold-issued bearer for their own admission.
	SentinelPath string
	// Realm is the realm name.
	Realm string
	// Account is the realm account public key (the identity plane's
	// account segment).
	Account string
	// Issuer is the OIDC authorization server (the bundled fold by
	// default); the shell registers itself via RFC 7591 DCR at startup.
	Issuer string
	// Ready, when set, is called with the bound listen address once
	// the surface serves.
	Ready func(addr string)
}

// Server is one running shell.
type Server struct {
	opts Options

	nc  *nats.Conn
	rc  *realm.Client
	dir *siclient.Client

	oidcState *oidcRP

	mu       sync.Mutex
	sessions map[string]*session
	keyCache map[string]string // persona -> public key (directory reads)

	httpSrv *http.Server
}

// Run starts the shell and blocks until ctx is canceled or the surface
// fails. Returned means drained: sessions closed, connections gone.
func Run(ctx context.Context, opts Options) error {
	if opts.Listen == "" || opts.NATSURL == "" || opts.Realm == "" ||
		opts.CredsPath == "" || opts.CredsUser == "" ||
		opts.SentinelPath == "" || opts.Account == "" || opts.Issuer == "" {
		return errors.New("shell: every Options field except Ready is required")
	}
	s := &Server{opts: opts, sessions: map[string]*session{}, keyCache: map[string]string{}}

	var err error
	s.nc, err = nats.Connect(opts.NATSURL, nats.UserCredentials(opts.CredsPath),
		nats.MaxReconnects(-1), nats.ReconnectWait(300*time.Millisecond))
	if err != nil {
		return fmt.Errorf("shell: read lane: %w", err)
	}
	defer s.nc.Close()
	s.rc, err = realm.NewClient(ctx, s.nc, realm.Config{Realm: opts.Realm})
	if err != nil {
		return fmt.Errorf("shell: realm client: %w", err)
	}
	s.dir = siclient.New(s.nc, opts.Account, opts.CredsUser)

	ln, err := listen(opts.Listen)
	if err != nil {
		return fmt.Errorf("shell: listen: %w", err)
	}
	boundAddr := ln.Addr().String()
	if s.oidcState, err = newOIDCRP(ctx, opts, boundAddr); err != nil {
		_ = ln.Close()
		return fmt.Errorf("shell: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(Assets())))
	mux.HandleFunc("GET /{$}", s.page)
	mux.HandleFunc("GET /live", s.live)
	mux.HandleFunc("GET /login", s.login)
	mux.HandleFunc("GET /callback", s.callback)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("POST /act/work-open", s.actWorkOpen)

	s.httpSrv = &http.Server{Addr: boundAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if opts.Ready != nil {
		opts.Ready(boundAddr)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.httpSrv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shCtx)
		s.closeAllSessions()
		return nil
	case err := <-errCh:
		s.closeAllSessions()
		return err
	}
}

// personaKey resolves and caches a persona's public key from the
// identity plane's open directory; the verdict is earned, not
// defaulted (design 0001 §3).
func (s *Server) personaKey(persona string) (string, bool) {
	s.mu.Lock()
	if k, ok := s.keyCache[persona]; ok {
		s.mu.Unlock()
		return k, true
	}
	s.mu.Unlock()
	k, err := s.dir.PersonaPublicKey(persona)
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	s.keyCache[persona] = k
	s.mu.Unlock()
	return k, true
}

func (s *Server) keyringFor(mt *topic.MaterializedTopic) *identity.Keyring {
	kr := &identity.Keyring{Keys: map[string][]string{}}
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		if k, ok := s.personaKey(p); ok {
			kr.Keys[p] = []string{k}
		}
	}
	for _, c := range mt.Contributions {
		add(c.Author)
	}
	for _, w := range mt.WorkItems {
		add(w.Author)
	}
	return kr
}

// view is one render of the whole observed state.
type view struct {
	Realm     string
	Who       string // "" when signed out
	Board     []topic.BoardEntry
	Topic     *topic.MaterializedTopic
	TopicPath string
	StreamMsg uint64
	StreamMB  float64
	FoldOK    bool
	Err       string
}

func (s *Server) observe(ctx context.Context, topicPath, who string) view {
	v := view{Realm: s.opts.Realm, Who: who, TopicPath: topicPath}
	entries, err := topic.Board(ctx, s.rc)
	if err != nil {
		v.Err = fmt.Sprintf("board: %v", err)
		return v
	}
	v.Board = entries
	if v.TopicPath == "" && len(entries) > 0 {
		v.TopicPath = entries[len(entries)-1].Path
	}
	if v.TopicPath != "" {
		th := topic.Open(s.rc, v.TopicPath)
		if mt, err := th.Materialise(ctx); err == nil {
			th.UseKeyring(s.keyringFor(mt))
			if mt2, err := th.Materialise(ctx); err == nil {
				mt = mt2
			}
			v.Topic = mt
		} else {
			v.Err = fmt.Sprintf("topic %s: %v", v.TopicPath, err)
		}
	}
	if info, err := s.rc.JetStream().Stream(ctx, "SOULSTREAM"); err == nil {
		if si, err := info.Info(ctx); err == nil {
			v.StreamMsg = si.State.Msgs
			v.StreamMB = float64(si.State.Bytes) / (1 << 20)
		}
	}
	v.FoldOK = probe(s.opts.Issuer + "/.well-known/openid-configuration")
	return v
}

func probe(url string) bool {
	cl := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := cl.Get(url)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}

func esc(s string) string { return html.EscapeString(s) }

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func sigMark(v topic.SigStatus) string {
	switch v {
	case topic.SigVerified:
		return `<span class="pill ok">verified</span>`
	case topic.SigUnknownKey:
		return `<span class="pill">unknown-key</span>`
	case topic.SigUnsigned:
		return `<span class="pill">unsigned</span>`
	default:
		return `<span class="pill warn">` + esc(string(v)) + `</span>`
	}
}

// renderDash renders the #dash fragment — the whole observed state in
// the cassette-light language, plain words on every label (C8).
func (s *Server) renderDash(v view) string {
	var b strings.Builder
	b.WriteString(`<div id="dash">`)
	if v.Err != "" {
		fmt.Fprintf(&b, `<p class="lede">%s</p>`, esc(v.Err))
	}
	// The plane cards.
	b.WriteString(`<div class="planes">`)
	fmt.Fprintf(&b, `<div class="card plane"><div class="head">%s<h2>Storage</h2></div>`+
		`<div class="row"><span class="pill ok"><span class="led machine"></span>keeping</span>`+
		`<span class="mono">%d ops · %.1f MB</span></div></div>`,
		Icon("cassette-tape"), v.StreamMsg, v.StreamMB)
	fold := `<span class="pill warn">unreachable</span>`
	if v.FoldOK {
		fold = `<span class="pill ok"><span class="led"></span>serving</span>`
	}
	fmt.Fprintf(&b, `<div class="card plane"><div class="head">%s<h2>People &amp; sign-in</h2></div>`+
		`<div class="row">%s<span class="mono">passkeys</span></div></div>`,
		Icon("key"), fold)
	open, claimed := 0, 0
	if v.Topic != nil {
		for _, w := range v.Topic.WorkItems {
			switch string(w.Status) {
			case "open":
				open++
			case "claimed":
				claimed++
			}
		}
	}
	fmt.Fprintf(&b, `<div class="card plane"><div class="head">%s<h2>Work</h2></div>`+
		`<div class="row"><span class="mono">open %d · claimed %d</span></div></div>`,
		Icon("activity"), open, claimed)
	b.WriteString(`</div>`)

	// Topics table.
	b.WriteString(`<div class="section"><div class="eyebrow">` + string(Icon("disc-3")) +
		`<span class="strip shell">active topics</span></div><div class="tablewrap"><table>` +
		`<thead><tr><th>Topic</th><th>Lifecycle</th></tr></thead><tbody>`)
	for _, e := range v.Board {
		cur := ""
		if e.Path == v.TopicPath {
			cur = " ▸"
		}
		fmt.Fprintf(&b, `<tr><td><a href="/?topic=%s"><b>%s</b></a>%s<br><span class="mono">%s</span></td><td class="mono">%s</td></tr>`,
			esc(e.Path), esc(e.Announcement.Name), cur, esc(e.Path), esc(string(e.Lifecycle)))
	}
	b.WriteString(`</tbody></table></div></div>`)

	// The selected topic.
	if v.Topic != nil {
		fmt.Fprintf(&b, `<div class="section"><div class="eyebrow">%s<span class="strip shell">latest activity · %s</span></div><div class="screen">`,
			Icon("database"), esc(v.TopicPath))
		for _, c := range v.Topic.Contributions {
			fmt.Fprintf(&b, `<p><span class="dim">%s</span> %s · %s %s</p>`,
				c.Timestamp.Format("15:04:05"), esc(trunc(c.Body, 80)), esc(c.Author), sigMark(c.Sig))
		}
		for _, w := range v.Topic.WorkItems {
			fmt.Fprintf(&b, `<p><span class="dim">%s</span> work %q · %s · %s</p>`,
				w.Timestamp.Format("15:04:05"), esc(w.Title), esc(string(w.Status)), esc(w.Owner))
		}
		b.WriteString(`<p class="dim">— recording —</p></div></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
