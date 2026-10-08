package agent

import (
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/mods"
)

// prepareMod materialises nat's embedded mod and answers the folder a
// launch's --plugin-dir names, or "" — launching without the mod rather than
// not at all, as [prepareStatusSink] degrades — when it cannot be written: a
// missing mod must never cost an agent its launch.
func prepareMod() string {
	dir, err := mods.Materialise()
	if err != nil {
		logging.Action("embedded mod disabled for a launch", "error", err.Error())
		return ""
	}
	return dir
}
