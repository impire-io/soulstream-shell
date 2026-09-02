// The topics-surface arm of the gate (hq design soulstream-shell 0012;
// research topic the-topics-surface, bars 1, 3 and 4 — bar 2's mechanism is
// core spec 022's own measured ground): the conversations list hides the
// machinery and orders by life, a deep-opened room says whose it is, the
// agent detail's composer posts the mention that wakes, and a mention
// landing in a hidden room still reaches the person and leads somewhere a
// click can follow.
package e2e

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-idp/authtest"
	"github.com/impire-io/soulstream-workloads/fleet"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

const roomAgent = "clerk"

func TestTopicsSurfaceGate(t *testing.T) {
	r, _ := startRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	if r.Placements == "" {
		t.Fatal("this arm needs a deployment that places agents and this one declares none")
	}

	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		t.Fatal(err)
	}
	cl := signIn(t, r, auth)
	who := whoRe.FindStringSubmatch(get(t, cl, r.ShellURL+"/"))
	if who == nil {
		t.Fatal("the frame names nobody as signed in")
	}
	me := who[1]

	// A conversation of the person's own, alive.
	owner, err := r.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parlour, err := topic.StartTopic(ctx, owner, topic.StartTopicInput{
		Name: "the parlour", SubjectMatter: "people talking"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parlour.PostTurn(ctx, "tea is ready"); err != nil {
		t.Fatal(err)
	}

	// The machinery: one agent declared TWICE — submission is additive, so
	// both made homes are placements of record — and one declared with the
	// placements topic itself as its home (the dispatcher spec allows it).
	for i := 0; i < 2; i++ {
		if out := postForm(t, cl, r.ShellURL+"/act/agent-declare", url.Values{
			"name": {roomAgent}, "wake_mention": {"on"},
		}); !strings.Contains(out, roomAgent+" is declared") {
			t.Fatalf("declaring %s (round %d) did not land:\n%s", roomAgent, i+1, out)
		}
	}
	ppath, ok := placementsTopic(ctx, t, r)
	if !ok {
		t.Fatal("declaring never started the placements topic")
	}
	if out := postForm(t, cl, r.ShellURL+"/act/agent-declare", url.Values{
		"name": {"porter"}, "wake_mention": {"on"}, "home": {ppath},
	}); !strings.Contains(out, "porter is declared") {
		t.Fatalf("declaring porter on the placements topic did not land:\n%s", out)
	}

	// Bar 1 — the rail lists the person's conversation and none of the
	// rooms, held until the partition settles (the watch behind it follows
	// the placements topic; a freshly declared room may take a beat).
	rail := watchTarget(t, cl, r.ShellURL+"/live", `id="conversations"`,
		40*time.Second, nil, func(el string) bool {
			return strings.Contains(el, "the parlour") &&
				!strings.Contains(el, `<span class="name">`+roomAgent) &&
				!strings.Contains(el, `<span class="name">porter`) &&
				!strings.Contains(el, `<span class="name">`+r.Placements)
		})
	if !strings.Contains(rail, `class="conv on"`) {
		t.Fatalf("the person's conversation is not the open one:\n%s", rail)
	}

	// The Home list partitions the same way, from the same watch.
	holdFor(t, 30*time.Second, "the Home list hiding the rooms", func() bool {
		home := get(t, cl, r.ShellURL+"/home")
		return strings.Contains(home, ">the parlour</a>") &&
			!strings.Contains(home, ">"+roomAgent+"</a>") &&
			!strings.Contains(home, ">"+r.Placements+"</a>")
	})

	// Deep-opening a hidden room is honest: whose it is, said plainly, with
	// the way to the agent beside it — and the rail still does not list it.
	clerkHome := homeOf(ctx, t, r, roomAgent)
	note := watchTarget(t, cl, r.ShellURL+"/live?topic="+url.QueryEscape(clerkHome),
		`id="dash"`, 30*time.Second, nil, func(el string) bool {
			return strings.Contains(el, "own room")
		})
	if !strings.Contains(note, "/agents/room?who="+roomAgent) {
		t.Fatalf("the room note does not point at the agent:\n%s", note)
	}

	// Bar 3 — the detail: the declaration's facts in words, the honest
	// waiting (nothing serves agents on this rig), and the composer whose
	// message carries the agent's name.
	room := get(t, cl, r.ShellURL+"/agents/room?who="+roomAgent)
	for _, want := range []string{"How it runs", ">mentions</span>",
		"nothing serves agents here yet", "Talk to it", `id="room-box"`,
		"Read its whole room"} {
		if !strings.Contains(room, want) {
			t.Fatalf("the agent detail is missing %q:\n%s", want, room)
		}
	}
	said := postForm(t, cl, r.ShellURL+"/act/agent-say?who="+roomAgent,
		url.Values{"body": {"hello in there"}})
	if !strings.Contains(said, "Said, with its name on it") {
		t.Fatalf("saying did not answer:\n%s", said)
	}
	// The record carries the mention — the wake's own trigger — authored by
	// the person, on their own admission.
	rc, nc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	mt, err := topic.Open(rc, clerkHome).Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range mt.Contributions {
		if c.Body != "hello in there" {
			continue
		}
		found = true
		if c.Author != me {
			t.Fatalf("the room's turn is authored by %q, not the person (%q)", c.Author, me)
		}
		if !contains(c.Mentions, roomAgent) {
			t.Fatalf("the turn does not mention the agent — nothing would wake: %+v", c.Mentions)
		}
	}
	if !found {
		t.Fatal("the said turn never reached the room's record")
	}
	// And the room shows it live, no reload.
	watchTarget(t, cl, r.ShellURL+"/agents/room/live?who="+roomAgent,
		`id="agent-room"`, 30*time.Second, nil, func(el string) bool {
			return strings.Contains(el, "hello in there")
		})

	// Bar 4 — nothing dark: a mention lands in the hidden room, the rail
	// grows its honest pointer, the spine tally counts it, the agent's row
	// carries the mark, and the click lands on the message.
	smith, err := r.Voice(ctx, "smith", "")
	if err != nil {
		t.Fatal(err)
	}
	pointer := watchTarget(t, cl, r.ShellURL+"/live", `id="conversations"`,
		40*time.Second, func() {
			if _, err := topic.Open(smith, clerkHome).PostTurnMentioning(ctx,
				"the clerk needs a word @"+me, []string{me}); err != nil {
				t.Error(err)
			}
		}, func(el string) bool {
			return strings.Contains(el, "in an agent")
		})
	if !strings.Contains(pointer, "/agents/room?who=") {
		t.Fatalf("the rail's pointer leads nowhere:\n%s", pointer)
	}
	watchTarget(t, cl, r.ShellURL+"/live", `id="mentions"`,
		20*time.Second, nil, func(el string) bool {
			return strings.Contains(el, `class="tally on"`)
		})
	holdFor(t, 20*time.Second, "the agent's row carrying the mark", func() bool {
		return strings.Contains(get(t, cl, r.ShellURL+"/agents"), `class="tally on"`)
	})
	if !strings.Contains(get(t, cl, r.ShellURL+"/agents/room?who="+roomAgent),
		"the clerk needs a word") {
		t.Fatal("the click did not land on the message")
	}
}

// homeOf is the room an agent's newest declaration names, read off the
// placements topic with the package that owns the wire format.
func homeOf(ctx context.Context, t *testing.T, r *rig.Rig, persona string) string {
	t.Helper()
	path, ok := placementsTopic(ctx, t, r)
	if !ok {
		t.Fatal("no placements topic to read")
	}
	rc, nc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	mt, err := topic.Open(rc, path).Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	home := ""
	for _, item := range mt.WorkItems {
		if d, ok := fleet.DeclarationOf(item); ok && d.Persona == persona {
			home = d.Topic
		}
	}
	if home == "" {
		t.Fatalf("nothing declares %s", persona)
	}
	return home
}

// contains is a plain membership test.
func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// holdFor polls a condition to its deadline — for the screens that are
// served whole rather than streamed.
func holdFor(t *testing.T, d time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// watchTarget opens a live channel, optionally makes something happen while
// it is open, and reads until the named target's frame satisfies the
// predicate — the arrival measurement, taken the way a person takes it: by
// watching a screen they never reloaded.
func watchTarget(t *testing.T, cl *http.Client, u, targetID string, d time.Duration,
	meanwhile func(), ok func(el string) bool,
) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if meanwhile != nil {
		go func() {
			time.Sleep(500 * time.Millisecond)
			meanwhile()
		}()
	}

	var read strings.Builder
	var frame []string
	open := false
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		read.WriteString(line)
		read.WriteString("\n")
		switch {
		case line == "event: datastar-patch-elements":
			open, frame = true, []string{line}
		case open && line == "":
			open = false
			el := elementsIn(frame)
			if strings.Contains(el, targetID) && ok(el) {
				return el
			}
		case open:
			frame = append(frame, line)
		}
	}
	t.Fatalf("the live channel never satisfied the watch on %s; the stream said:\n%s",
		targetID, read.String())
	return ""
}
