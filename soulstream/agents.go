package soulstream

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/identity"
	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/registry"
	siclient "github.com/impire-io/soulstream-identity/client"
)

// Agents: the machine voices a deployment answers for, and the credentials
// they arrive on.
//
// Two stores meet here and neither is the roster on its own. The record says
// who a voice is and who answers for it — a published card naming an
// operator, countersigned by that operator. The credential store says
// whether a voice can still get in. An agent whose credential was taken away
// is still on the record, and should be: it said things, and the things it
// said still need somebody's name against them.
//
// So the roster is the record, and the credential state is joined onto it.
// That also settles what an agent IS on this screen, which the record
// refuses to answer any other way: soulstream-core removed the persona
// `kind` taxonomy outright, because the protocol cannot verify what controls
// a key. A voice somebody else answers for is the honest, checkable version
// of the question, and it is the same fact the conversation screen reads to
// put a voice on the machine channel.
//
// WHAT IS NOT HERE. Last-seen. The credential record carries an account, a
// user, a label and an expiry, and nothing else — by the identity plane's own
// design, where a new field is the stated condition for reopening that
// decision. Every use of a credential is an audit line in the deployment's
// log, not a queryable fact, and this layer will not invent a store to keep
// one. The screen says when an agent was added, because the record knows
// that, and says nothing it would have to guess.

// Agent is one machine voice: what the record says about it, joined with
// whether it can still get in.
type Agent struct {
	// Handle is the name the record carries and every message is attributed
	// to.
	Handle string
	// ShownAs is what to call it on screen, "" when nobody published a name.
	ShownAs string
	// OperatedBy is the handle of whoever answers for it. Never empty — a
	// voice with no operator is not an agent, and is not listed here.
	OperatedBy string
	// Added is when the voice's card was published.
	Added time.Time
	// Handle of the credential, empty when none stands. It is what taking
	// the credential away is addressed to; the credential itself was shown
	// once and is kept nowhere.
	Credential string
	// Expires is when the credential stops being accepted, "" when it does
	// not expire on its own.
	Expires string
}

// Admitted reports whether this agent can still get in.
func (a Agent) Admitted() bool { return a.Credential != "" }

// Credential is what an agent needs to connect, assembled once and kept
// nowhere. Secret is the only secret half; everything else beside it is
// public or already on the deployment's disk.
type Credential struct {
	Handle  string
	ShownAs string
	// Secret is the credential itself. It exists in this struct, in one HTTP
	// response, and nowhere else — not in this layer, not in the record, not
	// on disk. The credential store keeps a digest it cannot reverse.
	Secret string
	// Dial, Realm and SentinelPath are the rest of what the agent's own
	// configuration needs; Sentinel is that file's contents, for an agent
	// that runs somewhere this deployment's disk is not.
	Dial         string
	Realm        string
	SentinelPath string
	Sentinel     string
}

// Agents is the deployment's agent roster and the authority to change it.
// Every act rides the node-standing lane the deployment handed this surface
// — the same lane its reads ride — because the credential ops are refused to
// a person's own admission by design, and the surface will not grow a
// side-channel to get around that.
type Agents struct{ sp *Support }

// Agents is this deployment's agent roster, nil when it issues no agent
// credentials — the absence a module reads to know it is not part of this
// deployment.
func (sp *Support) Agents() *Agents {
	if sp.cfg.AgentsDial == "" {
		return nil
	}
	return &Agents{sp: sp}
}

// AgentsDial is the address this deployment tells an agent to dial, "" when
// it issues no agent credentials. A declared deployment fact and nothing
// else: asking is the whole of what it costs.
func (sp *Support) AgentsDial() string { return sp.cfg.AgentsDial }

// List is the roster: every voice the record says somebody answers for, with
// the standing of its credential joined on.
func (ag *Agents) List(ctx context.Context) ([]Agent, error) {
	profiles, _, err := registry.All(ctx, ag.sp.rc)
	if err != nil {
		return nil, fmt.Errorf("reading the list of agents: %w", err)
	}
	standing, err := ag.standing()
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(profiles))
	for _, p := range profiles {
		if p.OperatedBy == "" {
			continue // answers for itself: a person, not an agent
		}
		a := Agent{Handle: p.Name, ShownAs: p.DisplayName,
			OperatedBy: p.OperatedBy, Added: p.CreatedAt}
		if e, ok := standing[p.Name]; ok {
			a.Credential, a.Expires = e.Digest, e.Expires
		}
		out = append(out, a)
	}
	return out, nil
}

// standing is every credential this deployment has issued, by the handle it
// admits as. A handle with more than one keeps the last read, which is the
// honest answer to a question the screen only asks in the singular: taking
// one away leaves the other standing, and the row will say so on the next
// read.
func (ag *Agents) standing() (map[string]siclient.TokenEntry, error) {
	entries, err := ag.sp.dir.Tokens()
	if err != nil {
		return nil, fmt.Errorf("reading which agents can still get in: %w", err)
	}
	out := map[string]siclient.TokenEntry{}
	for _, e := range entries {
		if e.Account == ag.sp.cfg.Account {
			out[e.User] = e
		}
	}
	return out, nil
}

// Create makes a new agent, in one act with three parts: a credential it can
// get in with, a card on the record saying what it is called, and the
// operator claim on that card naming the person who vouched for it.
//
// The claim is the part that has to be done exactly. The person signs it
// through their own session — their key, their admission, this surface
// holding neither — and it binds their name, the agent's name and the
// agent's key together, so it cannot be lifted onto a different agent later.
// The card carrying it is published by the agent itself over the credential
// just minted for it, because the directory takes an entry from nobody but
// the persona it belongs to. That credential passes through this layer for
// the length of this call and is then in the caller's hands only.
//
// Nothing half-made survives a failure: an agent that cannot finish being
// born has its credential taken away again before the error comes back.
func (ag *Agents) Create(ctx context.Context, sess *Session, handle, shownAs string) (Credential, error) {
	if sess == nil {
		return Credential{}, errors.New("nobody is signed in, so nobody can vouch for an agent")
	}
	if err := identity.CheckName(handle); err != nil {
		return Credential{}, fmt.Errorf("%q cannot be a handle: %w", handle, err)
	}
	if handle == sess.Persona {
		return Credential{}, errors.New("an agent cannot be the person who vouches for it")
	}
	if !sess.Signed {
		return Credential{}, errors.New(
			"your own signing key has not come through, so you cannot vouch for anybody yet — sign out and back in")
	}
	if _, found, err := registry.Lookup(ctx, ag.sp.rc, handle); err != nil {
		return Credential{}, fmt.Errorf("checking whether %q is taken: %w", handle, err)
	} else if found {
		return Credential{}, fmt.Errorf("%q is already somebody here — pick another handle", handle)
	}

	minted, err := ag.sp.dir.CreateToken(ag.sp.cfg.Account, handle, shownAs, 0)
	if err != nil {
		return Credential{}, fmt.Errorf("making a credential for %q: %w", handle, err)
	}
	cred, err := ag.found(ctx, sess, handle, shownAs, minted.Token)
	if err != nil {
		// The credential outlives the failure unless it is taken away here,
		// and a credential nobody was ever handed is the worst kind to leave
		// standing.
		if rerr := ag.sp.dir.RevokeToken(minted.Digest); rerr != nil {
			return Credential{}, fmt.Errorf("%w (and taking the unused credential back failed: %v)", err, rerr)
		}
		return Credential{}, err
	}
	return cred, nil
}

// found is the agent's own half of being created: it gets in with its new
// credential, its signing key comes into being on first touch, its operator
// signs for it, and it publishes its own card.
func (ag *Agents) found(ctx context.Context, sess *Session, handle, shownAs, secret string) (Credential, error) {
	nc, err := nats.Connect(ag.sp.cfg.NATSURL,
		nats.UserCredentials(ag.sp.cfg.SentinelPath), nats.Token(secret))
	if err != nil {
		return Credential{}, fmt.Errorf("%q could not get in with its new credential: %w", handle, err)
	}
	defer nc.Close()

	// The key is generated inside the deployment's own vault on first touch
	// and never leaves it — this surface learns the public half and nothing
	// more.
	signer, err := siclient.New(nc, ag.sp.cfg.Account, handle).PersonaSigner(handle)
	if err != nil {
		return Credential{}, fmt.Errorf("%q has no signing key yet: %w", handle, err)
	}
	stamp, err := registry.NewAttestationToken(sess.Client().Signer(), sess.Persona, handle, signer.PublicKey())
	if err != nil {
		return Credential{}, fmt.Errorf("%s could not vouch for %q: %w", sess.Persona, handle, err)
	}
	claim, err := registry.ParseAttestationToken(stamp)
	if err != nil {
		return Credential{}, fmt.Errorf("reading back what %s signed: %w", sess.Persona, err)
	}

	rc, err := realm.NewClient(ctx, nc, realm.Config{
		Realm: ag.sp.cfg.Realm, Persona: handle, Signer: signer,
	})
	if err != nil {
		return Credential{}, fmt.Errorf("%q could not reach the record: %w", handle, err)
	}
	now := time.Now()
	card := registry.Profile{
		Name: handle, DisplayName: shownAs, CreatedAt: now,
		SigningKey:          &registry.SigningKeyInfo{Ed25519: signer.PublicKey(), Since: now},
		OperatedBy:          sess.Persona,
		OperatorAttestation: &registry.OperatorAttestation{OperatedKey: claim.OperatedKey, Sig: claim.Sig},
	}
	if err := registry.Publish(ctx, rc, card); err != nil {
		return Credential{}, fmt.Errorf("%q could not publish its own card: %w", handle, err)
	}
	return ag.credential(handle, shownAs, secret), nil
}

// Remint hands an agent that already exists a new credential, and the old
// one stops working. The card stays as it is: who vouched for this voice is
// a thing that happened, and handing it a new way in does not unhappen it.
//
// The new credential is made before the old one is taken away, so a failure
// in the middle leaves the agent working rather than locked out of a
// deployment nobody is watching. The cost is the window between the two, and
// a failure to close it comes back named rather than swallowed — one
// credential too many is a thing somebody must be told about.
func (ag *Agents) Remint(ctx context.Context, handle string) (Credential, error) {
	p, found, err := registry.Lookup(ctx, ag.sp.rc, handle)
	if err != nil {
		return Credential{}, fmt.Errorf("looking up %q: %w", handle, err)
	}
	if !found || p.OperatedBy == "" {
		return Credential{}, fmt.Errorf("%q is not an agent here", handle)
	}
	standing, err := ag.standing()
	if err != nil {
		return Credential{}, err
	}
	minted, err := ag.sp.dir.CreateToken(ag.sp.cfg.Account, handle, p.DisplayName, 0)
	if err != nil {
		return Credential{}, fmt.Errorf("making a credential for %q: %w", handle, err)
	}
	cred := ag.credential(handle, p.DisplayName, minted.Token)
	if old, ok := standing[handle]; ok {
		if err := ag.sp.dir.RevokeToken(old.Digest); err != nil {
			return cred, fmt.Errorf(
				"%q has its new credential, but the one it had before is still accepted: %w", handle, err)
		}
	}
	return cred, nil
}

// TakeAway stops a credential being accepted. The next attempt to get in
// with it is refused outright; a connection already open ends when the
// identity it was given expires, which is the deployment's callout lifetime
// and the whole of the delay.
func (ag *Agents) TakeAway(handle string) error {
	standing, err := ag.standing()
	if err != nil {
		return err
	}
	e, ok := standing[handle]
	if !ok {
		return fmt.Errorf("%q has no credential to take away", handle)
	}
	if err := ag.sp.dir.RevokeToken(e.Digest); err != nil {
		return fmt.Errorf("taking %q's credential away: %w", handle, err)
	}
	return nil
}

// credential assembles what the agent's own configuration needs. The
// sentinel is read fresh and inlined because an agent may well run somewhere
// this deployment's disk is not; it is public by construction — a bearer
// that denies itself everything — so reading it costs nothing and hides
// nothing.
func (ag *Agents) credential(handle, shownAs, secret string) Credential {
	c := Credential{
		Handle: handle, ShownAs: shownAs, Secret: secret,
		Dial: ag.sp.cfg.AgentsDial, Realm: ag.sp.cfg.Realm,
		SentinelPath: ag.sp.cfg.SentinelPath,
	}
	if b, err := os.ReadFile(ag.sp.cfg.SentinelPath); err == nil {
		c.Sentinel = string(b)
	}
	return c
}
