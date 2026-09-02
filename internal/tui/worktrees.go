package tui

import (
	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// Worktrees is what the board needs of git's worktrees: where a slice's branch is
// already checked out, a worktree for it where it is not, and that worktree
// taken away again once the work has become a pull request. It is
// [actions.Worktrees], which is what a launch is actually driven through now.
type Worktrees = actions.Worktrees

// Repo is what a launch needs of git that the worktree package does not ask: the
// remote's news, and the branch the remote calls its default. It is
// [actions.Repo].
type Repo = actions.Repo

// The launch's two edges onto the outside, held as variables so the tests can
// stand in for them: the real ones drive the git binary on PATH.
var (
	newWorktrees = defaultWorktrees
	newRepo      = defaultRepo
)

// defaultWorktrees is the real git.
func defaultWorktrees() Worktrees { return worktree.New() }

// defaultRepo is the real git.
func defaultRepo() Repo { return git.New() }

// agentBranch is the branch a slice's worktree is on: the one recorded at
// hand-back where there is one, since what an agent actually pushed is what its
// worktree is checked out on, whatever the launch that cut it derived — and
// otherwise the slice's own derived branch, which is every slice nobody has
// handed back yet. It is [actions.AgentBranch], which the merge that takes a
// worktree away names it by too.
func agentBranch(s domain.Slice) string { return actions.AgentBranch(s) }
