package shell

import (
	"context"
	"net/http"
)

// The module contract: the whole of what the shell knows about a human
// surface. Four things — what it is called, whether this deployment runs
// it, what it puts on the rail, and what it serves — and nothing else.
//
// A module is built beside the shell, never inside it: module packages
// import this one, this one imports no module. Which modules a build runs
// is composition's business (see ../embed), so the shell can host a surface
// whose module path it has never heard of.

// A Module is one human surface the shell hosts.
type Module interface {
	// Identity names the module.
	Identity() Identity
	// Active reports whether this deployment runs what the module needs.
	// It is asked once, when the shell starts: an inactive module
	// contributes no navigation and mounts no routes, so its paths answer
	// 404 like any other path nobody claimed.
	Active(ctx context.Context) bool
	// Nav is what this module puts on the rail for this request — nothing,
	// for a module that is not a place a person goes. It is asked on every
	// render rather than once, because an entry may carry state: where the
	// person already is, or a count of what is waiting for them.
	Nav(r *http.Request) []NavEntry
	// Mount registers the module's routes. The module owns every path it
	// claims and answers on no other.
	Mount(Router)
}

// Identity is what a module is called: the slug is the shell's own key for
// it, the name is what a person would call it.
type Identity struct {
	Slug string
	Name string
}

// NavEntry is one place on the rail a module contributes.
type NavEntry struct {
	// Section is the module's own key for the screen this entry reaches.
	// The shell only compares it with what a page says it is showing
	// (Page.Section) to mark where the person is.
	Section string
	// Icon is a name from the shell's icon set.
	Icon string
	// Label is what the entry says — the hover title and the accessible
	// name too, since a collapsed rail shows icons alone.
	Label string
	// Href is where the entry goes. The module builds it, so it may carry
	// whatever the module needs to keep across screens.
	Href string
	// Mark is rendered markup that rides after the label — a count of what
	// is waiting, a lamp, nothing at all. The module renders it, so the
	// shell never has to know what is being counted.
	Mark string
	// Foot puts the entry at the foot of the rail rather than the top,
	// beside the way out.
	Foot bool
}

// Router is where a module mounts its routes: Go's own pattern syntax,
// method and all.
type Router interface {
	Handle(pattern string, h http.Handler)
	HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))
}
