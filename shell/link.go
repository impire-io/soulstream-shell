package shell

// Cross-linking: how one module puts a person on the way to another
// module's screen without knowing that module exists.
//
// A surface stops feeling like a pile of screens the moment a name in one
// place reaches the place that knows more about it. Doing that by import
// would end the shell: two modules that know each other's packages are one
// module with a seam drawn on it, and the second of them could no longer be
// left out of a build. So the ask goes through the frame instead. A module
// names who it wants to reach (a slug), what kind of screen it wants (a
// route), and what it is about (params); the shell puts that to the modules
// this deployment actually runs, and the module that owns the screen builds
// its own link or declines.
//
// Nothing resolves that should not. A module this build does not run is not
// in the registry, so the ask comes back empty and the asking module renders
// what it would render for a stranger — the honest fallback is the caller's
// to write, because only the caller knows what its screen should say when
// there is nowhere to point.
//
// What the two modules share is a vocabulary — a slug, a route name, the
// spelling of a param — and never a type. That is the whole coupling, it is
// visible in both packages, and when it is wrong the link simply does not
// resolve: the same outcome as a deployment that runs no such module, which
// is the failure mode to have.

// A Link is one resolved way into another module's screen.
type Link struct {
	// Href is where it goes. The module that owns the screen builds it, so
	// no other module ever spells its paths — and it is written into an
	// attribute as it comes back, the way a NavEntry's is.
	Href string
	// Label is what the place at the other end is called, for the asking
	// module to put in a title or beside the link. A module that leaves it
	// empty is named by the shell with the name it registered under, which
	// is usually the truest answer anyway.
	Label string
}

// Linked is the optional face of a module that is willing to be reached
// from elsewhere in the product. A module that does not implement it is
// reachable only from the rail — nothing can link into it, and nothing
// pretends to.
type Linked interface {
	// Link builds a link to one of this module's own screens: route is this
	// module's own key for the kind of screen wanted, params what the link
	// is about. False for a route it does not offer, or params it cannot
	// make a screen out of — the caller then has nowhere to point, and says
	// so in its own words.
	Link(route string, params map[string]string) (Link, bool)
}

// Link asks the module registered under slug for a way into one of its
// screens. Nothing comes back when this deployment does not run that
// module, when the module accepts no links, or when it declines this one —
// three different reasons for the same honest answer: there is nowhere to
// point.
//
// Only running modules are asked. A module that said no at activation is
// not in the registry at all, so a link into it cannot resolve any more
// than one of its routes could answer.
func (s *Shell) Link(slug, route string, params map[string]string) (Link, bool) {
	for _, m := range s.live {
		if m.Identity().Slug != slug {
			continue
		}
		owner, ok := m.(Linked)
		if !ok {
			return Link{}, false
		}
		l, ok := owner.Link(route, params)
		// A link with nowhere to go is no link, whatever the module said.
		if !ok || l.Href == "" {
			return Link{}, false
		}
		if l.Label == "" {
			l.Label = m.Identity().Name
		}
		return l, true
	}
	return Link{}, false
}
