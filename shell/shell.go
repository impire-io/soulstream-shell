// Package shell is a pure browser shell: sign-in, sessions, the frame every
// screen hangs in, and the registry every human surface plugs into as a
// module (module.go).
//
// It knows nothing about what its modules show. The shell's packages import
// no module and no component a module reads — the agnosticism is
// compiler-grade and checked mechanically from the import graph, not by eye
// (see ../internal/purity). What it owns is composition: generic OIDC
// sign-in with the bearer held in memory and nowhere else, the page chrome
// (top bar, icon rail, the design assets), the Datastar/SSE plumbing every
// module patches through, and the module contract itself.
//
// Modules import the shell; the shell imports no module. What a build runs
// and what those modules are handed lives above both (see ../embed).
package shell

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Options is everything the shell itself needs. Every field points at
// something the deployment already runs or already decided; the shell
// founds nothing and names nothing of its own.
type Options struct {
	// Listen is the loopback HTTP address for the surface.
	Listen string
	// Issuer is the OIDC authorization server people sign in through. The
	// shell registers itself there via RFC 7591 DCR at startup — no
	// pre-provisioned client.
	Issuer string
	// ClientName is what the shell calls itself when it registers.
	ClientName string
	// SessionCookie is the name the browser holds a session by.
	SessionCookie string
	// Home is where the shell sends somebody who has nowhere else to be.
	Home string
	// Brand is what the shell says on screen about the product it frames.
	Brand Brand
	// Ready, when set, is called with the bound listen address once the
	// surface serves.
	Ready func(addr string)
}

// Brand is the handful of words the shell puts on screen about the product
// it frames. The shell has no product name of its own — every one of these
// comes from composition, which is what lets the same frame carry a
// different product without a line of it changing.
type Brand struct {
	// Wordmark is the product, Strip the part of it this surface is, and
	// Where the deployment it frames.
	Wordmark string
	Strip    string
	Where    string
	// SignIn is the lede on the sign-in card and Action is what the key
	// under it says.
	SignIn string
	Action string
	// Promise is the line at the foot of every screen that is not live.
	Promise string
}

// Shell is one shell: the modules registered on it, the sessions signed
// into it, and the surface it serves.
type Shell struct {
	opts Options

	mods  []Module
	live  []Module
	hooks []hook

	oidc *oidcRP

	mu       sync.Mutex
	sessions map[string]*Session

	httpSrv *http.Server
}

// New readies a shell. Nothing is bound yet: modules register and support
// layers attach first, and Run serves.
func New(opts Options) (*Shell, error) {
	if opts.Listen == "" || opts.Issuer == "" || opts.ClientName == "" ||
		opts.SessionCookie == "" || opts.Home == "" || opts.Brand.Wordmark == "" {
		return nil, errors.New("shell: listen, issuer, client name, session cookie, " +
			"home and a wordmark are all required")
	}
	return &Shell{opts: opts, sessions: map[string]*Session{}}, nil
}

// Register adds modules, in the order they should appear on the rail.
// Whether each one actually runs is asked at Run.
func (s *Shell) Register(mods ...Module) {
	s.mods = append(s.mods, mods...)
}

// Modules is the modules this shell is running — everything registered
// that this deployment activates. Empty until Run.
func (s *Shell) Modules() []Module {
	out := make([]Module, len(s.live))
	copy(out, s.live)
	return out
}

// Home is where the shell sends somebody who has nowhere else to be.
func (s *Shell) Home() string { return s.opts.Home }

// activate asks every registered module whether this deployment runs it,
// and mounts what each active one claims. A module that says no is nowhere:
// no key on the spine, no route — its paths answer like any other path
// nobody claimed.
func (s *Shell) activate(ctx context.Context, rt Router) {
	for _, m := range s.mods {
		if !m.Active(ctx) {
			continue
		}
		s.live = append(s.live, m)
		m.Mount(rt)
	}
}

// Run serves until ctx is canceled or the surface fails. Returned means
// drained: sessions closed, whatever hung off them closed with them.
func (s *Shell) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.opts.Listen)
	if err != nil {
		return fmt.Errorf("shell: listen: %w", err)
	}
	boundAddr := ln.Addr().String()
	if s.oidc, err = newOIDCRP(ctx, s.opts, boundAddr); err != nil {
		_ = ln.Close()
		return fmt.Errorf("shell: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(Assets())))
	mux.HandleFunc("GET /favicon.ico", favicon)
	mux.HandleFunc("GET /login", s.login)
	mux.HandleFunc("GET /callback", s.callback)
	mux.HandleFunc("POST /logout", s.logout)
	s.activate(ctx, mux)

	s.httpSrv = &http.Server{Addr: boundAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if s.opts.Ready != nil {
		s.opts.Ready(boundAddr)
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
