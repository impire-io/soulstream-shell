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
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/identity"
	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/registry"
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

	mu        sync.Mutex
	sessions  map[string]*session
	keyCache  map[string]string    // persona -> public key (directory reads)
	nameCache map[string]nameEntry // persona -> on-screen name (registry reads)

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
	s := &Server{opts: opts, sessions: map[string]*session{},
		keyCache: map[string]string{}, nameCache: map[string]nameEntry{}}

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
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /live", s.live)
	mux.HandleFunc("GET /login", s.login)
	mux.HandleFunc("GET /callback", s.callback)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("POST /act/work-open", s.actWorkOpen)
	mux.HandleFunc("POST /act/post-turn", s.actPostTurn)
	mux.HandleFunc("GET /composer/reply", s.composerReply)

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

// nameEntry is one cached on-screen name. A persona the directory does not
// name yet is remembered too, briefly, so a missing profile costs one read a
// minute rather than one a second — and still appears when it is published.
type nameEntry struct {
	name  string
	found bool
	at    time.Time
}

// displayName resolves a persona's on-screen name from the realm's own
// persona directory (design 0001 §6 — the shell owns the id→name mapping on
// screen). A persona with no published profile keeps the id the record
// carries; a directory that does not answer is not an error here.
func (s *Server) displayName(ctx context.Context, persona string) string {
	s.mu.Lock()
	e, ok := s.nameCache[persona]
	s.mu.Unlock()
	if ok && (e.found || time.Since(e.at) < time.Minute) {
		return e.name
	}
	e = nameEntry{name: persona, at: time.Now()}
	if p, found, err := registry.Lookup(ctx, s.rc, persona); err == nil && found &&
		p.DisplayName != "" {
		e.name, e.found = p.DisplayName, true
	}
	s.mu.Lock()
	s.nameCache[persona] = e
	s.mu.Unlock()
	return e.name
}

// namesFor resolves every voice in a topic once per render.
func (s *Server) namesFor(ctx context.Context, mt *topic.MaterializedTopic) map[string]string {
	names := map[string]string{}
	add := func(p string) {
		if p == "" || names[p] != "" {
			return
		}
		names[p] = s.displayName(ctx, p)
	}
	if mt != nil {
		for _, c := range mt.Contributions {
			add(c.Author)
		}
		for _, w := range mt.WorkItems {
			add(w.Author)
		}
	}
	return names
}

// view is one render of the whole observed state.
type view struct {
	Realm string
	// Me is the signed-in person's own principal, taken from their session
	// and never from the request: it is what decides whose messages are
	// theirs. "" when signed out.
	Me string
	// Names maps a persona to the name shown for it on screen.
	Names     map[string]string
	Board     []topic.BoardEntry
	Topic     *topic.MaterializedTopic
	TopicPath string
	StreamMsg uint64
	StreamMB  float64
	FoldOK    bool
	Err       string
}

func (s *Server) observe(ctx context.Context, topicPath string, sess *session) view {
	v := view{Realm: s.opts.Realm, TopicPath: topicPath, Names: map[string]string{}}
	if sess != nil {
		v.Me = sess.Persona
		v.Names[sess.Persona] = sess.Display
	}
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
	for p, n := range s.namesFor(ctx, v.Topic) {
		if v.Names[p] == "" {
			v.Names[p] = n
		}
	}
	return v
}

// health fills in the house readouts. Only the system-status screen asks
// for them: the conversation re-renders once a second and has no business
// probing the sign-in surface that often.
func (s *Server) health(ctx context.Context, v *view) {
	if info, err := s.rc.JetStream().Stream(ctx, "SOULSTREAM"); err == nil {
		if si, err := info.Info(ctx); err == nil {
			v.StreamMsg = si.State.Msgs
			v.StreamMB = float64(si.State.Bytes) / (1 << 20)
		}
	}
	v.FoldOK = probe(s.opts.Issuer + "/.well-known/openid-configuration")
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
