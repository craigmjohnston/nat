package agent

import (
	"strings"

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

// sweepMods removes the materialised mods no live session loaded
// ([mods.Sweep]), read off the start command of every pane on the server —
// a launch's carries its --plugin-dir. A failed read removes nothing: a
// folder a live session loaded must never go, and an unread server could be
// holding one.
func (t *Tmux) sweepMods() {
	out, err := t.run("list-panes", "-a", "-F", "#{pane_start_command}")
	if err != nil {
		logging.Action("old embedded mods not swept", "error", err.Error())
		return
	}
	mods.Sweep(strings.Split(out, "\n"))
}
