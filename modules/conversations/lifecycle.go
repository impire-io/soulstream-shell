package conversations

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
)

// Where a conversation begins and where it ends. Both are acts on the
// record through the session's own admitted connection, like every act on
// this surface — the person's persona, the person's key.
//
// Starting answers with navigation: the new conversation is the outcome,
// so the browser goes there. The lifecycle acts answer into #convo-life,
// a target of their own beside the details panel — the panel itself
// belongs to the live stream, and a one-shot response and the stream must
// never write the same element.

// startFold is the fold at the head of the rail where a conversation
// begins: a name, one optional line about it, and Start.
func startFold() string {
	return `<details id="convo-start" class="archfold rail-start"><summary>` +
		string(shell.Icon("plus")) + `Start a conversation</summary>` +
		startForm() + `</details>`
}

// startForm is the form itself, shared by the fold and nothing else —
// Home draws its own card in its own shape, posting to the same act.
func startForm() string {
	return `<form data-on:submit="@post('/act/conversation-start', {contentType:'form'})">` +
		`<label class="field">Name` +
		`<input name="name" autocomplete="off" placeholder="what to call it"></label>` +
		`<label class="field">What it’s about` +
		`<input name="about" autocomplete="off" placeholder="one line — optional"></label>` +
		`<button class="btn" type="submit">Start</button>` +
		startNote("") + `</form>`
}

// startNote is the create form's own result line — only a failure ever
// lands here; a started conversation answers by going there.
func startNote(msg string) string {
	return `<div id="convo-start-note" class="note">` + esc(msg) + `</div>`
}

// actStart begins a conversation on the record and sends the person into
// it. The record is born with a name and, when given, what it is about;
// the first message is what brings it to life — so the empty conversation
// the browser lands in says exactly that.
func (m *Module) actStart(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		shell.Patch(w, startNote("Sign in first."))
		return
	}
	if err := r.ParseForm(); err != nil {
		shell.Patch(w, startNote("That could not be read."))
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		shell.Patch(w, startNote("A conversation needs a name."))
		return
	}
	h, err := topic.StartTopic(r.Context(), sess.Client(), topic.StartTopicInput{
		Name:          name,
		SubjectMatter: strings.TrimSpace(r.PostFormValue("about")),
	})
	if err != nil {
		shell.Patch(w, startNote("Not started — "+err.Error()))
		return
	}
	shell.Script(w, fmt.Sprintf("location.assign(%q)", "/?topic="+qesc(h.Path())))
}

// lifecycleActs is the next honest act on a conversation, offered where
// its status is read: a live one can be closed; a closed one can be
// archived, behind its own confirm; an archived one is done — nothing is
// offered, and no reopen exists to offer.
func lifecycleActs(v view) string {
	if v.Topic == nil || v.TopicPath == "" {
		return ""
	}
	switch v.Topic.Lifecycle {
	case topic.Proposed, topic.Active, topic.Dormant:
		return fmt.Sprintf(`<div class="acts"><button type="button" class="btn ghost"`+
			` data-on:click="@post('/act/conversation-close?topic=%s')">`+
			`Close this conversation</button></div>`, qesc(v.TopicPath))
	case topic.Closed:
		return fmt.Sprintf(`<div class="acts"><button type="button" class="btn ghost"`+
			` data-on:click="@get('/lifecycle/archive-ask?topic=%s')">%s`+
			`<span>Archive for good…</span></button></div>`,
			qesc(v.TopicPath), shell.Icon("archive"))
	default:
		return ""
	}
}

// lifeNote is the lifecycle acts' own result line — the dock beside the
// details panel, nothing while it has nothing to say.
func lifeNote(msg string) string {
	if msg == "" {
		return `<div id="convo-life" class="life-note"></div>`
	}
	return `<div id="convo-life" class="life-note"><p>` + esc(msg) + `</p></div>`
}

// archiveConfirm is the second step archiving stands behind: what archive
// means, in full, and the two ways out of the question.
func archiveConfirm(topicPath string) string {
	return fmt.Sprintf(`<div id="convo-life" class="life-note">`+
		`<p>Archive this conversation for good? It stays readable forever, but`+
		` nothing can ever be added again. There is no way back.</p>`+
		`<div class="acts"><button type="button" class="btn"`+
		` data-on:click="@post('/act/conversation-archive?topic=%s')">Yes, archive it</button>`+
		`<button type="button" class="btn ghost"`+
		` data-on:click="@get('/lifecycle/archive-ask')">Keep it as it is</button>`+
		`</div></div>`, qesc(topicPath))
}

// archiveAsk patches the confirm in — or, asked with no conversation,
// clears it, which is what "Keep it as it is" does.
func (m *Module) archiveAsk(w http.ResponseWriter, r *http.Request) {
	if m.sp.Session(r) == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	topicPath := r.URL.Query().Get("topic")
	if topicPath == "" {
		shell.Patch(w, lifeNote(""))
		return
	}
	shell.Patch(w, archiveConfirm(topicPath))
}

// actClose closes a conversation. The handle materialises first so the
// close parents onto what the person was looking at — and so a
// conversation somebody already archived is answered honestly rather
// than written to.
func (m *Module) actClose(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		shell.Patch(w, lifeNote("Sign in first."))
		return
	}
	topicPath := r.URL.Query().Get("topic")
	if topicPath == "" {
		shell.Patch(w, lifeNote("There is no conversation to act on."))
		return
	}
	h := topic.Open(sess.Client(), topicPath)
	if _, err := h.Materialise(r.Context()); err != nil {
		shell.Patch(w, lifeNote("That conversation could not be read — "+err.Error()))
		return
	}
	opID, err := h.Close(r.Context())
	shell.Patch(w, lifeNote(closeWords(opID, err)))
}

// actArchive is the deliberate, final act. Archive materialises
// internally, so no read comes first here; what it returns can be a
// half-truth either way, and archiveWords keeps the words honest.
func (m *Module) actArchive(w http.ResponseWriter, r *http.Request) {
	sess := m.sp.Session(r)
	if sess == nil {
		shell.Patch(w, lifeNote("Sign in first."))
		return
	}
	topicPath := r.URL.Query().Get("topic")
	if topicPath == "" {
		shell.Patch(w, lifeNote("There is no conversation to act on."))
		return
	}
	_, err := topic.Open(sess.Client(), topicPath).Archive(r.Context())
	shell.Patch(w, lifeNote(archiveWords(err)))
}

// closeWords says what closing did. The record can close a conversation
// and still hand back an error — the tidy-up behind the close is
// best-effort — so a standing close is never called a failure.
func closeWords(opID string, err error) string {
	switch {
	case err == nil:
		return "Closed — people can still read it, and it can be archived from here."
	case errors.Is(err, topic.ErrTopicArchived):
		return "This conversation is already archived."
	case opID != "":
		return "Closed. The tidy-up behind it did not finish; nothing is lost."
	default:
		return "Could not close — " + err.Error()
	}
}

// archiveWords says what archiving did. Already-archived is an answer,
// not a failure; a lost final compaction leaves the archive standing and
// says how to finish it.
func archiveWords(err error) string {
	switch {
	case err == nil:
		return "Archived — kept for reading, closed to writing."
	case errors.Is(err, topic.ErrTopicArchived):
		return "Already archived — kept for reading."
	case errors.Is(err, topic.ErrRollupLost):
		return "Archived, but the final tidy-up lost a race — archive again to finish it."
	default:
		return "Could not finish archiving — " + err.Error()
	}
}

// archivedDock stands where the composer would: an archived conversation
// is kept for reading, and the surface does not offer what the record
// would refuse.
func archivedDock() string {
	return `<div class="dock centred"><p class="dock-quiet">This conversation is` +
		` archived — kept for reading. Nothing new can be added.</p></div>`
}
