package conversations

import (
	"fmt"

	"github.com/impire-io/soulstream-shell/shell"
)

// The composer: the surface where a person writes into a conversation. It
// docks at the foot of the centre column, always in reach.
//
// It lives outside the live stream's targets on purpose. The stream
// re-morphs the rail and the conversation on every tick, and a half-written
// message must survive that; each of the composer's pieces owns a patch
// target so a one-shot act response and the stream never race for the same
// element.

const composerPrompt = "Write a message…"

// composerBox is the message box. On a landed message it is patched back
// in with mode replace, not morphed: the morph deliberately keeps what a
// person has typed, so it would never clear the box.
//
// Typing asks the server who is in the room — debounced, and only once
// there is an @ to answer. Where the caret sits and what has been typed
// after the @ is the one thing no server can know, so a page-local helper
// says; the list that comes back is the server's own (see picker.go).
func composerBox(topicPath string) string {
	return `<textarea id="composer-box" name="body" rows="1" required` +
		` placeholder="` + composerPrompt + `"` +
		` data-on:input__debounce.150ms="mentionQuery(el) === null ? mentionClose() :` +
		` @get('/composer/suggest?topic=` + qesc(topicPath) +
		`&amp;q=' + encodeURIComponent(mentionQuery(el)))"></textarea>`
}

// composerPicks is where the picker keeps who was chosen: one hidden field
// per pick, naming the persona the display text stands for. The browser
// writes them, the server weighs each against the body before it counts,
// and a landed message clears them the way it clears the box.
func composerPicks() string { return `<span id="mention-picks"></span>` }

// composerNote is the composer's own result line — what happened to the
// last message, in plain words.
func composerNote(msg string) string {
	return `<span id="composer-note" class="note">` + esc(msg) + `</span>`
}

// composerReplyTo is the reply state, shown above the input: empty for a
// new message, else the hidden anchor the form carries plus the line naming
// who is answered and the way out of it.
func composerReplyTo(opID, author string) string {
	if opID == "" {
		return `<div id="reply-to"></div>`
	}
	return fmt.Sprintf(`<div id="reply-to" class="reply-state">`+
		`<input type="hidden" name="reply-to" value="%s">`+
		`<span class="pill">replying to %s</span>`+
		`<button type="button" class="btn ghost"`+
		` data-on:click="@get('/composer/reply')">Cancel</button></div>`,
		esc(opID), esc(author))
}

// renderComposer is the whole message box as the page first serves it: the
// reply state, the picker's list (closed, which is to say empty), the box
// and the send, and the note under them. The form posts itself as form data
// — the picks ride it as fields, and nothing else is held on the browser to
// drift from the record.
func renderComposer(topicPath string) string {
	return fmt.Sprintf(`<form id="composer" class="dock centred"`+
		` data-on:submit="@post('/act/post-turn?topic=%s', {contentType:'form'})">`+
		`%s%s%s<div class="dock-row">%s`+
		`<button class="btn send" type="submit">%s<span>Send</span></button></div>`+
		`<div class="dock-note">%s</div></form>`,
		qesc(topicPath), composerReplyTo("", ""), composerPicks(), renderSuggest(nil),
		composerBox(topicPath), shell.Icon("send"), composerNote(""))
}

// replyLink is the per-message reply control inside the conversation. It
// only moves the composer's anchor; nothing is written from here.
func replyLink(topicPath, opID string) string {
	return fmt.Sprintf(`<button type="button" class="reply-btn"`+
		` data-on:click="@get('/composer/reply?topic=%s&amp;op=%s')">Reply</button>`,
		qesc(topicPath), qesc(opID))
}
