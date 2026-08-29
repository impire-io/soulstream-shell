// The declare arm of the gate: a person declares an agent from the
// browser, it lands on the record as an ordinary placement carrying the
// exact document the command line takes, and the screen watches it go from
// waiting to taken-up — live, from the record's own evidence and nothing
// else. Design 0009's acceptance criteria 2 through 5, walked.
//
// Nothing is seeded on the surface's behalf. The declaration is built by
// filling in the form the browser posts, the placement is read back off the
// topic with the package that owns the wire format, and the claim that
// makes the row change is an ordinary work op by an ordinary persona —
// because that is all the screen can see, and asserting against a real
// dispatcher would measure the dispatcher rather than the screen.
package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-idp/authtest"
	infercat "github.com/impire-io/soulstream-inference/catalogue"
	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/fleet"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

// The declaration block the screen shows, as a test reads it back.
var declarationRe = regexp.MustCompile(
	`(?s)<textarea readonly rows="\d+" data-declaration>(.*?)</textarea>`)

const (
	declaredName  = "minutes"
	declaredModel = "house-brain"
)

// TestDeclareGate walks the whole lane: the form, the document, the
// placement, and the row that follows it.
func TestDeclareGate(t *testing.T) {
	r, _ := startRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if r.Placements == "" {
		t.Fatal("this arm is meant to declare a placement topic and declares none")
	}
	nameModel(ctx, t, r, declaredModel)

	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		t.Fatal(err)
	}
	cl := signIn(t, r, auth)
	home := openConversation(ctx, t, r)

	// Who the person declaring is, as the frame itself says it. The
	// placement this test is about ends up authored by exactly this.
	who := whoRe.FindStringSubmatch(get(t, cl, r.ShellURL+"/"))
	if who == nil {
		t.Fatal("the frame names nobody as signed in")
	}
	me := who[1]

	// The lane is on the screen and starts empty, saying what declaring is
	// and pointing at the act — criterion 7.
	screen := get(t, cl, r.ShellURL+"/agents")
	for _, want := range []string{"<h2>Declared agents</h2>", "None yet",
		"runs on this soulstream", ">Declare agent</button>",
		`id="agent-declare"`, "<summary>Models</summary>"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the declare lane is missing %q:\n%s", want, screen)
		}
	}
	// Criterion 5: the picker offers exactly the names the realm holds, and
	// the read-only list says the same names.
	if !strings.Contains(screen, `<option value="`+declaredModel+`">`) {
		t.Fatalf("the model picker does not offer the name this realm holds:\n%s", screen)
	}
	if !strings.Contains(screen, `<span class="pill mono">`+declaredModel+`</span>`) {
		t.Fatalf("the models list does not hold the name this realm holds:\n%s", screen)
	}
	// And no field anywhere asks for a provider's key; the line that loads
	// one is offered instead.
	if strings.Contains(screen, `type="password"`) {
		t.Fatal("the declare screen asks for a secret")
	}
	if !strings.Contains(screen, "soulstream provider set") {
		t.Fatal("the screen never says where a provider key goes")
	}

	// The form as a person leaves it.
	form := url.Values{
		"name": {declaredName}, "home": {home},
		"wake_mention": {"on"}, "model": {declaredModel},
		"budget_hops": {"4"}, "budget_max": {"8"}, "budget_per": {"10m"},
	}

	// Criterion 1's other half, on a real screen: the JSON view shows the
	// exact file the command line takes, and the package that owns the
	// format accepts it.
	shown := declarationInPatch(t, postForm(t, cl, r.ShellURL+"/agents/declare-json", form))
	parsed, err := declaration.Parse([]byte(shown))
	if err != nil {
		t.Fatalf("the document the screen shows is not one the command line takes: %v\n%s",
			err, shown)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("the document the screen shows is refused: %v\n%s", err, shown)
	}

	// A refusal arrives in the words of the package that refuses. The
	// credential-shaped model is the paste a person makes once.
	bad := url.Values{}
	for k, v := range form {
		bad[k] = v
	}
	bad.Set("model", "sk-live-9f2c")
	refusal := postForm(t, cl, r.ShellURL+"/act/agent-declare", bad)
	if !strings.Contains(refusal, "looks like a credential") {
		t.Fatalf("a credential pasted as a model name is not refused by name:\n%s", refusal)
	}
	if placements(ctx, t, r) != 0 {
		t.Fatal("a refused declaration still placed something on the record")
	}

	// Criterion 2: the act lands the placement on the record through the
	// person's own admission.
	placed := postForm(t, cl, r.ShellURL+"/act/agent-declare", form)
	if !strings.Contains(placed, declaredName+" is declared") {
		t.Fatalf("declaring did not say it had happened:\n%s", placed)
	}
	if !strings.Contains(placed, "close this screen and it still arrives") {
		t.Fatalf("the answer does not say the screen holds nothing:\n%s", placed)
	}

	item := onlyPlacement(ctx, t, r)
	onRecord, ok := fleet.DeclarationOf(item)
	if !ok {
		t.Fatal("what landed is not a placement the fleet package can read")
	}
	if !sameDocument(onRecord, parsed) {
		t.Fatalf("what was placed is not what the screen showed:\n%+v\n%+v", onRecord, parsed)
	}
	if onRecord.Persona != declaredName || onRecord.Inference == nil ||
		onRecord.Inference.Model != declaredModel {
		t.Fatalf("the placed declaration is not the one filled in: %+v", onRecord)
	}
	// It was opened by the person who was signed in, on their own
	// admission — not by the surface, which acts as nobody and whose own
	// read lane is a different principal entirely.
	if item.Author != me {
		t.Fatalf("the placement was opened by %q, not by the person signed in (%q)",
			item.Author, me)
	}
	if item.Author == "ops" {
		t.Fatal("the placement was opened on the surface's own lane")
	}
	if item.Status != topic.WorkOpen {
		t.Fatalf("a fresh placement is %q, want it waiting for a machine to take it up",
			item.Status)
	}

	// Criterion 3, first half: waiting is said in words, never spun.
	screen = get(t, cl, r.ShellURL+"/agents")
	for _, want := range []string{declaredName,
		"declared; nothing serves agents here yet — the deployment enables the dispatcher plane",
		`<span class="pill warn">declared</span>`,
		"<summary>What was asked for</summary>"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("a placement nothing has taken up is missing %q:\n%s", want, screen)
		}
	}
	// Nothing offers to un-place it: nothing in the ecosystem can.
	for _, banned := range []string{"agent-retire", "Yes, retire"} {
		if strings.Contains(screen, banned) {
			t.Fatalf("the screen offers %q, which nothing can perform", banned)
		}
	}

	// Criterion 2's second half: the submitting session holds nothing. The
	// person signs out — the shell drops their admission and their
	// connection with it — and the placement is still on the record.
	if _, err := cl.Post(r.ShellURL+"/logout", "text/plain", nil); err != nil {
		t.Fatal(err)
	}
	if placements(ctx, t, r) != 1 {
		t.Fatal("the placement went away with the session that made it")
	}

	// Criterion 3, second half: somebody takes the placement up, and the
	// screen says so live, off the record's own evidence. The claim is an
	// ordinary work op by an ordinary persona, which is exactly as much as
	// the screen can see of any node that serves one.
	back := signIn(t, r, auth)
	live := watchDeclared(t, back, r.ShellURL+"/agents/live", "claimed by node-a",
		40*time.Second, func() { claimAs(ctx, t, r, item.ID) })
	if strings.Contains(live, "nothing serves agents here yet") {
		t.Fatalf("the waiting sentence outlived the wait:\n%s", live)
	}

	// Criterion 4: a second surface, which never witnessed any of this,
	// shows the same list — reconstructed from the log and from nothing
	// else, because there is nothing else.
	other, err := r.SecondShell()
	if err != nil {
		t.Fatal(err)
	}
	fresh, _, err := r.SignInTo(other, auth, ceremony.FoundingPersona)
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := get(t, fresh, other+"/agents")
	for _, want := range []string{declaredName, "claimed by node-a", declaredModel} {
		if !strings.Contains(elsewhere, want) {
			t.Fatalf("a surface that never saw the act cannot rebuild it (%q missing):\n%s",
				want, elsewhere)
		}
	}
	// And what it shows is the document, byte for byte the one the first
	// screen showed.
	if got := declarationOnPage(t, elsewhere); got != shown {
		t.Fatalf("the rebuilt document is not the one submitted:\n%s\n---\n%s", got, shown)
	}

	// A home left as "a new one": the act starts a conversation named
	// after the agent before placing it — the same start-on-first-use the
	// placements topic gets, so a board with nothing to offer is never a
	// dead end.
	madeHome := postForm(t, back, r.ShellURL+"/act/agent-declare", url.Values{
		"name": {"greeter"}, "wake_mention": {"on"}})
	if !strings.Contains(madeHome, "a new conversation named after it — its home") {
		t.Fatalf("declaring without a home did not say it made one:\n%s", madeHome)
	}
	rc, nc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	entries, err := topic.Board(ctx, rc)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Announcement.Name == "greeter" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no conversation named after the agent arrived on the board")
	}
}

// The other arm: a deployment that places no agents has no such lane, and
// the paths behind it answer like paths nobody claimed.
func TestDeclareAbsentArm(t *testing.T) {
	r, err := rig.StartExternalIdP(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	seed(t, r)
	if r.Placements != "" {
		t.Fatalf("this arm is meant to place no agents and declares %q", r.Placements)
	}
	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnrollWith(auth, ceremony.FoundingPersona, r.Invite); err != nil {
		t.Fatal(err)
	}
	cl, _, err := r.SignIn(auth, ceremony.FoundingPersona)
	if err != nil {
		t.Fatal(err)
	}
	// The whole agents module is absent in this shape, so its declare
	// paths are absent with it — 404 and not 403: a module this deployment
	// does not run is not a permission a person lacks.
	for _, p := range []string{"/agents/declare-json", "/act/agent-declare"} {
		if got := statusOf(t, cl, "POST", r.ShellURL+p); got != 404 {
			t.Errorf("POST %s answered %d, want 404", p, got)
		}
	}
}

// nameModel puts one name in this realm's model catalogue, the way the
// deployment's own `model set` does: the published contract's bucket and
// codec, one definition — the spelled constant this file once carried is
// retired at its source (design 0010, upstream ask #1).
func nameModel(ctx context.Context, t *testing.T, r *rig.Rig, name string) {
	t.Helper()
	rc, nc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	kv, err := rc.JetStream().CreateKeyValue(ctx, infercat.Config())
	if err != nil {
		if !errors.Is(err, jetstream.ErrBucketExists) {
			t.Fatalf("naming a model: %v", err)
		}
		if kv, err = rc.JetStream().KeyValue(ctx, infercat.Bucket); err != nil {
			t.Fatalf("naming a model: %v", err)
		}
	}
	if err := infercat.Set(ctx, kv, name, infercat.Entry{Capability: "chat"}); err != nil {
		t.Fatalf("naming a model: %v", err)
	}
}

// placementsTopic is where this deployment's declared agents land, resolved
// the way every reader resolves it: the declared NAME, looked up on the
// board. It is absent until the first declaration starts it.
func placementsTopic(ctx context.Context, t *testing.T, r *rig.Rig) (string, bool) {
	t.Helper()
	rc, nc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	entries, err := topic.Board(ctx, rc)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Announcement.Name == r.Placements {
			return e.Path, true
		}
	}
	return "", false
}

// placements is how many agents this deployment has been asked to run.
func placements(ctx context.Context, t *testing.T, r *rig.Rig) int {
	t.Helper()
	path, ok := placementsTopic(ctx, t, r)
	if !ok {
		return 0
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
	n := 0
	for _, item := range mt.WorkItems {
		if _, is := fleet.DeclarationOf(item); is {
			n++
		}
	}
	return n
}

// onlyPlacement is the one placement on the record, insisted upon: a lane
// that placed two things when a person asked once is a lane with a bug.
func onlyPlacement(ctx context.Context, t *testing.T, r *rig.Rig) topic.WorkItem {
	t.Helper()
	path, ok := placementsTopic(ctx, t, r)
	if !ok {
		t.Fatal("nothing started the topic declared agents are placed on")
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
	var found []topic.WorkItem
	for _, item := range mt.WorkItems {
		if _, is := fleet.DeclarationOf(item); is {
			found = append(found, item)
		}
	}
	if len(found) != 1 {
		t.Fatalf("the record holds %d placements, want the one that was asked for", len(found))
	}
	return found[0]
}

// claimAs takes a placement up the way anything that serves one does: an
// ordinary work.claim by an ordinary persona. Nothing privileged, nothing
// the record treats differently — which is the point.
func claimAs(ctx context.Context, t *testing.T, r *rig.Rig, itemID string) {
	t.Helper()
	path, ok := placementsTopic(ctx, t, r)
	if !ok {
		t.Fatal("nothing to claim")
	}
	node, err := r.Voice(ctx, "node-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := topic.Open(node, path).ClaimWork(ctx, itemID); err != nil {
		t.Fatalf("taking the placement up: %v", err)
	}
}

// declarationOnPage pulls the declaration block out of a served page.
func declarationOnPage(t *testing.T, body string) string {
	t.Helper()
	m := declarationRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no declaration block on the page:\n%s", body)
	}
	return html.UnescapeString(m[1])
}

// declarationInPatch pulls it out of what an act answered, where every line
// arrives with the stream's own prefix on it and the browser joins them
// back. Reading it the browser's way is the point: a block that arrives
// truncated is not the document it claims to be.
func declarationInPatch(t *testing.T, sse string) string {
	t.Helper()
	return declarationOnPage(t, frameWith(t, sse, "data-declaration"))
}

// sameDocument compares two declarations by the document they are, which is
// the only sense in which two of them are the same thing.
func sameDocument(a, b declaration.Declaration) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

// watchDeclared opens the screen's own live channel, makes something happen
// on the record while it is open, and reads until the channel carries it —
// the arrival measurement, taken the way a person takes it: by watching a
// screen they never reloaded.
func watchDeclared(t *testing.T, cl *http.Client, u, want string, d time.Duration,
	meanwhile func(),
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

	// The stream is open before the record changes, so what arrives is an
	// arrival and not a first render.
	go func() {
		time.Sleep(500 * time.Millisecond)
		meanwhile()
	}()

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
			if strings.Contains(el, `id="agents-declared"`) && strings.Contains(el, want) {
				return el
			}
		case open:
			frame = append(frame, line)
		}
	}
	t.Fatalf("the live channel never carried %q; the stream said:\n%s", want, read.String())
	return ""
}
