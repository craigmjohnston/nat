package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// PRRef names one pull request by where it lives: the repository's owner and
// name, lower-cased as [NormaliseURL] writes them, and its number.
type PRRef struct {
	Owner  string
	Repo   string
	Number int
}

// ParsePRURL is the pull request a URL names — https://<host>/<owner>/<repo>/
// pull/<number>, in whatever shape it was pasted ([NormaliseURL] undoes the
// rest). Anything else names none.
func ParsePRURL(url string) (PRRef, bool) {
	rest := NormaliseURL(url)
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+len("://"):]
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 5 || parts[1] == "" || parts[2] == "" || parts[3] != "pull" {
		return PRRef{}, false
	}
	n, err := strconv.Atoi(parts[4])
	if err != nil || n <= 0 {
		return PRRef{}, false
	}
	return PRRef{Owner: parts[1], Repo: parts[2], Number: n}, true
}

// ParseRemote is the repository a git remote URL names — https, ssh or
// scp-like (git@host:owner/repo.git) — lower-cased as a [PRRef] is, so a
// session's branches are asked about in the repository its pull requests
// live in.
func ParseRemote(url string) (owner, repo string, ok bool) {
	rest := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(url), "/"), ".git")
	if i := strings.Index(rest, "://"); i >= 0 {
		_, path, found := strings.Cut(rest[i+len("://"):], "/")
		if !found {
			return "", "", false
		}
		rest = path
	} else if _, path, found := strings.Cut(rest, ":"); found {
		rest = path
	} else {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return strings.ToLower(parts[0]), strings.ToLower(parts[1]), true
}

// HeadRef names one branch of a repository whose pull requests are wanted —
// an ad hoc session's, which has no pull request recorded anywhere to name.
type HeadRef struct {
	Owner  string
	Repo   string
	Branch string
}

// BatchQuery is everything one reading asks GitHub: pull requests by number,
// branches by name, and at most one pull request in full detail.
type BatchQuery struct {
	PRs    []PRRef
	Heads  []HeadRef
	Detail *PRRef
}

// RateLimit is GitHub's GraphQL budget as the reading's last document left it:
// the hour's points, what is left of them, and when the hour resets.
type RateLimit struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// Batch is what a reading found. Something asked for and absent is unread —
// its document failed, or GitHub could not resolve it — and concludes nothing.
// A pull request in PRs carries what [StatusOf] and a merge refusal read —
// state, merge time, review decision, mergeability, merge state, base, draft,
// checks — and nothing of the detail's (title, body, reviews…); Detail is the
// one asked for in full, every field [CLI.ViewPR] reads.
type Batch struct {
	PRs       map[PRRef]PR
	Heads     map[HeadRef][]HeadPR
	Detail    *PR
	RateLimit *RateLimit
}

// batchChunk is how many things — pull requests, branches, the detail — one
// document asks about. A document costs a point whatever it holds, up to
// GitHub's node limits; twenty-five keeps one well inside them.
const batchChunk = 25

// batchItem is one thing a document asks about: a pull request by number, a
// branch by name, or the pull request in full.
type batchItem struct {
	pr     PRRef
	head   HeadRef
	isHead bool
	detail bool
}

func (i batchItem) repo() (owner, name string) {
	if i.isHead {
		return i.head.Owner, i.head.Repo
	}
	return i.pr.Owner, i.pr.Repo
}

// ReadPRs takes one reading of everything q names, as one `gh api graphql`
// document per [batchChunk] things. gh brings the host and the auth; no
// repository directory is needed, since every repository is named in the
// document.
//
// A document that fails outright is logged, and everything it asked about is
// left out of the answer: unread, never "closed" or "merged". A node GitHub
// could not resolve (a repository renamed, a number that is no pull request)
// is that node alone left out, logged — one dead link on one slice must not
// blind the reading of every other. The error is the failed documents',
// joined; the batch beside it is still every node that was read.
func (c CLI) ReadPRs(q BatchQuery) (Batch, error) {
	out := Batch{PRs: map[PRRef]PR{}, Heads: map[HeadRef][]HeadPR{}}
	var items []batchItem
	if q.Detail != nil {
		items = append(items, batchItem{pr: *q.Detail, detail: true})
	}
	for _, pr := range q.PRs {
		items = append(items, batchItem{pr: pr})
	}
	for _, h := range q.Heads {
		items = append(items, batchItem{head: h, isHead: true})
	}
	var errs []error
	for start := 0; start < len(items); start += batchChunk {
		chunk := items[start:min(start+batchChunk, len(items))]
		if err := c.readDocument(chunk, &out); err != nil {
			errs = append(errs, err)
		}
	}
	return out, errors.Join(errs...)
}

// readDocument asks one document's worth of items and files what came back
// into out.
func (c CLI) readDocument(items []batchItem, out *Batch) error {
	doc, aliases := batchDocument(items)
	stdout, runErr := c.runner.Run("", Binary, "api", "graphql", "-f", "query="+doc)
	var resp struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Path    []any  `json:"path"`
		} `json:"errors"`
	}
	// gh exits non-zero on a document GitHub answered with errors, and still
	// prints the answer, so the answer is read first and the exit second.
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		if runErr == nil {
			runErr = fmt.Errorf("%s api graphql printed no readable answer: %w", Binary, err)
		}
		logging.Error("could not read the pull requests", "items", len(items), "error", runErr)
		return runErr
	}
	for _, e := range resp.Errors {
		if len(e.Path) == 0 {
			// An error about no node is about the whole document — a refusal
			// of the budget, a document GitHub would not run.
			logging.Error("GitHub refused the pull requests' reading", "items", len(items), "type", e.Type, "error", e.Message)
			return fmt.Errorf("GraphQL: %s", e.Message)
		}
		logging.Action("left a pull request GitHub could not resolve unread", "path", fmt.Sprint(e.Path), "type", e.Type)
	}
	if resp.Data == nil {
		err := fmt.Errorf("%s api graphql answered with no data", Binary)
		logging.Error("could not read the pull requests", "items", len(items), "error", err)
		return err
	}
	if raw, ok := resp.Data["rateLimit"]; ok {
		var rl struct {
			Limit     int       `json:"limit"`
			Remaining int       `json:"remaining"`
			ResetAt   time.Time `json:"resetAt"`
		}
		if json.Unmarshal(raw, &rl) == nil {
			out.RateLimit = &RateLimit{Limit: rl.Limit, Remaining: rl.Remaining, ResetAt: rl.ResetAt}
		}
	}
	for repoAlias, nodes := range aliases {
		var repo map[string]json.RawMessage
		if err := json.Unmarshal(resp.Data[repoAlias], &repo); err != nil || repo == nil {
			continue
		}
		for alias, item := range nodes {
			fileNode(repo[alias], item, out)
		}
	}
	logging.Action("read pull requests", "items", len(items))
	return nil
}

// fileNode decodes one aliased node into out; a null or unreadable one is
// left out, unread.
func fileNode(raw json.RawMessage, item batchItem, out *Batch) {
	if item.isHead {
		var conn struct {
			Nodes []struct {
				Number   int        `json:"number"`
				Title    string     `json:"title"`
				URL      string     `json:"url"`
				State    string     `json:"state"`
				MergedAt *time.Time `json:"mergedAt"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal(raw, &conn); err != nil || conn.Nodes == nil {
			return
		}
		prs := make([]HeadPR, len(conn.Nodes))
		for i, n := range conn.Nodes {
			prs[i] = HeadPR{Number: n.Number, Title: n.Title, URL: n.URL, State: n.State}
			if n.MergedAt != nil {
				prs[i].MergedAt = *n.MergedAt
			}
		}
		out.Heads[item.head] = prs
		return
	}
	var node *gqlPR
	if err := json.Unmarshal(raw, &node); err != nil || node == nil {
		return
	}
	pr := node.pr()
	if item.detail {
		out.Detail = &pr
		return
	}
	out.PRs[item.pr] = pr
}

// batchDocument writes the GraphQL document for items, grouped by repository:
// an aliased repository per repository (r0, r1…) holding an aliased field per
// item (p<n> a pull request by number, h<n> a branch's pull requests, d the
// detail), every pull request through the one status fragment. It returns the
// document and, per repository alias, what each of its aliases asked.
func batchDocument(items []batchItem) (string, map[string]map[string]batchItem) {
	type repoKey struct{ owner, name string }
	var order []repoKey
	byRepo := map[repoKey][]batchItem{}
	for _, item := range items {
		owner, name := item.repo()
		key := repoKey{owner, name}
		if _, seen := byRepo[key]; !seen {
			order = append(order, key)
		}
		byRepo[key] = append(byRepo[key], item)
	}

	var b strings.Builder
	b.WriteString("query {\n  rateLimit { limit remaining resetAt }\n")
	aliases := map[string]map[string]batchItem{}
	usesStatus, usesDetail := false, false
	n := 0
	for r, key := range order {
		repoAlias := fmt.Sprintf("r%d", r)
		aliases[repoAlias] = map[string]batchItem{}
		fmt.Fprintf(&b, "  %s: repository(owner: %s, name: %s) {\n", repoAlias, gqlString(key.owner), gqlString(key.name))
		for _, item := range byRepo[key] {
			switch {
			case item.detail:
				usesStatus, usesDetail = true, true
				aliases[repoAlias]["d"] = item
				fmt.Fprintf(&b, "    d: pullRequest(number: %d) { ...status ...detail }\n", item.pr.Number)
			case item.isHead:
				alias := fmt.Sprintf("h%d", n)
				aliases[repoAlias][alias] = item
				fmt.Fprintf(&b, "    %s: pullRequests(first: 10, headRefName: %s, "+
					"orderBy: {field: CREATED_AT, direction: DESC}) "+
					"{ nodes { number title url state mergedAt } }\n", alias, gqlString(item.head.Branch))
			default:
				usesStatus = true
				alias := fmt.Sprintf("p%d", n)
				aliases[repoAlias][alias] = item
				fmt.Fprintf(&b, "    %s: pullRequest(number: %d) { ...status }\n", alias, item.pr.Number)
			}
			n++
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	// GraphQL refuses a document defining a fragment it never spreads, so
	// each goes in only where something uses it.
	if usesStatus {
		b.WriteString(statusFragment)
	}
	if usesDetail {
		b.WriteString(detailFragment)
	}
	return b.String(), aliases
}

// statusFragment is what every pull request is read for: whether it is open,
// merged or closed and when it merged, what stands between it and its base —
// the review decision, mergeability, GitHub's merge state, whether it is a
// draft — and its head commit's check rollup, thirty contexts deep. The same
// facts `gh pr list --json` read, and the merge refusal weighs.
const statusFragment = `fragment status on PullRequest {
  number url state isDraft mergedAt reviewDecision mergeable mergeStateStatus baseRefName
  lastCommit: commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 30) { nodes {
    __typename
    ... on CheckRun { name status conclusion detailsUrl checkSuite { workflowRun { workflow { name } } } }
    ... on StatusContext { context state targetUrl }
  } } } } } }
}
`

// detailFragment is the rest of what the pull request screen draws, the
// fields [prViewFields] names: what it is, its change stats, the commit count
// (a count, never the commits), the latest hundred reviews and comments —
// the newest being the ones a reader is waiting for — and who is asked to
// review.
const detailFragment = `fragment detail on PullRequest {
  title body author { login } headRefName headRefOid additions deletions changedFiles
  allCommits: commits { totalCount }
  reviews(last: 100) { nodes { author { login } state body submittedAt } }
  comments(last: 100) { nodes { author { login } body createdAt url } }
  reviewRequests(first: 100) { nodes { requestedReviewer {
    __typename ... on User { login } ... on Team { slug name }
  } } }
}
`

// gqlString is s as a GraphQL string literal. JSON's escapes are a subset of
// GraphQL's, so a branch name with a quote in it stays one argument.
func gqlString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// gqlPR is a pull request as the GraphQL document answers it: the connections
// GitHub wraps lists in, and the rollup under the last commit, undone by
// [gqlPR.pr] into the [PR] gh pr view's JSON decodes to.
type gqlPR struct {
	Number           int        `json:"number"`
	Title            string     `json:"title"`
	Body             string     `json:"body"`
	State            string     `json:"state"`
	IsDraft          bool       `json:"isDraft"`
	MergedAt         *time.Time `json:"mergedAt"`
	Author           ghUser     `json:"author"`
	BaseRefName      string     `json:"baseRefName"`
	HeadRefName      string     `json:"headRefName"`
	HeadRefOid       string     `json:"headRefOid"`
	URL              string     `json:"url"`
	ReviewDecision   string     `json:"reviewDecision"`
	Mergeable        string     `json:"mergeable"`
	MergeStateStatus string     `json:"mergeStateStatus"`
	Additions        int        `json:"additions"`
	Deletions        int        `json:"deletions"`
	ChangedFiles     int        `json:"changedFiles"`
	LastCommit       struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []gqlContext `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"lastCommit"`
	AllCommits struct {
		TotalCount int `json:"totalCount"`
	} `json:"allCommits"`
	Reviews struct {
		Nodes []struct {
			Author      ghUser    `json:"author"`
			State       string    `json:"state"`
			Body        string    `json:"body"`
			SubmittedAt time.Time `json:"submittedAt"`
		} `json:"nodes"`
	} `json:"reviews"`
	Comments struct {
		Nodes []struct {
			Author    ghUser    `json:"author"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"createdAt"`
			URL       string    `json:"url"`
		} `json:"nodes"`
	} `json:"comments"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login"`
				Slug  string `json:"slug"`
				Name  string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
}

// gqlContext is one rollup context with both kinds' fields on it, the
// workflow's name nested where GraphQL nests it.
type gqlContext struct {
	TypeName   string `json:"__typename"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
	CheckSuite *struct {
		WorkflowRun *struct {
			Workflow struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
}

// pr is the node as the [PR] gh pr view decodes to — through [prView], so
// the check naming, review requests and the rest are undone in the one place
// they always were.
func (g gqlPR) pr() PR {
	v := prView{
		Number: g.Number, Title: g.Title, Body: g.Body, State: g.State, IsDraft: g.IsDraft, Author: g.Author,
		BaseRefName: g.BaseRefName, HeadRefName: g.HeadRefName, HeadRefOid: g.HeadRefOid, URL: g.URL,
		ReviewDecision: g.ReviewDecision, Mergeable: g.Mergeable, MergeStateStatus: g.MergeStateStatus,
		Additions: g.Additions, Deletions: g.Deletions, ChangedFiles: g.ChangedFiles,
	}
	for _, n := range g.LastCommit.Nodes {
		if n.Commit.StatusCheckRollup == nil {
			continue
		}
		for _, c := range n.Commit.StatusCheckRollup.Contexts.Nodes {
			roll := ghRoll{TypeName: c.TypeName, Name: c.Name, Status: c.Status, Conclusion: c.Conclusion,
				DetailsURL: c.DetailsURL, Context: c.Context, State: c.State, TargetURL: c.TargetURL}
			if c.CheckSuite != nil && c.CheckSuite.WorkflowRun != nil {
				roll.WorkflowName = c.CheckSuite.WorkflowRun.Workflow.Name
			}
			v.Rollup = append(v.Rollup, roll)
		}
	}
	for _, r := range g.Reviews.Nodes {
		v.Reviews = append(v.Reviews, struct {
			Author      ghUser    `json:"author"`
			State       string    `json:"state"`
			Body        string    `json:"body"`
			SubmittedAt time.Time `json:"submittedAt"`
		}(r))
	}
	for _, c := range g.Comments.Nodes {
		v.Comments = append(v.Comments, struct {
			Author    ghUser    `json:"author"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"createdAt"`
			URL       string    `json:"url"`
		}(c))
	}
	for _, r := range g.ReviewRequests.Nodes {
		v.ReviewRequests = append(v.ReviewRequests, struct {
			Login string `json:"login"`
			Slug  string `json:"slug"`
			Name  string `json:"name"`
		}(r.RequestedReviewer))
	}
	pr := v.pr()
	pr.Commits = g.AllCommits.TotalCount
	if g.MergedAt != nil {
		pr.MergedAt = *g.MergedAt
	}
	return pr
}
