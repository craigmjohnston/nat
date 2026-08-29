package gh

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// prViewFields is everything one pull request is read for, and nothing else:
// the fields the viewer draws. gh will hand back whatever is asked for and the
// rest is JSON nobody here decodes, so the list is the screen's own contents
// written out — what the pull request is (number, title, body, author, the two
// branches, its URL), whether it is open, merged or still a draft, what stands
// between it and main (the review decision, mergeability, GitHub's own summary
// of the merge state, the checks) and what has been said on it.
const prViewFields = "number,title,body,state,isDraft,author,baseRefName,headRefName,url," +
	"reviewDecision,mergeable,mergeStateStatus,statusCheckRollup,reviews,comments"

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
}

// Check is one entry of the status check rollup: what it is called, where it
// stands and where the run itself can be read. GitHub reports two kinds of
// them — a CheckRun, which an Actions workflow produces, and a StatusContext,
// which the older commit status API does — and the difference is in the
// wording rather than in anything a viewer would draw differently, so both
// arrive here as a name, a state and a link.
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
	TypeName   string `json:"__typename"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
	Context    string `json:"context"`
	State      string `json:"state"`
	TargetURL  string `json:"targetUrl"`
}

// check is the entry as one thing: the name it goes by and the state it is in.
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
		return Check{Name: r.Name, State: state, URL: r.DetailsURL}
	default:
		// A kind GitHub has added since. Both shapes name themselves in one
		// field or the other, so taking whichever is filled in draws it as
		// well as it can be drawn rather than dropping it from the rollup.
		if r.Name != "" {
			return Check{Name: r.Name, State: r.Status, URL: r.DetailsURL}
		}
		return Check{Name: r.Context, State: r.State, URL: r.TargetURL}
	}
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
	}
	for _, entry := range v.Rollup {
		pr.Checks = append(pr.Checks, entry.check())
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
