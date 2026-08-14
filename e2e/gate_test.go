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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/identity"
	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"
	siclient "github.com/impire-io/soulstream-identity/client"
	"github.com/impire-io/soulstream-idp/authtest"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

// seededTurn is the message the rig plants as the founding owner, so the
// gate has something to observe — and something to answer.
const seededTurn = "the gate is watching"

// startRig boots the deployment and seeds the conversation the ceremony
// reads: one topic, one message from the founding owner.
func startRig(t *testing.T) *rig.Rig {
	t.Helper()
	r, err := rig.Start(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, err := r.Owner(ctx)
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
	return r
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
		`id="conversations"`, `id="dash"`, `class="thread-body"`,
		`id="details"`, `id="composer"`, `class="dock centred"`, `id="composer-box"`,
		"Write a message…", `href="/status`, "System status",
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
	r := startRig(t)
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
	for _, want := range []string{seededTurn, "verified", `class="msg"`} {
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
	// Home is reachable from anywhere and renders inside the same frame: the
	// house at a glance, and a way into every conversation.
	overview := get(t, cl, r.ShellURL+"/home")
	for _, want := range []string{"Your realm at a glance", "Storage", "Conversations",
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
	ownTag := fmt.Sprintf(`<div class="msg mine" data-op=%q>`, posted.OpID)
	if !strings.Contains(seen, ownTag) {
		t.Fatalf("the person's own message is not rendered as theirs (%s):\n%s", ownTag, seen)
	}
	otherTag := fmt.Sprintf(`<div class="msg" data-op=%q>`, seeded.OpID)
	if !strings.Contains(seen, otherTag) {
		t.Fatalf("the owner's message is rendered as somebody else's (%s):\n%s", otherTag, seen)
	}
	if !strings.Contains(seen, ceremony.FoundingPersona) {
		t.Fatalf("the other person's message carries no name:\n%s", seen)
	}
	answerTag := fmt.Sprintf(`<div class="msg mine reply" data-op=%q>`, reply.OpID)
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
	if strings.Contains(side, "<a ") {
		t.Fatalf("the details panel offers a link that goes nowhere:\n%s", side)
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
	fmt.Printf("shell gate: ceremony, the chat shape (%s posted and answered), "+
		"custody, offline render — all green\n", posted.Author)
}
