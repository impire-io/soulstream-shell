// The other half of Bar 4: a module written outside the component, seated
// on the frame with nothing of this product anywhere near it.
//
// The arm composes the shell itself rather than through the product's embed
// seam, because that is the position being measured: somebody who has the
// exported frame, an authorization server, and modules of their own. No
// node, no record, no module-support layer, no soulstream package of any
// kind — and no shell change of any kind either. What is left is the
// contract, and either it is enough to make a whole surface or it is not.
package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-idp/authtest"

	"github.com/impire-io/soulstream-shell/shell"

	"soulstream-shell.invalid/e2e/rig"
	"soulstream-shell.invalid/moduleprobe"
)

// probePerson is the one human in this arm. The name is nobody's convention
// but this test's — the frame has no opinion about who signs in.
const probePerson = "outsider"

// outsideBrand is the product the frame carries here: not this one, and
// nothing this one supplied. Every word a person reads on these screens
// comes from this value, which is what makes "the shell is agnostic" a
// thing you can see rather than a thing the import graph implies.
func outsideBrand() shell.Brand {
	return shell.Brand{
		Wordmark: "outpost", Strip: "console", Where: "somebody else's deployment",
		SignIn:  "This console belongs to somebody else — sign in to see it.",
		Action:  "Sign in",
		Promise: "nothing on this screen is kept anywhere",
	}
}

// bystander is a second module from outside, and the reason the removal arm
// has anything to look at: with the probe left out, the frame still has a
// screen to render, and the rail on it is what shows the probe is gone
// rather than merely unlinked.
type bystander struct{ sh *shell.Shell }

func (b *bystander) Identity() shell.Identity {
	return shell.Identity{Slug: "bystander", Name: "Elsewhere"}
}
func (b *bystander) Active(context.Context) bool { return true }
func (b *bystander) Nav(*http.Request) []shell.NavEntry {
	return []shell.NavEntry{{Section: "elsewhere", Icon: "home", Label: "Elsewhere",
		Href: "/elsewhere"}}
}

func (b *bystander) Mount(rt shell.Router) {
	rt.HandleFunc("GET /elsewhere", func(w http.ResponseWriter, r *http.Request) {
		if b.sh.Session(r) == nil {
			b.sh.SignIn(w, r)
			return
		}
		b.sh.Render(w, r, shell.Page{Title: "elsewhere", Section: "elsewhere",
			Body: b.sh.Sheet(`<h1>Elsewhere</h1>`)})
	})
}

// frame composes and serves a shell the way somebody outside this product
// would: shell.New, whatever modules they wrote, shell.Run. It returns where
// it is serving.
func frame(t *testing.T, issuer, home string, mods func(*shell.Shell) []shell.Module) string {
	t.Helper()
	ready := make(chan string, 1)
	sh, err := shell.New(shell.Options{
		Listen: "127.0.0.1:0", Issuer: issuer, ClientName: "outpost console",
		SessionCookie: "outpost_session", Home: home, Brand: outsideBrand(),
		Ready: func(addr string) { ready <- addr },
	})
	if err != nil {
		t.Fatal(err)
	}
	sh.Register(mods(sh)...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	go func() { errCh <- sh.Run(ctx) }()
	select {
	case addr := <-ready:
		return "http://" + addr
	case err := <-errCh:
		t.Fatalf("the frame did not serve: %v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("the frame never became ready")
	}
	return ""
}

func TestOutsideModuleGate(t *testing.T) {
	// The whole deployment: an authorization server, and nothing else.
	iss, err := rig.StartIssuer(t.TempDir(), probePerson)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(iss.Close)
	auth, err := authtest.New("localhost", iss.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := iss.Enroll(auth, probePerson); err != nil {
		t.Fatal(err)
	}

	// Composed in: the outside module, beside another outsider, on the
	// exported frame. Nothing was handed to either of them.
	with := frame(t, iss.URL, "/probe", func(sh *shell.Shell) []shell.Module {
		return []shell.Module{moduleprobe.New(sh), &bystander{sh: sh}}
	})

	// Before anybody signs in, the module's path is the frame's own card —
	// in the words of the product composed here, not of this one.
	gate := get(t, &http.Client{}, with+"/probe")
	for _, want := range []string{"outpost", "console",
		"This console belongs to somebody else", `href="/login"`} {
		if !strings.Contains(gate, want) {
			t.Fatalf("the outsider's sign-in card is missing %q: %s", want, gate)
		}
	}

	cl, landed, err := iss.SignInTo(with, auth, probePerson)
	if err != nil {
		t.Fatal(err)
	}
	// Sign-in lands on the outside module's own screen, because that is
	// where this deployment said its people belong — and it is a whole
	// screen: the frame's sidebar with its wordmark and this module's key
	// marked on it, the frame's way out, and the module's own body.
	for _, want := range []string{
		"<title>probe — outpost</title>",
		`<span class="wordmark">outpost</span>`, `<span class="strip">console</span>`,
		`<div class="ir-brand">`, `<nav class="iconrail"`,
		`class="ir on" href="/probe"`, `<span class="lbl">Probe</span>`,
		`<span class="lbl">Elsewhere</span>`, "Sign out",
		"<h1>Probe</h1>", "compiled against the exported contract alone",
		`<p class="foot">outpost · console · nothing on this screen is kept anywhere</p>`,
	} {
		if !strings.Contains(landed, want) {
			t.Fatalf("the outside module's screen is missing %q: %s", want, landed)
		}
	}
	// And the whole of it says nothing about the product this frame usually
	// carries. The import graph says the shell is agnostic; this is the same
	// claim in the only place a person would ever check it.
	for _, said := range []string{"soulstream", "realm", "conversation", "sign-in name"} {
		if strings.Contains(strings.ToLower(landed), said) {
			t.Fatalf("the frame says %q on somebody else's product: %s", said, landed)
		}
	}
	// The frame claims no screen of its own: the only paths that answer are
	// sign-in, the assets, and what the modules claimed.
	if got := statusOf(t, cl, http.MethodGet, with+"/"); got != http.StatusNotFound {
		t.Fatalf("the frame serves / on its own = %d, want 404", got)
	}

	// Left out of the composition: the same frame, the same authorization
	// server, the same other module — and the probe nowhere. Not hidden, not
	// refused: absent, the way a module nobody registered is absent.
	without := frame(t, iss.URL, "/elsewhere", func(sh *shell.Shell) []shell.Module {
		return []shell.Module{&bystander{sh: sh}}
	})
	plain, page, err := iss.SignInTo(without, auth, probePerson)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, `<span class="lbl">Elsewhere</span>`) {
		t.Fatalf("the frame without the probe renders nothing: %s", page)
	}
	for _, gone := range []string{"Probe", `href="/probe"`} {
		if strings.Contains(page, gone) {
			t.Fatalf("a module nobody composed is still on the rail (%q): %s", gone, page)
		}
	}
	if got := statusOf(t, plain, http.MethodGet, without+"/probe"); got != http.StatusNotFound {
		t.Fatalf("the uncomposed module's path = %d, want 404", got)
	}

	// Sign out is the frame's, on somebody else's product too.
	if _, err := cl.Post(with+"/logout", "", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(get(t, cl, with+"/probe"), "Sign out") {
		t.Fatal("session survived logout")
	}
	fmt.Println("outside-module gate: a module from another module path, on the " +
		"exported contract alone, seated on a frame carrying somebody else's product")
}
