package shell

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// countingSource stands in for the issuer's refresh grant: every renewal
// mints a fresh token and is counted, so a test can tell a renewal from a
// re-read of the one already held.
type countingSource struct{ renewals int }

func (c *countingSource) Token() (*oauth2.Token, error) {
	c.renewals++
	return &oauth2.Token{AccessToken: fmt.Sprintf("renewed-%d", c.renewals),
		Expiry: time.Now().Add(time.Hour)}, nil
}

// deadSource is an issuer that no longer honours the grant.
type deadSource struct{}

func (deadSource) Token() (*oauth2.Token, error) {
	return nil, fmt.Errorf("invalid_grant")
}

// drain counts what the shell closes, to pin that an ended session is
// drained like a sign-out, never abandoned.
type drain struct{ closed int }

func (d *drain) SignedIn(context.Context, *Session) (any, error) { return d, nil }
func (d *drain) SignedOut(any)                                   { d.closed++ }

func live(tok string) *oauth2.Token {
	return &oauth2.Token{AccessToken: tok, Expiry: time.Now().Add(time.Hour)}
}

func spent(tok string) *oauth2.Token {
	return &oauth2.Token{AccessToken: tok, Expiry: time.Now().Add(-time.Minute)}
}

// The bearer renews behind a living session: a spent access token is
// replaced through the refresh grant without the person noticing, the
// renewal is kept, and a token still living is handed out as-is with no
// round-trip.
func TestTheBearerRenewsBehindTheSession(t *testing.T) {
	src := &countingSource{}
	sess := &Session{tok: spent("first"), renew: src}
	b, err := sess.Bearer()
	if err != nil || b != "renewed-1" {
		t.Fatalf("Bearer() = %q, %v — want the renewed token", b, err)
	}
	if b, _ := sess.Bearer(); b != "renewed-1" || src.renewals != 1 {
		t.Errorf("second read = %q after %d renewals, want the held one after 1",
			b, src.renewals)
	}
	alive := &Session{tok: live("still-good"), renew: src}
	if b, _ := alive.Bearer(); b != "still-good" || src.renewals != 1 {
		t.Errorf("a living token renewed anyway: %q, %d renewals", b, src.renewals)
	}
}

// A session lives exactly as long as a credential can be produced for it.
// When nothing renews the spent token — the issuer gave no refresh token,
// or no longer honours the one it gave — the session is over: the request
// reads as signed out, what hung off the session is drained exactly once,
// and the shell forgets it. A session whose token still renews stays, and
// so does one whose token is simply not spent yet.
func TestASessionEndsWhenItsCredentialDoes(t *testing.T) {
	cases := []struct {
		name  string
		tok   *oauth2.Token
		renew oauth2.TokenSource
		stays bool
	}{
		{"spent with nothing to renew it", spent("gone"), nil, false},
		{"spent and the issuer refuses", spent("gone"), deadSource{}, false},
		{"spent but renewable", spent("gone"), &countingSource{}, true},
		{"still living", live("good"), nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestShell(t)
			d := &drain{}
			s.Attach(struct{}{}, d)
			sess := &Session{tok: tc.tok, renew: tc.renew,
				attached: map[any]any{struct{}{}: d}}
			s.sessions["sid"] = sess
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.AddCookie(&http.Cookie{Name: "test_session", Value: "sid"})
			got := s.Session(r)
			if tc.stays {
				if got != sess {
					t.Fatal("a living session read as signed out")
				}
				if d.closed != 0 {
					t.Errorf("a living session was drained %d times", d.closed)
				}
				return
			}
			if got != nil {
				t.Fatal("a session with no credential left still answers")
			}
			if d.closed != 1 {
				t.Errorf("the ended session was drained %d times, want once", d.closed)
			}
			if len(s.sessions) != 0 {
				t.Error("the shell still holds the ended session")
			}
			// The same cookie stays signed out, and nothing drains twice.
			if s.Session(r) != nil || d.closed != 1 {
				t.Error("the ended session came back, or was drained again")
			}
		})
	}
}
