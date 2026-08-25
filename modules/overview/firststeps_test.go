package overview

import (
	"strings"
	"testing"
)

// Guidance is a reading: the steps derive from facts alone, done-marks
// and all, and unreadable facts contribute nothing — a step is offered
// on evidence, never on a guess, and a step nobody on this session
// could take is never listed.
func TestStepsDeriveFromTheHouseAlone(t *testing.T) {
	// A fresh house, the founder signed in: everything pending.
	f := stepFacts{AgentsOn: true, ToolsKnown: true, Admin: true,
		PeopleKnown: true, People: 1}
	steps := deriveSteps(f)
	if len(steps) != 4 {
		t.Fatalf("a fresh house offers %d steps, want 4", len(steps))
	}
	for _, s := range steps {
		if s.Done {
			t.Errorf("step %q born done", s.Title)
		}
	}

	// The house fills: every condition flips its own step.
	f.AgentsNamed, f.Talks, f.Tools, f.People = 1, 1, 1, 2
	for _, s := range deriveSteps(f) {
		if !s.Done {
			t.Errorf("step %q still pending in a furnished house", s.Title)
		}
	}

	// An unreadable roster says nothing rather than guessing.
	steps = deriveSteps(stepFacts{AgentsOn: true, AgentsUnread: true})
	if len(steps) != 1 || steps[0].Title != "Start a conversation" {
		t.Fatalf("unreadable facts still offered steps: %+v", steps)
	}

	// A non-administrator: never the invite step, never an add they
	// cannot take — but connecting is their own act once there is
	// something to connect, their own standing deciding done.
	found := false
	for _, s := range deriveSteps(stepFacts{ToolsKnown: true, Tools: 2, Remote: 1, Connected: true}) {
		switch s.Title {
		case "Connect a tool":
			found = true
			if !s.Done {
				t.Error("a connected person's step still pending")
			}
		case "Invite someone":
			t.Error("a non-admin offered the invite step")
		}
	}
	if !found {
		t.Error("connectable services offered nothing")
	}
	// An empty catalog offers a non-admin nothing: a step nobody on this
	// session could take is a wall, not a door.
	for _, s := range deriveSteps(stepFacts{ToolsKnown: true}) {
		if s.Title == "Connect a tool" {
			t.Error("a non-admin offered an add they cannot take")
		}
	}
}

// The card is absent the moment nothing remains — no dismissal, no
// flag, nothing stored anywhere: rendering is a pure function of the
// reading, which is the no-store property as a test.
func TestTheCardRetiresItself(t *testing.T) {
	pending := []step{{Title: "Set up your assistant", Href: "/agents", Detail: "x"}}
	done := []step{{Title: "Set up your assistant", Done: true}}
	if !strings.Contains(firstStepsCard(pending), "First steps") {
		t.Fatal("a pending step renders no card")
	}
	if got := firstStepsCard(done); got != "" {
		t.Fatalf("a finished card still renders:\n%s", got)
	}
	if got := firstStepsCard(nil); got != "" {
		t.Fatalf("no steps still renders:\n%s", got)
	}
	// Done steps stay visible while any remains, marked quiet — progress
	// a person can see, costing no store.
	body := firstStepsCard(append(done, pending...))
	if !strings.Contains(body, "done") || !strings.Contains(body, "/agents") {
		t.Fatalf("a mixed card lost a half:\n%s", body)
	}
}
