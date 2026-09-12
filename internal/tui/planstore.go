package tui

import (
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// planStore is the store the active project's plan is kept in, and the one
// place the board decides which backend it is talking to: every screen and
// every write below takes a [store.Store] from here and never asks again.
//
// A project kept in Notion is the client the board already holds, wrapped
// afresh each time because wrapping one costs nothing and holds nothing open. A
// project kept in a file is a database that does hold something open, so it is
// opened once and kept — one per project, since switching back and forth is a
// keystroke and re-opening a file on every plan load would be a keystroke's
// worth of work each time. They are given back together when the app goes.
//
// A store that will not open answers as none, with the reason kept for whatever
// is about to try to load: the board already has a reading for "nothing to
// load", and it is the load that has somewhere to say why.
func (a *App) planStore() store.Store {
	p, ok := a.activeProject()
	if !ok {
		return nil
	}
	if !p.IsLocal() {
		if a.client == nil {
			return nil
		}
		return store.Over(a.client)
	}
	id := a.cfg.ActiveProjectID
	if st, open := a.localStores[id]; open {
		return st
	}
	st, err := store.ForProject(id, p, nil)
	if err != nil {
		a.localErr = err
		logging.Action("could not open a local plan", "project", id, "error", err.Error())
		return nil
	}
	if a.localStores == nil {
		a.localStores = map[string]store.Store{}
	}
	a.localStores[id] = st
	a.localErr = nil
	return st
}

// closeLocalStores gives back every local plan this session opened. It is the
// end of the app rather than of any one project: a plan stays open while its
// project is one of the board's tabs, because the user switches back.
func (a *App) closeLocalStores() {
	for id, st := range a.localStores {
		if err := st.Close(); err != nil {
			logging.Action("could not close a local plan", "project", id, "error", err.Error())
		}
	}
	a.localStores = nil
}

// owner is who this board works the active project's slices as: the identity a
// claim is written with and an ownership check compares against, and the name
// the slice's page says out loud. The two are one thing or two depending on
// where the plan is kept, which is why they are resolved here beside the store.
//
// A plan kept in Notion records ownership as the workspace user onboarding
// resolved. A plan kept in a file has no directory of users behind it at all:
// the name is the identity, because the string a claim wrote is the string the
// ownership check reads back. A board with no configured name at all leaves
// both empty, which for a local plan is a project claimed by nobody in
// particular — the same thing a project with no Assignee column has always
// been.
func (a *App) owner() owner {
	if p, ok := a.activeProject(); ok && p.IsLocal() {
		return owner{ID: a.cfg.AssigneeUserName, Name: a.cfg.AssigneeUserName}
	}
	return owner{ID: a.cfg.AssigneeUserID, Name: a.cfg.AssigneeUserName}
}

// owner is the pair above: what a claim writes and what a reader sees.
type owner struct {
	ID   string
	Name string
}
