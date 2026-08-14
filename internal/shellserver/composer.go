package shellserver

import (
	"fmt"
	"net/url"
)

// The composer: the surface where a person writes into a conversation.
//
// It lives outside #dash on purpose. The live stream re-morphs #dash on
// every tick, and a half-written message must survive that; its three
// pieces each own a patch target so a one-shot act response and the
// stream never race for the same element.

const composerPrompt = "Write a message to this conversation…"

// qesc renders a value safe both as a query parameter and inside the
// HTML attribute that carries it.
func qesc(s string) string { return esc(url.QueryEscape(s)) }

// composerBox is the message box. On a landed message it is patched back
// in with mode replace, not morphed: the morph deliberately keeps what a
// person has typed, so it would never clear the box.
func composerBox() string {
	return `<textarea id="composer-box" name="body" rows="3" required` +
		` placeholder="` + composerPrompt + `"` +
		` style="width:100%;box-sizing:border-box;resize:vertical"></textarea>`
}

// composerNote is the composer's own result line — what happened to the
// last message, in plain words.
func composerNote(msg string) string {
	return `<span id="composer-note" class="mono" style="color:var(--text-muted)">` +
		esc(msg) + `</span>`
}

// composerReplyTo is the reply anchor: empty for a new message, else the
// hidden anchor the form carries plus the line naming who is answered.
func composerReplyTo(opID, author string) string {
	if opID == "" {
		return `<div id="reply-to"></div>`
	}
	return fmt.Sprintf(`<div id="reply-to" style="display:flex;align-items:center;`+
		`gap:var(--space-4);margin-bottom:var(--space-5)">`+
		`<input type="hidden" name="reply-to" value="%s">`+
		`<span class="pill">replying to %s</span>`+
		`<button type="button" class="btn ghost" style="padding:2px 8px"`+
		` data-on:click="@get('/composer/reply')">Cancel</button></div>`,
		esc(opID), esc(author))
}

// renderComposer is the whole message box as the page first serves it.
// The form posts itself as form data — no client-held state to drift
// from the record.
func renderComposer(topicPath string) string {
	return fmt.Sprintf(`<form id="composer" class="card" style="margin-top:var(--space-7)"`+
		` data-on:submit="@post('/act/post-turn?topic=%s', {contentType:'form'})">`+
		`<div class="head">%s<h2>Add to the conversation</h2></div>`+
		`%s%s`+
		`<div style="display:flex;align-items:center;gap:var(--space-5);margin-top:var(--space-5)">`+
		`<button class="btn" type="submit">Post</button>%s</div></form>`,
		qesc(topicPath), Icon("mic"), composerReplyTo("", ""), composerBox(),
		composerNote(""))
}

// replyLink is the per-message reply control inside the live view. It
// only moves the composer's anchor; nothing is written from here.
func replyLink(topicPath, opID string) string {
	return fmt.Sprintf(`<button type="button" style="background:none;border:0;padding:0;`+
		`color:var(--text-screen);opacity:.6;font:var(--type-data);font-size:var(--text-2xs);`+
		`text-decoration:underline;cursor:pointer"`+
		` data-on:click="@get('/composer/reply?topic=%s&amp;op=%s')">reply</button>`,
		qesc(topicPath), qesc(opID))
}
