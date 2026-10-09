package domain

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckBriefOpening(t *testing.T) {
	words := func(n int) string { return strings.TrimSpace(strings.Repeat("word ", n)) }
	for _, tc := range []struct {
		name, brief string
		refused     int // the count the refusal names, 0 for none
	}{
		{"empty", "", 0},
		{"one short paragraph", "Make the sidebar show every project.", 0},
		{"exactly the cap", words(MaxBriefOpeningWords), 0},
		{"a long first paragraph", words(61), 61},
		{"a long one wrapped over lines", words(30) + "\n" + words(31) + "\n\nDetail.", 61},
		{"a short first paragraph before a long one", "One sentence.\n\n" + words(200), 0},
		{"leading blank lines ignored", "\n  \n\r\n" + words(61), 61},
		{"a blank line of spaces ends it", "One sentence.\n   \n" + words(200), 0},
	} {
		err := CheckBriefOpening(tc.brief)
		if tc.refused == 0 {
			if err != nil {
				t.Errorf("%s: refused: %v", tc.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: allowed", tc.name)
			continue
		}
		for _, want := range []string{fmt.Sprintf("first paragraph is %d words", tc.refused), "over the 60",
			"in a paragraph of their own"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: refusal %q does not say %q", tc.name, err, want)
			}
		}
	}
}

func TestBriefOpening(t *testing.T) {
	if got := BriefOpening("\n\nFirst line\nsecond line\n\nDetail."); got != "First line\nsecond line" {
		t.Errorf("BriefOpening = %q", got)
	}
}
