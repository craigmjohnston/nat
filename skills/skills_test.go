package skills

import (
	"io/fs"
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
	for _, want := range []string{"next-slice", "queue-work"} {
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
		"next-slice": {"nat next-slice", "nat start-slice", "nat complete-slice"},
		"queue-work": {"nat info", "nat plan-apply"},
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
		"nat complete-slice <slice> --branch <branch>",
		"push the branch",
		"do not open a pull request",
		"Never open or merge a pull request",
		"never push to main",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the next-slice skill does not say %q", want)
		}
	}
	if strings.Contains(text, "--pr") {
		t.Error("the next-slice skill still offers the agent the --pr ending")
	}
}

// A project keeps its whole plan on one page: a milestone is an option of the
// slices' Milestone column, with no status of its own and no page to name it
// by. The skills ship inside the binary, so an instruction that assumes
// otherwise cannot be corrected where it is read — an agent told to have the
// user activate a milestone would be asking for something the board refuses.
func TestSkillsDoNotAssumeMilestonePages(t *testing.T) {
	for _, skill := range []string{"next-slice", "queue-work"} {
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
