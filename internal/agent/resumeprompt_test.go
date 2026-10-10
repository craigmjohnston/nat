package agent

import (
	"strings"
	"testing"
	"time"
)

// resumePromptContext is a resume after a review was sent back while the
// session was gone; the template walks in prompt_test.go read it.
func resumePromptContext() ResumeContext {
	return ResumeContext{
		SliceName: "Info view",
		SliceID:   "3b738308-f654-8170-8c99-eccab4463d8f",
		ProjectID: testProjectID,
		StartedAt: time.Date(2026, 10, 10, 11, 0, 0, 0, time.UTC),
		Changes:   "### Sent back, at 2026-10-10T12:00:00Z\n\nRename the column.",
	}
}

// The resume prompt names the slice, says the transcript is the agent's own,
// carries what changed since and the hand-back pinned to the project — and is
// short: the brief is already in the transcript.
func TestResumePrompt(t *testing.T) {
	c := resumePromptContext()
	text := ResumePrompt(c)
	for _, want := range []string{
		`You are continuing the slice "Info view" (slice ID ` + c.SliceID + `)`,
		"own earlier session on it, resumed",
		"without reading the brief again",
		"## Since that session started",
		"recorded this since 2026-10-10T11:00:00Z",
		"### Sent back, at 2026-10-10T12:00:00Z\n\nRename the column.\n",
		"    --project " + testProjectID,
		"    nat complete-slice " + c.SliceID + " --project " + testProjectID,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("ResumePrompt does not say %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "## Brief") || len(text) > 4000 {
		t.Errorf("ResumePrompt carries more than a resume needs (%d bytes)", len(text))
	}
}

// Nothing recorded since the session started is said so.
func TestResumePromptWithNothingSince(t *testing.T) {
	c := resumePromptContext()
	c.Changes = "  \n"
	text := ResumePrompt(c)
	if !strings.Contains(text, "Nothing has been recorded on the slice since then.") || strings.Contains(text, "recorded this since") {
		t.Errorf("ResumePrompt with nothing since:\n%s", text)
	}
}
