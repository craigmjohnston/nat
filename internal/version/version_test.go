package version

import (
	"runtime/debug"
	"testing"
)

func TestVersion(t *testing.T) {
	cases := []struct {
		name  string
		stamp string
		info  *debug.BuildInfo
		want  string
	}{
		{"stamp wins", "1.2.3", &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, "1.2.3"},
		{"module version", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.4.0"}}, "v0.4.0"},
		{"checkout build", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, Fallback},
		{"empty module version", "", &debug.BuildInfo{}, Fallback},
		{"no build info", "", nil, Fallback},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			oldStamp, oldRead := stamped, readBuildInfo
			t.Cleanup(func() { stamped, readBuildInfo = oldStamp, oldRead })
			stamped = c.stamp
			readBuildInfo = func() (*debug.BuildInfo, bool) { return c.info, c.info != nil }
			if got := Version(); got != c.want {
				t.Fatalf("Version() = %q, want %q", got, c.want)
			}
		})
	}
}
