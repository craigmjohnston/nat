// Package version answers which build of nat this is.
package version

import "runtime/debug"

// Fallback is what a build with no version information at all reports.
const Fallback = "devel"

// stamped is set at link time (-ldflags "-X .../internal/version.stamped=1.2.3")
// by local and released builds, and wins over everything else.
var stamped string

// readBuildInfo is debug.ReadBuildInfo, held as a variable so tests can stand
// in for the toolchain's answer.
var readBuildInfo = debug.ReadBuildInfo

// Version is the build's version: the ldflags stamp, else the module version
// `go install ...@version` records, else Fallback. Never empty.
func Version() string {
	if stamped != "" {
		return stamped
	}
	if info, ok := readBuildInfo(); ok {
		// A build from a checkout reports "(devel)", which is no version.
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return Fallback
}
