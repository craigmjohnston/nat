package agent

import "testing"

// Each prompt kind opens on one line naming what to do and where the brief
// is; a session picking work back up is told to continue it.
func TestOpeningLines(t *testing.T) {
	for name, tt := range map[string]struct{ got, want string }{
		"slice": {OpeningLine(testContext()),
			`Work the slice "tmux integration + agent prompt template": your brief is the natBrief block of this message.`},
		"relaunch": {OpeningLine(resumeContext()),
			`Continue the slice "tmux integration + agent prompt template": your brief is the natBrief block of this message.`},
		"plan": {PlanOpeningLine(),
			"Workshop the plan with the user: your brief is the natBrief block of this message."},
		"new project": {NewProjectOpeningLine(),
			"Workshop a new project with the user: your brief is the natBrief block of this message."},
	} {
		if tt.got != tt.want {
			t.Errorf("%s: opening line = %q, want %q", name, tt.got, tt.want)
		}
	}
}
