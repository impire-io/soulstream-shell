package agents

import (
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-shell/soulstream"
)

// The Around column is a reading judged fresh at render: the person's
// words — in, left {when}, seen {when} — and an honest dash where the
// realm has never seen the voice. The moment rides the hover, and none
// of it gates anything: the acts render the same whatever the sign says.
func TestTheAroundColumnSpeaksThePersonsWords(t *testing.T) {
	now := time.Now()
	signs := map[string]soulstream.LifeSign{
		"scribe":  {Present: true, When: now, Doing: "answering mentions"},
		"nightly": {Left: true, When: now.Add(-10 * time.Minute)},
	}
	body := renderTable(agentList(), nil, names(), "", signs)
	if !strings.Contains(body, "<th>Around</th>") {
		t.Fatal("no Around column")
	}
	if !strings.Contains(body, `>in</span>`) {
		t.Errorf("a held lease does not read as in:\n%s", body)
	}
	if !strings.Contains(body, "left 10m ago") {
		t.Errorf("a farewell does not read as left:\n%s", body)
	}
	if !strings.Contains(body, "answering mentions") {
		t.Error("the voice's own line does not ride the hover")
	}

	// Silence past the horizon reads as seen — and a voice the realm has
	// never seen gets a dash, never a guess.
	signs = map[string]soulstream.LifeSign{
		"scribe": {When: now.Add(-3 * time.Hour)},
	}
	body = renderTable(agentList(), nil, names(), "", signs)
	if !strings.Contains(body, "seen 3h ago") {
		t.Errorf("stale silence does not read as seen:\n%s", body)
	}
	if !strings.Contains(body, ">—</span>") {
		t.Errorf("an unseen voice does not get its dash:\n%s", body)
	}
}

// The paste card tells the person what comes next — the row will show
// in, and a mention's answer is the proof the whole path works — so the
// first hour has a thread, not a dead end at a terminal.
func TestThePasteCardNamesTheNextStep(t *testing.T) {
	c := soulstream.Credential{Handle: "scribe", ShownAs: "Scribe", Secret: "s3cr3t"}
	body := renderCredential(c, "is ready")
	for _, want := range []string{"start a conversation", "mention Scribe"} {
		if !strings.Contains(body, want) {
			t.Errorf("the card does not say %q", want)
		}
	}
}
