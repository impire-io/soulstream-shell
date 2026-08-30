package conversations

import (
	"fmt"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// Being tapped on the shoulder, as this screen shows it.
//
// The tray itself belongs to the support layer — the slips arrive on the
// person's own connection, and the overview screen counts them too. The
// mark on a message that said the name rides the shared thread rendering
// (soulstream.MentionsMe). What is left here is what this module alone
// does with the tray: the count on its own key on the spine.

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
