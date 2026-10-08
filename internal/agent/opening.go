package agent

// briefBlock is the context block the embedded mod hands a session its brief
// in (`mods/embedded/hooks/register.ts`, `prompt.context`): what every opening
// line points the agent at.
const briefBlock = "natBrief"

// briefPointer ends every opening line: where the brief the line stands in for
// is.
const briefPointer = ": your brief is the " + briefBlock + " block of this message."

// OpeningLine is the one line a slice session is started with where the
// embedded mod carries its brief ([Prompt]) as hidden context: the pane draws
// this and nothing else of the brief. A session [Resuming] work is told to
// continue it, as its brief tells it.
func OpeningLine(c PromptContext) string {
	verb := "Work"
	if Resuming(c) {
		verb = "Continue"
	}
	return verb + ` the slice "` + c.Slice.Name + `"` + briefPointer
}

// PlanOpeningLine is [OpeningLine] for a planning session ([PlanPrompt]).
func PlanOpeningLine() string {
	return "Workshop the plan with the user" + briefPointer
}

// NewProjectOpeningLine is [OpeningLine] for a new-project session
// ([NewProjectPrompt]).
func NewProjectOpeningLine() string {
	return "Workshop a new project with the user" + briefPointer
}
