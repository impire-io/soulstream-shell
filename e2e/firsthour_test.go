// The first hour, walked: a fresh house leads with the first-steps card,
// each furnishing act flips its own step on the next render with nothing
// stored anywhere, and when everything is done the card is gone — design
// 0008's acceptance walk, end to end over real screens. The roster's
// Around column rides beside it: an honest dash for a voice the realm
// has never seen, and a live channel the screen opens on its own.
package e2e

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-idp/authtest"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

// pollHome reads /home until it contains (or, contains == false, stops
// containing) the given substring — the realm's reads are eventually
// fresh, and the card derives from them, so the walk waits on the house
// rather than on luck.
func pollHome(t *testing.T, cl *http.Client, base, want string, contains bool) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last string
	for {
		last = get(t, cl, base+"/home")
		if strings.Contains(last, want) == contains {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("home never settled on %q=%v:\n%s", want, contains, last)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func TestFirstHourGate(t *testing.T) {
	r, err := rig.Start(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		t.Fatal(err)
	}
	cl := signIn(t, r, auth)

	// A fresh house: the card leads with every step pending, each a door —
	// the longer sentence on the hover, the on-screen act's door opening
	// the slide-over it leads into (the calm pass).
	home := get(t, cl, r.ShellURL+"/home")
	for _, want := range []string{"First steps",
		`<a href="/agents" title="`, `>Set up your assistant</a>`,
		`<a href="#convo-start" title="`, `>Start a conversation</a>`,
		`<a href="/tools" title="`, `>Connect a tool</a>`,
		`<a href="/people" title="`, `>Invite someone</a>`} {
		if !strings.Contains(home, want) {
			t.Fatalf("a fresh home is missing %q:\n%s", want, home)
		}
	}

	// Each act flips exactly its own step on the next render — derived
	// from the realm, with no store anywhere that could disagree. A done
	// step is the filled dot beside its muted words.
	postForm(t, cl, r.ShellURL+"/act/agent-add",
		url.Values{"handle": {"scribe"}, "shown": {"Scribe"}})
	home = pollHome(t, cl, r.ShellURL, `</span>Set up your assistant</li>`, true)
	if !strings.Contains(home, `>Start a conversation</a>`) {
		t.Fatal("an unrelated step flipped with the agent's")
	}

	postForm(t, cl, r.ShellURL+"/act/conversation-start",
		url.Values{"name": {"hello"}, "about": {"the first one"}})
	pollHome(t, cl, r.ShellURL, `</span>Start a conversation</li>`, true)

	postForm(t, cl, r.ShellURL+"/act/tool-add", url.Values{
		"name": {"notes"}, "kind": {"workload"}, "persona": {"notes-tool"},
		"endpoint": {"http://127.0.0.1:9999/mcp"}, "description": {"the notes tool"}})
	pollHome(t, cl, r.ShellURL, `</span>Connect a tool</li>`, true)

	postForm(t, cl, r.ShellURL+"/act/person-add",
		url.Values{"username": {"librarian"}, "shown": {"Librarian"}, "groups": {"realm"}})

	// Everything done: no card, and nothing to dismiss — it derives away.
	pollHome(t, cl, r.ShellURL, "First steps", false)

	// The roster's Around column: the page opens its own live channel,
	// the never-seen voice gets a dash rather than a guess, and the live
	// route re-serves the table.
	screen := get(t, cl, r.ShellURL+"/agents")
	for _, want := range []string{"<th>Around</th>",
		`data-init="@get('/agents/live')"`, ">—</span>"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the roster is missing %q:\n%s", want, screen)
		}
	}
	live := readSSE(t, cl, r.ShellURL+"/agents/live", 1500*time.Millisecond)
	if !strings.Contains(live, `id="agents-table"`) {
		t.Fatalf("the live channel does not carry the table:\n%s", live)
	}
}
