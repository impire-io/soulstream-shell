package models

import (
	"fmt"
	"sort"
	"strings"

	infercat "github.com/impire-io/soulstream-inference/catalogue"

	"github.com/impire-io/soulstream-shell/shell"
	"github.com/impire-io/soulstream-shell/soulstream"
)

// The screen: the model names as they stand, what serves right now, and
// the administrator's form. Plain words — people read about model names
// and what serves them; the machine room's own vocabulary stays there.

// The screen's patch targets. The form's result line is its own, inside
// the slide-over holding the form.
const (
	resultID   = "models-result"
	listID     = "models-list"
	servingID  = "models-serving"
	formID     = "model-form"
	formNoteID = "models-form-note"
)

// view is one read of everything the screen shows.
type view struct {
	Models     []soulstream.Model
	ModelsErr  string
	Serving    []soulstream.Serving
	ServingErr string
	// On is the deployment's declared word that it serves models itself —
	// it shapes the empty states' words, never the reading.
	On    bool
	Admin bool
}

// renderModels is the whole screen's body. The names lead; the form
// waits in the slide-over behind its own key, because most visits are
// about where the names point today.
func renderModels(v view) string {
	var b strings.Builder
	b.WriteString(`<h1>Models</h1>`)
	b.WriteString(`<p class="lede">The model names agents think through. A name is not a model — ` +
		`it points at one, and changing where it points moves every agent naming it, with ` +
		`nothing restarted.</p>`)
	key := ""
	if v.Admin {
		key = `<p class="act"><button type="button" class="btn" data-on:click="$panel = true">` +
			`Name a model</button></p>`
	}
	b.WriteString(`<div class="section">` + key + renderList(v) + `</div>`)
	b.WriteString(resultNote(""))
	b.WriteString(`<div class="section">` + renderServing(v))
	if v.Admin {
		b.WriteString(providerFold())
	}
	b.WriteString(`</div>`)
	if v.Admin {
		b.WriteString(shell.SlideOver("Name a model", formPanel("", infercat.Entry{}, knownFrom(v.Serving))))
	}
	return b.String()
}

// renderList is the names as they stand — and a patch target of its own
// so an act can hand back what is now true.
func renderList(v view) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, listID)
	switch {
	case v.ModelsErr != "":
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.ModelsErr))
	case len(v.Models) == 0 && v.Admin:
		b.WriteString(`<p class="blank">No model names yet. A model name is what an agent is ` +
			`declared to think through — name the first with the key above, or from the ` +
			`deployment&#39;s own terminal:</p>` +
			`<pre class="mono">soulstream model set sonnet --pin claude-sonnet-5</pre>`)
	case len(v.Models) == 0:
		b.WriteString(`<p class="blank">No model names yet. A model name is what an agent is ` +
			`declared to think through — an administrator names the first.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Name</th><th>Kind of work</th><th>Points at</th><th>Actions</th>` +
			`</tr></thead><tbody>`)
		for _, mo := range v.Models {
			b.WriteString(modelRow(mo, v))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// modelRow is one name: where it points in the person's words, the
// stored truth folded whole, and the acts the admin column honestly
// offers.
func modelRow(mo soulstream.Model, v view) string {
	points := esc(pointsWords(mo.Entry))
	stored := ""
	if mo.JSON != "" {
		stored = fmt.Sprintf(`<details class="stow"><summary>as stored</summary>`+
			`<pre class="mono">%s</pre></details>`, esc(mo.JSON))
	}
	var acts []string
	if v.Admin {
		acts = append(acts, fmt.Sprintf(
			`<button class="btn ghost" data-on:click="@get('/models/edit?name=%s')">Change</button>`,
			qesc(mo.Name)))
		// Removing stands behind its own question (askRemove): one stray
		// tap takes nothing away from the agents naming it.
		acts = append(acts, fmt.Sprintf(
			`<button class="btn ghost" data-on:click="@get('/models/remove-ask?name=%s')">Remove</button>`,
			qesc(mo.Name)))
	}
	return fmt.Sprintf(`<tr><td class="mono">%s</td><td>%s</td><td>%s%s</td>`+
		`<td><div class="acts">%s</div></td></tr>`,
		esc(mo.Name), esc(mo.Entry.Capability), points, stored, strings.Join(acts, ""))
}

// pointsWords is where an entry points, in the person's words — the row's
// column and the act's answer say the same thing.
func pointsWords(e infercat.Entry) string {
	switch {
	case e.ModelPin != "":
		return e.ModelPin
	case len(e.Tags) > 0:
		pairs := make([]string, 0, len(e.Tags))
		for k, v := range e.Tags {
			pairs = append(pairs, k+":"+v)
		}
		sort.Strings(pairs)
		return "whatever matches " + strings.Join(pairs, " ")
	default:
		return "any instance that serves it"
	}
}

// pointsMode is the form's mode for one standing entry.
func pointsMode(e infercat.Entry) string {
	switch {
	case e.ModelPin != "":
		return "model"
	case len(e.Tags) > 0:
		return "tags"
	default:
		return "any"
	}
}

// renderServing is what actually serves right now — a reading of the
// moment, morphed whole by the live channel. The deployment's declared
// word shapes only the empty state: what serves is discovered, so an
// instance run beside the deployment still shows.
func renderServing(v view) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s"><h2>Serving now</h2>`, servingID)
	switch {
	case v.ServingErr != "":
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.ServingErr))
	case len(v.Serving) == 0 && !v.On:
		b.WriteString(`<p class="blank">Nothing serves models here yet — model serving is part ` +
			`of the deployment&#39;s configuration, switched on where it runs.</p>`)
	case len(v.Serving) == 0:
		b.WriteString(`<p class="blank">Model serving is on. No instance is answering right now.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Model</th><th>Kind of work</th><th>Details</th>` +
			`</tr></thead><tbody>`)
		for _, s := range v.Serving {
			b.WriteString(servingRow(s))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// servingRow is one instance: the model it wraps, what it serves, and
// its particulars — the raw instance id rides the hover (design 0007).
func servingRow(s soulstream.Serving) string {
	var details []string
	pairs := make([]string, 0, len(s.Tags))
	for k, val := range s.Tags {
		if k == "model" {
			continue // the model column already says it
		}
		pairs = append(pairs, k+":"+val)
	}
	sort.Strings(pairs)
	details = append(details, pairs...)
	if len(s.Formats) > 0 {
		details = append(details, "answers "+strings.Join(s.Formats, ", "))
	}
	return fmt.Sprintf(`<tr><td class="mono" title="%s">%s</td><td>%s</td><td>%s</td></tr>`,
		esc(s.ID), esc(s.Model), esc(s.Capability), esc(strings.Join(details, " · ")))
}

// providerFold is the administrator's folded reminder of where provider
// keys actually go: not through this screen. The command carries a
// placeholder the person fills on the deployment's own machine — no key
// value exists anywhere this surface serves.
func providerFold() string {
	return `<details class="stow"><summary>Give it a provider key</summary>` +
		`<p class="note">Provider keys never pass through this screen — they live where the ` +
		`deployment runs, in its own keeping. On that machine:</p>` +
		`<pre class="mono">env SOULSTREAM_PROVIDER_KEY=&lt;your key&gt; soulstream provider set anthropic</pre>` +
		`<p class="note">Then name the provider in the deployment&#39;s model-serving ` +
		`configuration, and what serves shows up above on its own.</p></details>`
}

// formPanel is the one form, naming and re-pointing alike — the
// slide-over's whole body, and the edit act's patch target. Its output
// is the identical entry the deployment's own model verb writes; the
// refusals a bad one earns are upstream's, surfaced in its own words.
func formPanel(name string, e infercat.Entry, known []string) string {
	var b strings.Builder
	mode := pointsMode(e)
	fmt.Fprintf(&b, `<form id="%s" data-signals="{points:'%s'}" `+
		`data-on:submit="@post('/act/model-set', {contentType:'form'})">`, formID, mode)
	b.WriteString(`<p class="lede">A model name is what agents are declared to think through. ` +
		`Where it points can change any time — every agent naming it follows at its next ` +
		`request, nothing restarted.</p>`)
	b.WriteString(`<div class="fields">`)
	fmt.Fprintf(&b, `<label class="field">Name<input name="name" required autocomplete="off" `+
		`spellcheck="false" placeholder="lowercase, no spaces" value="%s"></label>`, esc(name))
	fmt.Fprintf(&b, `<label class="field">Kind of work<input name="capability" autocomplete="off" `+
		`spellcheck="false" value="%s"></label>`, esc(capabilityOr(e, "chat")))
	b.WriteString(`<label class="field">Points at<select name="points" data-bind:points>` +
		option("any", "any instance that serves it", mode) +
		option("model", "one model, wherever it serves", mode) +
		option("tags", "whatever matches tags", mode) +
		`</select></label>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div data-show="$points == 'model'"><div class="fields">`)
	fmt.Fprintf(&b, `<label class="field">Model<input name="model_pin" `+
		`data-attr:disabled="$points != 'model'" autocomplete="off" spellcheck="false" `+
		`list="models-known" placeholder="as the serving instance names it" value="%s"></label>`,
		esc(e.ModelPin))
	b.WriteString(datalist(known))
	b.WriteString(`</div></div>`)
	b.WriteString(`<div data-show="$points == 'tags'"><div class="fields">`)
	fmt.Fprintf(&b, `<label class="field">Tags<input name="tags" `+
		`data-attr:disabled="$points != 'tags'" autocomplete="off" spellcheck="false" `+
		`placeholder="key:value, space-separated — every one must match" value="%s"></label>`,
		esc(tagsValue(e)))
	b.WriteString(`</div></div>`)
	b.WriteString(`<button class="btn" type="submit">Save model name</button>`)
	b.WriteString(formNote(""))
	b.WriteString(`</form>`)
	return b.String()
}

// capabilityOr fills the form's default for a fresh name and keeps the
// entry's own word for a standing one.
func capabilityOr(e infercat.Entry, fresh string) string {
	if e.Capability != "" {
		return e.Capability
	}
	return fresh
}

// option is one select option, marked when it is the standing mode.
func option(value, label, mode string) string {
	sel := ""
	if value == mode {
		sel = " selected"
	}
	return fmt.Sprintf(`<option value="%s"%s>%s</option>`, value, sel, label)
}

// datalist is the pin field's suggestions — what discovery answers right
// now, never the field's bounds.
func datalist(known []string) string {
	var b strings.Builder
	b.WriteString(`<datalist id="models-known">`)
	for _, k := range known {
		fmt.Fprintf(&b, `<option value="%s"></option>`, esc(k))
	}
	b.WriteString(`</datalist>`)
	return b.String()
}

// tagsValue is a standing entry's tags the way the field takes them.
func tagsValue(e infercat.Entry) string {
	pairs := make([]string, 0, len(e.Tags))
	for k, v := range e.Tags {
		pairs = append(pairs, k+":"+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, " ")
}

// knownFrom is the distinct models discovery answered, in serving order.
func knownFrom(serving []soulstream.Serving) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range serving {
		if s.Model != "" && !seen[s.Model] {
			seen[s.Model] = true
			out = append(out, s.Model)
		}
	}
	return out
}

// removeConfirm is the question removing stands behind: what it does to
// the agents naming the name, and the two ways out.
func removeConfirm(name, usage string) string {
	sentence := "Remove " + name + " for everyone?"
	if usage != "" {
		sentence += " " + usage
	}
	return fmt.Sprintf(`<div id="%s" class="note">`+
		`<p class="confirm">%s</p>`+
		`<div class="acts">`+
		`<button class="btn" data-on:click="@post('/act/model-remove?name=%s')">Yes, remove it</button>`+
		`<button class="btn ghost" data-on:click="@get('/models/remove-ask')">Keep it</button>`+
		`</div></div>`, resultID, esc(sentence), qesc(name))
}

// formNote is the slide-over form's own result line.
func formNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, formNoteID, esc(msg))
}

// resultNote is the screen's result line — the patch target every act
// answers to, and never the live channel's to write.
func resultNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, resultID, esc(msg))
}
