package agent

import (
	"strings"
	"testing"
)

const testWorkspaceID = "np-3f2c9a"

func TestNewProjectPrompt(t *testing.T) {
	golden(t, "new-project-prompt", NewProjectPrompt(testWorkspaceID,
		"A CLI that mirrors a Notion database into a local SQLite cache."))
}

// The workspace id stands in for --project on the one command this session
// is ever told about — there is no project yet, so nothing here can be
// pinned to one.
func TestNewProjectPromptPinsTheWorkspace(t *testing.T) {
	got := NewProjectPrompt(testWorkspaceID, "Some request.")
	for _, want := range []string{
		"nat plan-propose --workspace " + testWorkspaceID,
		"--workspace " + testWorkspaceID,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
}

// The description rides in verbatim: the agent starts drafting rather than
// asking what the user wants to build.
func TestNewProjectPromptCarriesTheDescriptionVerbatim(t *testing.T) {
	const description = "Track flaky integration tests and page whoever broke them."
	got := NewProjectPrompt(testWorkspaceID, description)
	if !strings.Contains(got, description) {
		t.Errorf("prompt does not carry the description verbatim:\n%s", got)
	}
}

// The agent proposes; the user's Accept is what creates the project and
// files the plan. A session that ran project-create or plan-apply itself
// would skip the review the proposal is for.
func TestNewProjectPromptNeverRunsProjectCreateOrPlanApply(t *testing.T) {
	got := NewProjectPrompt(testWorkspaceID, "Some request.")
	for _, want := range []string{"Never run `nat project-create` or `nat plan-apply`", "nat plan-propose"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q", want)
		}
	}
}

// This is the one prompt with no --project at all: there is no project yet.
func TestNewProjectPromptNamesNoProject(t *testing.T) {
	got := NewProjectPrompt(testWorkspaceID, "Some request.")
	if strings.Contains(got, "--project") {
		t.Errorf("new-project prompt names --project, but there is no project yet:\n%s", got)
	}
}
