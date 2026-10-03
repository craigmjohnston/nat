package plugin

import (
	"testing"
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{
		10 * time.Second:     "just now",
		-time.Hour:           "just now", // a clock a little ahead
		5 * time.Minute:      "5m ago",
		3 * time.Hour:        "3h ago",
		2 * 24 * time.Hour:   "2d ago",
		15 * 24 * time.Hour:  "18 Sep",
		400 * 24 * time.Hour: "29 Aug 2025",
	} {
		if got := ago(now, now.Add(-d)); got != want {
			t.Errorf("ago(-%v) = %q, want %q", d, got, want)
		}
	}
	if got := ago(now, time.Time{}); got != "—" {
		t.Errorf("ago(zero) = %q", got)
	}
}

func TestCode(t *testing.T) {
	for _, tc := range []struct{ mention, name, want string }{
		{"native-app", "Native App", "NA"},
		{"search", "Search", "SE"},
		{"a", "", "A"},
		{"", "Developer Experience Platform Team", "DEP"},
		{"", "", ""},
		{"--", "", ""},
	} {
		if got := code(tc.mention, tc.name); got != tc.want {
			t.Errorf("code(%q, %q) = %q, want %q", tc.mention, tc.name, got, tc.want)
		}
	}
}

func TestColorEstimate(t *testing.T) {
	for _, tc := range []struct{ color, key, want string }{
		{"#4F6BD8", "red", "#4f6bd8"}, // a hex colour overrides the key
		{"", "midnight-blue", "#2c3e7a"},
		{"", "Yellow-Green", "#8fb738"},
		{"", "grey", neutral},
		{"#fff", "fuchsia", "#c2479c"}, // not #rrggbb: the key decides
		{"blue", "", neutral},
		{"", "chartreuse", neutral},
	} {
		if got := teamColor(shortcut.Group{Color: tc.color, ColorKey: tc.key}); got != tc.want {
			t.Errorf("teamColor(%q, %q) = %q, want %q", tc.color, tc.key, got, tc.want)
		}
	}
	for _, key := range []string{"red", "orange", "yellow", "yellow-green", "green", "turquoise", "sky-blue", "blue",
		"midnight-blue", "purple", "fuchsia", "pink", "brass", "slate", "gray", "grey", "black"} {
		if !hexColor.MatchString(colorKeys[key]) {
			t.Errorf("color_key %q has no hex", key)
		}
	}
	zero, one, five := int64(0), int64(1), int64(5)
	for e, want := range map[*int64]string{nil: "", &zero: "0 pts", &one: "1 pt", &five: "5 pts"} {
		if got := estimate(e); got != want {
			t.Errorf("estimate = %q, want %q", got, want)
		}
	}
	if compare(2, 1) != 1 || compare(1, 2) != -1 || compare(1, 1) != 0 {
		t.Error("compare")
	}
}

func TestWithMe(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"owner:me", "owner:craig"},
		{"owner:me is:started", "owner:craig is:started"},
		{"type:bug OWNER:Me !is:done", "type:bug owner:craig !is:done"},
		{"!owner:me -owner:me", "!owner:craig -owner:craig"},
		{"owner:dana !is:done", "owner:dana !is:done"},
		{"owner:meg owner:me-too xowner:me", "owner:meg owner:me-too xowner:me"},
		{`state:"Ready for Dev"  owner:me`, `state:"Ready for Dev"  owner:craig`},
	} {
		if got := withMe(tc.in, "craig"); got != tc.want {
			t.Errorf("withMe(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := withMe("owner:me", ""); got != "owner:me" {
		t.Errorf("no mention name: %q", got)
	}
}

func TestName(t *testing.T) {
	var a, b, c shortcut.Member
	a.ID, a.Profile.Name = "a", "Ann"
	b.ID, b.Profile.MentionName = "b", "bee"
	c.ID = "c"
	r := refs{members: []shortcut.Member{a, b, c}}
	for id, want := range map[string]string{"a": "Ann", "b": "bee", "c": "someone", "zz": "someone", "": "someone"} {
		if got := r.name(id); got != want {
			t.Errorf("name(%q) = %q, want %q", id, got, want)
		}
	}
}
