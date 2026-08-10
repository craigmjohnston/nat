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
		ProjectPageID: "3b738308-f654-811c-948d-e1fb36f71df3",
		WorkingDir:    "/Users/craig/Projects/notion-agent-tracker",
		AssigneeName:  "Craig Johnston",
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
	c.ProjectPageID = ""
	golden(t, "prompt-minimal", Prompt(c))
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
