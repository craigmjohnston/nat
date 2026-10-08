package mods

import (
	"io/fs"
	"testing"
)

// Only the files Claude Code loads are embedded, at the mod's root.
func TestEmbedded(t *testing.T) {
	var got []string
	_ = fs.WalkDir(Embedded(), ".", func(p string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			got = append(got, p)
		}
		return err
	})
	want := []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/register.ts", "types/index.d.ts"}
	if len(got) != len(want) {
		t.Fatalf("embedded = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("embedded = %v, want %v", got, want)
		}
	}
}
