package actions

import "strings"

// StripAgentAttribution drops the footer Claude Code appends to what an agent
// writes for a pull request — its "Generated with Claude Code" line, whatever
// emoji or link it wears, and a bare link to the session under
// claude.ai/code/session — neither of which belongs on a pull request nat
// opens. Each such line goes whole, and the blank lines around it collapse to
// at most one between what is kept, none at either end; nothing else in the
// text is touched.
func StripAgentAttribution(description string) string {
	var kept []string
	dropped, blankDropped := false, false
	for _, line := range strings.Split(description, "\n") {
		blank := strings.TrimSpace(line) == ""
		switch {
		case isAgentAttribution(line):
			for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
				kept = kept[:len(kept)-1]
				blankDropped = true
			}
			dropped = true
		case blank && dropped:
			blankDropped = true
		default:
			if dropped && blankDropped && len(kept) > 0 {
				kept = append(kept, "")
			}
			kept = append(kept, line)
			dropped, blankDropped = false, false
		}
	}
	return strings.Join(kept, "\n")
}

// isAgentAttribution is one line of that footer: the generated-with line or
// the session link.
func isAgentAttribution(line string) bool {
	l := strings.ToLower(line)
	return strings.Contains(l, "claude.ai/code/session") ||
		strings.Contains(l, "generated with") && strings.Contains(l, "claude code")
}
