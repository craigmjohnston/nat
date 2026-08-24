package gh

import (
	"encoding/json"
	"fmt"

	"github.com/craigmjohnston/nat/internal/logging"
)

// GitHub's own words for the two facts that decide whether a pull request is
// still waiting on anyone. reviewDecision is empty on a repository that
// requires no review and has had none, and otherwise one of APPROVED,
// CHANGES_REQUESTED or REVIEW_REQUIRED; mergeable is MERGEABLE, CONFLICTING or
// UNKNOWN, the last being GitHub still working the merge out. Only the two
// affirmative ones mean anything here — everything else is a pull request that
// is not ready, which is what an unread one is taken as too.
const (
	reviewApproved = "APPROVED"
	stateMergeable = "MERGEABLE"
	prStatusFields = "reviewDecision,mergeable"
)

// PRStatus is what gh says about a pull request that bears on whether it is
// still waiting to be reviewed: whether a review has approved it, and whether
// GitHub can merge it as it stands. Both false is a pull request with a review
// still to come — and equally the zero value, which is what a read that never
// happened comes back as.
type PRStatus struct {
	Approved  bool
	Mergeable bool
}

// PRStatus reads what GitHub currently says about the pull request at url, from
// the repository at dir.
//
// It asks gh for the two fields rather than the whole pull request: the board
// takes this reading on its own poll, for every slice whose work is out, and
// the rest of what `gh pr view` can print is a great deal of JSON nothing here
// reads. A gh that fails — not installed, not authenticated, no such pull
// request, no network — is logged and returned as itself: the caller's answer
// to that is to leave the slice where it was.
func (c CLI) PRStatus(dir, url string) (PRStatus, error) {
	out, err := c.runner.Run(dir, Binary, "pr", "view", url, "--json", prStatusFields)
	if err != nil {
		logging.Error("could not read the state of a pull request", "dir", dir, "url", url, "error", err)
		return PRStatus{}, err
	}
	var view struct {
		ReviewDecision string `json:"reviewDecision"`
		Mergeable      string `json:"mergeable"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		logging.Error("could not read what gh said about a pull request", "dir", dir, "url", url, "error", err)
		return PRStatus{}, fmt.Errorf("%s pr view printed no readable JSON: %w", Binary, err)
	}
	return PRStatus{
		Approved:  view.ReviewDecision == reviewApproved,
		Mergeable: view.Mergeable == stateMergeable,
	}, nil
}
