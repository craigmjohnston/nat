package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// StorageReader reads the signed-in user's GitHub artifact storage — a
// [gh.CLI] in production.
type StorageReader interface {
	ArtifactStorage(now time.Time) (gh.StorageReading, error)
}

// newStorageReader is the gh storage-usage reads through: one keeping no
// budget, since these are two REST reads made when the user opens a page, not
// the GraphQL polling the budget throttles — and a refusal of them must not
// pause that polling either. Replaced in tests.
var newStorageReader = func() StorageReader { return gh.NewWithRunner(gh.ExecRunner{}) }

// storageNow is the clock the month and its days left are read off;
// time.Now in production.
var storageNow = time.Now

// storageUsage prints this month's GitHub artifact storage by project:
// GitHub's billing report for the signed-in user, each repository it lists
// given to the project that claims it — a project's working directory's
// origin, a source project's tasks' repositories — and the rest left as
// other repositories. It takes no --project: the reading is the account's.
func storageUsage(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("storage-usage", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("storage-usage: takes no arguments, given %d", len(rest))
	}
	cfg, _, err := env.Load()
	if err != nil {
		return err
	}

	now := storageNow()
	reading, err := newStorageReader().ArtifactStorage(now)
	if errors.Is(err, gh.ErrBillingScope) && *asJSON {
		// Not a failure to gnat but a choice the user has yet to make: it
		// draws the command to run, and the section's footing, from this.
		return writeJSON(env.Out, storageScopeJSON{NeedsScope: "user", ScopeCommand: gh.ScopeCommand})
	}
	if err != nil {
		return fmt.Errorf("read GitHub's artifact storage: %w", err)
	}
	doc := storageDocOf(reading, storageProjects(ctx, env, cfg), now)
	if *asJSON {
		return writeJSON(env.Out, doc)
	}
	return writeStorageText(env.Out, doc)
}

// storageScopeJSON is storage-usage's JSON where gh lacks the "user" scope:
// which scope, and the command that grants it.
type storageScopeJSON struct {
	NeedsScope   string `json:"needs_scope"`
	ScopeCommand string `json:"scope_command"`
}

// storageProject is one project and the repositories it claims.
type storageProject struct {
	id, name, color string
	repos           []string
}

// storageProjects is every tracked project with the GitHub repositories it
// claims, in name order (then ID), which is also the order a repository two
// projects claim goes to the first of. A directory with no GitHub origin, or
// one git cannot read, claims nothing.
func storageProjects(ctx context.Context, env Env, cfg config.Config) []storageProject {
	git := env.NewGit()
	repoOf := map[string]string{}
	remote := func(dir string) string {
		dir = actions.ExpandHome(strings.TrimSpace(dir))
		if dir == "" {
			return ""
		}
		if r, ok := repoOf[dir]; ok {
			return r
		}
		url, err := git.RemoteURL(dir)
		r := ""
		if err == nil {
			if owner, name, ok := gh.ParseRemote(url); ok {
				r = owner + "/" + name
			}
		}
		repoOf[dir] = r
		return r
	}

	var out []storageProject
	for id, p := range cfg.Projects {
		sp := storageProject{id: id, name: p.Name, color: p.Color}
		dirs := []string{p.WorkingDir}
		if p.IsSource() {
			sp.name = sourceProjectName(ctx, env, p.Source)
			dirs = append(dirs, sourceRepos(ctx, id, p)...)
		}
		seen := map[string]bool{}
		for _, d := range dirs {
			if r := remote(d); r != "" && !seen[r] {
				seen[r] = true
				sp.repos = append(sp.repos, r)
			}
		}
		sort.Strings(sp.repos)
		out = append(out, sp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].id < out[j].id
	})
	return out
}

// sourceRepos is every repository a source project's tasks record — read off
// its plan file alone, never its plugin. A plan that cannot be read claims
// nothing, logged.
func sourceRepos(ctx context.Context, id string, p config.ProjectConfig) []string {
	sp := storeProject(id, p)
	local, err := store.OpenProject(sp)
	if err != nil {
		logging.Error("storage-usage could not open a source project's plan", "project", id, "err", err)
		return nil
	}
	defer func() { _ = local.Close() }()
	plan, err := local.Plan(ctx, sp)
	if err != nil {
		logging.Error("storage-usage could not read a source project's plan", "project", id, "err", err)
		return nil
	}
	var dirs []string
	for _, s := range plan.Project.Slices {
		if s.Repo != "" {
			dirs = append(dirs, s.Repo)
		}
	}
	return dirs
}

// storageDoc is storage-usage's JSON: the month, the plan's allowance, the
// total, each project's share and every repository no project claims.
type storageDoc struct {
	Login       string               `json:"login"`
	Plan        string               `json:"plan"`
	AllowanceGB float64              `json:"allowance_gb"`
	Year        int                  `json:"year"`
	Month       int                  `json:"month"`
	DaysLeft    int                  `json:"days_left"`
	TotalGB     float64              `json:"total_gb"`
	Projects    []storageProjectJSON `json:"projects"`
	Other       storageOtherJSON     `json:"other"`
}

// storageProjectJSON is one project's share: every project is listed, one
// with no repository or no storage at zero.
type storageProjectJSON struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Color string   `json:"color,omitempty"`
	Repos []string `json:"repos"`
	GB    float64  `json:"gb"`
}

// storageOtherJSON is every repository GitHub listed that no project claims.
type storageOtherJSON struct {
	GB    float64           `json:"gb"`
	Repos []storageRepoJSON `json:"repos"`
}

// storageRepoJSON is one unclaimed repository's storage.
type storageRepoJSON struct {
	Repo string  `json:"repo"`
	GB   float64 `json:"gb"`
}

// storageDocOf shares the reading out between the projects: each repository
// to the first project claiming it, the rest to Other. Projects are listed
// largest first (then in name order), Other's repositories the same.
func storageDocOf(r gh.StorageReading, projects []storageProject, now time.Time) storageDoc {
	now = now.UTC()
	doc := storageDoc{
		Login: r.Login, Plan: r.Plan, AllowanceGB: r.AllowanceGB,
		Year: now.Year(), Month: int(now.Month()), DaysLeft: daysLeftInMonth(now),
		Projects: []storageProjectJSON{}, Other: storageOtherJSON{Repos: []storageRepoJSON{}},
	}
	claimed := map[string]bool{}
	for _, p := range projects {
		pj := storageProjectJSON{ID: p.id, Name: p.name, Color: p.color, Repos: []string{}}
		for _, repo := range p.repos {
			pj.Repos = append(pj.Repos, repo)
			if claimed[repo] {
				continue
			}
			claimed[repo] = true
			pj.GB += r.Repos[repo]
		}
		doc.Projects = append(doc.Projects, pj)
	}
	for repo, gb := range r.Repos {
		doc.TotalGB += gb
		if !claimed[repo] {
			doc.Other.GB += gb
			doc.Other.Repos = append(doc.Other.Repos, storageRepoJSON{Repo: repo, GB: gb})
		}
	}
	sort.SliceStable(doc.Projects, func(i, j int) bool { return doc.Projects[i].GB > doc.Projects[j].GB })
	sort.Slice(doc.Other.Repos, func(i, j int) bool {
		a, b := doc.Other.Repos[i], doc.Other.Repos[j]
		if a.GB != b.GB {
			return a.GB > b.GB
		}
		return a.Repo < b.Repo
	})
	return doc
}

// daysLeftInMonth is the days until the (UTC) month GitHub bills by turns
// over, today counted while any of it is left.
func daysLeftInMonth(now time.Time) int {
	next := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	return int(math.Ceil(next.Sub(now).Hours() / 24))
}

// writeStorageText is storage-usage's plain output: the total against the
// allowance, then a line per project and one for other repositories.
func writeStorageText(out io.Writer, doc storageDoc) error {
	var b strings.Builder
	allowance := "no known allowance"
	if doc.AllowanceGB > 0 {
		allowance = "of " + gh.FormatGB(doc.AllowanceGB) + " GB"
	}
	fmt.Fprintf(&b, "Artifact storage %d-%02d: %s GB %s · %d days left\n",
		doc.Year, doc.Month, gh.FormatGB(doc.TotalGB), allowance, doc.DaysLeft)
	for _, p := range doc.Projects {
		fmt.Fprintf(&b, "  %s  %s GB\n", p.Name, gh.FormatGB(p.GB))
	}
	fmt.Fprintf(&b, "  Other repositories  %s GB\n", gh.FormatGB(doc.Other.GB))
	_, err := io.WriteString(out, b.String())
	return err
}
