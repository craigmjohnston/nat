package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxSliceTitleLen is the longest a slice title may be, in runes once
// trimmed. A title is a name, not a summary: the sidebar and the breadcrumb
// truncate a long one anyway, and the list of what a slice covers belongs in
// its brief. Every write that sets a title holds to it; titles already on the
// board are never re-read against it.
const MaxSliceTitleLen = 64

// CheckSliceTitle refuses a title longer than [MaxSliceTitleLen], naming it
// and its length. The title is trimmed before it is counted, as every write
// trims it before it is filed.
func CheckSliceTitle(title string) error {
	title = strings.TrimSpace(title)
	if n := utf8.RuneCountInString(title); n > MaxSliceTitleLen {
		return fmt.Errorf("the title %q is %d characters, over the %d a slice title may be: "+
			"name the one change, and put the detail in the brief", title, n, MaxSliceTitleLen)
	}
	return nil
}
