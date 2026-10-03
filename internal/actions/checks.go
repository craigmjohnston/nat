package actions

import (
	"context"
	"strings"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// PromptSender is what a nudge needs of tmux: one turn typed at a session.
type PromptSender interface {
	SendPrompt(session, text string) error
}

// ChecksStore is what [NoticeFailingChecks] does to a plan: read a slice's
// task log, and file one event on it — a Sent back where an agent was told, a
// Checks failed where none was there to be.
type ChecksStore interface {
	Body(ctx context.Context, id string) (string, error)
	RecordSentBack(ctx context.Context, id, comments string) error
	RecordChecksFailed(ctx context.Context, id, checks string) error
}

// FailingChecks is one slice whose pull request a reading found red, with the
// checks that failed.
type FailingChecks struct {
	Slice   domain.Slice
	Failing []gh.Check
}

// NoticeFailingChecks acts on a reading's red pull requests, once per failure:
// it tells the live agent on each such slice, or — with no agent live — puts
// the failure on the record, where the board and the app read it from.
//
// A failure is the set of its failing checks' run URLs (a check with none goes
// by its name). It is news only where the slice's most recent Checks failed or
// Sent back event names a different set: the same set is a failure already
// told or recorded, so a board polling every thirty seconds never nags, and a
// re-push that fails again — a new run, a new URL — is news again. A task log
// that cannot be read concludes nothing and the slice is passed over.
//
// A live session (live maps slice ID to session; a session that outlived its
// hand-back counts, and so does a fix session) is sent [agent.ChecksPrompt],
// then a Sent back naming the checks is filed — the existing send-back note,
// with Branch left alone, since the pull request hangs off it. The send goes
// first: a send that fails is logged and nothing is written, so the next
// reading tries again; a record that fails after a send that worked is
// logged, and costs at worst one repeated nudge.
//
// It reports whether anything was written, so the caller knows a refresh is
// owed.
func NoticeFailingChecks(ctx context.Context, st ChecksStore, sender PromptSender, live map[string]string,
	projectID string, failing []FailingChecks) bool {
	wrote := false
	for _, f := range failing {
		body, err := st.Body(ctx, f.Slice.ID)
		if err != nil {
			logging.Action("left a failing pull request unnoticed: its task log could not be read",
				"slice", f.Slice.ID, "error", err)
			continue
		}
		if sameFailure(lastChecksRecord(body), f.Failing) {
			continue
		}
		lines := checkLines(f.Failing)
		session := live[f.Slice.ID]
		if session == "" || sender == nil {
			if err := st.RecordChecksFailed(ctx, f.Slice.ID, "The pull request's checks failed:\n\n"+lines); err != nil {
				logging.Action("could not record a pull request's failed checks", "slice", f.Slice.ID, "error", err)
				continue
			}
			wrote = true
			continue
		}
		prompt := agent.ChecksPrompt(agent.ChecksContext{
			SliceID: f.Slice.ID, ProjectID: projectID, PRURL: f.Slice.PRURL,
			Branch: AgentBranch(f.Slice), Failing: failedChecks(f.Failing),
		})
		if err := sender.SendPrompt(session, prompt); err != nil {
			logging.Action("could not tell an agent its pull request's checks are failing",
				"slice", f.Slice.ID, "session", session, "error", err)
			continue
		}
		logging.Action("told an agent its pull request's checks are failing", "slice", f.Slice.ID, "session", session)
		if err := st.RecordSentBack(ctx, f.Slice.ID, "The pull request's checks failed, and the agent was told:\n\n"+lines); err != nil {
			logging.Action("could not record a checks nudge", "slice", f.Slice.ID, "error", err)
			continue
		}
		wrote = true
	}
	return wrote
}

// checkLines is the failed checks as the record names them, one bullet each:
// its name, then its run URL where it has one.
func checkLines(checks []gh.Check) string {
	var b strings.Builder
	for _, c := range checks {
		b.WriteString("- " + c.Name)
		if c.URL != "" {
			b.WriteString(": " + c.URL)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// failedChecks is the checks as the prompt names them.
func failedChecks(checks []gh.Check) []agent.FailedCheck {
	out := make([]agent.FailedCheck, len(checks))
	for i, c := range checks {
		out[i] = agent.FailedCheck{Name: c.Name, URL: c.URL}
	}
	return out
}

// lastChecksRecord is the text of the most recent Checks failed or Sent back
// event of a task log, or "" where there is neither.
func lastChecksRecord(body string) string {
	events := store.TaskEvents(body)
	for i := len(events) - 1; i >= 0; i-- {
		if k := events[i].Kind; k == store.ChecksFailedKind || k == store.SentBackKind {
			return events[i].Note
		}
	}
	return ""
}

// sameFailure reports whether a recorded event names exactly the failure the
// reading found: the same set of identities, each a run URL or, for a check
// with none, its name. The record's identities are read back off the bullets
// [checkLines] writes — a bullet's last word where it is a URL, its whole text
// otherwise — so a Sent back that is a review's own comments names whatever
// its bullets happen to say, and is a different failure.
func sameFailure(record string, checks []gh.Check) bool {
	recorded := map[string]bool{}
	for _, line := range strings.Split(record, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "- ")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if n := len(fields); n > 0 && (strings.HasPrefix(fields[n-1], "https://") || strings.HasPrefix(fields[n-1], "http://")) {
			recorded[fields[n-1]] = true
			continue
		}
		recorded[strings.TrimSpace(rest)] = true
	}
	found := map[string]bool{}
	for _, c := range checks {
		if c.URL != "" {
			found[c.URL] = true
		} else {
			found[c.Name] = true
		}
	}
	if len(found) != len(recorded) {
		return false
	}
	for k := range found {
		if !recorded[k] {
			return false
		}
	}
	return true
}
