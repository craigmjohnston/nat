package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// sessionList prints every ad hoc session on the project: whether tmux still
// has it, its branch, and its pull request summary. The pull request read
// is the one thing here that is not free — a session with a branch recorded
// gets exactly one `gh pr list`, which is cheap enough to run for every
// session in the list; a session with none recorded is skipped rather than
// asked about, since there is nothing yet for gh to answer.
func sessionList(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("session-list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("session-list: takes no positional arguments, given %d", len(rest))
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	sessions, err := st.Sessions(ctx, storeProject(projectID, project))
	if err != nil {
		return fmt.Errorf("read the sessions: %w", err)
	}

	live, err := env.NewTmux().LiveSlices()
	if err != nil {
		return fmt.Errorf("could not read live sessions: %w", err)
	}

	rows := make([]sessionRow, len(sessions))
	for i, s := range sessions {
		tag := agent.SessionTag(projectID, s.ID)
		tmuxSession, isLive := live[tag]
		row := sessionRow{Session: s, Tag: tag, Tmux: tmuxSession, Live: isLive}
		if s.Branch != "" {
			if prs, err := env.NewGH().ListPRsForHead(s.Dir, s.Branch); err == nil {
				row.PRs = prs
			} else {
				logging.Action("could not read a session's pull requests", "session", s.ID, "branch", s.Branch, "err", err)
				row.PRsStale = true
			}
		}
		rows[i] = row
	}

	if *asJSON {
		return writeSessionListJSON(env.Out, rows)
	}
	_, err = io.WriteString(env.Out, sessionListMarkdown(rows))
	return err
}

// sessionRow is one session as [sessionList] reports it, with the live tmux
// reading and the pull requests its branch has opened folded in.
type sessionRow struct {
	Session  domain.Session
	Tag      string
	Tmux     string
	Live     bool
	PRs      []gh.HeadPR
	PRsStale bool
}

type sessionListJSON struct {
	ID        string          `json:"id"`
	Tag       string          `json:"tag"`
	Live      bool            `json:"live"`
	Session   string          `json:"session,omitempty"`
	StartedAt time.Time       `json:"started_at"`
	Dir       string          `json:"dir"`
	Branch    string          `json:"branch,omitempty"`
	Ended     bool            `json:"ended"`
	PRs       []headPRJSON    `json:"prs,omitempty"`
	PRsStale  bool            `json:"prs_stale,omitempty"`
}

type headPRJSON struct {
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	State    string    `json:"state"`
	MergedAt time.Time `json:"merged_at,omitempty"`
}

func headPRsJSON(prs []gh.HeadPR) []headPRJSON {
	out := make([]headPRJSON, len(prs))
	for i, pr := range prs {
		out[i] = headPRJSON{Number: pr.Number, Title: pr.Title, URL: pr.URL, State: pr.State, MergedAt: pr.MergedAt}
	}
	return out
}

func writeSessionListJSON(out io.Writer, rows []sessionRow) error {
	doc := make([]sessionListJSON, len(rows))
	for i, r := range rows {
		doc[i] = sessionListJSON{
			ID: r.Session.ID, Tag: r.Tag, Live: r.Live, Session: r.Tmux, StartedAt: r.Session.StartedAt,
			Dir: r.Session.Dir, Branch: r.Session.Branch, Ended: r.Session.Ended(),
			PRs: headPRsJSON(r.PRs), PRsStale: r.PRsStale,
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func sessionListMarkdown(rows []sessionRow) string {
	if len(rows) == 0 {
		return "No ad hoc sessions.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Sessions\n\n")
	for _, r := range rows {
		state := "gone"
		if r.Live {
			state = "live: " + r.Tmux
		} else if r.Session.Ended() {
			state = "ended"
		}
		fmt.Fprintf(&b, "- %s — %s", r.Session.ID, state)
		if r.Session.Branch != "" {
			fmt.Fprintf(&b, ", branch %s", r.Session.Branch)
		}
		switch {
		case r.PRsStale:
			fmt.Fprintf(&b, ", pull requests: could not refresh")
		case len(r.PRs) > 0:
			fmt.Fprintf(&b, ", pull requests: %s", prSummary(r.PRs))
		}
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

// prSummary is a short count of a branch's pull requests by state, for the
// markdown listing — the JSON form carries every field instead.
func prSummary(prs []gh.HeadPR) string {
	var open, merged, closed int
	for _, pr := range prs {
		switch pr.State {
		case "OPEN":
			open++
		case "MERGED":
			merged++
		default:
			closed++
		}
	}
	var parts []string
	if open > 0 {
		parts = append(parts, fmt.Sprintf("%d open", open))
	}
	if merged > 0 {
		parts = append(parts, fmt.Sprintf("%d merged", merged))
	}
	if closed > 0 {
		parts = append(parts, fmt.Sprintf("%d closed", closed))
	}
	return strings.Join(parts, ", ")
}
