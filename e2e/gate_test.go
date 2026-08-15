// The shell's consumer-position gate — the research bars as standing
// tests. This module's path sits outside the impire-io namespace, so an
// internal/ import cannot compile (the pure-consumer article,
// compiler-checked); every upstream arrives at its published tag and only
// the shell itself is replaced. The gate boots a whole soulnode realm in
// process through the shared rig, runs the shell through its public embed
// seam, and walks the entire human ceremony: passkey enrolment, sign-in,
// reading a conversation, writing into it, and the custody scan with its
// positive control.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/identity"
	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/registry"
	"github.com/impire-io/soulstream-core/topic"
	siclient "github.com/impire-io/soulstream-identity/client"
	"github.com/impire-io/soulstream-idp/authtest"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

// seededTurn is the message the rig plants as the founding owner, so the
// gate has something to observe — and something to answer.
const seededTurn = "the gate is watching"

// startRig boots the deployment and seeds the two conversations the ceremony
// reads: the gate's own, and an annexe for being tapped on the shoulder from
// somewhere the person is not already looking. It returns the rig and the
// annexe's path.
//
// The board comes back sorted by path, and the shell opens the last of them,
// so "annexe" sorting ahead of "helm-gate" is what keeps the gate's own
// conversation the one in the middle. The seeded message is the check: it is
// in the gate's own room, and the ceremony reads it from the default.
func startRig(t *testing.T) (*rig.Rig, string) {
	t.Helper()
	r, err := rig.Start(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, seed(t, r)
}

// seed plants those two conversations against a rig that is already up, so
// both deployment shapes read the same record.
func seed(t *testing.T, r *rig.Rig) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, err := r.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	annexe, err := topic.StartTopic(ctx, owner, topic.StartTopicInput{
		Name: "annexe", SubjectMatter: "the room nobody is looking at",
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := topic.StartTopic(ctx, owner, topic.StartTopicInput{
		Name: "helm-gate", SubjectMatter: "the standing consumer-position gate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.PostTurn(ctx, seededTurn); err != nil {
		t.Fatal(err)
	}
	return annexe.Path()
}

// signIn walks the ceremony and checks the surface a person lands on is the
// chat shape: the spine of sections, a rail of conversations, one
// conversation with the details beside it, a docked composer.
func signIn(t *testing.T, r *rig.Rig, auth *authtest.Authenticator) *http.Client {
	t.Helper()
	cl, body, err := r.SignIn(auth, ceremony.FoundingPersona)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Sign out") {
		t.Fatalf("shell page shows no session: %s", body)
	}
	// The shape, in plain words.
	for _, want := range []string{
		`class="iconrail"`, `href="/home`, "Home", "Conversations",
		`id="conversations"`, `id="dash"`, `class="thread-body"`, `id="mentions"`,
		`id="details"`, `id="composer"`, `class="dock centred"`, `id="composer-box"`,
		"Write a message…", `href="/status`, "System status",
		// And the shape holds at any width: the browser is told how wide it
		// is, and the column that gives way when there is no room for four —
		// the list of conversations — is still reachable from its own key on
		// the spine, which is where it is at every other width too.
		`<meta name="viewport" content="width=device-width, initial-scale=1">`,
		`<body class="chat" data-signals="{rail:false,panel:false}"`,
		`data-on:click="evt.preventDefault(); $panel = !$panel"`,
		`class="rail" data-class:open="$panel"`, `class="rail-scrim"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("the signed-in page is not the chat shape (%q missing): %s", want, body)
		}
	}
	return cl
}

// readSSE reads one SSE response for up to the given duration.
func readSSE(t *testing.T, cl *http.Client, u string, d time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteString("\n")
		if b.Len() > 1<<18 {
			break
		}
	}
	return b.String()
}

// get reads a page over the session.
func get(t *testing.T, cl *http.Client, u string) string {
	t.Helper()
	resp, err := cl.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return string(out)
}

// post takes an act over the session and returns the patch response, the
// way the browser's own click does.
func post(t *testing.T, cl *http.Client, u string) string {
	t.Helper()
	resp, err := cl.Post(u, "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return string(out)
}

// statusOf is what a path answers, for the paths a deployment is meant not
// to have.
func statusOf(t *testing.T, cl *http.Client, method, u string) int {
	t.Helper()
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// lookupRe pulls the ways into another module's screen out of a rendered
// panel: where each one goes, and what the place at the other end is called.
var lookupRe = regexp.MustCompile(`<a class="lookup" href="([^"]+)" title="([^"]+)"`)

// markedRow is the one row a screen marks, from <tr to </tr>.
func markedRow(screen string) string {
	i := strings.Index(screen, `<tr class="on">`)
	if i < 0 {
		return ""
	}
	j := strings.Index(screen[i:], "</tr>")
	if j < 0 {
		return screen[i:]
	}
	return screen[i : i+j]
}

// inviteRe pulls the shown-once invite out of the fragment that shows it.
// The prefix is the sign-in surface's own, so a screen inventing a token
// would not even match here.
var inviteRe = regexp.MustCompile(`value="(sfi_[0-9a-f]+)"`)

// verified materialises a topic with a keyring built from the identity
// plane's open directory, so every op carries an earned verdict rather
// than the unknown-key default.
func verified(ctx context.Context, t *testing.T, rc *realm.Client, nc *nats.Conn,
	account, path string,
) *topic.MaterializedTopic {
	t.Helper()
	h := topic.Open(rc, path)
	mt, err := h.Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := siclient.New(nc, account, ceremony.FoundingPersona)
	kr := &identity.Keyring{Keys: map[string][]string{}}
	for _, c := range mt.Contributions {
		if _, ok := kr.Keys[c.Author]; ok {
			continue
		}
		k, err := dir.PersonaPublicKey(c.Author)
		if err != nil {
			t.Fatalf("no published key for %s: %v", c.Author, err)
		}
		kr.Keys[c.Author] = []string{k}
	}
	h.UseKeyring(kr)
	if mt, err = h.Materialise(ctx); err != nil {
		t.Fatal(err)
	}
	return mt
}

// find returns the contribution with the given body.
func find(mt *topic.MaterializedTopic, body string) *topic.Contribution {
	for i := range mt.Contributions {
		if mt.Contributions[i].Body == body {
			return &mt.Contributions[i]
		}
	}
	return nil
}

// patchFrames returns the lines of every complete datastar-patch-elements
// event in an SSE response.
func patchFrames(t *testing.T, sse string) [][]string {
	t.Helper()
	var frames [][]string
	var frame []string
	open := false
	for _, line := range strings.Split(sse, "\n") {
		switch {
		case line == "event: datastar-patch-elements":
			open, frame = true, []string{line}
		case open && line == "":
			frames, open = append(frames, frame), false
		case open:
			frame = append(frame, line)
		}
	}
	if len(frames) == 0 {
		t.Fatalf("no complete patch frame in:\n%s", sse)
	}
	return frames
}

// frameFor returns the frame carrying the given patch target — the live
// stream writes one per tick for the rail and one for the conversation.
func frameFor(t *testing.T, frames [][]string, id string) []string {
	t.Helper()
	for _, f := range frames {
		if strings.Contains(elementsIn(f), id) {
			return f
		}
	}
	t.Fatalf("no patch frame carries %s", id)
	return nil
}

// frameWith returns the content of the LAST patch frame carrying the target
// in one read of the stream, so a read spanning several ticks is judged at
// the state it ended in rather than the one it started in.
func frameWith(t *testing.T, sse, id string) string {
	t.Helper()
	var last string
	for _, f := range patchFrames(t, sse) {
		if el := elementsIn(f); strings.Contains(el, id) {
			last = el
		}
	}
	if last == "" {
		t.Fatalf("no patch frame carries %s in:\n%s", id, sse)
	}
	return last
}

// waitFor reads the live stream until some patch frame carrying target also
// carries want, and returns everything read up to the end of that tick — so
// the caller can pull the tick's other targets out of the same read. Slips
// reach a person's tray from their own inbox on their own connection, so the
// gate waits for that rather than betting on which tick it lands in.
//
// A tick is the four frames the stream writes before it flushes, and the
// mentions tally is the last of them: that is how this knows the tick is
// whole.
func waitFor(t *testing.T, cl *http.Client, u, target, want string, d time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var read strings.Builder
	var frame []string
	open, found := false, false
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
			if strings.Contains(el, target) && strings.Contains(el, want) {
				found = true
			}
			if found && strings.Contains(el, `id="mentions"`) {
				return read.String()
			}
		case open:
			frame = append(frame, line)
		}
	}
	t.Fatalf("%s never carried %q; the stream said:\n%s", target, want, read.String())
	return ""
}

// elementsIn joins a frame's element lines the way the browser joins
// them. Everything else on the frame the browser drops — which is the
// point: a fragment written with raw newlines arrives truncated.
func elementsIn(frame []string) string {
	var lines []string
	for _, l := range frame {
		if v, ok := strings.CutPrefix(l, "data: elements "); ok {
			lines = append(lines, v)
		}
	}
	return strings.Join(lines, "\n")
}

// scanFor walks the state dir (skipping the realm's own churn) looking
// for the needle — the Bar 2 scan shape.
func scanFor(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "jetstream", "archive", "fold":
				return filepath.SkipDir
			}
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(raw, []byte(needle)) {
			hits = append(hits, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func TestShellGate(t *testing.T) {
	r, annexe := startRig(t)
	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}

	// The surface is closed until sign-in: no realm content, and the
	// live channel refuses.
	plain, err := http.Get(r.ShellURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	anon, _ := io.ReadAll(plain.Body)
	plain.Body.Close()
	if strings.Contains(string(anon), "helm-gate") {
		t.Fatal("unauthenticated page leaks realm content")
	}
	if resp, _ := http.Get(r.ShellURL + "/live"); resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthenticated /live must refuse")
	}

	// The ceremony: enrol, sign in, read, write, sign out.
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		t.Fatal(err)
	}
	cl := signIn(t, r, auth)

	// The live stream fills both of its targets: the rail names the
	// conversation, the centre carries what was said in it.
	live := readSSE(t, cl, r.ShellURL+"/live", 3*time.Second)
	rail := elementsIn(frameFor(t, patchFrames(t, live), `id="conversations"`))
	for _, want := range []string{`class="conv on"`, "helm-gate", "active"} {
		if !strings.Contains(rail, want) {
			t.Fatalf("the rail of conversations is missing %q:\n%s", want, rail)
		}
	}
	thread := elementsIn(frameFor(t, patchFrames(t, live), `id="dash"`))
	for _, want := range []string{seededTurn, "verified", `class="msg human"`} {
		if !strings.Contains(thread, want) {
			t.Fatalf("the conversation is missing %q:\n%s", want, thread)
		}
	}
	// The details beside the conversation are the stream's third target:
	// who is in here, read off the record, and nothing waiting yet.
	details := elementsIn(frameFor(t, patchFrames(t, live), `id="details"`))
	for _, want := range []string{"People", ceremony.FoundingPersona, "1 message",
		"Status", "Going on", "Waiting on", "Nothing is waiting on anyone"} {
		if !strings.Contains(details, want) {
			t.Fatalf("the details panel is missing %q:\n%s", want, details)
		}
	}

	// The house readouts moved off the conversation and onto their own
	// screens — reachable from the spine, no longer the centre.
	if strings.Contains(thread, "Storage") {
		t.Fatalf("the plane readouts are still in the conversation:\n%s", thread)
	}
	status := get(t, cl, r.ShellURL+"/status")
	for _, want := range []string{"Storage", "People &amp; sign-in", "Work", "ops ·",
		`class="iconrail"`, `class="ir on" href="/status`} {
		if !strings.Contains(status, want) {
			t.Fatalf("the system-status screen is missing %q: %s", want, status)
		}
	}
	// From a screen with no list of conversations on it, the key on the spine
	// is a plain way to one and claims nothing of the click: the drawer
	// belongs to the screen that has a list to pull out, and to no other.
	if strings.Contains(status, "$panel = !$panel") {
		t.Fatalf("a screen with no conversations list still pulls one out: %s", status)
	}
	// Home is reachable from anywhere and renders inside the same frame: the
	// house at a glance, and a way into every conversation.
	overview := get(t, cl, r.ShellURL+"/home")
	for _, want := range []string{"Your soulstream at a glance", "Storage", "Conversations",
		"helm-gate", `class="row" href="/?topic=`, `class="ir on" href="/home`} {
		if !strings.Contains(overview, want) {
			t.Fatalf("the overview is missing %q: %s", want, overview)
		}
	}

	// Class (a): an op on the record as the signed-in principal.
	actResp, err := cl.Post(r.ShellURL+"/act/work-open", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	act, _ := io.ReadAll(actResp.Body)
	actResp.Body.Close()
	if !strings.Contains(string(act), "work.open ok") {
		t.Fatalf("work.open failed: %s", act)
	}

	// The act is attributed to the fold principal in the realm itself,
	// not to the shell or the owner.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	mt, err := topic.Open(rc, path).Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(mt.WorkItems) != 1 {
		t.Fatalf("work items = %d, want 1", len(mt.WorkItems))
	}
	author := mt.WorkItems[0].Author
	if author == "" || author == ceremony.FoundingPersona {
		t.Fatalf("work item author = %q — not the fold principal", author)
	}

	// The composer: the signed-in person writes into the conversation.
	// The rig posts what the browser's form posts — the same encoding
	// Datastar's form mode puts on the wire.
	const said = "posted from the composer"
	got, err := r.Post(cl, "", url.Values{"body": {said}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Posted as") {
		t.Fatalf("the composer did not post: %s", got)
	}

	// The message is on the record: the session's own principal wrote it,
	// signed with that principal's own key.
	mt = verified(ctx, t, rc, rnc, r.State.RealmPub, path)
	posted := find(mt, said)
	if posted == nil {
		t.Fatalf("the message never reached the record: %+v", mt.Contributions)
	}
	if posted.Author == "" || posted.Author == ceremony.FoundingPersona {
		t.Fatalf("message author = %q — not the signed-in principal", posted.Author)
	}
	if posted.Author != author {
		t.Fatalf("message author = %q, work author = %q — one session, one principal",
			posted.Author, author)
	}
	if posted.Type != topic.TypeTurnPost {
		t.Fatalf("message type = %q, want %q", posted.Type, topic.TypeTurnPost)
	}
	if posted.Sig != topic.SigVerified {
		t.Fatalf("message signature = %q, want %q", posted.Sig, topic.SigVerified)
	}

	// Answering: the composer takes its anchor from the record, and the
	// answer lands as a reply on the message it names.
	seeded := find(mt, seededTurn)
	if seeded == nil {
		t.Fatalf("the seeded message is gone: %+v", mt.Contributions)
	}
	anchor := get(t, cl, r.ShellURL+"/composer/reply?op="+url.QueryEscape(seeded.OpID))
	if !strings.Contains(anchor, `name="reply-to" value="`+seeded.OpID+`"`) {
		t.Fatalf("the composer did not take the anchor: %s", anchor)
	}
	if !strings.Contains(anchor, "replying to") || !strings.Contains(anchor, "Cancel") {
		t.Fatalf("the reply state is not shown above the input: %s", anchor)
	}
	const answered = "answered from the composer"
	got, err = r.Post(cl, "", url.Values{"body": {answered}, "reply-to": {seeded.OpID}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Posted as") {
		t.Fatalf("the composer did not answer: %s", got)
	}
	mt = verified(ctx, t, rc, rnc, r.State.RealmPub, path)
	reply := find(mt, answered)
	if reply == nil {
		t.Fatalf("the answer never reached the record: %+v", mt.Contributions)
	}
	if reply.Anchor != seeded.OpID || reply.Dangling {
		t.Fatalf("answer anchor = %q (dangling=%v), want %q",
			reply.Anchor, reply.Dangling, seeded.OpID)
	}
	if reply.Author != posted.Author || reply.Sig != topic.SigVerified {
		t.Fatalf("answer by %q signature %q — want %q / %q",
			reply.Author, reply.Sig, posted.Author, topic.SigVerified)
	}

	// And both arrive in the view the ordinary way: the live stream's
	// next morph carries them, no reload asked of anyone.
	stream := readSSE(t, cl, r.ShellURL+"/live", 3*time.Second)
	frame := frameFor(t, patchFrames(t, stream), `id="dash"`)
	for _, l := range frame[1:] {
		if !strings.HasPrefix(l, "data: ") {
			t.Fatalf("the browser drops this line of the patch frame: %q", l)
		}
	}
	seen := elementsIn(frame)
	if !strings.HasPrefix(seen, `<div id="dash" class="thread-body">`) ||
		!strings.HasSuffix(seen, "</div>") {
		t.Fatalf("the view the browser receives is not a whole fragment:\n%s", seen)
	}

	// Whose message is whose comes from the record and the session, and it
	// is legible in the HTML the browser is handed: the signed-in person's
	// own message is theirs, the owner's is not, and the answer hangs off
	// the message it answers.
	ownTag := fmt.Sprintf(`<div class="msg human mine" data-op=%q>`, posted.OpID)
	if !strings.Contains(seen, ownTag) {
		t.Fatalf("the person's own message is not rendered as theirs (%s):\n%s", ownTag, seen)
	}
	otherTag := fmt.Sprintf(`<div class="msg human" data-op=%q>`, seeded.OpID)
	if !strings.Contains(seen, otherTag) {
		t.Fatalf("the owner's message is rendered as somebody else's (%s):\n%s", otherTag, seen)
	}
	if !strings.Contains(seen, ceremony.FoundingPersona) {
		t.Fatalf("the other person's message carries no name:\n%s", seen)
	}
	answerTag := fmt.Sprintf(`<div class="msg human mine reply" data-op=%q>`, reply.OpID)
	i, j := strings.Index(seen, otherTag), strings.Index(seen, answerTag)
	nested := strings.Index(seen, `<div class="replies">`)
	if j < 0 || nested < i || j < nested {
		t.Fatalf("the answer does not hang off the message it answers:\n%s", seen)
	}
	if !strings.Contains(seen, "Reply") {
		t.Fatalf("no per-message reply control in the conversation:\n%s", seen)
	}

	// And the details keep up with it: both voices are in the room, and the
	// work opened earlier is waiting on somebody.
	side := elementsIn(frameFor(t, patchFrames(t, stream), `id="details"`))
	for _, want := range []string{ceremony.FoundingPersona, posted.Author,
		`<span class="you">you</span>`, "Waiting for someone to pick up"} {
		if !strings.Contains(side, want) {
			t.Fatalf("the details panel is missing %q:\n%s", want, side)
		}
	}
	// Bar 4, cross-linking, the arm where the other module is running.
	//
	// Every name in the panel is a way into the screen that administers that
	// person's sign-in. The module that renders the panel imports no part of
	// the module that owns that screen: it names it, says what kind of screen
	// it wants and who it is about, and the frame puts that to the modules
	// this deployment actually runs. So the path is the other module's own
	// spelling, and the words on the link are the name it registered under —
	// neither of them written anywhere in the module that shows them.
	links := lookupRe.FindAllStringSubmatch(side, -1)
	if n := strings.Count(side, "<a "); n == 0 || n != len(links) {
		t.Fatalf("%d links in the panel and %d of them into another module's screen:\n%s",
			n, len(links), side)
	}
	into := map[string]string{}
	for _, l := range links {
		if l[2] != "People &amp; sign-in" {
			t.Fatalf("the way into the other module's screen is called %q here:\n%s", l[2], side)
		}
		u, err := url.Parse(html.UnescapeString(l[1]))
		if err != nil {
			t.Fatalf("the panel offers an unparseable link %q: %v", l[1], err)
		}
		if u.Path != "/people" || u.Query().Get("topic") != path {
			t.Fatalf("the link reads %q — the wrong screen, or it drops the conversation "+
				"the person would come back to", l[1])
		}
		into[u.Query().Get("who")] = html.UnescapeString(l[1])
	}
	for _, who := range []string{ceremony.FoundingPersona, posted.Author} {
		if into[who] == "" {
			t.Fatalf("the panel points nowhere for %s:\n%s", who, side)
		}
	}

	// And it goes where it says. The screen at the other end answers about
	// the person it was followed for: their row marked, and said in words.
	marked := get(t, cl, r.ShellURL+into[ceremony.FoundingPersona])
	for _, want := range []string{`class="ir on" href="/people`,
		`Looking up <span class="mono">` + ceremony.FoundingPersona + `</span>`} {
		if !strings.Contains(marked, want) {
			t.Fatalf("the followed link did not land on that person (%q missing): %s",
				want, marked)
		}
	}
	if n := strings.Count(marked, `<tr class="on">`); n != 1 {
		t.Fatalf("%d rows are marked on the screen the link landed on: %s", n, marked)
	}
	if row := markedRow(marked); !strings.Contains(row,
		`<td class="mono" title="`+ceremony.FoundingPersona+`">`+
			ceremony.FoundingPersona+`</td>`) {
		t.Fatalf("the marked row is somebody else's: %s", row)
	}
	// A voice on the record that was never a sign-in resolves too, and is
	// answered rather than left hunting a row that was never there. The
	// panel cannot know which of its people this deployment administers —
	// that is the other module's to say, and it says it.
	stranger := get(t, cl, r.ShellURL+into[posted.Author])
	if !strings.Contains(stranger, `Nobody who signs in here answers to `+
		`<span class="mono">`+posted.Author+`</span>`) {
		t.Fatalf("the screen does not say it has nothing on %s: %s", posted.Author, stranger)
	}
	if strings.Contains(stranger, `<tr class="on">`) {
		t.Fatalf("a row is marked for somebody who cannot sign in here at all: %s", stranger)
	}

	// A person reads their own name on their own screen. The fold mints an
	// id and keeps whatever it knows about the human to itself, so the name
	// lives in the realm's own directory — and the shell asks it again on
	// every render until it answers, so a name published now reaches a
	// session that signed in before it existed.
	if err := r.Name(ctx, posted.Author, "Daan"); err != nil {
		t.Fatal(err)
	}
	named := get(t, cl, r.ShellURL+"/")
	if !strings.Contains(named, `<span class="who" title="`+posted.Author+`">Daan</span>`) {
		t.Fatalf("the top bar numbers the person instead of naming them: %s", named)
	}
	if strings.Contains(named, ">"+posted.Author+"<") {
		t.Fatalf("the raw id is on screen rather than behind the name: %s", named)
	}
	people := frameWith(t, readSSE(t, cl, r.ShellURL+"/live", 1500*time.Millisecond),
		`id="details"`)
	if !strings.Contains(people,
		`<span class="who" title="@`+posted.Author+`">Daan</span><span class="you">you</span>`) {
		t.Fatalf("the People list does not put the pill on the person's name:\n%s", people)
	}

	// Somebody says your name in a room you are not standing in — and says it
	// as a name, not as an id. The body is what a person would write and read;
	// who it taps rides beside it, the shape the composer's picker puts on the
	// wire. Nothing rewrites the message to make the grammar work.
	avery, err := r.Voice(ctx, "avery", "Avery")
	if err != nil {
		t.Fatal(err)
	}
	ah := topic.Open(avery, annexe)
	if _, err := ah.Materialise(ctx); err != nil {
		t.Fatal(err)
	}
	const asked = "@Daan did the good coffee come in a bag or a tin?"
	mentionOp, err := ah.PostTurnMentioning(ctx, asked, []string{posted.Author})
	if err != nil {
		t.Fatal(err)
	}
	amt, err := topic.Open(rc, annexe).Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tap := find(amt, asked)
	if tap == nil || len(tap.Mentions) != 1 || tap.Mentions[0] != posted.Author {
		t.Fatalf("the record did not carry the resolved mention: %+v", tap)
	}
	if strings.Contains(tap.Body, posted.Author) {
		t.Fatalf("the body was rewritten into an id: %q", tap.Body)
	}

	// It reaches the person's tray over their own connection, and the spine
	// counts it — from a conversation they are not looking at.
	chat := r.ShellURL + "/live?topic=" + url.QueryEscape(path)
	tapped := waitFor(t, cl, chat, `id="mentions"`, `class="tally on"`, 20*time.Second)
	if tally := frameWith(t, tapped, `id="mentions"`); !strings.Contains(tally, ">1</span>") {
		t.Fatalf("the spine counts the wrong number of waiting messages: %s", tally)
	}
	if list := frameWith(t, tapped, `id="conversations"`); !strings.Contains(list, `class="conv unread"`) {
		t.Fatalf("the conversation holding the mention is not marked in the rail:\n%s", list)
	}

	// The control: a message in the same room with nobody's name in it. The
	// mark has to be the mention and not the room.
	quiet, err := ah.PostTurn(ctx, "and one for nobody in particular")
	if err != nil {
		t.Fatal(err)
	}

	// Opening the room is reading it: the message that said the person's
	// name stands out where it was said, the one that did not is a message
	// like any other, and the count comes off the spine.
	opened := waitFor(t, cl, r.ShellURL+"/live?topic="+url.QueryEscape(annexe),
		`id="dash"`, `class="msg human mentions"`, 20*time.Second)
	room := frameWith(t, opened, `id="dash"`)
	if !strings.Contains(room, fmt.Sprintf(`<div class="msg human mentions" data-op=%q>`, mentionOp)) {
		t.Fatalf("the message that says the person's name is not marked:\n%s", room)
	}
	if !strings.Contains(room, fmt.Sprintf(`<div class="msg human" data-op=%q>`, quiet)) {
		t.Fatalf("a message with nobody's name in it is marked too:\n%s", room)
	}
	if tally := frameWith(t, opened, `id="mentions"`); strings.Contains(tally, "tally on") {
		t.Fatalf("the count survives opening the room it pointed at: %s", tally)
	}
	// And it reads as it was written: the name, verbatim, marked because it
	// actually tapped somebody — never the id it resolved to.
	if !strings.Contains(room, `<p class="body"><span class="mtoken human">@Daan</span> did the`+
		` good coffee come in a bag or a tin?</p>`) {
		t.Fatalf("the message does not read as it was written:\n%s", room)
	}

	// The picker itself, from the room the person is now standing in. The
	// server offers who is in here; typing narrows it; a fragment nobody
	// answers to closes it.
	suggest := r.ShellURL + "/composer/suggest?topic=" + url.QueryEscape(annexe) + "&q="
	offered := get(t, cl, suggest+"av")
	for _, want := range []string{`id="mention-suggest"`, `data-mention="avery"`,
		`data-name="Avery"`, `<span class="handle">@avery</span>`} {
		if !strings.Contains(offered, want) {
			t.Fatalf("the picker does not offer the room (%q missing):\n%s", want, offered)
		}
	}
	if closed := get(t, cl, suggest+"zzz"); strings.Contains(closed, "data-mention") {
		t.Fatalf("the picker offers somebody nobody asked for:\n%s", closed)
	}

	// And picking one posts the shape the ceremony just measured from the
	// other side: the body says the name, the pick says who, and the record
	// keeps both apart.
	const thanked = "@Avery a bag, and there is one left"
	if got, err := r.Post(cl, annexe, url.Values{
		"body": {thanked}, "mention": {"avery"},
	}); err != nil || !strings.Contains(got, "Posted as") {
		t.Fatalf("the composer did not post the picked mention: %v %s", err, got)
	}
	// A name typed by hand resolves too, when it can only mean one person;
	// a name nobody in the room answers to stays exactly as typed.
	const guessed = "@Avery and @Nobody, for the record"
	if got, err := r.Post(cl, annexe, url.Values{"body": {guessed}}); err != nil ||
		!strings.Contains(got, "Posted as") {
		t.Fatalf("the composer did not post the typed mention: %v %s", err, got)
	}
	amt, err = topic.Open(rc, annexe).Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ what, body string }{
		{"the picked mention", thanked}, {"the typed mention", guessed},
	} {
		said := find(amt, c.body)
		if said == nil {
			t.Fatalf("%s never reached the record: %+v", c.what, amt.Contributions)
		}
		if said.Body != c.body {
			t.Fatalf("%s was rewritten: %q, want %q", c.what, said.Body, c.body)
		}
		if len(said.Mentions) != 1 || said.Mentions[0] != "avery" {
			t.Fatalf("%s carries %v, want just avery", c.what, said.Mentions)
		}
	}

	// The second channel, and the only honest way onto it. Soulstream has no
	// human/agent field to set — the persona taxonomy was removed outright,
	// because the protocol cannot verify what controls a key — so a voice
	// says it is not a person answering for themselves by naming an operator
	// and having that operator countersign the claim. The shell reads that
	// claim and nothing else, and both accents end up in one room at equal
	// weight.
	scribe, err := r.Operated(ctx, "scribe", "Scribe", posted.Author)
	if err != nil {
		t.Fatal(err)
	}
	sh := topic.Open(scribe, annexe)
	if _, err := sh.Materialise(ctx); err != nil {
		t.Fatal(err)
	}
	machineOp, err := sh.PostTurn(ctx, "noted, and on the list")
	if err != nil {
		t.Fatal(err)
	}
	profile, found, err := registry.Lookup(ctx, rc, "scribe")
	if err != nil || !found {
		t.Fatalf("the operated voice has no directory card: found=%v %v", found, err)
	}
	if profile.OperatedBy != posted.Author || profile.OperatorAttestation == nil {
		t.Fatalf("the card carries no countersigned operator claim: %+v", profile)
	}

	// And the screen reads it off the record: the card and the lamp both go
	// teal, the claim is in words beside the name in the People panel, and
	// everybody who answers for themselves stays amber in the same room.
	channels := waitFor(t, cl, r.ShellURL+"/live?topic="+url.QueryEscape(annexe),
		`id="dash"`, `class="msg machine"`, 20*time.Second)
	both := frameWith(t, channels, `id="dash"`)
	if !strings.Contains(both, fmt.Sprintf(`<div class="msg machine" data-op=%q>`, machineOp)) {
		t.Fatalf("the operated voice is not on the machine channel:\n%s", both)
	}
	if !strings.Contains(both, `<span class="led machine" title="operated by Daan"></span>`) {
		t.Fatalf("the machine channel carries no lamp of its own:\n%s", both)
	}
	if !strings.Contains(both, `<div class="msg human" data-op=`) {
		t.Fatalf("the human channel left the room when the machine one arrived:\n%s", both)
	}
	panel := frameWith(t, channels, `id="details"`)
	if !strings.Contains(panel, `<span class="who" title="@scribe">Scribe</span>`) ||
		!strings.Contains(panel, "operated by Daan") {
		t.Fatalf("the People panel does not say who answers for the voice:\n%s", panel)
	}

	// The word the operator retired. The Go keeps it — realm.Client, the
	// realm package, the flag a deployment sets — but nothing a person is
	// served says it.
	tokens, err := http.Get(r.ShellURL + "/assets/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	tokenSrc, _ := io.ReadAll(tokens.Body)
	tokens.Body.Close()
	for _, page := range []struct{ what, body string }{
		{"the sign-in card", string(anon)},
		{"the conversation", named},
		{"the overview", get(t, cl, r.ShellURL+"/home")},
		{"the system-status screen", get(t, cl, r.ShellURL+"/status")},
		{"the live stream", readSSE(t, cl, r.ShellURL+"/live", 1200*time.Millisecond)},
		{"the token source", string(tokenSrc)},
	} {
		if i := strings.Index(strings.ToLower(page.body), "realm"); i >= 0 {
			t.Fatalf("%s says the retired word: …%s…", page.what,
				page.body[max(0, i-70):min(len(page.body), i+70)])
		}
	}

	// Bar 2, the arm where the sign-in plane is present: the module that
	// administers people is part of this build, and it acts.
	//
	// It is on the spine because the deployment declares a surface to
	// administer, and for no other reason — the same registration, through
	// the same contract, as the two modules that are always there.
	if r.AdminBase == "" {
		t.Fatal("the bundled deployment declared no administration surface")
	}
	screen := get(t, cl, r.ShellURL+"/people")
	for _, want := range []string{`class="ir on" href="/people`, "People &amp; sign-in",
		"Sign-in name", "Passkeys", `id="people-table"`, `id="people-result"`,
		`<td class="mono" title="` + ceremony.FoundingPersona + `">` +
			ceremony.FoundingPersona + `</td>`,
		`<span class="pill ok"><span class="led ok"></span>yes</span>`,
		// The groups the sign-in surface holds for this person — editable in
		// place, because administering them is this screen's job now.
		`@post('/act/groups?who=` + ceremony.FoundingPersona,
		`name="groups"`,
		`@post('/act/invite?who=` + ceremony.FoundingPersona,
	} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the people screen is missing %q: %s", want, screen)
		}
	}

	// The one key the screen does not draw. This person is the only one
	// who can administer sign-ins here, so taking theirs away would leave
	// the deployment with nobody to administer it — which the sign-in
	// surface refuses. The screen does not offer a key that only ever
	// earns a refusal; it says the thing the refusal would have said.
	if strings.Contains(screen, `/act/disable?who=`+ceremony.FoundingPersona) {
		t.Fatalf("the screen offers to lock the deployment out of itself:\n%s", screen)
	}
	if !strings.Contains(screen, "the last administrator stays") {
		t.Fatalf("the screen withholds the key without saying why:\n%s", screen)
	}

	// The act, round-tripped: the shell asks the sign-in surface as the
	// signed-in person, and what comes back is an invite that surface will
	// honour. A token this screen invented would be refused at enrolment.
	minted := post(t, cl, r.ShellURL+"/act/invite?who="+ceremony.FoundingPersona)
	shown := frameWith(t, minted, `id="people-result"`)
	m := inviteRe.FindStringSubmatch(shown)
	if m == nil {
		t.Fatalf("no invite on the screen that asked for one:\n%s", shown)
	}
	if !strings.Contains(shown, "Shown once") {
		t.Fatalf("the screen does not say the invite is shown once:\n%s", shown)
	}
	second, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnrollWith(second, ceremony.FoundingPersona, m[1]); err != nil {
		t.Fatalf("the sign-in surface refused the invite its own screen showed: %v", err)
	}
	// And it was single-use, which is the surface's own semantic rather
	// than anything this module gets to decide.
	third, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnrollWith(third, ceremony.FoundingPersona, m[1]); err == nil {
		t.Fatal("the invite was spent twice")
	}

	// The act the screen withheld, asked for anyway — a stale page, a
	// second browser, anything. The rule does not live on the page, so the
	// answer comes back from the sign-in surface, and the screen puts up
	// the words that surface used rather than a refusal of its own
	// invention.
	held := frameWith(t, post(t, cl, r.ShellURL+"/act/disable?who="+ceremony.FoundingPersona),
		`id="people-result"`)
	if !strings.Contains(held, "the last administrator stays") {
		t.Fatalf("the shell swallowed the sign-in surface's refusal:\n%s", held)
	}
	if strings.Contains(held, "can no longer sign in") {
		t.Fatalf("the screen reported an act the surface refused:\n%s", held)
	}
	if !strings.Contains(get(t, cl, r.ShellURL+"/people"),
		`<span class="pill ok"><span class="led ok"></span>yes</span>`) {
		t.Fatal("the refused act took the last administrator's sign-in away anyway")
	}

	// And the same refusal asked for the way a machine would: a bearer of
	// this deployment's own issuing, straight at the surface, with no
	// screen in between. The shell is not what is holding the line — the
	// rule is enforced where it is authoritative, and the screen only
	// reflects it.
	bearer, err := r.Bearer(auth, ceremony.FoundingPersona)
	if err != nil {
		t.Fatal(err)
	}
	for _, lethal := range []struct {
		what, path string
		body       any
	}{
		{"disabling the last administrator",
			"/api/admin/users/" + ceremony.FoundingPersona + "/status",
			map[string]any{"status": "disabled"}},
		{"taking administration off the last administrator",
			"/api/admin/users/" + ceremony.FoundingPersona + "/groups",
			map[string]any{"groups": []string{"realm"}}},
	} {
		status, said, err := r.Ask(bearer, http.MethodPost, lethal.path, lethal.body)
		if err != nil {
			t.Fatal(err)
		}
		if status != http.StatusConflict || !strings.Contains(said, "the last administrator stays") {
			t.Fatalf("%s: the surface answered %d %s", lethal.what, status, said)
		}
	}

	// A second administrator, enrolled the whole way: created through the
	// surface, invited through it, and their passkey registered against
	// that invite. The rule is about the last administrator, never a
	// particular one — so with two standing it lets go of the first.
	const deputy = "deputy"
	if status, said, err := r.Ask(bearer, http.MethodPost, "/api/admin/users",
		map[string]any{"username": deputy, "display_name": "Deputy",
			"groups": []string{"admin"}}); err != nil || status != http.StatusCreated {
		t.Fatalf("creating a second administrator: %d %s %v", status, said, err)
	}
	invited, answer, err := r.Ask(bearer, http.MethodPost, "/api/admin/invites",
		map[string]any{"username": deputy})
	if err != nil || invited != http.StatusCreated {
		t.Fatalf("inviting the second administrator: %d %s %v", invited, answer, err)
	}
	var forDeputy struct {
		Invite string `json:"invite"`
	}
	if err := json.Unmarshal([]byte(answer), &forDeputy); err != nil || forDeputy.Invite == "" {
		t.Fatalf("no invite for the second administrator in %s", answer)
	}
	deputyAuth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnrollWith(deputyAuth, deputy, forDeputy.Invite); err != nil {
		t.Fatal(err)
	}

	// The whole lifecycle, shell-native: an administrator names a new
	// person from this screen, changes their groups, shuts them out and
	// lets them back in, and administers the applications that sign people
	// in — the reason the shell exists as the one console.
	const librarian = "librarian"
	added := postForm(t, cl, r.ShellURL+"/act/person-add",
		url.Values{"username": {librarian}, "shown": {"Librarian"}, "groups": {"realm"}})
	if !strings.Contains(frameWith(t, added, `id="people-result"`), librarian+" exists now") {
		t.Fatalf("adding a person did not say so:\n%s", added)
	}
	if !strings.Contains(frameWith(t, added, `id="people-table"`), librarian) {
		t.Fatalf("the re-read list does not hold the new person:\n%s", added)
	}
	regrouped := postForm(t, cl, r.ShellURL+"/act/groups?who="+librarian,
		url.Values{"groups": {"realm keeper"}})
	if note := frameWith(t, regrouped, `id="people-result"`); !strings.Contains(note, "keeper") {
		t.Fatalf("changing groups did not say what they are now:\n%s", note)
	}
	_ = post(t, cl, r.ShellURL+"/act/disable?who="+librarian)
	backIn := post(t, cl, r.ShellURL+"/act/enable?who="+librarian)
	if !strings.Contains(frameWith(t, backIn, `id="people-result"`), librarian+" can sign in again") {
		t.Fatalf("enabling did not say so:\n%s", backIn)
	}

	// The applications, administered from the same screen: registered,
	// listed on a re-read of the surface, and removed.
	appsOn := get(t, cl, r.ShellURL+"/people")
	if !strings.Contains(appsOn, "Apps that sign people in") ||
		!strings.Contains(appsOn, `id="people-clients"`) {
		t.Fatalf("the screen does not administer the apps:\n%s", appsOn)
	}
	registered := postForm(t, cl, r.ShellURL+"/act/client-add", url.Values{
		"id": {"kiosk"}, "name": {"Kiosk"}, "uris": {"http://127.0.0.1:9999/cb"}})
	if !strings.Contains(frameWith(t, registered, `id="people-clients"`), "kiosk") {
		t.Fatalf("the registered app is not on the re-read list:\n%s", registered)
	}
	removed := post(t, cl, r.ShellURL+"/act/client-delete?id=kiosk")
	if strings.Contains(frameWith(t, removed, `id="people-clients"`), `title="kiosk"`) {
		t.Fatalf("the removed app is still on the list:\n%s", removed)
	}

	// Administration is the administrators': a person without the role is
	// shown no key to this screen, and when they come anyway the sign-in
	// surface — not the shell — refuses them, in its own words.
	librarianInvite := post(t, cl, r.ShellURL+"/act/invite?who="+librarian)
	lm := inviteRe.FindStringSubmatch(frameWith(t, librarianInvite, `id="people-result"`))
	if lm == nil {
		t.Fatalf("no invite for the librarian:\n%s", librarianInvite)
	}
	librarianAuth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnrollWith(librarianAuth, librarian, lm[1]); err != nil {
		t.Fatal(err)
	}
	lcl, lhome, err := r.SignIn(librarianAuth, librarian)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lhome, "People &amp; sign-in") {
		t.Fatalf("a person who cannot administer sign-ins is shown the key:\n%s", lhome)
	}
	asLibrarian := get(t, lcl, r.ShellURL+"/people")
	if !strings.Contains(asLibrarian, "needs an account that administers sign-ins") {
		t.Fatalf("the surface's refusal did not reach the screen:\n%s", asLibrarian)
	}

	// The other act, now that it means something: taking a sign-in away,
	// and the screen re-reads the surface rather than believing the click.
	// The list that comes back is the sign-in surface's own answer to a
	// fresh question.
	back := get(t, cl, r.ShellURL+"/people")
	if !strings.Contains(back, `/act/disable?who=`+ceremony.FoundingPersona) {
		t.Fatalf("with two administrators the key never came back:\n%s", back)
	}
	off := post(t, cl, r.ShellURL+"/act/disable?who="+ceremony.FoundingPersona)
	if note := frameWith(t, off, `id="people-result"`); !strings.Contains(note,
		ceremony.FoundingPersona+" can no longer sign in") {
		t.Fatalf("the screen does not say what it did:\n%s", note)
	}
	table := frameWith(t, off, `id="people-table"`)
	if !strings.Contains(table, `<span class="pill warn">no</span>`) {
		t.Fatalf("the re-read list still says they can sign in:\n%s", table)
	}
	if strings.Contains(table, "/act/disable?who="+ceremony.FoundingPersona) {
		t.Fatalf("the screen offers to take away a sign-in that is already gone:\n%s", table)
	}
	if !strings.Contains(table, "/act/enable?who="+ceremony.FoundingPersona) {
		t.Fatalf("somebody shut out is offered no way back in:\n%s", table)
	}

	// Sign out closes the session.
	if _, err := cl.Post(r.ShellURL+"/logout", "", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(get(t, cl, r.ShellURL+"/"), "Sign out") {
		t.Fatal("session survived logout")
	}

	// The custody scan (Bar 2's shape): the session cookie value must
	// exist nowhere on disk; the positive control must fire.
	sid := "helm-session-needle-never-on-disk"
	if c, ok := r.Cookie(cl); ok {
		// The jar drops expired cookies; a fixed needle stands in.
		sid = c.Value
	}
	if hits := scanFor(t, r.Dir, sid); len(hits) != 0 {
		t.Fatalf("session material on disk: %v", hits)
	}
	control := filepath.Join(r.Dir, "planted-control")
	if err := os.WriteFile(control, []byte("x"+sid+"x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := scanFor(t, r.Dir, sid); len(hits) != 1 {
		t.Fatalf("positive control did not fire: %v", hits)
	}
	os.Remove(control)

	// The offline-render gate (Bar 4's shape): the token source is
	// self-contained and the fonts are real.
	css, err := http.Get(r.ShellURL + "/assets/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	cssBody, _ := io.ReadAll(css.Body)
	css.Body.Close()
	if strings.Contains(string(cssBody), "https://") {
		t.Fatal("token source reaches an external host")
	}
	if !strings.Contains(string(cssBody), "@font-face") {
		t.Fatal("token source carries no vendored fonts")
	}
	// The conversation is capped and centred rather than run to the width of
	// whatever screen it is on.
	for _, want := range []string{"--chat-max:", ".centred{padding-inline:max("} {
		if !strings.Contains(string(cssBody), want) {
			t.Fatalf("the token source does not hold the conversation's measure (%q)", want)
		}
	}
	// The icon the browser asks for on its own is served, and vendored: no
	// 404 in the log, no fetch off the machine.
	ico, err := http.Get(r.ShellURL + "/favicon.ico")
	if err != nil {
		t.Fatal(err)
	}
	icoBody, _ := io.ReadAll(ico.Body)
	ico.Body.Close()
	if ico.StatusCode != http.StatusOK {
		t.Fatalf("/favicon.ico = %d", ico.StatusCode)
	}
	if len(icoBody) < 4 || !bytes.Equal(icoBody[:4], []byte{0, 0, 1, 0}) {
		t.Fatalf("/favicon.ico is not an icon: %x", icoBody[:min(8, len(icoBody))])
	}
	font, err := http.Get(r.ShellURL + "/assets/fonts/archivo-400-800.woff2")
	if err != nil {
		t.Fatal(err)
	}
	magic := make([]byte, 4)
	io.ReadFull(font.Body, magic)
	font.Body.Close()
	if string(magic) != "wOF2" {
		t.Fatalf("font magic = %q", magic)
	}
	fmt.Printf("shell gate: ceremony, the chat shape (%s posted, answered and "+
		"tapped somebody by name), custody, offline render — all green\n", posted.Author)
}
