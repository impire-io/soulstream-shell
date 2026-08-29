// Package models is the shell module where a person sees the model names
// agents think through and where it all stands — and where an
// administrator names a model, points it somewhere else, or takes the
// name away (hq design soulstream-shell 0010, the models surface).
//
// The lane rules are the design's: reading rides the shared read lane —
// the names are the realm's own configuration and this surface is the
// party managing it, discovery included; writing is the person's own act
// on their own admission, because the substrate's own permission carries
// it for every persona alike. The admin gate on the acts is drawn and
// named for what it is — a courtesy line, not a wall: the screen must
// never imply it narrows what the realm actually permits.
package models

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	infercat "github.com/impire-io/soulstream-inference/catalogue"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// esc and qesc are the frame's own escaping, named short because most of
// this module is markup.
var (
	esc  = shell.Esc
	qesc = shell.QueryEsc
)

// This module's key on the spine.
const sectionModels = "models"

// Module is the models surface.
type Module struct {
	sh *shell.Shell
	sp *soulstream.Support
}

// New builds the module over a shell and the Soulstream support layer.
func New(sh *shell.Shell, sp *soulstream.Support) *Module {
	return &Module{sh: sh, sp: sp}
}

// Identity names the module.
func (m *Module) Identity() shell.Identity {
	return shell.Identity{Slug: "models", Name: "Models"}
}

// Active reports that this deployment runs the module: the names are
// realm configuration any deployment may hold — seeded before anything
// serves them, even — so a deployment with a record to read has a models
// screen, and an empty one's whole answer is the way to add the first.
func (m *Module) Active(context.Context) bool { return true }

// Nav is this module's one key, carrying the open conversation the way
// every key on the spine does.
func (m *Module) Nav(r *http.Request) []shell.NavEntry {
	return []shell.NavEntry{{
		Section: sectionModels, Icon: "activity", Label: "Models",
		Href: "/models" + topicQuery(r.URL.Query().Get("topic")),
	}}
}

// Mount claims the screen, its live channel, the form's prefill, the
// question removing stands behind, and the acts.
func (m *Module) Mount(rt shell.Router) {
	rt.HandleFunc("GET /models", m.models)
	rt.HandleFunc("GET /models/live", m.live)
	rt.HandleFunc("GET /models/edit", m.edit)
	rt.HandleFunc("GET /models/remove-ask", m.askRemove)
	rt.HandleFunc("POST /act/model-set", m.actSet)
	rt.HandleFunc("POST /act/model-remove", m.actRemove)
}

// topicQuery carries the open conversation across screens.
func topicQuery(topicPath string) string {
	if topicPath == "" {
		return ""
	}
	return "?topic=" + qesc(topicPath)
}

// models is the screen: the names as they stand, what serves right now,
// and the administrator's form waiting in the slide-over.
func (m *Module) models(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Redirect(w, r, m.sh.Home(), http.StatusFound)
		return
	}
	m.sh.Render(w, r, shell.Page{
		Title: "models", Section: sectionModels, Live: true,
		Init: "@get('/models/live')",
		Body: m.sh.Sheet(renderModels(m.view(r.Context(), sess))),
	})
}

// view is one read of everything the screen shows — reconstructed at
// every render that asks, no store.
func (m *Module) view(ctx context.Context, sess *soulstream.Session) view {
	v := view{Admin: sess.IsAdmin(), On: m.sp.InferenceOn()}
	models, err := m.sp.Models(ctx)
	if err != nil {
		v.ModelsErr = err.Error()
	}
	v.Models = models
	serving, err := m.sp.Serving()
	if err != nil {
		v.ServingErr = err.Error()
	}
	v.Serving = serving
	return v
}

// live re-reads the names and re-scatters discovery every few seconds and
// morphs both elements whole — where a name points and what serves are
// each a judgment of the moment, and a screen left open must keep
// judging. The result line is never touched: it belongs to the acts, and
// a one-shot answer and the stream must never write the same element.
func (m *Module) live(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	shell.Stream(w, r, 5*time.Second, func(out io.Writer) {
		v := m.view(r.Context(), sess)
		shell.WriteElements(out, renderList(v))
		shell.WriteElements(out, renderServing(v))
	})
}

// edit hands the form back prefilled from one standing entry and opens
// the panel — pointing a name somewhere else is the same act as naming
// it, so it is the same form.
func (m *Module) edit(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	name := r.URL.Query().Get("name")
	models, err := m.sp.Models(r.Context())
	if err != nil {
		shell.Patch(w, resultNote("Reading "+name+" failed: "+err.Error()))
		return
	}
	for _, mo := range models {
		if mo.Name == name {
			shell.Patch(w, formPanel(mo.Name, mo.Entry, m.knownModels()))
			shell.PatchSignals(w, fmt.Sprintf(`{panel: true, points: %q}`, pointsMode(mo.Entry)))
			return
		}
	}
	shell.Patch(w, resultNote("There is no model name called "+name+" here now."))
}

// askRemove patches the question removing stands behind — or, asked
// about nothing, clears it, which is what "Keep it" does. The question
// counts the declared agents naming the name where this deployment
// places any, because taking a name away is exactly what makes their
// next serve refuse.
func (m *Module) askRemove(w http.ResponseWriter, r *http.Request) {
	if m.sp.Session(r) == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		shell.Patch(w, resultNote(""))
		return
	}
	shell.Patch(w, removeConfirm(name, m.usageWords(r.Context(), name)))
}

// usageWords is the honest count of declared agents naming one model
// name — counted where the deployment places agents at all, said as
// uncountable when the reading fails, and absent where no placements
// exist to count.
func (m *Module) usageWords(ctx context.Context, name string) string {
	if m.sp.PlacementsTopic() == "" {
		return ""
	}
	declared, err := m.sp.Declared(ctx)
	if err != nil {
		return "Which declared agents name it could not be checked right now."
	}
	n := 0
	for _, d := range declared {
		if d.Model == name {
			n++
		}
	}
	switch n {
	case 0:
		return "No declared agent names it."
	case 1:
		return "One declared agent names it — its next serve refuses until the name points again."
	default:
		return fmt.Sprintf("%d declared agents name it — their next serves refuse until the name points again.", n)
	}
}

// actSet names a model or points it somewhere else — one act, the same
// entry the deployment's own model verb writes, refusals in upstream's
// own words. Admin-gated the way the design says: the shell checks the
// session's role as a courtesy line, and claims nothing more for it.
func (m *Module) actSet(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil || !sess.IsAdmin() {
		shell.Patch(w, formNote("naming models needs an account that administers this deployment"))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, formNote("That form did not arrive whole: "+err.Error()))
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	e := infercat.Entry{Capability: strings.TrimSpace(r.PostFormValue("capability"))}
	switch r.PostFormValue("points") {
	case "model":
		e.ModelPin = strings.TrimSpace(r.PostFormValue("model_pin"))
	case "tags":
		tags, err := parseTags(r.PostFormValue("tags"))
		if err != nil {
			shell.Patch(w, formNote(err.Error()))
			return
		}
		e.Tags = tags
	}
	if err := sess.SetModel(r.Context(), name, e); err != nil {
		shell.Patch(w, formNote(err.Error()))
		return
	}
	// The panel goes away so the list holding the name is in front of the
	// person, with the answer under it.
	shell.PatchSignals(w, `{panel: false}`)
	shell.Patch(w, formNote(""))
	shell.Patch(w, resultNote(name+" points at "+pointsWords(e)+" now — every agent naming it follows at its next request."))
	m.patchList(w, r, sess)
}

// actRemove takes one name away, behind askRemove's question.
func (m *Module) actRemove(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil || !sess.IsAdmin() {
		shell.Patch(w, resultNote("removing model names needs an account that administers this deployment"))
		return
	}
	name := r.URL.Query().Get("name")
	if err := sess.RemoveModel(r.Context(), name); err != nil {
		shell.Patch(w, resultNote("Removing "+name+" failed: "+err.Error()))
		return
	}
	shell.Patch(w, resultNote(name+" is gone. Declared agents naming it refuse to serve until a name points again."))
	m.patchList(w, r, sess)
}

// patchList hands back the list as it now stands.
func (m *Module) patchList(w http.ResponseWriter, r *http.Request, sess *soulstream.Session) {
	shell.Patch(w, renderList(m.view(r.Context(), sess)))
}

// knownModels is what discovery answers right now, distinct and in
// order — the pin field's suggestions, never its bounds: a name may
// honestly point at a model nobody serves yet.
func (m *Module) knownModels() []string {
	serving, err := m.sp.Serving()
	if err != nil {
		return nil
	}
	return knownFrom(serving)
}

// parseTags reads key:value pairs the way a person types them —
// space- or comma-separated.
func parseTags(raw string) (map[string]string, error) {
	tags := map[string]string{}
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool { return r == ' ' || r == ',' }) {
		k, v, ok := strings.Cut(field, ":")
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("tags are key:value pairs — %q is not one", field)
		}
		tags[k] = v
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("pointing at tags needs at least one key:value pair")
	}
	return tags, nil
}
