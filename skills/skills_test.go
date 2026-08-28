package skills

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// The skills are named one by one in the embed directive, so what the binary
// carries is worth stating: a skill left out of it would otherwise be missing
// from every install without anything failing.
func TestFSCarriesEverySkill(t *testing.T) {
	entries, err := fs.ReadDir(FS(), ".")
	if err != nil {
		t.Fatalf("read the embedded skills: %v", err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("%s is not a skill directory", e.Name())
			continue
		}
		got[e.Name()] = true
		if _, err := fs.ReadFile(FS(), e.Name()+"/SKILL.md"); err != nil {
			t.Errorf("%s has no SKILL.md: %v", e.Name(), err)
		}
	}
	for _, want := range []string{"next-slice", "queue-project", "queue-work"} {
		if !got[want] {
			t.Errorf("the %s skill is not embedded", want)
		}
	}
}

// The skills reach the tracker through the CLI and nothing else, so that an
// agent needs no Notion access of its own and cannot move a slice in a way the
// commands would have refused. A skill that talked to Notion directly would
// route around every guardrail the commands enforce, and would do it quietly.
func TestSkillsWriteThroughTheCLI(t *testing.T) {
	for skill, want := range map[string][]string{
		"next-slice":    {"nat next-slice", "nat start-slice", "nat complete-slice"},
		"queue-project": {"nat info", "nat project-create", "nat plan-apply --project"},
		"queue-work":    {"nat info", "nat plan-apply"},
	} {
		body, err := fs.ReadFile(FS(), skill+"/SKILL.md")
		if err != nil {
			t.Errorf("read the %s skill: %v", skill, err)
			continue
		}
		text := string(body)
		for _, command := range want {
			if !strings.Contains(text, command) {
				t.Errorf("the %s skill does not use `%s`", skill, command)
			}
		}
		if strings.Contains(text, "Notion MCP") {
			t.Errorf("the %s skill still writes to Notion directly", skill)
		}
	}
}

// The next-slice skill ships inside the binary, so an ending it names wrongly
// cannot be corrected where it is read. A slice ends handed back on a pushed
// branch — the board's approve key is what turns that into a pull request —
// and a skill that still told the agent to open one would route the work past
// the review the hand-back exists for.
func TestNextSliceHandsTheBranchBack(t *testing.T) {
	body, err := fs.ReadFile(FS(), "next-slice/SKILL.md")
	if err != nil {
		t.Fatalf("read the next-slice skill: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"nat complete-slice <slice> --project <project> --branch <branch>",
		"push the branch",
		"do not open a pull request",
		"Never open or merge a pull request",
		"never push to main",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the next-slice skill does not say %q", want)
		}
	}
	// `--pr` itself and not `--pr-description`, which is the hand-back's own
	// flag: the description the board opens the pull request with.
	if prEnding.MatchString(text) {
		t.Error("the next-slice skill still offers the agent the --pr ending")
	}
}

// prEnding matches the --pr flag alone, so the --pr-description one it prefixes
// does not read as it.
var prEnding = regexp.MustCompile(`--pr($|[^-\w])`)

// A slice launched from the board is placed in a worktree of its own, and a
// session started from the skill has to arrive at the same branch in the same
// way — otherwise the same slice hands back one branch from the board and
// another from the terminal. The command and the branch rule are what the
// agent acts on, so they are worth naming here; the fallbacks are too, since a
// machine with no worktrunk is one that would otherwise do nothing at all.
func TestNextSliceCutsTheSlicesWorktree(t *testing.T) {
	body, err := fs.ReadFile(FS(), "next-slice/SKILL.md")
	if err != nil {
		t.Fatalf("read the next-slice skill: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"wt switch --create slice/<slug> --no-cd",
		"`slice/` followed by the name lowercased",
		"If `wt` is not installed, or the working directory is not a git repository",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the next-slice skill does not say %q", want)
		}
	}
}

// A project keeps its whole plan on one page: a milestone is an option of the
// slices' Milestone column, with no status of its own and no page to name it
// by. The skills ship inside the binary, so an instruction that assumes
// otherwise cannot be corrected where it is read — an agent told to have the
// user activate a milestone would be asking for something the board refuses.
func TestSkillsDoNotAssumeMilestonePages(t *testing.T) {
	for _, skill := range []string{"next-slice", "queue-project", "queue-work"} {
		body, err := fs.ReadFile(FS(), skill+"/SKILL.md")
		if err != nil {
			t.Errorf("read the %s skill: %v", skill, err)
			continue
		}
		text := strings.ToLower(string(body))
		for phrase, why := range map[string]string{
			"activat":          "a milestone has no status to activate",
			"active milestone": "a milestone has no status of its own",
			"milestone page":   "a milestone has no page",
			"url or id":        "a milestone is named by name alone",
		} {
			if strings.Contains(text, phrase) {
				t.Errorf("the %s skill says %q: %s", skill, phrase, why)
			}
		}
	}
}

// The queue-project skill drives two commands and the order is the whole of it:
// `plan-apply` run before the project exists, or run without the `--project` the
// create printed, files a whole plan into whichever project happens to be
// active — the one thing here that cannot be undone by a second run. The skill
// ships inside the binary, so an order it stated wrongly cannot be corrected
// where it is read.
func TestQueueProjectCreatesBeforeItFilesThePlan(t *testing.T) {
	body, err := fs.ReadFile(FS(), "queue-project/SKILL.md")
	if err != nil {
		t.Fatalf("read the queue-project skill: %v", err)
	}
	text := string(body)
	create := strings.Index(text, "nat project-create")
	apply := strings.Index(text, "nat plan-apply --project")
	if create < 0 || apply < 0 {
		t.Fatalf("the queue-project skill does not run both commands: create at %d, apply at %d", create, apply)
	}
	if create > apply {
		t.Error("the queue-project skill files the plan before it creates the project")
	}
	if !strings.Contains(text, "--description -") {
		t.Error("the queue-project skill does not write the project's brief onto its page")
	}
}

// Nothing is written before the user has approved it, and a project is the one
// thing here with no way back: `plan-apply` only ever creates, but a project
// created by mistake stays created, and no `nat` command deletes one.
func TestQueueProjectWritesNothingBeforeApproval(t *testing.T) {
	body, err := fs.ReadFile(FS(), "queue-project/SKILL.md")
	if err != nil {
		t.Fatalf("read the queue-project skill: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"Write nothing until the user explicitly approves",
		"There is no `nat` command that deletes a project",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the queue-project skill does not say %q", want)
		}
	}
}

// A created project is not the active one — `project-create` leaves the board
// on whatever it was on, and the headless commands have no project switch at
// all — so the last step belongs to the user and the skill has to hand it over.
// A skill that claimed otherwise would leave the user's next `nat next-slice`
// silently pointed at the old project.
func TestQueueProjectSendsTheUserToTheSwitchPicker(t *testing.T) {
	body, err := fs.ReadFile(FS(), "queue-project/SKILL.md")
	if err != nil {
		t.Fatalf("read the queue-project skill: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "not the active one") || !strings.Contains(text, "press") {
		t.Errorf("the queue-project skill does not send the user to the board to switch to the new project")
	}
	if strings.Contains(strings.ToLower(text), "nat switch") {
		t.Error("the queue-project skill invents a CLI project switch")
	}
}

// natCommand matches a `nat` invocation by its subcommand, so the prose that
// describes what a command does is not read as a call to it. wishlist-clear
// comes before wishlist, since the alternation is tried in order.
var natCommand = regexp.MustCompile(`\bnat (info|next-slice|start-slice|complete-slice|release-slice|milestone-add|slice-add|slice-depends|wishlist-clear|wishlist|plan-apply|project-create)\b`)

// fencedNatCommands are the `nat` invocations inside a skill's fenced code
// blocks: the lines an agent copies and runs, as against the backticked prose
// around them, which as often as not is describing a command rather than
// calling it. A line continued with a trailing backslash is read whole.
func fencedNatCommands(text string) []string {
	lines := strings.Split(text, "\n")
	var cmds []string
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		loc := natCommand.FindStringIndex(line)
		if !fenced || loc == nil {
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

// A command with no --project acts on whichever project the board is on, which
// the user switches while a session runs: a skill that ran one bare would claim,
// hand back or file a whole plan somewhere it never read. Every command a skill
// spells out for copying names its project, bar the two that cannot — the one
// read that finds the ID in the first place, and `project-create`, which is what
// makes the project there is no ID for yet.
func TestSkillCommandsNameTheProject(t *testing.T) {
	for _, skill := range []string{"next-slice", "queue-project", "queue-work"} {
		body, err := fs.ReadFile(FS(), skill+"/SKILL.md")
		if err != nil {
			t.Errorf("read the %s skill: %v", skill, err)
			continue
		}
		for _, cmd := range fencedNatCommands(string(body)) {
			if strings.HasPrefix(cmd, "nat project-create") || cmd == "nat info --json" {
				continue
			}
			if !strings.Contains(cmd, "--project ") {
				t.Errorf("the %s skill runs %q without naming the project", skill, cmd)
			}
		}
	}
}

// The commands a skill names in prose are run just as the fenced ones are, and
// the pin is worth explaining as well as applying: an agent that knows why puts
// --project on the calls it makes of its own accord.
func TestSkillsPinTheProjectTheyWereGiven(t *testing.T) {
	for skill, want := range map[string][]string{
		"next-slice": {
			"nat next-slice --project <project>",
			"nat start-slice <URL|ID> --project <project>",
			"nat info --json",
			"whichever project the user's board is on",
		},
		"queue-work": {
			"nat info --project <project>",
			"nat plan-apply --project <project>",
			"nat wishlist --project <project>",
			"nat wishlist-clear <block-id>... --project <project>",
			"nat info --json",
			"whichever project the user's board is on",
		},
		"queue-project": {
			"nat plan-apply --project <the id project-create printed>",
			"whichever project the user's board is on",
		},
	} {
		body, err := fs.ReadFile(FS(), skill+"/SKILL.md")
		if err != nil {
			t.Errorf("read the %s skill: %v", skill, err)
			continue
		}
		for _, command := range want {
			if !strings.Contains(string(body), command) {
				t.Errorf("the %s skill does not say %q", skill, command)
			}
		}
	}
}
