package domain

import (
	"fmt"
	"strings"
)

// MaxBriefOpeningWords is the most words a brief's first paragraph may run
// to. That paragraph is the brief's summary — what changes for the user and
// why, in a sentence or two — and gnat shows it alone as the task's
// description, so it has to be short enough to be one. Every write where an
// agent sets a brief holds to it; briefs already on the board, and the user's
// own from gnat's sheets, are never read against it.
const MaxBriefOpeningWords = 60

// CheckBriefOpening refuses a brief whose first paragraph — the first run of
// non-blank lines, leading blank lines skipped — is longer than
// [MaxBriefOpeningWords], counted by [strings.Fields]. Only the count is
// checked, so a brief that opens with a command is not refused for it. An
// empty brief passes.
func CheckBriefOpening(brief string) error {
	if n := len(strings.Fields(BriefOpening(brief))); n > MaxBriefOpeningWords {
		return fmt.Errorf("the brief's first paragraph is %d words, over the %d a summary may be: "+
			"open with one or two sentences saying what changes for the user, in a paragraph of "+
			"their own, before the detail", n, MaxBriefOpeningWords)
	}
	return nil
}

// BriefOpening is a brief's first paragraph: its lines from the first
// non-blank one up to the next blank one, joined as written. Empty for an
// empty brief.
func BriefOpening(brief string) string {
	var para []string
	for _, line := range strings.Split(strings.ReplaceAll(brief, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, line)
	}
	return strings.Join(para, "\n")
}
