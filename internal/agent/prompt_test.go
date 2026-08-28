package agent

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// testProjectID is the project every test prompt is pinned to: the page ID the
// launch carried, which is what --project names on every command in a prompt.
const testProjectID = "3b738308-f654-8117-9f2e-c3b0a9b7f001"

// golden compares got against testdata/<name>.golden, rewriting it under
// -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v — rerun with -update to create it", err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func testContext() PromptContext {
	return PromptContext{
		Slice: domain.Slice{
			ID:   "3b738308-f654-8170-8c99-eccab4463d8f",
			Name: "tmux integration + agent prompt template",
			URL:  "https://app.notion.com/p/3b738308f65481708c99eccab4463d8f",
		},
		Project: config.ProjectConfig{
			Name:       "notion-agent-tracker",
			WorkingDir: "/Users/craig/Projects/notion-agent-tracker",
		},
		ProjectID:    testProjectID,
		WorkingDir:   "/Users/craig/Projects/notion-agent-tracker",
		AssigneeName: "Craig Johnston",
	}
}

func TestPrompt(t *testing.T) {
	golden(t, "prompt-default", Prompt(testContext()))
}

func TestPromptWithRepoOverride(t *testing.T) {
	c := testContext()
	c.Slice.Repo = "/Users/craig/Projects/other"
	c.WorkingDir = c.Slice.Repo
	golden(t, "prompt-repo-override", Prompt(c))
}

func TestPromptWithoutOptionalContext(t *testing.T) {
	c := testContext()
	c.Slice.URL = ""
	golden(t, "prompt-minimal", Prompt(c))
}

// The prompt is the whole of what an agent is told, so what it does not say
// matters as much as what it does: an agent that learns Notion is behind the
// commands has a second way to move a slice, and none of the guardrails the
// commands enforce apply to it.
func TestPromptRoutesEverythingThroughTheCLI(t *testing.T) {
	got := Prompt(testContext())
	for _, want := range []string{
		"nat start-slice 3b738308-f654-8170-8c99-eccab4463d8f --project " + testProjectID,
		"nat complete-slice 3b738308-f654-8170-8c99-eccab4463d8f --project " + testProjectID,
		"--branch",
		"--blocked",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not tell the agent to run %q", want)
		}
	}
	for _, unwanted := range []string{"Notion MCP", "`Status`", "`Assignee`", "`PR` property"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("prompt still tells the agent about %s rather than the commands", unwanted)
		}
	}
}

// An agent ends by handing its branch back, not by opening a pull request:
// the board's approve key is the review the hand-back is waiting for, and an
// agent that opened its own would put the work straight past it. The prompt is
// the only place a launched agent is told this, so it has to say it outright
// rather than merely leaving it out.
func TestPromptTellsTheAgentToHandTheBranchBack(t *testing.T) {
	got := Prompt(testContext())
	for _, want := range []string{
		"push the branch",
		"Do not run `gh`",
		"do not\nopen a pull request",
		"Never open or merge a pull request",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q", want)
		}
	}
	// `--pr` itself and not `--pr-description`, which is the hand-back's own
	// flag: the description the board opens the pull request with.
	if prEnding.MatchString(got) {
		t.Error("prompt still offers the agent the --pr ending")
	}
}

// prEnding matches the --pr flag alone, so the --pr-description one it prefixes
// does not read as it.
var prEnding = regexp.MustCompile(`--pr($|[^-\w])`)

func TestPromptNamesTheSlice(t *testing.T) {
	got := Prompt(testContext())
	for _, want := range []string{
		"tmux integration + agent prompt template",
		"3b738308-f654-8170-8c99-eccab4463d8f",
		"https://app.notion.com/p/3b738308f65481708c99eccab4463d8f",
		"Craig Johnston",
		"/Users/craig/Projects/notion-agent-tracker",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not mention %q", want)
		}
	}
}

func TestPlanPrompt(t *testing.T) {
	golden(t, "plan-prompt", PlanPrompt(testProjectID, "notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker", ""))
}

func TestPlanPromptWithRequest(t *testing.T) {
	golden(t, "plan-prompt-request", PlanPrompt(testProjectID, "notion-agent-tracker",
		"/Users/craig/Projects/notion-agent-tracker", "Split the reporting milestone into smaller slices."))
}

// The planning prompt points the agent at the planning workflow and nothing
// else: the skill, the read, and the three writes. The slice commands stay out
// of it — a planning agent that knows how to claim a slice is one prompt away
// from executing the plan instead of workshopping it — and so does Notion, for
// the same reason the slice prompt keeps quiet about it.
func TestPlanPromptRoutesEverythingThroughThePlanningCommands(t *testing.T) {
	got := PlanPrompt(testProjectID, "notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker", "")
	for _, want := range []string{
		"notion-agent-tracker",
		"/Users/craig/Projects/notion-agent-tracker",
		"/queue-work",
		"nat info",
		"nat plan-apply",
		"nat milestone-add",
		"nat slice-add",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plan prompt does not mention %q", want)
		}
	}
	for _, unwanted := range []string{"start-slice", "complete-slice", "next-slice", "Notion"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("plan prompt should not mention %q", unwanted)
		}
	}
}

func TestPromptFlagsARepoOverrideOnlyWhenItDiffers(t *testing.T) {
	const note = "overrides the project default"

	if got := Prompt(testContext()); strings.Contains(got, note) {
		t.Error("prompt flags an override when the working dir is the project default")
	}

	c := testContext()
	c.WorkingDir = "/Users/craig/Projects/other"
	if got := Prompt(c); !strings.Contains(got, note) {
		t.Error("prompt does not flag the working dir overriding the project default")
	}

	// With no project default configured there is nothing to override.
	c.Project.WorkingDir = ""
	if got := Prompt(c); strings.Contains(got, note) {
		t.Error("prompt flags an override with no project default set")
	}
}

// worktreeContext is the launch as it arrives with a worktree behind it: the
// session's directory is the worktree, and the checkout it was cut from is the
// project default it is measured against.
func worktreeContext() PromptContext {
	c := testContext()
	c.WorkingDir = "/Users/craig/Projects/notion-agent-tracker-worktrees/slice/tmux-integration-agent-prompt-template"
	c.Branch = "slice/tmux-integration-agent-prompt-template"
	c.Repo = "/Users/craig/Projects/notion-agent-tracker"
	return c
}

func TestPromptWithAWorktree(t *testing.T) {
	golden(t, "prompt-worktree", Prompt(worktreeContext()))
}

// A session launched into a worktree is already on the branch it hands back, so
// the prompt tells it to commit and push that branch rather than to make one —
// an agent that branched again would hand back a name the board cannot see the
// diff of, and one that switched would take the worktree off its own work.
func TestPromptTellsAWorktreeAgentToUseItsBranch(t *testing.T) {
	c := worktreeContext()
	got := Prompt(c)
	for _, want := range []string{
		"- Branch: " + c.Branch + " (the working directory is a worktree already on it)",
		"git worktree cut for this slice alone",
		"push " + c.Branch,
		"Do not\ncreate a branch of your own",
		"nat complete-slice " + c.Slice.ID + " --project " + testProjectID,
		"--branch " + c.Branch + " --summary",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"branch for the slice", "--branch <branch>"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("prompt still tells the agent to make its own branch (%q)", unwanted)
		}
	}
}

// The fallback launch is the one that was there before there were worktrees: no
// branch is named anywhere, and the agent is told to make one.
func TestPromptWithoutAWorktreeStillAsksForABranch(t *testing.T) {
	got := Prompt(testContext())
	for _, want := range []string{
		"If the work is code: branch for the slice",
		"nat complete-slice " + testContext().Slice.ID + " --project " + testProjectID,
		"--branch <branch> --summary",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q", want)
		}
	}
	if strings.Contains(got, "- Branch:") {
		t.Error("prompt names a branch for a session that has none")
	}
}

// A worktree is never the project's own directory, so measuring the override
// against it would flag every launch. The checkout it was cut from is what the
// note is about.
func TestPromptFlagsAWorktreesOverrideByItsCheckout(t *testing.T) {
	const note = "overrides the project default"

	if got := Prompt(worktreeContext()); strings.Contains(got, note) {
		t.Error("prompt flags an override for a worktree of the project's own checkout")
	}

	c := worktreeContext()
	c.Repo = "/Users/craig/Projects/other"
	if got := Prompt(c); !strings.Contains(got, note) {
		t.Error("prompt does not flag a worktree cut from another checkout")
	}
}

// testWishlist is a wishlist as the client reads it off the project page: the
// second item carries a sub-bullet, because the items go into the prompt whole.
func testWishlist() []notion.WishlistItem {
	return []notion.WishlistItem{
		{ID: "3bd38308-f654-8100-9b7a-c1f8e2d4a001", Markdown: "- Add a newline between the status bar and the key hints"},
		{ID: "3bd38308-f654-8100-9b7a-c1f8e2d4a002", Markdown: "- Selective refresh\n    - only the milestones that moved"},
	}
}

func TestWishlistPrompt(t *testing.T) {
	golden(t, "wishlist-prompt", WishlistPrompt(testProjectID, "notion-agent-tracker",
		"/Users/craig/Projects/notion-agent-tracker", testWishlist()))
}

// The wishlist is the request, so the items ride in whole and the command that
// clears them names every one of them — the IDs, not the text, because that is
// what `nat wishlist-clear` addresses.
func TestWishlistPromptCarriesTheItemsAndTheirIDs(t *testing.T) {
	got := WishlistPrompt(testProjectID, "notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker", testWishlist())
	for _, want := range []string{
		"## The request",
		"- Add a newline between the status bar and the key hints",
		"    - only the milestones that moved",
		"nat wishlist-clear 3bd38308-f654-8100-9b7a-c1f8e2d4a001 3bd38308-f654-8100-9b7a-c1f8e2d4a002",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wishlist prompt does not mention %q", want)
		}
	}
	// The planning session is the same one: the wishlist decides what it works
	// on, not what it is allowed to do.
	for _, want := range []string{"/queue-work", "nat info", "nat plan-apply"} {
		if !strings.Contains(got, want) {
			t.Errorf("wishlist prompt does not mention %q", want)
		}
	}
}

// Clearing is the last step and only ever the agent's own items: an item
// cleared before the plan is written is an idea lost, and one typed while the
// session ran belongs to nobody but the user.
func TestWishlistPromptClearsOnlyAfterThePlanIsWritten(t *testing.T) {
	got := WishlistPrompt(testProjectID, "notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker", testWishlist())
	for _, want := range []string{
		"once the plan is written, and not before",
		"Name only the items above",
		"typed while\nthis session ran",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wishlist prompt does not say %q", want)
		}
	}
}

// An empty wishlist is a plain planning session: nothing to start on, and
// nothing to clear afterwards.
func TestWishlistPromptWithNoItemsIsThePlainPlanningPrompt(t *testing.T) {
	const project, dir = "notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker"
	got := WishlistPrompt(testProjectID, project, dir, nil)
	if want := PlanPrompt(testProjectID, project, dir, ""); got != want {
		t.Errorf("prompt = %q, want the plain planning prompt %q", got, want)
	}
	if strings.Contains(got, "wishlist-clear") {
		t.Errorf("prompt tells the agent to clear a wishlist it was not given:\n%s", got)
	}
}

// natCommand matches a `nat` invocation by its subcommand, so the prose that
// merely says "the `nat` commands" is not read as one. wishlist-clear comes
// before wishlist, since the alternation is tried in order.
var natCommand = regexp.MustCompile(`\bnat (info|next-slice|start-slice|complete-slice|release-slice|milestone-add|slice-add|slice-depends|wishlist-clear|wishlist|plan-apply|project-create)\b`)

// natCommands are the invocations a prompt names: each from the command word to
// the end of its line, and on through the lines a trailing backslash continues
// it onto — which is what an agent copying the prompt would actually run.
func natCommands(text string) []string {
	lines := strings.Split(text, "\n")
	var cmds []string
	for i, line := range lines {
		loc := natCommand.FindStringIndex(line)
		if loc == nil {
			continue
		}
		cmd := line[loc[0]:]
		for j := i; j+1 < len(lines) && strings.HasSuffix(strings.TrimRight(lines[j], " "), `\`); j++ {
			cmd += " " + strings.TrimSpace(lines[j+1])
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

// A command with no --project acts on whatever project the board is on, which
// the user can switch while the session runs: an agent launched on one project
// would then claim, hand back or plan into another. Every command a prompt
// names is pinned to the project of the launch, so there is no bare one left to
// copy.
func TestEveryCommandInAPromptNamesTheProject(t *testing.T) {
	const name, dir = "notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker"
	for prompt, text := range map[string]string{
		"slice":          Prompt(testContext()),
		"slice worktree": Prompt(worktreeContext()),
		"plan":           PlanPrompt(testProjectID, name, dir, ""),
		"plan request":   PlanPrompt(testProjectID, name, dir, "Split the reporting milestone."),
		"wishlist":       WishlistPrompt(testProjectID, name, dir, testWishlist()),
	} {
		cmds := natCommands(text)
		if len(cmds) == 0 {
			t.Errorf("the %s prompt names no `nat` command at all:\n%s", prompt, text)
		}
		for _, cmd := range cmds {
			if !strings.Contains(cmd, "--project "+testProjectID) {
				t.Errorf("the %s prompt runs %q without naming the project", prompt, cmd)
			}
		}
	}
}

// The pin is worth explaining as well as applying: an agent that understands
// why puts --project on the commands it runs of its own accord, which are the
// ones no prompt can spell out for it.
func TestPromptsSayWhyTheProjectIsPinned(t *testing.T) {
	const name, dir = "notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker"
	for prompt, text := range map[string]string{
		"slice": Prompt(testContext()),
		"plan":  PlanPrompt(testProjectID, name, dir, ""),
	} {
		for _, want := range []string{
			"whatever project the user's board is on",
			"    --project " + testProjectID,
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the %s prompt does not say %q", prompt, want)
			}
		}
	}
}
