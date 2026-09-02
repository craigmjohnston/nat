// Package actions is the board's launch and approve flows without the
// board: the writes and subprocesses that claiming a slice, placing its
// agent in a worktree, starting the tmux session and opening its pull
// request actually do, with no bubbletea in the mix at all — so a headless
// command can run exactly the same flow the board's own l and a keys do.
package actions
