// The models arm of the gate: an administrator names a model from the
// browser, the entry that lands is byte-identical to the one the
// deployment's own verb writes, the declare picker serves the same
// reading, re-pointing moves the very next read, removing stands behind
// its question — and Serving now is the plane's own discovery of a real
// stand-in instance, not a list anybody keeps. Design 0010's acceptance
// criteria, walked on the thinking arm of the rig.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-idp/authtest"
	infercat "github.com/impire-io/soulstream-inference/catalogue"
	"github.com/impire-io/soulstream/ceremony"
	"github.com/impire-io/soulstream/node"

	"soulstream-shell.invalid/e2e/rig"
)

// TestModelsGate walks the whole sheet: the empty offer, the act, the one
// codec, the picker, the live channel, the re-point, the refusals, the
// courtesy line, and the question removing stands behind.
func TestModelsGate(t *testing.T) {
	r, err := rig.StartThinking(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	seed(t, r)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		t.Fatal(err)
	}
	cl := signIn(t, r, auth)

	// The screen, empty: the act offered with the deployment's own hand
	// beside it, the serving half already answering with the stand-in the
	// plane runs — and no secret asked anywhere, the provider line a
	// placeholder a person fills elsewhere.
	screen := get(t, cl, r.ShellURL+"/models")
	for _, want := range []string{"<h1>Models</h1>", ">Name a model</button>",
		"soulstream model set", "standin-1", "Serving now",
		"SOULSTREAM_PROVIDER_KEY=&lt;your key&gt;", `title="`} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the models screen is missing %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, `type="password"`) {
		t.Fatal("the models screen asks for a secret")
	}
	for _, banned := range []string{"anycast", "catalogue", "bucket"} {
		if strings.Contains(strings.ToLower(screen), banned) {
			t.Fatalf("the models screen says %q — machine-room vocabulary on a served page", banned)
		}
	}

	// The act: a name pinned to the model the stand-in wraps.
	set := postForm(t, cl, r.ShellURL+"/act/model-set", url.Values{
		"name": {"sonnet"}, "capability": {"chat"},
		"points": {"model"}, "model_pin": {"standin-1"},
	})
	if !strings.Contains(set, "sonnet points at standin-1 now") {
		t.Fatalf("naming a model did not say it had happened:\n%s", set)
	}

	// One codec, measured rather than assumed: the deployment's own hand
	// writes a twin, and the stored bytes are identical.
	rc, nc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	if err := node.CatalogueSet(ctx, rc.JetStream(), "twin",
		infercat.Entry{Capability: "chat", ModelPin: "standin-1"}); err != nil {
		t.Fatal(err)
	}
	kv, err := rc.JetStream().KeyValue(ctx, infercat.Bucket)
	if err != nil {
		t.Fatal(err)
	}
	fromScreen, err := kv.Get(ctx, "sonnet")
	if err != nil {
		t.Fatalf("the screen's entry is not on the record: %v", err)
	}
	fromHand, err := kv.Get(ctx, "twin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromScreen.Value(), fromHand.Value()) {
		t.Fatalf("the sheet and the deployment's verb wrote different bytes:\n%s\n%s",
			fromScreen.Value(), fromHand.Value())
	}

	// The row says where it points and folds the stored truth whole; the
	// declare picker serves the same reading.
	screen = get(t, cl, r.ShellURL+"/models")
	for _, want := range []string{"sonnet", ">as stored<",
		"&#34;model_pin&#34;: &#34;standin-1&#34;"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the named row is missing %q:\n%s", want, screen)
		}
	}
	agents := get(t, cl, r.ShellURL+"/agents")
	if !strings.Contains(agents, `<option value="sonnet">`) {
		t.Fatalf("the declare picker does not offer the name the sheet wrote:\n%s", agents)
	}

	// The live channel carries a name added elsewhere: the list is
	// reconstructed from the realm at every render, so there is no store
	// to go stale.
	watchModels(t, cl, r.ShellURL+"/models/live", "elsewhere", 30*time.Second, func() {
		if err := node.CatalogueSet(ctx, rc.JetStream(), "elsewhere",
			infercat.Entry{Capability: "chat"}); err != nil {
			t.Errorf("naming a model from elsewhere: %v", err)
		}
	})

	// Re-point: the same act, and the very next read moved — no restart,
	// nothing redeployed.
	repoint := postForm(t, cl, r.ShellURL+"/act/model-set", url.Values{
		"name": {"sonnet"}, "capability": {"chat"}, "points": {"any"},
	})
	if !strings.Contains(repoint, "sonnet points at any instance that serves it now") {
		t.Fatalf("re-pointing did not say where the name points:\n%s", repoint)
	}
	entry, found, err := infercat.Get(ctx, kv, "sonnet")
	if err != nil || !found {
		t.Fatalf("the re-pointed entry is unreadable: found=%v err=%v", found, err)
	}
	if entry.ModelPin != "" {
		t.Fatalf("the pin outlived the re-point: %+v", entry)
	}

	// Refusals arrive in the words of what refuses — the record's grammar,
	// the codec's capability — and a refused act lands nothing.
	before := len(catalogueNames(ctx, t, kv))
	badName := postForm(t, cl, r.ShellURL+"/act/model-set", url.Values{
		"name": {"Bad Name"}, "capability": {"chat"}, "points": {"any"},
	})
	if !strings.Contains(badName, "is not a model name") {
		t.Fatalf("a bad name is not refused by the grammar's own words:\n%s", badName)
	}
	noCap := postForm(t, cl, r.ShellURL+"/act/model-set", url.Values{
		"name": {"nocap"}, "capability": {""}, "points": {"any"},
	})
	if !strings.Contains(noCap, "needs a capability") {
		t.Fatalf("a capability-less entry is not refused in the codec's words:\n%s", noCap)
	}
	if after := len(catalogueNames(ctx, t, kv)); after != before {
		t.Fatalf("a refused act still landed something: %d names, was %d", after, before)
	}

	// The courtesy line, drawn and honest: a person who does not
	// administer is offered no act, and their posted act refuses.
	const guest = "guest"
	added := postForm(t, cl, r.ShellURL+"/act/person-add",
		url.Values{"username": {guest}, "shown": {"Guest"}, "groups": {"realm"}})
	if !strings.Contains(frameWith(t, added, `id="people-result"`), guest+" exists now") {
		t.Fatalf("adding the guest failed:\n%s", added)
	}
	gm := inviteRe.FindStringSubmatch(frameWith(t,
		post(t, cl, r.ShellURL+"/act/invite?who="+guest), `id="people-result"`))
	if gm == nil {
		t.Fatal("no invite for the guest")
	}
	guestAuth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnrollWith(guestAuth, guest, gm[1]); err != nil {
		t.Fatal(err)
	}
	gcl, _, err := r.SignIn(guestAuth, guest)
	if err != nil {
		t.Fatal(err)
	}
	guestScreen := get(t, gcl, r.ShellURL+"/models")
	for _, gone := range []string{"Name a model", "/models/edit", "/models/remove-ask",
		"SOULSTREAM_PROVIDER_KEY"} {
		if strings.Contains(guestScreen, gone) {
			t.Fatalf("a person who does not administer is offered %q:\n%s", gone, guestScreen)
		}
	}
	refused := postForm(t, gcl, r.ShellURL+"/act/model-set", url.Values{
		"name": {"stolen"}, "capability": {"chat"}, "points": {"any"},
	})
	if !strings.Contains(refused, "needs an account that administers") {
		t.Fatalf("the guest's act is not refused:\n%s", refused)
	}
	if _, found, _ := infercat.Get(ctx, kv, "stolen"); found {
		t.Fatal("the refused guest act still landed")
	}

	// Removing stands behind its question, and the question counts the
	// declared agents naming the name — none here, said so.
	ask := get(t, cl, r.ShellURL+"/models/remove-ask?name=sonnet")
	for _, want := range []string{"Remove sonnet for everyone?", "No declared agent names it.",
		"Yes, remove it", "Keep it"} {
		if !strings.Contains(ask, want) {
			t.Fatalf("the removing question is missing %q:\n%s", want, ask)
		}
	}
	removed := post(t, cl, r.ShellURL+"/act/model-remove?name=sonnet")
	if !strings.Contains(removed, "sonnet is gone") {
		t.Fatalf("removing did not say it had happened:\n%s", removed)
	}
	if _, found, _ := infercat.Get(ctx, kv, "sonnet"); found {
		t.Fatal("the removed name still answers")
	}
	if strings.Contains(get(t, cl, r.ShellURL+"/agents"), `<option value="sonnet">`) {
		t.Fatal("the picker still offers the removed name")
	}
}

// catalogueNames is every name the record holds right now.
func catalogueNames(ctx context.Context, t *testing.T, kv jetstream.KeyValue) []string {
	t.Helper()
	named, err := infercat.List(ctx, kv)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(named))
	for _, n := range named {
		out = append(out, n.Name)
	}
	return out
}

// watchModels opens the sheet's own live channel, makes something happen
// on the record while it is open, and reads until the names list carries
// it — the reading measured the way a person takes it: on a screen they
// never reloaded.
func watchModels(t *testing.T, cl *http.Client, u, want string, d time.Duration,
	meanwhile func(),
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

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
			if strings.Contains(el, `id="models-list"`) && strings.Contains(el, want) {
				return
			}
		case open:
			frame = append(frame, line)
		}
	}
	t.Fatalf("the live channel never carried %q; the stream said:\n%s", want, read.String())
}
