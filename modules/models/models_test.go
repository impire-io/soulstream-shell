package models

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	infercat "github.com/impire-io/soulstream-inference/catalogue"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// routes records what a module claims, without serving any of it.
type routes struct{ patterns []string }

func (rt *routes) Handle(pattern string, _ http.Handler) {
	rt.patterns = append(rt.patterns, pattern)
}

func (rt *routes) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	rt.patterns = append(rt.patterns, pattern)
}

// The module claims its screen, its live channel, the form's prefill, the
// question removing stands behind, and its acts — and nothing another
// surface could be serving.
func TestTheModuleClaimsItsOwnRoutes(t *testing.T) {
	m := New(nil, nil)
	if got := m.Identity(); got.Slug != "models" || got.Name != "Models" {
		t.Errorf("the module names itself %+v", got)
	}
	var rt routes
	m.Mount(&rt)
	want := []string{"GET /models", "GET /models/live", "GET /models/edit",
		"GET /models/remove-ask",
		"POST /act/model-set", "POST /act/model-remove"}
	if !reflect.DeepEqual(rt.patterns, want) {
		t.Errorf("the module mounts %v, want %v", rt.patterns, want)
	}
}

// One key, carrying the open conversation like every key on the spine.
func TestTheModuleContributesOneKey(t *testing.T) {
	m := New(nil, nil)
	nav := m.Nav(httptest.NewRequest(http.MethodGet, "/models?topic=home%2Fkitchen", nil))
	if len(nav) != 1 || nav[0].Href != "/models?topic=home%2Fkitchen" || nav[0].Foot {
		t.Fatalf("the key is %+v", nav)
	}
}

// Every honest state of a row: where a name points is said in the
// person's words, the stored truth folds open beside it, and only the
// admin column offers Change and Remove — Remove behind its question.
func TestTheRowsSayTheirHonestStates(t *testing.T) {
	pinned := soulstream.Model{Name: "sonnet",
		Entry: infercat.Entry{Capability: "chat", ModelPin: "claude-sonnet-5"},
		JSON:  "{\n  \"capability\": \"chat\",\n  \"model_pin\": \"claude-sonnet-5\"\n}"}
	tagged := soulstream.Model{Name: "fast",
		Entry: infercat.Entry{Capability: "chat", Tags: map[string]string{"tier": "fast", "region": "eu"}}}
	loose := soulstream.Model{Name: "any", Entry: infercat.Entry{Capability: "chat"}}

	admin := renderList(view{Admin: true, Models: []soulstream.Model{pinned, tagged, loose}})
	for _, want := range []string{
		"claude-sonnet-5",                                // the pin, said plainly
		"whatever matches region:eu tier:fast",           // tags sorted, all must match
		"any instance that serves it",                    // the anycast entry in words
		">as stored<",                                    // the fold over the stored truth
		"&#34;model_pin&#34;: &#34;claude-sonnet-5&#34;", // the stored JSON, escaped, whole
		"/models/edit?name=sonnet",
		"/models/remove-ask?name=fast",
	} {
		if !strings.Contains(admin, want) {
			t.Errorf("the admin list is missing %q:\n%s", want, admin)
		}
	}

	reader := renderList(view{Models: []soulstream.Model{pinned}})
	for _, gone := range []string{"/models/edit", "/models/remove-ask"} {
		if strings.Contains(reader, gone) {
			t.Errorf("a non-admin list offers %q:\n%s", gone, reader)
		}
	}
}

// Empty states offer their act (design 0008's rule), and the serving
// block's words follow the deployment's declared fact — never a spinner,
// never an empty table implying a fault.
func TestTheEmptyStatesOfferTheirAct(t *testing.T) {
	adminEmpty := renderModels(view{Admin: true})
	for _, want := range []string{
		"Name a model", // the key above the empty list
		"soulstream model set sonnet --pin claude-sonnet-5", // the deployment's own hand
		"switched on where it runs",                         // fact absent: serving is configuration
		"SOULSTREAM_PROVIDER_KEY=&lt;your key&gt;",          // the placeholder, never a value
	} {
		if !strings.Contains(adminEmpty, want) {
			t.Errorf("the admin empty screen is missing %q", want)
		}
	}

	readerEmpty := renderModels(view{})
	if !strings.Contains(readerEmpty, "an administrator names the first") {
		t.Error("the reader's empty list does not say whose act is missing")
	}
	for _, gone := range []string{"Name a model", "SOULSTREAM_PROVIDER_KEY"} {
		if strings.Contains(readerEmpty, gone) {
			t.Errorf("the reader's screen offers %q", gone)
		}
	}

	waiting := renderServing(view{On: true})
	if !strings.Contains(waiting, "No instance is answering right now") {
		t.Error("the fact-on empty serving block does not say honest waiting")
	}

	serving := renderServing(view{Serving: []soulstream.Serving{{
		ID: "NBS3…", Model: "claude-sonnet-5", Capability: "chat",
		Tags:    map[string]string{"model": "claude-sonnet-5", "tier": "fast"},
		Formats: []string{"text/plain"},
	}}})
	for _, want := range []string{
		`title="NBS3…"`, // the raw id rides the hover
		"tier:fast",     // tags shown
		"answers text/plain",
	} {
		if !strings.Contains(serving, want) {
			t.Errorf("the serving row is missing %q:\n%s", want, serving)
		}
	}
	if strings.Contains(serving, "model:claude-sonnet-5") {
		t.Error("the serving row repeats the model as a tag the column already says")
	}
}

// The form prefills from a standing entry — pointing a name somewhere
// else is the same act as naming it, so it is the same form.
func TestTheFormPrefills(t *testing.T) {
	pinned := formPanel("sonnet", infercat.Entry{Capability: "chat", ModelPin: "claude-sonnet-5"},
		[]string{"claude-sonnet-5", "claude-fable-5"})
	for _, want := range []string{
		`value="sonnet"`, `value="chat"`, `value="claude-sonnet-5"`,
		`{points:'model'}`, `<option value="model" selected>`,
		`<option value="claude-fable-5">`, // discovery's suggestion, never a bound
	} {
		if !strings.Contains(pinned, want) {
			t.Errorf("the prefilled form is missing %q", want)
		}
	}
	tagged := formPanel("fast", infercat.Entry{Capability: "chat",
		Tags: map[string]string{"tier": "fast", "region": "eu"}}, nil)
	if !strings.Contains(tagged, `value="region:eu tier:fast"`) {
		t.Errorf("the tags field does not carry the standing pairs:\n%s", tagged)
	}
}

// The result line belongs to the acts: what the live channel morphs must
// not contain it, or a one-shot answer and the stream would write the
// same element.
func TestTheLiveElementsNeverCarryTheResultLine(t *testing.T) {
	v := view{Admin: true, On: true,
		Models:  []soulstream.Model{{Name: "sonnet", Entry: infercat.Entry{Capability: "chat"}}},
		Serving: []soulstream.Serving{{ID: "x", Model: "m", Capability: "chat"}}}
	for name, frag := range map[string]string{"list": renderList(v), "serving": renderServing(v)} {
		if strings.Contains(frag, resultID) {
			t.Errorf("the %s fragment carries the result line", name)
		}
	}
}

// Tags are read the way a person types them, and what is not a pair is
// refused with the pair spelled out.
func TestTagsParseLikeAPersonTypes(t *testing.T) {
	tags, err := parseTags("tier:fast, region:eu")
	if err != nil || tags["tier"] != "fast" || tags["region"] != "eu" {
		t.Fatalf("parseTags: %v %v", tags, err)
	}
	if _, err := parseTags("fast"); err == nil || !strings.Contains(err.Error(), "key:value") {
		t.Fatalf("a bare word must refuse with the shape spelled out, got %v", err)
	}
	if _, err := parseTags("  "); err == nil {
		t.Fatal("empty tags must refuse — the mode asked for at least one pair")
	}
}

// The screen keeps plain words: the machine room's vocabulary appears
// nowhere a person is served.
func TestTheScreenKeepsPlainWords(t *testing.T) {
	v := view{Admin: true, On: true,
		Models: []soulstream.Model{
			{Name: "sonnet", Entry: infercat.Entry{Capability: "chat", ModelPin: "m"}},
			{Name: "fast", Entry: infercat.Entry{Capability: "chat", Tags: map[string]string{"t": "v"}}},
		},
		Serving: []soulstream.Serving{{ID: "x", Model: "m", Capability: "chat"}}}
	screen := renderModels(v) + removeConfirm("sonnet", "No declared agent names it.") +
		formPanel("", infercat.Entry{}, nil)
	for _, banned := range []string{"realm", "catalogue", "anycast", "unicast", "bucket"} {
		if strings.Contains(strings.ToLower(screen), banned) {
			t.Errorf("the screen says %q — machine-room vocabulary on a served page", banned)
		}
	}
}
