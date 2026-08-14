// Package moduleprobe is a shell module written from outside the product
// the shell usually frames.
//
// It imports one package — the exported frame — and nothing else: no
// module-support layer, no module beside it, no component of the ecosystem,
// and (by module path, so the compiler enforces it rather than a reviewer)
// nothing internal to the shell. On those terms it is a whole human surface:
// it names itself, says this deployment runs it, puts a key on the rail, and
// claims a route it answers on.
//
// What it is for is the claim that the contract is a contract. The product's
// own modules are written by the people who wrote the shell, in the same
// repository, and could be plugging into something narrower than an exported
// seam without anybody noticing. This one cannot be: it was compiled from
// outside, it was composed by a deployment the shell knows nothing about, and
// the shell did not change by a line to seat it.
package moduleprobe

import (
	"context"
	"net/http"

	"github.com/impire-io/soulstream-shell/shell"
)

// section is this module's own key for its one screen — the shell compares
// it with what a page says it is showing, and marks the rail with it.
const section = "probe"

// Module is the outside surface.
type Module struct{ sh *shell.Shell }

// New builds the module over the frame it hangs in. There is no second
// argument: this module reads nothing, writes nothing and is handed nothing,
// which is what makes it a fair test of the frame alone.
func New(sh *shell.Shell) *Module { return &Module{sh: sh} }

// Identity names the module — the slug the shell keys it by, and what a
// person would call it.
func (m *Module) Identity() shell.Identity {
	return shell.Identity{Slug: "probe", Name: "Probe"}
}

// Active reports that this deployment runs it: this module needs nothing of
// a deployment, so there is nothing for it to find missing.
func (m *Module) Active(context.Context) bool { return true }

// Nav is its one key on the rail.
func (m *Module) Nav(*http.Request) []shell.NavEntry {
	return []shell.NavEntry{{
		Section: section, Icon: "radio", Label: "Probe", Href: "/probe",
	}}
}

// Mount claims the one path it answers on, and no other.
func (m *Module) Mount(rt shell.Router) { rt.HandleFunc("GET /probe", m.screen) }

// screen is the whole surface: the frame's own sheet with a few honest
// sentences on it. Sign-in belongs to the frame, so an unsigned visitor gets
// the frame's card rather than anything this module invents.
func (m *Module) screen(w http.ResponseWriter, r *http.Request) {
	if m.sh.Session(r) == nil {
		m.sh.SignIn(w, r)
		return
	}
	m.sh.Render(w, r, shell.Page{
		Title: "probe", Section: section,
		Body: m.sh.Sheet(`<h1>Probe</h1>` +
			`<p class="lede">This screen was written outside the frame that is ` +
			`drawing it, and compiled against the exported contract alone.</p>` +
			`<p class="lede">Everything around it — the bar above, the keys at ` +
			`the left, the way out at the foot of them, the words this ` +
			`deployment calls itself by — belongs to the frame. Everything ` +
			`between the keys and this sentence belongs to a module the frame ` +
			`had never heard of when it was built.</p>`),
	})
}
