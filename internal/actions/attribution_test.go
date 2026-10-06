package actions

import "testing"

func TestStripAgentAttribution(t *testing.T) {
	const footer = "🤖 Generated with [Claude Code](https://claude.com/claude-code)"
	const link = "https://claude.ai/code/session_01FkS7nGtDzMZ9eFX6pxXK6L"
	for _, tc := range []struct{ name, in, want string }{
		{"neither", "Title\n\nBody line.\n\nMore.", "Title\n\nBody line.\n\nMore."},
		{"neither keeps a trailing newline", "Title\n\nBody.\n", "Title\n\nBody.\n"},
		{"footer alone", "Title\n\nBody.\n\n" + footer, "Title\n\nBody."},
		{"link alone", "Title\n\nBody.\n\n" + link + "\n", "Title\n\nBody."},
		{"both", "Title\n\nBody.\n\n" + footer + "\n\n" + link + "\n", "Title\n\nBody."},
		{"both on adjacent lines", "Title\nBody.\n" + footer + "\n" + link, "Title\nBody."},
		{"footer in the middle", "Title\n\nFirst.\n\n" + footer + "\n\nSecond.", "Title\n\nFirst.\n\nSecond."},
		{"footer in the middle with no blanks", "First.\n" + footer + "\nSecond.", "First.\nSecond."},
		{"footer first", footer + "\n\nTitle", "Title"},
		{"another emoji and link text", "Body.\n\n✨ generated WITH claude code", "Body."},
		{"a plain line of it", "Body.\n\nGenerated with Claude Code", "Body."},
		{"a link inside a line", "Body.\n\nClaude-Session: " + link, "Body."},
		{"generated with something else", "Body.\n\nGenerated with care", "Body.\n\nGenerated with care"},
		{"claude code alone", "Body mentions Claude Code.", "Body mentions Claude Code."},
		{"a non-session claude link", "See https://claude.ai/code for more.", "See https://claude.ai/code for more."},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripAgentAttribution(tc.in); got != tc.want {
				t.Errorf("StripAgentAttribution(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
