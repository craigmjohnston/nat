package cli

import (
	"fmt"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
)

// tagOrPageID is a live session's name as [agentKill] and [agentSend] both
// accept it: an ad hoc session's own pane tag ([agent.SessionTag]), taken
// literally since it names nothing a Notion page ID could be extracted
// from, or a slice's URL/ID otherwise, read by [pageID] exactly as it
// always was. A session's tag is what [sessionList] prints for it, and what
// the app hands back here to end or nudge one.
func tagOrPageID(command, ref string) (string, error) {
	if agent.IsSessionTag(ref) {
		return ref, nil
	}
	return pageID(command, ref)
}

// findSession is one session of a project's, by ID, and false where the
// project has none by that ID — every session read this package does reads
// the whole list and then looks one up in it, rather than adding a second
// per-ID read to [store.Store] for a handful of rows a query already
// returns in full.
func findSession(sessions []domain.Session, id string) (domain.Session, bool) {
	for _, s := range sessions {
		if s.ID == id {
			return s, true
		}
	}
	return domain.Session{}, false
}

// noSessionError is what every session command refuses an unknown ID with.
func noSessionError(id string) error {
	return fmt.Errorf("no session %s for this project", id)
}
