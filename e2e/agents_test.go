// The agents arm of the gate: a person creates an agent from the browser,
// the agent gets in with exactly the configuration that browser printed,
// says something, is spoken to, and is then shut out again — all of it the
// real path, with nothing seeded on the agent's behalf.
package e2e

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/registry"
	"github.com/impire-io/soulstream-core/topic"
	siclient "github.com/impire-io/soulstream-identity/client"
	"github.com/impire-io/soulstream-idp/authtest"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

// The two things the screen carries that a test has to be able to read
// back: who the frame says is signed in, and the configuration block.
var (
	whoRe        = regexp.MustCompile(`<span class="who" title="([^"]+)">`)
	credentialRe = regexp.MustCompile(`(?s)<textarea readonly rows="\d+" data-credential>(.*?)</textarea>`)
)

// mcpConfig is the shape the screen prints, as the agent's own program would
// read it back.
type mcpConfig struct {
	Servers map[string]struct {
		Command string            `json:"command"`
		Env     map[string]string `json:"env"`
	} `json:"mcpServers"`
}

// credentialFrom pulls the printed configuration out of an act's response
// and decodes it. Decoding rather than pattern-matching is the point: a
// block that is not valid configuration is not a credential anybody could
// use, however good it looks on a screen.
func credentialFrom(t *testing.T, sse string) map[string]string {
	t.Helper()
	el := frameWith(t, sse, "data-credential")
	m := credentialRe.FindStringSubmatch(el)
	if m == nil {
		t.Fatalf("no configuration block in the response:\n%s", el)
	}
	var cfg mcpConfig
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &cfg); err != nil {
		t.Fatalf("the printed configuration is not valid: %v\n%s", err, m[1])
	}
	entry, ok := cfg.Servers["soulstream"]
	if !ok {
		t.Fatalf("the configuration names no server to launch:\n%s", m[1])
	}
	if entry.Command == "" {
		t.Fatal("the configuration launches nothing")
	}
	return entry.Env
}

// dialAs opens the connection the printed configuration describes, reading
// every value out of it rather than off the rig — so what is measured is
// whether that block is enough to get in, which is the only question a
// person copying it is asking.
func dialAs(ctx context.Context, env map[string]string) (*realm.Client, error) {
	return realm.Connect(ctx, realm.Config{
		URL:       env["SOULSTREAM_URL"],
		CredsFile: env["SOULSTREAM_CREDS"],
		Token:     env["SOULSTREAM_TOKEN"],
		Realm:     env["SOULSTREAM_REALM"],
		Persona:   env["SOULSTREAM_PERSONA"],
	})
}

// postForm takes an act carrying form fields, the way the browser's own
// form does.
func postForm(t *testing.T, cl *http.Client, u string, form url.Values) string {
	t.Helper()
	resp, err := cl.PostForm(u, form)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

const (
	agentHandle = "scribe"
	agentShown  = "Scribe"
	agentSaid   = "the minutes are up to date"
)

// The whole ceremony, in the order a person would live it.
func TestAgentsGate(t *testing.T) {
	r, _ := startRig(t)
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

	// Who the person vouching is, as the frame itself says it. The claim
	// this test is about ends up naming exactly this.
	home := get(t, cl, r.ShellURL+"/")
	who := whoRe.FindStringSubmatch(home)
	if who == nil {
		t.Fatalf("the frame names nobody as signed in: %s", home)
	}
	me := who[1]

	// The screen is on the rail and starts empty.
	screen := get(t, cl, r.ShellURL+"/agents")
	for _, want := range []string{"<h1>Agents</h1>", "No agents yet.",
		`class="ir on" href="/agents`, "Add an agent"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the agents screen is missing %q: %s", want, screen)
		}
	}

	// Creating one. The credential comes back once, inside a configuration
	// block, and the roster comes back beside it already holding the agent.
	addedBody := postForm(t, cl, r.ShellURL+"/act/agent-add",
		url.Values{"handle": {agentHandle}, "shown": {agentShown}})
	env := credentialFrom(t, addedBody)
	for _, k := range []string{"SOULSTREAM_URL", "SOULSTREAM_CREDS", "SOULSTREAM_TOKEN",
		"SOULSTREAM_REALM", "SOULSTREAM_PERSONA"} {
		if env[k] == "" {
			t.Fatalf("the printed configuration has no %s: %v", k, env)
		}
	}
	if env["SOULSTREAM_PERSONA"] != agentHandle {
		t.Fatalf("the configuration is for %q, not the agent just made", env["SOULSTREAM_PERSONA"])
	}
	if !strings.HasPrefix(env["SOULSTREAM_TOKEN"], "sit_") {
		t.Fatalf("the credential is not one the deployment mints: %q", env["SOULSTREAM_TOKEN"])
	}
	if !strings.Contains(addedBody, "Shown once") {
		t.Fatalf("the credential is handed over without saying it is the only time:\n%s", addedBody)
	}

	// The roster says what it is, who answers for it, and that it can get
	// in — and puts the machine channel's lamp on it, from the operator
	// claim on the record and nothing else.
	screen = get(t, cl, r.ShellURL+"/agents")
	for _, want := range []string{agentShown, `<td class="mono">` + agentHandle + `</td>`,
		`class="led machine" title="operated by `, "@" + me,
		"Take the credential away"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the roster is missing %q: %s", want, screen)
		}
	}

	// The credential exists in that one response and nowhere else. The
	// deployment keeps a digest it cannot reverse, and this surface keeps
	// nothing at all — so the plaintext must not be findable anywhere on
	// disk, which is asserted with a control that has to fire or the sweep
	// proves nothing.
	if hits := scanFor(t, r.Dir, env["SOULSTREAM_TOKEN"]); len(hits) != 0 {
		t.Fatalf("the credential was written down: %v", hits)
	}
	control := filepath.Join(r.Dir, "planted-agent-control")
	if err := os.WriteFile(control, []byte("x"+env["SOULSTREAM_TOKEN"]+"x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := scanFor(t, r.Dir, env["SOULSTREAM_TOKEN"]); len(hits) != 1 {
		t.Fatalf("the sweep found %d planted credentials, want 1 — it proves nothing", len(hits))
	}
	if err := os.Remove(control); err != nil {
		t.Fatal(err)
	}

	// The agent gets in — with the block, and only the block.
	ac, err := dialAs(ctx, env)
	if err != nil {
		t.Fatalf("the printed configuration does not get the agent in: %v", err)
	}
	defer func() { _ = ac.Close() }()

	// Its inbox, followed over its own connection, before anybody says its
	// name — the way a running agent would.
	inbox := make(chan topic.Notification, 4)
	ictx, istop := context.WithCancel(ctx)
	defer istop()
	go func() { _ = topic.FollowInbox(ictx, ac, agentHandle, nil, func(n topic.Notification) { inbox <- n }) }()

	// It says something in the conversation the person is looking at.
	path := openConversation(ctx, t, r)
	h := topic.Open(ac, path)
	if _, err := h.Materialise(ctx); err != nil {
		t.Fatalf("the agent cannot read the conversation: %v", err)
	}
	if _, err := h.PostTurn(ctx, agentSaid); err != nil {
		t.Fatalf("the agent cannot say anything: %v", err)
	}

	// And it arrives on the person's screen as a machine voice: teal, with
	// the operator claim as the lamp's own words. Nothing was seeded to make
	// this happen — the colour is read from the card the agent published
	// when it was created, carrying the signature of the person above.
	chat := r.ShellURL + "/live?topic=" + url.QueryEscape(path)
	thread := waitFor(t, cl, chat, `id="dash"`, agentSaid, 20*time.Second)
	said := frameWith(t, thread, `id="dash"`)
	for _, want := range []string{`class="msg machine"`, `class="led machine"`,
		"operated by", agentSaid} {
		if !strings.Contains(said, want) {
			t.Fatalf("the agent's message is not on the machine channel (%q missing):\n%s",
				want, said)
		}
	}
	// The claim behind that colour is not merely present, it verifies: the
	// countersignature checks out against the operator's own key chain, from
	// the directory, with nothing taken on trust. This is the whole of what
	// "vouched for" means, and it is worth asserting where the record keeps
	// it rather than only where a screen paints it.
	attested(ctx, t, r, agentHandle, me)

	// What the agent said is attributed and UNSIGNED, and the screen says so
	// in the same words it uses for anybody else. That is the honest state of
	// the lane today: the stdio door signs from a local key file, and an
	// agent configured entirely by the block above has none, so its messages
	// carry a name the deployment proved on admission and no signature over
	// the words. The verdict is pinned here so that gap is a standing fact
	// somebody has to come and change, not a sentence in a report.
	if !strings.Contains(said, `<span class="verdict">unsigned</span>`) {
		t.Fatalf("the agent's message does not carry an honest verdict:\n%s", said)
	}

	// The People panel beside the conversation names who answers for it, so
	// what the colour was read from is on the screen rather than implied.
	people := frameWith(t, thread, `id="details"`)
	if !strings.Contains(people, "operated by") || !strings.Contains(people, agentHandle) {
		t.Fatalf("the People panel does not say who answers for the agent:\n%s", people)
	}

	// The person says the agent's name from the composer, and the slip
	// reaches the agent's own inbox over the agent's own connection.
	if _, err := r.Post(cl, path, url.Values{
		"body": {"@" + agentShown + " please write that down"}, "mention": {agentHandle},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-inbox:
		if n.Topic != path {
			t.Fatalf("the agent was tapped about %q, not the conversation it is in", n.Topic)
		}
		if n.Author != me {
			t.Fatalf("the slip names %q as the one who said it, not the person who did", n.Author)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the agent's inbox never got the slip")
	}

	// Taking the credential away. The screen says what it changed, and the
	// roster still holds the agent: it said things, and the things it said
	// still need a name against them.
	revoked := post(t, cl, r.ShellURL+"/act/agent-revoke?who="+agentHandle)
	if !strings.Contains(revoked, "cannot get in again") {
		t.Fatalf("taking the credential away did not say so:\n%s", revoked)
	}
	screen = get(t, cl, r.ShellURL+"/agents")
	if !strings.Contains(screen, agentShown) {
		t.Fatalf("the agent vanished from the roster when its credential went: %s", screen)
	}
	if strings.Contains(screen, "Take the credential away") {
		t.Fatalf("the roster still offers to take away a credential that is gone: %s", screen)
	}

	// And the credential is refused. This is the bound the deployment
	// promises and the only one asserted here: the NEXT attempt to get in is
	// refused outright. A connection already open outlives it by design,
	// until the identity it was admitted on runs out — the deployment's
	// callout lifetime — which is why the one above is not re-used to
	// measure this.
	if again, err := dialAs(ctx, env); err == nil {
		_ = again.Close()
		t.Fatal("a credential that was taken away still gets an agent in")
	}

	// And an agent shut out can be let back in, without being made again.
	// The card does not move: who vouched for this voice is a thing that
	// happened, and handing it a new way in does not unhappen it.
	fresh := credentialFrom(t, post(t, cl, r.ShellURL+"/act/agent-credential?who="+agentHandle))
	if fresh["SOULSTREAM_TOKEN"] == env["SOULSTREAM_TOKEN"] {
		t.Fatal("the new credential is the old one")
	}
	back, err := dialAs(ctx, fresh)
	if err != nil {
		t.Fatalf("the replacement credential does not get the agent back in: %v", err)
	}
	_ = back.Close()
	attested(ctx, t, r, agentHandle, me)
}

// The other arm: a deployment that declares no agents surface has none —
// nothing on the rail, and every path this module would claim answers like a
// path nobody claimed.
func TestAgentsAbsentArm(t *testing.T) {
	r, err := rig.StartExternalIdP(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	seed(t, r)
	if r.AgentsDial != "" {
		t.Fatalf("this arm is meant to declare no agents surface, and declares %q", r.AgentsDial)
	}

	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnrollWith(auth, ceremony.FoundingPersona, r.Invite); err != nil {
		t.Fatal(err)
	}
	cl, body, err := r.SignIn(auth, ceremony.FoundingPersona)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "/agents") || strings.Contains(body, ">Agents<") {
		t.Fatalf("a deployment that issues no agent credentials still has them on the rail: %s", body)
	}
	// 404 and not 403: a module this deployment does not run is not a
	// permission a person lacks, it is a path nobody claimed.
	for _, p := range []struct{ method, path string }{
		{http.MethodGet, "/agents"},
		{http.MethodPost, "/act/agent-add"},
		{http.MethodPost, "/act/agent-credential?who=scribe"},
		{http.MethodPost, "/act/agent-revoke?who=scribe"},
	} {
		if got := statusOf(t, cl, p.method, r.ShellURL+p.path); got != http.StatusNotFound {
			t.Errorf("%s %s answered %d, want 404", p.method, p.path, got)
		}
	}
}

// attested checks the operator claim on an agent's card the way this
// deployment's own readers check a signature: resolve the operator's key
// from the realm's key directory, then run the record's own verdict rule
// over the claim. Nothing is taken on trust — the countersignature either
// verifies against a key somebody else published, or it does not.
//
// A NOTE ON WHERE THE KEY COMES FROM, because it is the one seam here. The
// record's own chain rule reads keys off published profile cards, and a
// person who signed in through this surface has no card: profiles are
// self-published, this surface signs as nobody, and there is no screen yet
// where a person publishes their own. So the profile-only reading of this
// claim is "unverified" — not failed, but unproven — until the operator
// publishes a card. What is used below is the same directory the
// conversation screen already verifies every message against: the identity
// plane's open key directory, which is this realm's key directory. The claim
// is genuinely good; the two readings disagree only about where to look.
func attested(ctx context.Context, t *testing.T, r *rig.Rig, handle, operator string) {
	t.Helper()
	rc, nc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	profiles, _, err := registry.All(ctx, rc)
	if err != nil {
		t.Fatal(err)
	}
	var card registry.Profile
	for _, p := range profiles {
		if p.Name == handle {
			card = p
		}
	}
	if card.Name == "" {
		t.Fatal("the agent published no card of its own")
	}
	if card.OperatedBy != operator {
		t.Fatalf("the card says %q answers for the agent, not the person who made it", card.OperatedBy)
	}
	if card.OperatorAttestation == nil || card.OperatorAttestation.Sig == "" {
		t.Fatal("the card names an operator but carries no countersignature")
	}
	if card.OperatorAttestation.OperatedKey == "" {
		t.Fatal("the claim binds to a name only — it could be lifted onto another agent")
	}
	opKey, err := siclient.New(nc, r.State.RealmPub, ceremony.FoundingPersona).
		PersonaPublicKey(operator)
	if err != nil {
		t.Fatalf("the directory has no key for the person who vouched: %v", err)
	}
	kr, _ := registry.BuildKeyring(profiles, nil)
	agentChain, _ := kr.ChainFor(handle)
	if got := registry.AttestationStatus(card, []string{opKey}, false, agentChain); got != registry.ClaimAttested {
		t.Fatalf("the operator claim on the agent reads %q, want %q", got, registry.ClaimAttested)
	}
}

// openConversation is the conversation the shell opens by default — the same
// one the person is looking at.
func openConversation(ctx context.Context, t *testing.T, r *rig.Rig) string {
	t.Helper()
	rc, nc, err := r.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	entries, err := topic.Board(ctx, rc)
	if err != nil || len(entries) == 0 {
		t.Fatalf("board: %v (%d entries)", err, len(entries))
	}
	return entries[len(entries)-1].Path
}
