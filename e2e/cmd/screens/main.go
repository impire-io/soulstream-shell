// Command screens stands a whole deployment up and leaves it running, so a
// person can look at the shell rather than read about it: a soulnode with
// its realm, identity plane and fold, the shell on top, one seeded
// conversation with more than one voice in it, and a signed-in session.
//
// It prints the address and the session cookie, and blocks until it is
// interrupted. There is no browser in here — the headless gate stays
// headless; this only makes something worth pointing a browser at.
//
//	SHELL_URL=http://127.0.0.1:54321/?topic=kitchen-table-abcd
//	SESSION_COOKIE=helm_session=…
//
// The address names the seeded conversation, so a browser lands on the one
// with people in it; the rail reaches the others.
//
// The signed-in person is the passkey-enrolled founding persona, so their
// own messages are the ones that render as theirs. Everyone else in the
// conversation is somebody else on the record, and renders that way.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"
	"github.com/impire-io/soulstream-idp/authtest"
	"github.com/impire-io/soulstream/ceremony"

	"soulstream-shell.invalid/e2e/rig"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
	if err := run(); err != nil {
		log.Fatalf("screens: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dir, err := os.MkdirTemp("", "soulstream-screens-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	log.Printf("founding a realm in %s …", dir)
	r, err := rig.Start(dir)
	if err != nil {
		return err
	}
	defer r.Close()

	boot, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	owner, err := r.Owner(boot)
	if err != nil {
		return err
	}
	avery, err := r.Voice(boot, "avery", "Avery")
	if err != nil {
		return err
	}

	// A quieter conversation first, so the rail has more than one row and
	// the open one is visibly the open one. The last topic announced is the
	// one the shell opens by default.
	if err := seedQuiet(boot, owner); err != nil {
		return err
	}
	kitchen, err := topic.StartTopic(boot, owner, topic.StartTopicInput{
		Name: "Kitchen table", SubjectMatter: "the house, out loud",
	})
	if err != nil {
		return fmt.Errorf("start the conversation: %w", err)
	}
	path := kitchen.Path()

	log.Printf("enrolling a passkey and signing in …")
	auth, err := authtest.New("localhost", r.Issuer)
	if err != nil {
		return err
	}
	if err := r.Enroll(auth, ceremony.FoundingPersona); err != nil {
		return err
	}
	cl, _, err := r.SignIn(auth, ceremony.FoundingPersona)
	if err != nil {
		return err
	}

	if err := seedConversation(boot, r, cl, owner, avery, path); err != nil {
		return err
	}

	cookie, ok := r.Cookie(cl)
	if !ok {
		return fmt.Errorf("the session carries no %s cookie", rig.SessionCookie)
	}
	fmt.Printf("SHELL_URL=%s/?topic=%s\n", r.ShellURL, url.QueryEscape(path))
	fmt.Printf("SESSION_COOKIE=%s=%s\n", cookie.Name, cookie.Value)
	log.Printf("serving — ^C to tear it all down")
	<-ctx.Done()
	log.Printf("draining …")
	return nil
}

// seedQuiet is the second conversation in the rail: announced, barely used.
func seedQuiet(ctx context.Context, owner *realm.Client) error {
	h, err := topic.StartTopic(ctx, owner, topic.StartTopicInput{
		Name: "Release checklist", SubjectMatter: "what has to be true before we ship",
	})
	if err != nil {
		return fmt.Errorf("start the quiet conversation: %w", err)
	}
	_, err = h.PostTurn(ctx, "Parking this here until the shell is worth showing.")
	return err
}

// seedConversation writes the transcript the screenshot is of: two other
// voices on the record, the signed-in person's own messages through the
// composer they would actually use, answers hanging off messages on both
// sides, a message with the signed-in person's name in it, and work in every
// state the details panel can show.
func seedConversation(ctx context.Context, r *rig.Rig, cl *http.Client,
	owner, avery *realm.Client, path string,
) error {
	oh, ah := topic.Open(owner, path), topic.Open(avery, path)
	if _, err := ah.Materialise(ctx); err != nil {
		return fmt.Errorf("read the conversation as avery: %w", err)
	}

	opening, err := oh.PostTurn(ctx, "Morning — kettle is on. Anyone need anything from the shop?")
	if err != nil {
		return fmt.Errorf("the owner's opening message: %w", err)
	}
	if _, err := ah.Materialise(ctx); err != nil {
		return err
	}
	if _, err := ah.PostTurn(ctx, "Milk, if you are going. And the good coffee."); err != nil {
		return fmt.Errorf("avery's message: %w", err)
	}

	// The signed-in person writes through the composer, over their own
	// session — the same lane a browser uses, so the record attributes
	// these to them and the shell renders them as theirs.
	if err := post(r, cl, path, url.Values{
		"body": {"Going in ten. Adding milk and the coffee."}, "reply-to": {opening},
	}); err != nil {
		return err
	}
	if err := post(r, cl, path, url.Values{"body": {"Back in twenty."}}); err != nil {
		return err
	}

	mine, err := latestBy(ctx, ah, "Back in twenty.")
	if err != nil {
		return err
	}
	if _, err := ah.AddComment(ctx, "You are a hero.", mine); err != nil {
		return fmt.Errorf("avery's answer: %w", err)
	}

	// Now that the record says who the signed-in person is, they can be
	// named — and tapped on the shoulder by that name. This is the shape the
	// composer's picker puts on the wire: the body says what somebody would
	// actually write, and who it taps rides beside it, so a screenshot shows
	// a meaningful tag rather than a machine-minted id.
	me, err := authorOf(ctx, ah, "Back in twenty.")
	if err != nil {
		return err
	}
	if err := r.Name(ctx, me, "Daan"); err != nil {
		return fmt.Errorf("name the signed-in person: %w", err)
	}
	if _, err := ah.PostTurnMentioning(ctx,
		"@Daan did the good coffee come in a bag or a tin?", []string{me}); err != nil {
		return fmt.Errorf("avery's mention: %w", err)
	}
	return seedWork(ctx, oh, ah)
}

// seedWork puts all three states of the work vocabulary on the record, so
// the details panel beside the conversation has something real to say: one
// thing waiting for anyone, one somebody already has in hand, one done and
// quietly counted.
func seedWork(ctx context.Context, oh, ah *topic.Handle) error {
	if _, err := oh.Materialise(ctx); err != nil {
		return err
	}
	if _, err := oh.OpenWork(ctx, "restock the coffee", "the good one, not the tin"); err != nil {
		return fmt.Errorf("the waiting work: %w", err)
	}
	claimed, err := oh.OpenWork(ctx, "fix the dripping tap", "it has been going all week")
	if err != nil {
		return fmt.Errorf("the claimed work: %w", err)
	}
	if _, err := ah.Materialise(ctx); err != nil {
		return err
	}
	if _, err := ah.ClaimWork(ctx, claimed); err != nil {
		return fmt.Errorf("avery takes it on: %w", err)
	}
	done, err := oh.OpenWork(ctx, "wipe the counter", "before the coffee dries on")
	if err != nil {
		return fmt.Errorf("the finished work: %w", err)
	}
	if _, err := oh.ClaimWork(ctx, done); err != nil {
		return fmt.Errorf("claim the finished work: %w", err)
	}
	if _, err := oh.CompleteWork(ctx, done); err != nil {
		return fmt.Errorf("finish the work: %w", err)
	}
	return nil
}

// post writes one message the way the browser's composer does.
func post(r *rig.Rig, cl *http.Client, path string, form url.Values) error {
	out, err := r.Post(cl, path, form)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "Posted as") {
		return fmt.Errorf("the composer refused %q: %s", form.Get("body"), out)
	}
	return nil
}

// latestBy finds the op id of a message by its body — the anchor an answer
// needs, resolved against the record like every other anchor.
func latestBy(ctx context.Context, h *topic.Handle, body string) (string, error) {
	c, err := latest(ctx, h, body)
	if err != nil {
		return "", err
	}
	return c.OpID, nil
}

// authorOf is how anything outside the shell learns the persona behind a
// session: the fold's subject reaches the realm only as the author of what
// that session writes, so the record is the one public witness of it.
func authorOf(ctx context.Context, h *topic.Handle, body string) (string, error) {
	c, err := latest(ctx, h, body)
	if err != nil {
		return "", err
	}
	return c.Author, nil
}

func latest(ctx context.Context, h *topic.Handle, body string) (*topic.Contribution, error) {
	mt, err := h.Materialise(ctx)
	if err != nil {
		return nil, err
	}
	for i := len(mt.Contributions) - 1; i >= 0; i-- {
		if mt.Contributions[i].Body == body {
			return &mt.Contributions[i], nil
		}
	}
	return nil, fmt.Errorf("no message %q on the record", body)
}
