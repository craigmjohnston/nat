package gh

import (
	"encoding/json"
	"fmt"
	"strings"

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
	prListFields   = "url,reviewDecision,mergeable"
)

// prListLimit is how many open pull requests one listing will carry. gh's own
// default is thirty, which a busy repository passes without saying so, and the
// three fields asked for are small enough that a hundred costs nothing worth
// counting. A repository with more open than that has its oldest left out of
// the answer, which reads here as a pull request that is no longer open — the
// same thing an unread one reads as, and the quiet direction to be wrong in.
const prListLimit = "100"

// PRStatus is what gh says about a pull request that bears on whether it is
// still waiting to be reviewed: whether a review has approved it, and whether
// GitHub can merge it as it stands. Both false is a pull request with a review
// still to come — and equally the zero value, which is what a read that never
// happened comes back as.
type PRStatus struct {
	Approved  bool
	Mergeable bool
}

// OpenPRs is every pull request the repository at dir currently has open, keyed
// by its URL as [NormaliseURL] writes it.
//
// It is one listing per repository rather than one view per pull request,
// because the board takes this reading on its own poll and for every slice that
// has a pull request recorded — a mature plan's worth of Done slices included,
// since a slice's pull request being open is what keeps it in the board's
// Active section. A gh per slice would grow with the plan forever; a listing
// does not grow at all.
//
// Being in the answer is itself the fact the caller is after: a pull request
// that has merged or been closed is simply not listed, which is how the board
// tells work that has landed from work that is still out. That inference rests
// on the listing having been read at all — a gh that fails is logged and
// returned as itself, and nothing may be concluded from the nothing it said.
func (c CLI) OpenPRs(dir string) (map[string]PRStatus, error) {
	out, err := c.runner.Run(dir, Binary,
		"pr", "list", "--state", "open", "--json", prListFields, "--limit", prListLimit)
	if err != nil {
		logging.Error("could not list the open pull requests of a repository", "dir", dir, "error", err)
		return nil, err
	}
	var list []struct {
		URL            string `json:"url"`
		ReviewDecision string `json:"reviewDecision"`
		Mergeable      string `json:"mergeable"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		logging.Error("could not read what gh said about a repository's pull requests", "dir", dir, "error", err)
		return nil, fmt.Errorf("%s pr list printed no readable JSON: %w", Binary, err)
	}
	open := make(map[string]PRStatus, len(list))
	for _, pr := range list {
		open[NormaliseURL(pr.URL)] = PRStatus{
			Approved:  pr.ReviewDecision == reviewApproved,
			Mergeable: pr.Mergeable == stateMergeable,
		}
	}
	return open, nil
}

// NormaliseURL is a pull request URL as the listing is keyed by it, so a URL
// typed onto a Notion page matches the canonical one gh prints. A query string
// or a fragment is whatever the link was copied from — a review, a file, a
// comment — and names the same pull request, a trailing slash is nothing at
// all, and the case of an owner or a repository is not a distinction GitHub
// makes.
func NormaliseURL(url string) string {
	url, _, _ = strings.Cut(url, "?")
	url, _, _ = strings.Cut(url, "#")
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(url), "/"))
}
