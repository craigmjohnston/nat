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
