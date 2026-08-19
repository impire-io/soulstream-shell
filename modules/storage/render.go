package storage

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/impire-io/soulstream-core/record"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/shell"
)

// The screen: what to look at above, what is there below, and one message
// whole in the panel under both.
//
// Plain words for what a person is doing — looking at storage, at messages,
// in a conversation — and the machine's own spelling wherever the machine's
// own spelling is the point. This is the one screen where a subject, a
// stream name and a header are what somebody came for, so they are shown
// exactly as they are rather than translated into something friendlier and
// less true.

// The screen's patch targets. The list is the live tail's; the panel is the
// one a message opens into. Neither ever writes the other.
const (
	listID = "storage-list"
	opID   = "storage-op"
)

// renderScreen is the whole body.
func renderScreen(a ask, v view) string {
	var b strings.Builder
	b.WriteString(`<h1>Storage</h1>`)
	b.WriteString(`<p class="lede">Every message your soulstream is keeping, exactly as it ` +
		`was written. Read live with your own sign-in — the shell keeps none of it.</p>`)
	b.WriteString(scopeNote())
	b.WriteString(`<div class="section">` + renderPicker(a, v) + `</div>`)
	b.WriteString(`<div class="section">` + renderList(a, v) + `</div>`)
	b.WriteString(renderOp(opView{}))
	return b.String()
}

// scopeNote is the sentence this screen must never get wrong.
//
// Reading rides the person's own sign-in, and that is a custody fact, not a
// privacy one: in this deployment every admitted persona is granted the
// whole subject space, so this screen shows the whole store to anybody
// signed in. Saying "your messages" here would be a lie, and a comfortable
// one, which is the worst kind on a screen people come to when they have
// stopped trusting what they were told.
func scopeNote() string {
	return `<p class="note">Read with your own sign-in: this shows what your sign-in is ` +
		`permitted to read, which in this deployment is the whole store — not only ` +
		`the parts about you.</p>`
}

// renderPicker is what to look at: which store, and which part of its
// subject space. It is a plain form that navigates, so where somebody is
// lives in the address bar and survives a reload, a bookmark, and a paste
// into somebody else's message.
func renderPicker(a ask, v view) string {
	var opts strings.Builder
	for _, s := range stores {
		sel := ""
		if s.Key == v.Store.Key {
			sel = " selected"
		}
		fmt.Fprintf(&opts, `<option value="%s"%s>%s — %s</option>`,
			esc(s.Key), sel, esc(s.Label), esc(s.Stream))
	}
	topicField := ""
	if a.Topic != "" {
		topicField = fmt.Sprintf(`<input type="hidden" name="topic" value="%s">`, esc(a.Topic))
	}
	follow := fmt.Sprintf(`<a class="btn ghost" href="/storage%s">Follow the tail</a>`,
		a.query("follow=1"))
	if a.Follow {
		follow = fmt.Sprintf(`<a class="btn ghost" href="/storage%s">Stop following</a>`,
			a.query())
	}
	return fmt.Sprintf(`<div class="card"><h2>What to look at</h2>`+
		`<p class="lede">%s</p>`+
		`<form method="get" action="/storage">%s`+
		`<div class="fields">`+
		`<label class="field">Store<select name="store">%s</select></label>`+
		`<label class="field">Subject<input name="filter" value="%s" autocomplete="off" `+
		`spellcheck="false" placeholder="%s"></label>`+
		`</div>`+
		`<div class="acts"><button class="btn" type="submit">Look</button>%s</div>`+
		`</form>%s</div>`,
		esc(v.Store.About), topicField, opts.String(),
		esc(a.Filter), esc(v.Store.Space), follow, patternNote(a, v))
}

// patternNote is what the box will and will not take. A pattern is a subject
// pattern — the record's own way of being read — so the note says so, says
// what is wrong when something is, and says plainly that there is no search
// here and why.
func patternNote(a ask, v view) string {
	if v.PatternErr != "" {
		return fmt.Sprintf(`<p class="note">%s is not a subject pattern: %s</p>`,
			esc(a.Filter), esc(v.PatternErr))
	}
	if v.ServiceLane {
		return fmt.Sprintf(`<p class="note">Nothing is kept on %s. Requests and their `+
			`replies are answered and gone — no store captures them, by design.</p>`,
			esc(serviceLane+">"))
	}
	return `<p class="note">A subject pattern: <span class="mono">*</span> stands for one ` +
		`part, <span class="mono">&gt;</span> for the rest. There is no text search — the ` +
		`record keeps no index to search, and this screen will not pretend otherwise.</p>`
}

// renderList is the messages themselves, newest first — and a patch target
// of its own, so a followed screen can hand back what is now there without
// touching the form above it or the message open below.
func renderList(a ask, v view) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, listID)
	b.WriteString(renderHeld(v))
	switch {
	case v.PatternErr != "":
		b.WriteString(`<p class="blank">Nothing was read — fix the subject above.</p>`)
	case v.Err != "":
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
	case v.Empty:
		fmt.Fprintf(&b, `<p class="blank">%s is empty — nothing has been written here yet.</p>`,
			esc(v.Store.Label))
	case len(v.Ops) == 0:
		fmt.Fprintf(&b, `<p class="blank">No message in %s matches that subject.</p>`,
			esc(v.Store.Label))
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Seq</th><th>Stored</th><th>Subject</th><th>Type</th>` +
			`<th>Author</th><th>Signature</th><th>Size</th><th>Actions</th>` +
			`</tr></thead><tbody>`)
		for _, o := range v.Ops {
			b.WriteString(opRow(a, o))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(renderReach(a, v))
	b.WriteString(`</div>`)
	return b.String()
}

// renderHeld is the line above the list: what the store holds in total, and
// which sequences it still has. Both are the store's own numbers, said the
// way the house readout says them.
func renderHeld(v view) string {
	if v.Err != "" || v.PatternErr != "" {
		return ""
	}
	word := "messages"
	if v.Msgs == 1 {
		word = "message"
	}
	held := fmt.Sprintf(`<span class="mono">%d %s · %s</span>`,
		v.Msgs, word, esc(shell.SizeWords(v.Bytes)))
	if v.Msgs == 0 {
		return fmt.Sprintf(`<p class="readout"><span class="cap">%s</span>%s</p>`,
			esc(v.Store.Stream), held)
	}
	return fmt.Sprintf(`<p class="readout"><span class="cap">%s</span>%s`+
		`<span class="mono">seq %d–%d</span></p>`,
		esc(v.Store.Stream), held, v.First, v.Last)
}

// renderReach says how far back this read actually looked, and offers the
// way further. A page that stopped at the scan limit says so out loud: a
// silent stop would read as "that is everything", which on a debugging
// screen is the one lie that costs somebody an afternoon.
func renderReach(a ask, v view) string {
	if v.Err != "" || v.PatternErr != "" || v.Empty {
		return ""
	}
	var notes []string
	if v.Capped {
		notes = append(notes, fmt.Sprintf(
			"Stopped after examining %d sequences back from %d. Nothing older than %d "+
				"was looked at.", v.Examined, ceilingOf(a, v), v.Oldest))
	}
	older := ""
	if v.Oldest > v.First && v.Oldest > 1 {
		older = fmt.Sprintf(`<a class="btn ghost" href="/storage%s">Older messages</a>`,
			a.query(fmt.Sprintf("before=%d", v.Oldest-1)))
	}
	if a.Before != 0 {
		notes = append(notes, fmt.Sprintf("Reading back from sequence %d.", a.Before))
		older += fmt.Sprintf(`<a class="btn ghost" href="/storage%s">Back to the newest</a>`,
			a.query())
	}
	out := ""
	if len(notes) > 0 {
		out += fmt.Sprintf(`<p class="note">%s</p>`, esc(strings.Join(notes, " ")))
	}
	if older != "" {
		out += `<p class="act"><span class="acts">` + older + `</span></p>`
	}
	return out
}

// ceilingOf is the sequence a read started from, for the note that says so.
func ceilingOf(a ask, v view) uint64 {
	if a.Before != 0 && a.Before <= v.Last {
		return a.Before
	}
	return v.Last
}

// opRow is one message in the list. The subject is the widest thing on it
// and the thing people scan by, so it takes the room; everything else is
// one word.
func opRow(a ask, o op) string {
	kind, author := esc(o.Rec.Type), esc(o.Rec.Author)
	if o.Bad != "" {
		kind, author = `<span class="pill warn">not a record</span>`, `<span class="note">—</span>`
	}
	return fmt.Sprintf(`<tr><td class="mono whole">%d</td><td class="mono whole">%s</td>`+
		`<td class="mono whole">%s</td><td class="mono whole">%s</td>`+
		`<td class="mono whole">%s</td>`+
		`<td>%s</td><td class="mono whole">%s</td>`+
		`<td><div class="acts"><button class="btn ghost" `+
		`data-on:click="@get('/storage/op%s')">Open</button></div></td></tr>`,
		o.Seq, esc(stamp(o.Stored)), esc(o.Subject), kind, author,
		sigMark(o), esc(shell.SizeWords(uint64(o.Size))),
		a.query(fmt.Sprintf("seq=%d", o.Seq)))
}

// stamp is a stored-at time as a person reads one on an instrument: to the
// second, in the reader's own offset-free form, because two messages a
// second apart is the distinction that matters here.
func stamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05Z")
}

// sigMark is the earned verdict, in the vocabulary the conversation view
// already uses. A message that is not a record has no verdict to earn and
// says nothing rather than defaulting to one.
func sigMark(o op) string {
	if o.Bad != "" {
		return `<span class="note">—</span>`
	}
	switch o.Sig {
	case topic.SigVerified:
		return `<span class="verdict ok">verified</span>`
	case topic.SigUnsigned:
		return `<span class="verdict">unsigned</span>`
	case topic.SigUnknownKey:
		return `<span class="verdict warn">unknown key</span>`
	default:
		return `<span class="verdict warn">` + esc(string(o.Sig)) + `</span>`
	}
}

// renderOp is one message whole, and the panel it lives in — served empty
// with the screen, filled when somebody opens a message, and never touched
// by the tail's tick.
//
// Everything is here on purpose. The headers verbatim, because a header
// nobody expected is exactly the bug. The payload as it is, because a
// prettied payload is different bytes. The canonical form beside it, because
// that — not the payload — is what a signature is over, and the difference
// between the two is the thing people get wrong about this record.
func renderOp(v opView) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, opID)
	switch {
	case v.Err != "":
		fmt.Fprintf(&b, `<div class="card"><h2>Message</h2><p class="blank">%s</p></div>`,
			esc(v.Err))
	case !v.Found:
		b.WriteString("")
	default:
		o := v.Op
		b.WriteString(`<div class="card raised">`)
		fmt.Fprintf(&b, `<h2>%s · sequence %d</h2>`, esc(v.Store.Label), o.Seq)
		fmt.Fprintf(&b, `<p class="lede">On <span class="mono">%s</span>, stored `+
			`<span class="mono">%s</span>, <span class="mono">%s</span> of payload.</p>`,
			esc(o.Subject), esc(stamp(o.Stored)), esc(shell.SizeWords(uint64(o.Size))))
		if o.Bad != "" {
			fmt.Fprintf(&b, `<p class="note">This message is on a record subject but is `+
				`not a well-formed record: %s</p>`, esc(o.Bad))
		}
		b.WriteString(renderHeaders(o))
		b.WriteString(renderVerdict(o))
		b.WriteString(blockOf("Payload", o.Payload,
			"The message's own data, byte for byte."))
		b.WriteString(renderCanonical(o))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderHeaders is the record as its headers spell it — the parsed fields
// named the way the wire names them, and every unknown Soulstream- header
// kept rather than dropped, because the ones nobody planned for are the
// ones worth seeing.
func renderHeaders(o op) string {
	if o.Bad != "" {
		return ""
	}
	rows := []struct{ k, v string }{
		{record.HeaderVersion, o.Version},
		{record.HeaderMsgID, o.Rec.ID},
		{record.HeaderAuthor, o.Rec.Author},
		{record.HeaderActing, o.Rec.Acting},
		{record.HeaderType, o.Rec.Type},
		{record.HeaderTs, o.Rec.Timestamp.Format(time.RFC3339Nano)},
		{record.HeaderParents, strings.Join(o.Rec.Parents, ", ")},
		{record.HeaderSig, o.Rec.Signature},
	}
	for k, v := range o.Rec.Extras {
		rows = append(rows, struct{ k, v string }{k, v})
	}
	var b strings.Builder
	b.WriteString(`<h3 class="label">Headers</h3><div class="tablewrap"><table><tbody>`)
	for _, row := range rows {
		val := esc(row.v)
		if row.v == "" {
			val = `<span class="note">not set</span>`
		}
		fmt.Fprintf(&b, `<tr><td class="mono">%s</td><td class="mono">%s</td></tr>`,
			esc(row.k), val)
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// renderVerdict says what the signature earned and what it was measured
// against — the author's own published key, and the value the subject binds
// it to. A verdict with no working shown is an opinion.
func renderVerdict(o op) string {
	if o.Bad != "" {
		return ""
	}
	bound := esc(o.Binding)
	if o.Binding == "" {
		bound = `<span class="note">nothing — this subject is outside the binding rule</span>`
	}
	return fmt.Sprintf(`<h3 class="label">Signature</h3>`+
		`<p class="readout">%s<span class="mono">bound to %s</span></p>`+
		`<p class="note">The verdict is earned against the author's own published key, `+
		`read from the persona directory. Unknown key is not a failure — it means no key `+
		`has been published for this author yet, and the same message can verify later.</p>`,
		sigMark(o), bound)
}

// renderCanonical is the byte sequence a signature actually covers, beside
// the payload it is not. When there is none, the reason is the answer.
func renderCanonical(o op) string {
	if o.Bad != "" {
		return ""
	}
	if o.CanonErr != "" {
		return fmt.Sprintf(`<h3 class="label">Signed bytes</h3><p class="note">%s</p>`,
			esc(o.CanonErr))
	}
	return blockOf("Signed bytes", o.Canonical,
		"The canonical form the signature is over — not the payload, and bound to "+
			"this soulstream's own key and to this subject, so it cannot be lifted "+
			"into another one.")
}

// blockOf is one block of bytes on the dark surface, or an honest sentence
// about why it is not shown. Text is shown as text; anything else says what
// it is, because a screen full of mojibake tells a person less than one
// sentence about the bytes does.
func blockOf(heading string, data []byte, about string) string {
	head := fmt.Sprintf(`<h3 class="label">%s</h3><p class="note">%s</p>`,
		esc(heading), esc(about))
	switch {
	case len(data) == 0:
		return head + `<p class="note">Empty.</p>`
	case len(data) > payloadCap:
		return head + fmt.Sprintf(`<p class="note">%s — too much to put on a screen. `+
			`Nothing is shown rather than a piece that could be mistaken for the whole.</p>`,
			esc(shell.SizeWords(uint64(len(data)))))
	case !utf8.Valid(data):
		return head + fmt.Sprintf(`<p class="note">%s of bytes that are not text. `+
			`Nothing is shown rather than something that would only look like it.</p>`,
			esc(shell.SizeWords(uint64(len(data)))))
	}
	return head + fmt.Sprintf(`<pre class="screen">%s</pre>`, esc(string(data)))
}
