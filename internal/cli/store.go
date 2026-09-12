package cli

import (
	"fmt"
	"os/user"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/store"
)

// storeFor opens the store the named project's plan is kept in, which is the
// one place a headless command decides which backend it is talking to: every
// command takes a [store.Store] from here and never asks again.
//
// The Notion client is built only for a project that is kept in Notion. That is
// not a saving — building one costs nothing and fetches no token — but a
// statement: a machine tracking nothing but local projects has no Notion on any
// path it runs, and a seam that quietly made a client anyway would be one
// nobody could hold to that.
//
// The store has to be closed, which for a local project gives back the file it
// has open. Every caller does it with a deferred close whose error is dropped:
// the command has already said whatever it had to say, and a file that would
// not close is not news anybody can act on.
func (e Env) storeFor(projectID string, project config.ProjectConfig) (store.Store, error) {
	var api store.API
	if !project.IsLocal() {
		api = e.NewClient(e.Tokens.Token)
	}
	return store.ForProject(projectID, project, api)
}

// notionFor is the other half, for the few commands that read something only a
// workspace keeps — a project's wishlist, and the page a new project is created
// beside. A local project has none of it, so it is refused by name rather than
// handed a client that would go looking for a page that was never there.
func (e Env) notionFor(project config.ProjectConfig, doing string) (API, error) {
	if project.IsLocal() {
		return nil, fmt.Errorf("%s: %q keeps its plan in a file of nat's own, and that is a Notion feature", doing, project.Name)
	}
	return e.NewClient(e.Tokens.Token), nil
}

// owner is who this machine works a project's slices as: the identity a claim
// is written with and an ownership check compares against, and the name a
// reader sees. The two are one thing or two depending on where the plan is
// kept, which is exactly why they are resolved here beside the store rather
// than read straight off the config.
type owner struct {
	// ID is what a claim records and what [store.Holds] is asked about.
	ID string
	// Name is what the slice's page says out loud, for the sentences a command
	// writes about work somebody else holds.
	Name string
}

// ownerOf resolves that identity for one project. A plan kept in Notion records
// ownership as the workspace user onboarding resolved, so the ID is a Notion
// user and the name is what the workspace calls them. A plan kept in a file has
// no directory of users behind it at all: the name is the identity, because the
// string a claim wrote is the string an ownership check reads back.
//
// Which is also why a local project needs nothing set up to be worked. Where
// the config names a user the name is theirs, and where it does not — a machine
// that has never onboarded, because it has never had a Notion to onboard
// against — it is whoever is logged in, which is the only answer a file on this
// machine could have.
func ownerOf(cfg config.Config, project config.ProjectConfig) (owner, error) {
	if !project.IsLocal() {
		if cfg.AssigneeUserID == "" {
			return owner{}, fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
		}
		return owner{ID: cfg.AssigneeUserID, Name: cfg.AssigneeUserName}, nil
	}
	if name := strings.TrimSpace(cfg.AssigneeUserName); name != "" {
		return owner{ID: name, Name: name}, nil
	}
	name, err := loginName()
	if err != nil {
		return owner{}, fmt.Errorf("work out who is working this project: %w", err)
	}
	return owner{ID: name, Name: name}, nil
}

// loginName is whoever is logged in, which is who works a plan kept on their
// own machine. A machine whose own account cannot be read, or which has one
// with no name, is not one to guess a name for: an empty owner would claim
// every slice for nobody and read back as somebody else's.
func loginName() (string, error) {
	u, err := currentUser()
	if err != nil {
		return "", err
	}
	if name := strings.TrimSpace(u.Username); name != "" {
		return name, nil
	}
	return "", fmt.Errorf("this machine's account has no name")
}

// currentUser is os/user's answer, held as a variable so a test can reach both
// of the ways it fails to give one.
var currentUser = user.Current
