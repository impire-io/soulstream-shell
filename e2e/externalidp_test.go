// The other arm of Bar 2: the same product, the same shell, the same three
// registered modules — deployed without a sign-in plane of its own.
//
// Sessions here ride an authorization server standing entirely outside the
// product: a fold run standalone through its own public embed seam, on its
// own store, which this deployment only knows the URL of. That is the whole
// of the difference. The person signing in carries the same groups, admin
// included, so nothing about the arms differs except the shape of the
// deployment — and the module that administers people is nevertheless not
// part of this build at all.
package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-idp/authtest"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

func TestExternalIdPGate(t *testing.T) {
	r, err := rig.StartExternalIdP(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	annexe := seed(t, r)

	// The deployment says so itself, before anything is asked of it: there
	// is no administration surface here, because the people signing in are
	// not this node's to administer.
	if r.AdminBase != "" {
		t.Fatalf("the deployment declared an administration surface at %q", r.AdminBase)
	}
	if r.State.FoldEnabled {
		t.Fatal("the arm is not external: the deployment runs a sign-in plane of its own")
	}
	if r.Issuer == r.State.FoldIssuer {
		t.Fatalf("the arm is not external: sessions sign in against %q", r.Issuer)
	}

	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	// The whole human ceremony against somebody else's authorization
	// server: an invite it minted, a passkey it enrolled, a session the
	// shell opened from the token it issued.
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		t.Fatal(err)
	}
	cl := signIn(t, r, auth)

	// The module is nowhere. Not on the spine —
	landed := get(t, cl, r.ShellURL+"/")
	for _, absent := range []string{`href="/people`, "People &amp; sign-in"} {
		if strings.Contains(landed, absent) {
			t.Fatalf("the rail offers %q in a deployment that administers nobody: %s",
				absent, landed)
		}
	}
	// — and not on any of its paths, which answer like any path nobody
	// claimed rather than refusing, because nothing here claimed them.
	for _, gone := range []struct {
		method, path string
	}{
		{http.MethodGet, "/people"},
		// Including the path a link from another screen would have pointed
		// at, had one resolved.
		{http.MethodGet, "/people?who=" + ceremony.FoundingPersona},
		{http.MethodPost, "/act/invite?who=" + ceremony.FoundingPersona},
		{http.MethodPost, "/act/disable?who=" + ceremony.FoundingPersona},
	} {
		if got := statusOf(t, cl, gone.method, r.ShellURL+gone.path); got != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404", gone.method, gone.path, got)
		}
	}

	// Everything else is the product a person came for. The two modules
	// this deployment does run are untouched by the absence of the third:
	// the house at a glance, the readouts behind it.
	overview := get(t, cl, r.ShellURL+"/home")
	for _, want := range []string{"Your soulstream at a glance", "Storage",
		"Conversations", "helm-gate", `class="ir on" href="/home`} {
		if !strings.Contains(overview, want) {
			t.Fatalf("the overview is missing %q: %s", want, overview)
		}
	}
	if !strings.Contains(get(t, cl, r.ShellURL+"/status"), "Storage") {
		t.Fatal("the system-status screen did not survive the missing sign-in plane")
	}

	// The conversation reads, over a session admitted from a token this
	// deployment did not issue.
	live := readSSE(t, cl, r.ShellURL+"/live", 3*time.Second)
	thread := elementsIn(frameFor(t, patchFrames(t, live), `id="dash"`))
	if !strings.Contains(thread, seededTurn) {
		t.Fatalf("the conversation is empty:\n%s", thread)
	}

	// Bar 4, cross-linking, the arm where there is nowhere to point. The same
	// panel, rendered by the same module, asking the same question of the
	// frame — and this build runs nothing that administers a sign-in, so the
	// ask comes back empty and every name is plain text. Not a greyed-out
	// control, not a link to a 404: the panel simply says who is in the room,
	// the way it did before any of this existed.
	side := elementsIn(frameFor(t, patchFrames(t, live), `id="details"`))
	if !strings.Contains(side, `<span class="who" title="@`+ceremony.FoundingPersona+`">`) {
		t.Fatalf("the People panel does not name the voice in the room:\n%s", side)
	}
	if strings.Contains(side, "<a ") {
		t.Fatalf("the panel points at a screen this deployment does not run:\n%s", side)
	}

	// And it writes: the message lands on the record as the signed-in
	// principal, signed with that principal's own key.
	const said = "posted with the sign-in plane switched off"
	if got, err := r.Post(cl, "", url.Values{"body": {said}}); err != nil ||
		!strings.Contains(got, "Posted as") {
		t.Fatalf("the composer did not post: %v %s", err, got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rc, rnc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	entries, err := topic.Board(ctx, rc)
	if err != nil || len(entries) == 0 {
		t.Fatalf("board: %v (%d)", err, len(entries))
	}
	path := entries[len(entries)-1].Path
	mt := verified(ctx, t, rc, rnc, r.State.RealmPub, path)
	posted := find(mt, said)
	if posted == nil {
		t.Fatalf("the message never reached the record: %+v", mt.Contributions)
	}
	if posted.Author == "" || posted.Author == ceremony.FoundingPersona {
		t.Fatalf("message author = %q — not the signed-in principal", posted.Author)
	}
	if posted.Sig != topic.SigVerified {
		t.Fatalf("message signature = %q, want %q", posted.Sig, topic.SigVerified)
	}

	// Mentions too: somebody says this person's name in a room they are not
	// standing in, and the spine counts it.
	avery, err := r.Voice(ctx, "avery", "Avery")
	if err != nil {
		t.Fatal(err)
	}
	ah := topic.Open(avery, annexe)
	if _, err := ah.Materialise(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := ah.PostTurnMentioning(ctx, "@you a word when you have one",
		[]string{posted.Author}); err != nil {
		t.Fatal(err)
	}
	chat := r.ShellURL + "/live?topic=" + url.QueryEscape(path)
	tapped := waitFor(t, cl, chat, `id="mentions"`, `class="tally on"`, 20*time.Second)
	if tally := frameWith(t, tapped, `id="mentions"`); !strings.Contains(tally, ">1</span>") {
		t.Fatalf("the spine counts the wrong number of waiting messages: %s", tally)
	}

	// Sign out closes the session here as it does anywhere.
	if _, err := cl.Post(r.ShellURL+"/logout", "", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(get(t, cl, r.ShellURL+"/"), "Sign out") {
		t.Fatal("session survived logout")
	}
	fmt.Printf("external-IdP gate: ceremony against an authorization server outside " +
		"the product, conversations and mentions green, and no people module anywhere\n")
}
