package agent

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

var update = flag.Bool("update", false, "rewrite the golden files")

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
		"nat start-slice 3b738308-f654-8170-8c99-eccab4463d8f",
		"nat complete-slice 3b738308-f654-8170-8c99-eccab4463d8f --pr",
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
	golden(t, "plan-prompt", PlanPrompt("notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker"))
}

// The planning prompt points the agent at the planning workflow and nothing
// else: the skill, the read, and the three writes. The slice commands stay out
// of it — a planning agent that knows how to claim a slice is one prompt away
// from executing the plan instead of workshopping it — and so does Notion, for
// the same reason the slice prompt keeps quiet about it.
func TestPlanPromptRoutesEverythingThroughThePlanningCommands(t *testing.T) {
	got := PlanPrompt("notion-agent-tracker", "/Users/craig/Projects/notion-agent-tracker")
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
