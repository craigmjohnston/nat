package gh

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// prViewFields is everything one pull request is read for, and nothing else:
// the fields the viewer draws. gh will hand back whatever is asked for and the
// rest is JSON nobody here decodes, so the list is the screen's own contents
// written out — what the pull request is (number, title, body, author, the two
// branches, its URL), whether it is open, merged or still a draft, what stands
// between it and main (the review decision, mergeability, GitHub's own summary
// of the merge state, the checks), what has been said on it, and its change
// stats — additions, deletions, changed files, and the commit count — and who
// has been asked to review it. gh's own
// "commits" field is a full object per commit; only its length is kept, since
// the commits themselves are read through internal/git instead.
const prViewFields = "number,title,body,state,isDraft,author,baseRefName,headRefName,url," +
	"reviewDecision,mergeable,mergeStateStatus,statusCheckRollup,reviews,comments," +
	"additions,deletions,changedFiles,commits,reviewRequests"

// PR is one pull request as it is drawn: gh's answer decoded into the fields
// the viewer has a use for. GitHub's vocabulary is kept as GitHub writes it —
// State is OPEN, CLOSED or MERGED, ReviewDecision and Mergeable are the words
// [OpenPRs] already reads, MergeStateStatus is CLEAN, BLOCKED, DIRTY, BEHIND,
// UNSTABLE and the rest — because deciding what any of them means is the
// caller's, and a word this package invented would only have to be turned back.
type PR struct {
	Number           int
	Title            string
	Body             string
	State            string
	IsDraft          bool
	Author           string
	BaseRefName      string
	HeadRefName      string
	URL              string
	ReviewDecision   string
	Mergeable        string
	MergeStateStatus string
	Checks           []Check
	Reviews          []Review
	Comments         []Comment
	// Additions, Deletions and ChangedFiles are the pull request's own change
	// stats, gh's numbers passed through as numbers. Commits is how many
	// commits it holds — a count rather than the commits themselves, which gh
	// reports as a full object apiece and this package has no use for.
	Additions    int
	Deletions    int
	ChangedFiles int
	Commits      int
	// ReviewRequests is who has been asked for a review and not yet given
	// one: a user by login, a team as its slug.
	ReviewRequests []string
}

// The two states a pull request is in that a reader acts on: GitHub's own
// words, exported because what any of them means is the caller's question. OPEN
// is not among them on purpose — it is what a pull request is when it is
// neither of these, so a reader that tested for it would have to know every
// word GitHub might add as well.
const (
	PRStateMerged = "MERGED"
	PRStateClosed = "CLOSED"
)

// Check is one entry of the status check rollup: what it is called, where it
// stands and where the run itself can be read. GitHub reports two kinds of
// them — a CheckRun, which an Actions workflow produces, and a StatusContext,
// which the older commit status API does — and the difference is in the
// wording rather than in anything a viewer would draw differently, so both
// arrive here as a name, a state and a link. A CheckRun's name is its job's
// own led by its workflow ("CI / test", "Pull request / Gate"), as GitHub's
// checks list shows it — see [checksOf], which also orders them as that list
// does.
type Check struct {
	Name  string
	State string
	URL   string
}

// Review is one review left on the pull request: who submitted it, what they
// submitted it as (APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED) and what
// they wrote with it, which is empty for the great many reviews that are a
// verdict and no words. A review still pending has never been submitted and so
// carries no time at all.
type Review struct {
	Author      string
	State       string
	Body        string
	SubmittedAt time.Time
}

// Comment is one comment on the pull request itself — the conversation, not
// the comments left on lines of the diff, which gh's pr view does not carry.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
	URL       string
}

// The two shapes GitHub reports a check in, told apart by the __typename it
// stamps each entry of the rollup with. A CheckRun says what it is doing and,
// once it has finished, what came of it; a StatusContext has only ever had the
// one word for both.
const (
	typeCheckRun      = "CheckRun"
	statusCompleted   = "COMPLETED"
	typeStatusContext = "StatusContext"
)

// ViewPR is the pull request ref names, read in the repository at dir.
//
// The ref is whatever identifies it — a branch, a number or the URL recorded on
// the slice's PR property — and is handed to gh as it stands, since gh already
// knows how to read all three. It is required: gh with no ref at all reads the
// pull request of whatever branch the directory happens to be on, which for a
// shared checkout is nobody's slice in particular, and answering with the wrong
// pull request is worse than refusing to answer.
//
// A gh that ran and refused — no pull request for that branch, a repository it
// cannot see, an unauthenticated gh — comes back as the [*ExitError] the runner
// made of it, carrying gh's own first line, which is the sentence worth showing.
func (c CLI) ViewPR(dir, ref string) (PR, error) {
	if ref == "" {
		return PR{}, fmt.Errorf("%s pr view needs a pull request to read", Binary)
	}
	out, err := c.runner.Run(dir, Binary, "pr", "view", ref, "--json", prViewFields)
	if err != nil {
		logging.Error("could not read a pull request", "dir", dir, "ref", ref, "error", err)
		return PR{}, err
	}
	var view prView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		logging.Error("could not read what gh said about a pull request",
			"dir", dir, "ref", ref, "error", err)
		return PR{}, fmt.Errorf("%s pr view printed no readable JSON: %w", Binary, err)
	}
	return view.pr(), nil
}

// ReviewComments is the raw text gh prints for `gh pr view <ref> --comments`
// — the conversation on the pull request, exactly as a fix session's own
// prompt tells the agent it may read it. Read once at launch and inlined
// rather than decoded through [CLI.ViewPR]'s JSON, so what the agent is
// handed is the same text the command itself would have printed.
func (c CLI) ReviewComments(dir, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("%s pr view needs a pull request to read", Binary)
	}
	out, err := c.runner.Run(dir, Binary, "pr", "view", ref, "--comments")
	if err != nil {
		logging.Error("could not read a pull request's comments", "dir", dir, "ref", ref, "error", err)
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// Checks is the raw text gh prints for `gh pr checks <ref>` — the same second
// read a fix session's prompt names.
//
// gh exits non-zero whenever any check is failing or still running, with the
// check table printed regardless — and a check failing is exactly why a fix
// session exists, so that table is not a failed read: only an error with
// nothing printed at all (no checks reported, an unauthenticated gh) is
// treated as one.
func (c CLI) Checks(dir, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("%s pr checks needs a pull request to read", Binary)
	}
	out, err := c.runner.Run(dir, Binary, "pr", "checks", ref)
	trimmed := strings.TrimRight(out, "\n")
	if err != nil && trimmed == "" {
		logging.Error("could not read a pull request's checks", "dir", dir, "ref", ref, "error", err)
		return "", err
	}
	return trimmed, nil
}

// headPRFields is everything [CLI.ListPRsForHead] reads about each pull
// request of a branch: what it is, its URL, whether it is open, closed or
// merged, and when it merged.
const headPRFields = "number,title,url,state,mergedAt"

// HeadPR is one pull request [CLI.ListPRsForHead] lists for a branch. State
// is GitHub's own word — OPEN, CLOSED or MERGED — kept as GitHub writes it
// for the reason [PR.State] is: deciding what it means is the caller's.
type HeadPR struct {
	Number   int
	Title    string
	URL      string
	State    string
	MergedAt time.Time
}

// ListPRsForHead is every pull request the repository at dir has ever had
// for branch as its head, open or not — an ad hoc session's own branches may
// each have opened one, and a session that made three branches and three
// pull requests has to be told about all three, not only whichever is still
// open.
//
// An empty branch is refused before gh ever runs, for the same reason
// [CLI.ViewPR] refuses an empty ref: gh given nothing named would answer for
// whatever branch the directory happens to be on, which is not the question
// being asked.
func (c CLI) ListPRsForHead(dir, branch string) ([]HeadPR, error) {
	if branch == "" {
		return nil, fmt.Errorf("%s pr list needs a branch to read", Binary)
	}
	out, err := c.runner.Run(dir, Binary,
		"pr", "list", "--head", branch, "--state", "all", "--json", headPRFields)
	if err != nil {
		logging.Error("could not list a branch's pull requests", "dir", dir, "branch", branch, "error", err)
		return nil, err
	}
	var list []struct {
		Number   int       `json:"number"`
		Title    string    `json:"title"`
		URL      string    `json:"url"`
		State    string    `json:"state"`
		MergedAt time.Time `json:"mergedAt"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		logging.Error("could not read what gh said about a branch's pull requests", "dir", dir, "branch", branch, "error", err)
		return nil, fmt.Errorf("%s pr list printed no readable JSON: %w", Binary, err)
	}
	prs := make([]HeadPR, len(list))
	for i, pr := range list {
		prs[i] = HeadPR{Number: pr.Number, Title: pr.Title, URL: pr.URL, State: pr.State, MergedAt: pr.MergedAt}
	}
	return prs, nil
}

// prView is gh's JSON as gh writes it, kept apart from [PR] so the nesting
// GitHub puts an author in — and the two shapes a check arrives in — are
// undone in one place rather than left for every reader of a PR to know about.
type prView struct {
	Number           int      `json:"number"`
	Title            string   `json:"title"`
	Body             string   `json:"body"`
	State            string   `json:"state"`
	IsDraft          bool     `json:"isDraft"`
	Author           ghUser   `json:"author"`
	BaseRefName      string   `json:"baseRefName"`
	HeadRefName      string   `json:"headRefName"`
	URL              string   `json:"url"`
	ReviewDecision   string   `json:"reviewDecision"`
	Mergeable        string   `json:"mergeable"`
	MergeStateStatus string   `json:"mergeStateStatus"`
	Rollup           []ghRoll `json:"statusCheckRollup"`
	Reviews          []struct {
		Author      ghUser    `json:"author"`
		State       string    `json:"state"`
		Body        string    `json:"body"`
		SubmittedAt time.Time `json:"submittedAt"`
	} `json:"reviews"`
	Comments []struct {
		Author    ghUser    `json:"author"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"createdAt"`
		URL       string    `json:"url"`
	} `json:"comments"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	ChangedFiles int `json:"changedFiles"`
	// Commits is decoded as raw JSON rather than a struct this package would
	// otherwise have to keep in step with GitHub's own commit shape: only the
	// length of it is ever read.
	Commits        []json.RawMessage `json:"commits"`
	ReviewRequests []struct {
		Login string `json:"login"`
		Slug  string `json:"slug"`
		Name  string `json:"name"`
	} `json:"reviewRequests"`
}

// ghUser is whoever GitHub names, of which only the login is drawn. A comment
// or review left by an account since deleted has no author at all, which
// decodes as the empty login it reads as.
type ghUser struct {
	Login string `json:"login"`
}

// ghRoll is one rollup entry with both kinds' fields on it, since which of
// them are filled in is what __typename says.
type ghRoll struct {
	TypeName string `json:"__typename"`
	Name     string `json:"name"`
	// WorkflowName is the Actions workflow a CheckRun ran under ("CI"), ""
	// for a StatusContext or a check from an app outside Actions — what
	// leads every run's name ([checksOf]).
	WorkflowName string `json:"workflowName"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	DetailsURL   string `json:"detailsUrl"`
	Context      string `json:"context"`
	State        string `json:"state"`
	TargetURL    string `json:"targetUrl"`
}

// checkNameSep is what GitHub joins a job's path with, and what a run's
// workflow is joined to it with.
const checkNameSep = " / "

// checksOf is a whole rollup as [Check]s, in the order GitHub's own checks
// list shows them. Every run is led by its workflow ([ghRoll.workflowLed]):
// "CI / lint", "CI / test", "macOS App CI / test". Where two runs still read
// the same after that — two workflows calling one reusable job — those alone
// take the full path GitHub gives them under their workflow ("Pull request /
// checks / Gate"); names that clash even then are left so, since the run's
// URL tells them apart. A StatusContext keeps its context whole, and an entry
// with no workflow its bare name, with nothing leading it. The triggering
// event GitHub's page appends ("(pull_request)") is not in gh's rollup and is
// never added.
//
// GitHub's list is the rollup sorted by its displayed name — gh hands the
// rollup back in the order the runs were created, which put "CI / lint" after
// both tests on this repo's own pull requests (checked against one of them,
// October 2026) — so the checks are sorted here by their final name,
// case-insensitively and stably, so names that compare equal keep the
// rollup's order. Sorted once, here, so no face reorders them.
func checksOf(rollup []ghRoll) []Check {
	var checks []Check
	uses := map[string]int{}
	for _, entry := range rollup {
		check := entry.check()
		if entry.isRun() && entry.WorkflowName != "" {
			check.Name = entry.workflowLed(entry.jobName())
		}
		checks = append(checks, check)
		uses[check.Name]++
	}
	for i, entry := range rollup {
		if uses[checks[i].Name] > 1 && entry.isRun() && entry.WorkflowName != "" {
			checks[i].Name = entry.workflowLed(entry.Name)
		}
	}
	slices.SortStableFunc(checks, func(a, b Check) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return checks
}

// isRun is whether the entry was named off its run's name — a CheckRun, or a
// kind GitHub has added since that carries one — rather than a context.
func (r ghRoll) isRun() bool {
	return r.TypeName != typeStatusContext && r.Name != ""
}

// workflowLed is name led by the workflow the run ran under, as GitHub's
// checks list leads it.
func (r ghRoll) workflowLed(name string) string {
	return r.WorkflowName + checkNameSep + name
}

// check is the entry as one thing: the name it goes by and the state it is in.
// A CheckRun goes by its job's own name ([ghRoll.jobName]), which [checksOf]
// leads with its workflow; a StatusContext by its context, which is no job
// path and is left whole.
// A CheckRun that has finished is worth its conclusion — SUCCESS, FAILURE,
// CANCELLED — and one still going is worth its status instead, since a run
// that has not concluded has no conclusion to report; a StatusContext has only
// its state, whichever of the two it is.
func (r ghRoll) check() Check {
	switch r.TypeName {
	case typeStatusContext:
		return Check{Name: r.Context, State: r.State, URL: r.TargetURL}
	case typeCheckRun:
		state := r.Status
		if r.Status == statusCompleted {
			state = r.Conclusion
		}
		return Check{Name: r.jobName(), State: state, URL: r.DetailsURL}
	default:
		// A kind GitHub has added since. Both shapes name themselves in one
		// field or the other, so taking whichever is filled in draws it as
		// well as it can be drawn rather than dropping it from the rollup.
		if r.Name != "" {
			return Check{Name: r.jobName(), State: r.Status, URL: r.DetailsURL}
		}
		return Check{Name: r.Context, State: r.State, URL: r.TargetURL}
	}
}

// jobName is a CheckRun-shaped entry's name cut to its last " / " segment —
// the job's own. GitHub names a job run through a reusable workflow by its
// caller-job path ("checks / Gate"); a row is for a reader glancing down a
// list, to whom the caller job is noise beside the workflow and "Gate"
// ("Pull request / Gate"), and the run's URL still leads to the full context.
// Only a clash in the rollup earns the whole path back ([checksOf]).
func (r ghRoll) jobName() string {
	if i := strings.LastIndex(r.Name, checkNameSep); i >= 0 {
		return r.Name[i+len(checkNameSep):]
	}
	return r.Name
}

// pr is the decoded view flattened into what the viewer draws.
func (v prView) pr() PR {
	pr := PR{
		Number:           v.Number,
		Title:            v.Title,
		Body:             v.Body,
		State:            v.State,
		IsDraft:          v.IsDraft,
		Author:           v.Author.Login,
		BaseRefName:      v.BaseRefName,
		HeadRefName:      v.HeadRefName,
		URL:              v.URL,
		ReviewDecision:   v.ReviewDecision,
		Mergeable:        v.Mergeable,
		MergeStateStatus: v.MergeStateStatus,
		Additions:        v.Additions,
		Deletions:        v.Deletions,
		ChangedFiles:     v.ChangedFiles,
		Commits:          len(v.Commits),
	}
	pr.Checks = checksOf(v.Rollup)
	for _, request := range v.ReviewRequests {
		// A user names itself by login; a team has none, and goes by its slug.
		name := request.Login
		if name == "" {
			name = request.Slug
		}
		if name == "" {
			name = request.Name
		}
		if name != "" {
			pr.ReviewRequests = append(pr.ReviewRequests, name)
		}
	}
	for _, review := range v.Reviews {
		pr.Reviews = append(pr.Reviews, Review{
			Author:      review.Author.Login,
			State:       review.State,
			Body:        review.Body,
			SubmittedAt: review.SubmittedAt,
		})
	}
	for _, comment := range v.Comments {
		pr.Comments = append(pr.Comments, Comment{
			Author:    comment.Author.Login,
			Body:      comment.Body,
			CreatedAt: comment.CreatedAt,
			URL:       comment.URL,
		})
	}
	return pr
}
