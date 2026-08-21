package soulstream

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/impire-io/soulstream-core/toolcatalog"
	siclient "github.com/impire-io/soulstream-identity/client"
)

// The tools and approvals facilities: what the two module surfaces of the
// external-tools and approvals designs read and act through.
//
// The lane rules are the designs' own (hq soulstream-shell 0005 §3,
// 0006 §3). Catalog reads ride the shared read lane — the realm's public
// shape, the board's own class. Catalog writes and every management op
// ride the node-standing lane the deployment handed this surface, the
// agents facility's precedent: those ops are refused to a person's own
// admission by design, and no side-channel is grown. Linking and approval
// MINTING are each person's own acts on their own session (session.go).
// Approval DELIVERY is the third thing: carrying a person's signed
// artifact to the originator's tail — the surface signs nothing as
// anyone; the plane verifies the signer and the actor binding.

// Tool is one row of the merged catalog: the record's discovery entry,
// the plane's resource where one exists, and nothing of anyone's standing
// — that is per person and joined by the module from the session.
type Tool struct {
	Name        string
	Kind        toolcatalog.Kind
	Persona     string
	Endpoint    string
	Description string
	// OnPlane says the identity plane holds a resource under this name —
	// the ceremony half a remote tool needs. A remote entry without one
	// isn't serving, and the screen says so.
	OnPlane bool
	// Declared marks a plane resource from the deployment's configuration
	// (not removable through the op).
	Declared bool
}

// Tools reads the merged catalog: the record's entries joined by name
// with the plane's resources. A catalog nobody wrote and a plane with no
// resources is an empty list, not an error.
func (sp *Support) Tools(ctx context.Context) ([]Tool, []string, error) {
	entries, warnings, err := toolcatalog.All(ctx, sp.rc)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the tool catalog: %w", err)
	}
	var notes []string
	for _, w := range warnings {
		notes = append(notes, fmt.Sprintf("tool %s: unreadable catalog entry", w.Name))
	}
	resources, err := sp.dir.Resources()
	if err != nil {
		return nil, nil, fmt.Errorf("reading the plane's resources: %w", err)
	}
	onPlane := make(map[string]siclient.ResourceInfo, len(resources))
	for _, r := range resources {
		onPlane[r.Name] = r
	}
	out := make([]Tool, 0, len(entries))
	seen := map[string]bool{}
	for _, e := range entries {
		t := Tool{Name: e.Name, Kind: e.Kind, Persona: e.Persona,
			Endpoint: e.Endpoint, Description: e.Description}
		if r, ok := onPlane[e.Name]; ok {
			t.OnPlane, t.Declared = true, r.Declared
		}
		out = append(out, t)
		seen[e.Name] = true
	}
	// A plane resource with no catalog entry is half a tool — shown, so
	// the drift is legible rather than invisible.
	for _, r := range resources {
		if seen[r.Name] {
			continue
		}
		out = append(out, Tool{Name: r.Name, Kind: toolcatalog.KindRemote,
			OnPlane: true, Declared: r.Declared,
			Description: "on the identity plane, not in the catalog"})
	}
	return out, notes, nil
}

// AddRemoteTool writes both halves in one act (the writer writes both —
// external-tools.md D39): the plane first, the catalog second, and the
// error names which half failed.
func (sp *Support) AddRemoteTool(ctx context.Context, r siclient.ResourceConfig,
	endpoint, description string,
) error {
	if err := sp.dir.ResourceAdd(r); err != nil {
		return fmt.Errorf("the identity plane refused the tool: %w", err)
	}
	if err := toolcatalog.Publish(ctx, sp.rc, toolcatalog.Entry{
		Name: r.Name, Kind: toolcatalog.KindRemote,
		Endpoint: endpoint, Description: description,
	}); err != nil {
		return fmt.Errorf("the plane holds the tool but the catalog write failed "+
			"(add again to finish): %w", err)
	}
	return nil
}

// AddWorkloadTool writes the catalog half a run-here tool has.
func (sp *Support) AddWorkloadTool(ctx context.Context, name, persona, endpoint, description string) error {
	return toolcatalog.Publish(ctx, sp.rc, toolcatalog.Entry{
		Name: name, Kind: toolcatalog.KindWorkload,
		Persona: persona, Endpoint: endpoint, Description: description,
	})
}

// RemoveTool reverses both halves. Standing grants keep their custody —
// the plane's own semantic, which the screen says out loud.
func (sp *Support) RemoveTool(ctx context.Context, name string) error {
	if err := sp.dir.ResourceRemove(name); err != nil {
		return fmt.Errorf("the identity plane refused: %w", err)
	}
	if err := toolcatalog.Remove(ctx, sp.rc, name); err != nil {
		return fmt.Errorf("the plane's half is gone but the catalog entry is not "+
			"(remove again to finish): %w", err)
	}
	return nil
}

// ApprovalsOn is the declared deployment fact the approvals module
// activates by (0006 §4): the identity plane runs the guardrail. No
// probe — asking is the whole of what it costs.
func (sp *Support) ApprovalsOn() bool { return sp.cfg.GuardrailOn }

// PendingApprovals is the tickets awaiting a decision, on the
// node-standing lane (the management read the plane gates by template).
func (sp *Support) PendingApprovals() ([]siclient.ApprovalTicket, error) {
	return sp.dir.PendingApprovals()
}

// GuardrailRules is the standing rule set — read for the approvers
// clauses that decide who is offered the screen; authority stays the
// plane's, which refuses an outside approver by name.
func (sp *Support) GuardrailRules() ([]siclient.GuardrailRule, error) {
	return sp.dir.GuardrailRules()
}

// Deliver carries a person's signed answer to the originator's own tail:
// approvals.present or approvals.deny as the originator, on the
// node-standing lane. Delivery, not authority — the artifact converts
// only the invocation its signer named, for the actor it names, and the
// plane verifies both.
func (sp *Support) Deliver(originator string, approve bool, invocationID string,
	d siclient.Delegation,
) error {
	user := originator
	if i := strings.LastIndex(originator, "/"); i >= 0 {
		user = originator[i+1:]
	}
	cl := siclient.New(sp.nc, sp.cfg.Account, user, siclient.WithTimeout(10*time.Second))
	if approve {
		return cl.PresentApproval(invocationID, d)
	}
	return cl.DenyApproval(invocationID, d)
}
