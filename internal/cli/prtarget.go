package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
)

// prTarget is the pull request a write to one names — pr-comment's, pr-edit's
// — as its command line gave it: a slice's recorded pull request, or, with
// --session, one of an ad hoc session's, named by URL or number. Settled from
// the arguments alone, before anything else is read, so a misuse is refused
// having read nothing.
type prTarget struct {
	command string
	// sliceID is the slice, where no session was named.
	sliceID string
	// sessionID and ref are the session and its pull request, where one was.
	sessionID string
	ref       string
}

// parsePRTarget reads the target off the arguments left after the flags.
func parsePRTarget(command, sessionID string, rest []string) (prTarget, error) {
	if sessionID != "" {
		if len(rest) != 1 {
			return prTarget{}, usageErrorf("%s: want exactly one pull request, by URL or number, given %d", command, len(rest))
		}
		id, err := pageID(command, sessionID)
		if err != nil {
			return prTarget{}, err
		}
		return prTarget{command: command, sessionID: id, ref: rest[0]}, nil
	}
	if len(rest) != 1 {
		return prTarget{}, usageErrorf("%s: want exactly one slice, by URL or ID, given %d", command, len(rest))
	}
	id, err := pageID(command, rest[0])
	if err != nil {
		return prTarget{}, err
	}
	return prTarget{command: command, sliceID: id}, nil
}

// resolve answers the directory gh runs in and the pull request it is given.
// A slice's is its recorded pull request in its repository, refused where it
// has none ("nothing to <verb>"). A session's is the one its pull requests,
// as the last batched reading kept them ([sessionHeldPR]), name — in the
// session's own directory — refused where it holds no such pull request:
// a write lands on GitHub, and one on a pull request no session of this
// project opened is a slip of the argument, not a write to make.
func (t prTarget) resolve(ctx context.Context, env Env, projectRef, verb string) (dir, url string, err error) {
	if t.sessionID != "" {
		sess, err := lookupSession(ctx, env, projectRef, t.sessionID)
		if err != nil {
			return "", "", err
		}
		held, ok := sessionHeldPR(env.loadLastReading(), sess, t.ref)
		if !ok {
			return "", "", fmt.Errorf("session %s holds no pull request %s: nothing to %s", sess.ID, t.ref, verb)
		}
		return sess.Dir, held, nil
	}
	_, projectID, project, err := env.projectFor(projectRef)
	if err != nil {
		return "", "", err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return "", "", err
	}
	s, _, err := st.Slice(ctx, t.sliceID)
	if err != nil {
		return "", "", fmt.Errorf("load the slice: %w", err)
	}
	if s.PRURL == "" {
		return "", "", fmt.Errorf("%q has no pull request recorded: nothing to %s", s.Name, verb)
	}
	return actions.WorkdirFor(s, project), s.PRURL, nil
}

// lookupSession is one of a project's ad hoc sessions by ID — the lookup
// pr-view --session, pr-comment --session and pr-edit --session share.
func lookupSession(ctx context.Context, env Env, projectRef, id string) (domain.Session, error) {
	_, projectID, project, err := env.projectFor(projectRef)
	if err != nil {
		return domain.Session{}, err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return domain.Session{}, err
	}
	sessions, err := st.Sessions(ctx, storeProject(projectID, project))
	if err != nil {
		return domain.Session{}, fmt.Errorf("read the sessions: %w", err)
	}
	sess, found := findSession(sessions, id)
	if !found {
		return domain.Session{}, noSessionError(id)
	}
	return sess, nil
}

// sessionHeldPR is the URL of the pull request ref names — by URL, whatever
// its case, query or trailing slash, or by number — among every pull request
// the last batched reading kept for the session's branches: exactly what
// session-list hands gnat to show. False where none matches, a reading that
// has kept nothing for the session included.
func sessionHeldPR(kept lastReading, sess domain.Session, ref string) (string, bool) {
	branches := make([]string, 0, len(kept.Sessions[sess.ID]))
	for b := range kept.Sessions[sess.ID] {
		branches = append(branches, b)
	}
	sort.Strings(branches)
	want := gh.NormaliseURL(ref)
	number, _ := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(ref), "#"))
	for _, b := range branches {
		for _, pr := range kept.Sessions[sess.ID][b] {
			if gh.NormaliseURL(pr.URL) == want || (number > 0 && pr.Number == number) {
				return pr.URL, true
			}
		}
	}
	return "", false
}
