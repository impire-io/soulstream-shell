package conversations

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// The mention picker: typing @ into the message box offers the people in
// this conversation by the names a person reads, and picking one writes
// that name.
//
// Two things have to stay apart for this to be honest. What somebody typed
// is what the record keeps — the body is never rewritten, so "@Daan" stays
// "@Daan" wherever it is read. Who they meant rides beside it: the persona
// name the library taps, resolved here from what the record already says
// about who is in the room, and handed to the library's supplying arm.
//
// The list is served, not held: the endpoint filters the conversation's
// participants and morphs one element of its own inside the composer, so
// neither the live stream nor the composer's other pieces ever write it.
// The caret, the arrow keys and the picks are the browser's own business —
// page-local JS, which is what C4 allows (design 0001 §5).
//
// Two things about this interaction the server genuinely cannot know, and
// both are held on the page rather than asked for: where the caret is, and
// that somebody pressed Escape on a list they did not want. Neither is
// state about the record — the record's answer is re-read every time — but
// they are state, and naming them is the point: C4's reversal turns on
// whether the morph model can express an interaction, and this one needed
// that much of the browser to feel right.

// suggestLimit is how many people the list offers at once. Past a handful
// it stops being a glance and starts being a directory.
const suggestLimit = 6

// peopleIn is who is in a conversation, named — the same set the details
// panel shows, read straight off the record over the shell's read lane. No
// keyring: this is who may be tapped, not who signed what.
func (m *Module) peopleIn(ctx context.Context, sess *soulstream.Session, path string) []participant {
	if path == "" {
		return nil
	}
	mt, err := topic.Open(m.sp.Reader(), path).Materialise(ctx)
	if err != nil {
		return nil
	}
	names, voices := m.directory(ctx, mt)
	v := view{Names: names, Voices: voices, Topic: mt}
	if sess != nil {
		v.Me = sess.Persona
	}
	return participants(v)
}

// suggestions is the people offered for what has been typed after the @, in
// the order the record first heard them. A person is not offered themselves
// — a message about you is a message you are already reading.
//
// Matching is by the start of the name (any word of it) or of the handle,
// case-insensitively: typing enough of somebody narrows to them, and typing
// nothing after the @ offers the room.
func suggestions(people []participant, q string) []participant {
	want := strings.ToLower(strings.TrimSpace(q))
	var out []participant
	for _, p := range people {
		if len(out) == suggestLimit {
			break
		}
		if p.Me {
			continue
		}
		if want == "" || startsWith(p.Name, want) || strings.HasPrefix(p.Persona, want) {
			out = append(out, p)
		}
	}
	return out
}

// startsWith reports whether any word of name begins with the (lowercased)
// fragment — so "bl" finds "Avery Blake".
func startsWith(name, want string) bool {
	for _, word := range strings.Fields(strings.ToLower(name)) {
		if strings.HasPrefix(word, want) {
			return true
		}
	}
	return false
}

// renderSuggest is the picker's list — its own patch target inside the
// composer. Empty is the closed state: the token source hides a list with
// nothing in it, so there is no separate "open" to keep track of.
//
// The first row is marked, so Enter picks the obvious one without anybody
// reaching for an arrow key.
func renderSuggest(people []participant) string {
	if len(people) == 0 {
		return `<div id="mention-suggest" class="suggest"></div>`
	}
	var b strings.Builder
	b.WriteString(`<div id="mention-suggest" class="suggest" role="listbox"` +
		` aria-label="People in this conversation">`)
	for i, p := range people {
		cls, selected := "sug", "false"
		if i == 0 {
			cls, selected = "sug on", "true"
		}
		// The lamp says which channel the name about to be written belongs to:
		// tapping a voice somebody else answers for is a different act from
		// tapping the person who answers for themselves, and the list is where
		// a person finds that out — before the name is in the message.
		fmt.Fprintf(&b, `<button type="button" class="%s" role="option" aria-selected="%s"`+
			` data-mention="%s" data-name="%s" data-on:click="mentionPick(el)">`+
			`%s<span class="who">%s</span><span class="handle">@%s</span></button>`,
			cls, selected, esc(p.Persona), esc(p.Name), p.pip(), esc(p.Name), esc(p.Persona))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// resolveMentions is who a message is about, decided against the record and
// never on the browser's word.
//
// A pick counts while the body still carries the name it was picked for:
// somebody who chose Daan, thought better of it and deleted the token has
// tapped nobody, whatever the form still says. A name typed by hand counts
// when exactly one person in the room answers to it — two Averys is
// ambiguous, and ambiguous stays as typed. Everything else is left to the
// library, which parses the body's own slug grammar as it always has.
func resolveMentions(body string, picks []string, people []participant) []string {
	if len(people) == 0 {
		return nil
	}
	byPersona := make(map[string]participant, len(people))
	answering := map[string]int{} // lowercased name -> how many people answer to it
	for _, p := range people {
		byPersona[p.Persona] = p
		answering[strings.ToLower(p.Name)]++
	}

	var out []string
	seen := map[string]bool{}
	add := func(persona string) {
		if persona == "" || seen[persona] {
			return
		}
		seen[persona] = true
		out = append(out, persona)
	}
	for _, pick := range picks {
		if p, ok := byPersona[pick]; ok && namedIn(body, p.Name) {
			add(pick)
		}
	}
	for _, p := range people {
		if answering[strings.ToLower(p.Name)] == 1 && namedIn(body, p.Name) {
			add(p.Persona)
		}
	}
	return out
}

// namedIn reports whether body writes "@name", case-insensitively and only
// where the name ends the token — so "@Avery" is not read out of
// "@Averyson", and a name with a space in it still matches whole.
func namedIn(body, name string) bool {
	if name == "" {
		return false
	}
	lowBody, token := strings.ToLower(body), "@"+strings.ToLower(name)
	for i := 0; i+len(token) <= len(lowBody); {
		j := strings.Index(lowBody[i:], token)
		if j < 0 {
			return false
		}
		at := i + j
		if endsToken(lowBody, at+len(token)) {
			return true
		}
		i = at + 1
	}
	return false
}

// endsToken reports whether a mention token may end at i: at the end of the
// text, or before something that is not part of a name.
func endsToken(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
}

// mentionScript is the picker's caret and keys: where the @ being typed
// starts, what the arrow keys move, and what picking one writes into the
// box. Datastar fetches the list and morphs it in — this is only the part
// no server can know, which is the C4 rule's page-local JS and nothing
// more. Nothing here holds state: the list is the server's, the picks are
// form fields, and the body is whatever the person typed.
const mentionScript = `<script>
(() => {
  const box = () => document.getElementById("composer-box");
  const list = () => document.getElementById("mention-suggest");
  const rows = () => Array.from(list() ? list().querySelectorAll(".sug") : []);
  const mark = (items, i) => items.forEach((n, k) => {
    n.classList.toggle("on", k === i);
    n.setAttribute("aria-selected", k === i ? "true" : "false");
  });

  // Where the @ being typed starts, or -1 when the caret sits nowhere near
  // one. A name may hold a space, so one is allowed inside the fragment and
  // a second ends it — past that somebody is writing a sentence, not a name.
  const opening = (el) => {
    const upto = el.value.slice(0, el.selectionStart);
    const at = upto.lastIndexOf("@");
    if (at < 0) return -1;
    const frag = upto.slice(at + 1);
    if (frag.includes("\n") || frag.length > 32) return -1;
    if ((frag.match(/ /g) || []).length > 1) return -1;
    return at;
  };

  // Escape means "not for this one". A person waving the list away is the
  // one thing about this the server cannot be told without being asked —
  // so it is remembered here, against the @ it was aimed at, and forgotten
  // the moment the caret is at a different one.
  let waved = -1;

  // What has been typed after the @, or null when there is no @ to answer.
  // Datastar's debounced input handler asks this before it asks the server.
  window.mentionQuery = (el) => {
    const at = opening(el);
    if (at !== waved) waved = -1;
    return at < 0 || at === waved ? null : el.value.slice(at + 1, el.selectionStart);
  };

  window.mentionClose = () => { const l = list(); if (l) l.replaceChildren(); };

  // Picking writes the name a person reads and remembers the handle it
  // stands for. The body keeps what was typed; the handle rides beside it
  // as a form field the server checks against the body before counting it.
  window.mentionPick = (node) => {
    const el = box();
    if (!el || !node) return;
    const at = opening(el);
    if (at < 0) return;
    const token = "@" + node.dataset.name + " ";
    const tail = el.value.slice(el.selectionStart);
    el.value = el.value.slice(0, at) + token + tail;
    const caret = at + token.length;
    el.setSelectionRange(caret, caret);
    el.focus();
    const picks = document.getElementById("mention-picks");
    if (picks && !Array.from(picks.children).some((f) => f.value === node.dataset.mention)) {
      const f = document.createElement("input");
      f.type = "hidden";
      f.name = "mention";
      f.value = node.dataset.mention;
      picks.appendChild(f);
    }
    window.mentionClose();
  };

  // The keys, while the list is open. Delegated, because the box is patched
  // back in on every landed message.
  document.addEventListener("keydown", (e) => {
    if (e.target !== box()) return;
    const items = rows();
    if (!items.length) return;
    const at = items.findIndex((n) => n.classList.contains("on"));
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault(); mark(items, (at + 1) % items.length); break;
      case "ArrowUp":
        e.preventDefault(); mark(items, (at - 1 + items.length) % items.length); break;
      case "Enter":
      case "Tab":
        e.preventDefault(); window.mentionPick(items[at < 0 ? 0 : at]); break;
      case "Escape":
        e.preventDefault(); waved = opening(e.target); window.mentionClose(); break;
    }
  });
})();
</script>`
