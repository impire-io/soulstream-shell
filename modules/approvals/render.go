package approvals

import (
	"fmt"
	"strings"
	"time"
)

// The screen: everything awaiting a human's decision, each row one tap
// from its answer. Plain words — a person reads who asked, what they asked
// to do, and under which rule; the machine's names (the invocation
// fingerprint) stay visible because this is the screen where they are the
// point, and the ARGUMENTS stay absent because that is the ticket's own
// privacy line, said rather than left looking broken.

// The screen's patch targets.
const (
	resultID = "approvals-result"
	listID   = "approvals-list"
)

// ticket is one pending decision as the screen shows it.
type ticket struct {
	InvocationID string
	Principal    string
	Who          string
	Action       string
	Rule         string
	ExpiresAt    string
}

// view is one read of the screen.
type view struct {
	Tickets []ticket
	MayAct  bool
	Err     string
}

// spineTally is the mark on this module's key: how many decisions wait.
func spineTally(n int) string {
	word := "decisions wait"
	if n == 1 {
		word = "decision waits"
	}
	return fmt.Sprintf(`<span class="tally on" title="%d %s">%d</span>`, n, word, n)
}

// renderApprovals is the whole screen's body.
func renderApprovals(v view) string {
	var b strings.Builder
	b.WriteString(`<h1>Approvals</h1>`)
	b.WriteString(`<p class="lede">Acts the guardrail held for a human. Your yes or no is ` +
		`signed with your own key and answers exactly one request — nothing else, ` +
		`nothing later.</p>`)
	b.WriteString(`<p class="note">A request is named by its fingerprint, never its ` +
		`contents — the rule is your context. Saying yes lets the asker retry that one ` +
		`act within a few minutes.</p>`)
	b.WriteString(`<div class="section">` + renderList(v) + `</div>`)
	b.WriteString(resultNote(""))
	return b.String()
}

// renderList is the pending tickets — and a patch target of its own so an
// answer can hand back what is now true.
func renderList(v view) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div id="%s">`, listID)
	switch {
	case v.Err != "":
		fmt.Fprintf(&b, `<p class="blank">%s</p>`, esc(v.Err))
	case len(v.Tickets) == 0:
		b.WriteString(`<p class="blank">Nothing is waiting for a decision.</p>`)
	default:
		b.WriteString(`<div class="tablewrap"><table><thead><tr>` +
			`<th>Who</th><th>Wants to</th><th>Rule</th><th>Window</th><th>Request</th>` +
			`<th>Actions</th></tr></thead><tbody>`)
		for _, t := range v.Tickets {
			b.WriteString(ticketRow(t, v.MayAct))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// ticketRow is one decision: who, what, under which rule, how long the
// window has left, the fingerprint, and the two taps.
func ticketRow(t ticket, mayAct bool) string {
	who := esc(t.Who)
	if who == "" {
		who = esc(t.Principal)
	} else {
		who = fmt.Sprintf(`%s <span class="mono">%s</span>`, who, esc(t.Principal))
	}
	window := esc(t.ExpiresAt)
	if exp, err := time.Parse(time.RFC3339, t.ExpiresAt); err == nil {
		left := time.Until(exp).Round(time.Second)
		if left > 0 {
			window = left.String() + " left"
		} else {
			window = "expiring"
		}
	}
	acts := `<span class="note">not yours to answer</span>`
	if mayAct {
		acts = fmt.Sprintf(
			`<button class="btn" data-on:click="@post('/act/approval-approve?invocation=%s&amp;principal=%s')">`+
				`Approve</button>`+
				`<button class="btn ghost" data-on:click="@post('/act/approval-deny?invocation=%s&amp;principal=%s')">`+
				`Deny</button>`,
			qesc(t.InvocationID), qesc(t.Principal), qesc(t.InvocationID), qesc(t.Principal))
	}
	return fmt.Sprintf(`<tr><td>%s</td><td class="mono">%s</td><td class="mono">%s</td>`+
		`<td>%s</td><td class="mono" title="%s">%s</td><td><div class="acts">%s</div></td></tr>`,
		who, esc(t.Action), esc(t.Rule), window,
		esc(t.InvocationID), esc(short(t.InvocationID)), acts)
}

// short is a fingerprint a table column can hold; the whole rides the
// hover.
func short(id string) string {
	if len(id) > 12 {
		return id[:12] + "…"
	}
	return id
}

// resultNote is the screen's result line — the patch target every act
// answers to.
func resultNote(msg string) string {
	return fmt.Sprintf(`<div id="%s" class="note">%s</div>`, resultID, esc(msg))
}
