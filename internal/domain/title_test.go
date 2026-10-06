package domain

import (
	"strings"
	"testing"
)

func TestCheckSliceTitleCapsAt64Runes(t *testing.T) {
	if err := CheckSliceTitle(strings.Repeat("a", MaxSliceTitleLen)); err != nil {
		t.Errorf("a 64-character title: %v", err)
	}
	// Surrounding space is trimmed before counting, as every write trims it.
	if err := CheckSliceTitle("  " + strings.Repeat("a", MaxSliceTitleLen) + "\n"); err != nil {
		t.Errorf("a padded 64-character title: %v", err)
	}
	// Runes, not bytes: 64 two-byte runes are 128 bytes and still allowed.
	if err := CheckSliceTitle(strings.Repeat("é", MaxSliceTitleLen)); err != nil {
		t.Errorf("a 64-rune multibyte title: %v", err)
	}
	long := strings.Repeat("é", MaxSliceTitleLen+1)
	err := CheckSliceTitle(long)
	if err == nil {
		t.Fatal("a 65-rune title was allowed")
	}
	for _, want := range []string{long, "is 65 characters", "over the 64", "put the detail in the brief"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not say %q", err, want)
		}
	}
}
