package conversations

import (
	"fmt"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// Being tapped on the shoulder, as this screen shows it.
//
// The tray itself belongs to the support layer — the slips arrive on the
// person's own connection, and the overview screen counts them too. What is
// here is what this module does with it: the count on its own key on the
// spine, and the mark on the message that said the name, standing out where
// it was said.

// mentionsMe reports whether a message tapped the signed-in person on the
// shoulder. The record says so itself — the library writes the names it
// parsed out of a body onto the message — so the mark is right whether or
// not the slip ever reached anyone's inbox.
func mentionsMe(v view, c *topic.Contribution) bool {
	if v.Me == "" {
		return false
	}
	for _, m := range c.Mentions {
		if m == v.Me {
			return true
		}
	}
	return false
}

// spineTally is the whole tray as one mark on this module's key on the
// spine. It is its own patch target inside the spine, so the live stream
// keeps it current without morphing the spine around it — an expanded spine
// survives every tick, as it did before there was anything to count.
func spineTally(n int) string {
	if n <= 0 {
		return `<span id="mentions" class="tally"></span>`
	}
	return fmt.Sprintf(`<span id="mentions" class="tally on" title="%s">%d</span>`,
		esc(soulstream.TallyWords(n)), n)
}
