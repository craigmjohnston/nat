package agent

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// PromptContext is everything a fresh agent session needs to be told about the
// slice it is picking up. WorkingDir is the directory the agent will start in,
// resolved by the caller — the slice's Repo override, the project default, or
// whatever the launch form was edited to, and the worktree cut from any of them
// where there is one.
//
// Branch and Repo describe that worktree: the branch it is already on, and the
// checkout it was cut from. Both are empty where the session runs in the
// checkout itself, which is what the agent is told to branch for itself.
//
// ProjectID is the project's own page ID, which is what every `nat` command in
// the prompt names with --project. It comes from the caller rather than from
// Project, since a ProjectConfig is a value of the config file's Projects map
// and the ID is the key it is filed under.
//
// Fix says the session is not working the slice but the review of the pull
// request it already produced: the slice has a pull request recorded, that
// pull request is still open, and the agent is being sent at the comments and
// the failing checks on it. It is the launch's own word rather than something read back off the
// slice, since it is the launch that established the pull request is open —
// see [fixPrompt].
//
// Brief and Conventions are the slice's own page body and the project's
// conventions, read by the launch after the claim succeeds and written
// straight into the prompt — the same document `nat start-slice` prints, so
// the agent needs no command of its own to see it. Both are empty for a fix
// launch, which reads neither.
//
// Milestone and MilestoneSlices are the raw material a launch renders
// MilestoneDigest from: the slice's own milestone and every sibling slice
// under it, excluding this one, in plan order. They come from the plan
// already in the caller's hand — the board's own copy, or the one a headless
// launch just read to find them — rather than from a further read of it, so
// a caller that has no plan in hand simply leaves both unset.
//
// MilestoneDigest is [MilestoneDigest] already rendered from them, plus a
// page fetch per Done sibling for its hand-back summary — the settled state
// of the slice's own milestone, handed over so the agent does not have to go
// and read it with `nat info` itself. Empty for a fix launch, and for a slice
// filed under no milestone.
//
// Frontend says which surface launched the session — see [Frontend] — so the
// prompt's user-facing guidance about picking up changes and approving or
// merging work names the right one. The zero value is unspecified, which
// reads exactly as every template did before this field existed.
//
// GitBase, GitLog and GitDiffStat are a resume or fix launch's read of the
// worktree, taken by [actions.Launch] right after PlaceAgent resolves it:
// the base the branch is measured against, `git log --oneline <base>..HEAD`
// and `git diff --stat <base>...HEAD` — separating an earlier session's
// commits from whatever the base has moved on by since, and naming the files
// already touched, neither of which Claude Code's own injected git status
// snapshot carries. A first-time launch never gathers them — there is
// nothing yet on the branch worth reading — and a gather that fails leaves
// whichever of GitLog/GitDiffStat failed empty, which is what tells [Prompt]
// and [fixPrompt] to leave the section out rather than print half of it.
//
// ReviewComments and ReviewChecks are a fix launch's read of the pull
// request's review, taken the same way: `gh pr view <url> --comments` and
// `gh pr checks <url>` — the first the one `gh` read [fixPrompt] lets the
// agent run again itself, the second re-read with `nat slice-checks`, the way
// every agent reads CI. Each is independently left empty on a failed read,
// the project's usual reads-conclude-nothing posture — a launch never fails
// over missing context.
//
// Container is the container a source project's slice hangs off, read off the
// plugin at launch by [actions.Launch]; nil for every other project, for a fix
// launch, and where the read failed — the prompt then simply has no section
// for it.
//
// RepoUnknown says the slice has no repository to work in yet: a source
// project, which has no working directory of its own, on a task none has been
// recorded for. The session starts in the home directory (WorkingDir), with no
// worktree, and the prompt sends it to work the repository out from the
// container, record it with `nat slice-repo`, and cut the worktree itself —
// see [repoPassage].
type PromptContext struct {
	Slice           domain.Slice
	Project         config.ProjectConfig
	ProjectID       string
	WorkingDir      string
	Branch          string
	Repo            string
	AssigneeName    string
	Fix             bool
	Brief           string
	Conventions     string
	Milestone       domain.Milestone
	MilestoneSlices []domain.Slice
	MilestoneDigest string
	Frontend        Frontend
	GitBase         string
	GitLog          string
	GitDiffStat     string
	ReviewComments  string
	ReviewChecks    string
	Container       *PromptContainer
	RepoUnknown     bool
}

// PromptContainer is the container a task hangs off in a source project — the
// card, story or ticket in the other tracker — as the agent is told of it.
type PromptContainer struct {
	Noun        string // the plugin's container_noun ("card"); "container" when empty
	Title       string
	ExternalURL string
	Prose       string // the container's prose sections, Markdown, in order
}

// containerSection tells the agent about the container its slice hangs off,
// as context only: the brief is the work, and the other tracker is nat's to
// keep in step, so the agent is told to leave it alone. Empty where there is
// no container.
func containerSection(c PromptContext) string {
	ct := c.Container
	if ct == nil {
		return ""
	}
	noun := ct.Noun
	if noun == "" {
		noun = "container"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n## The %s\n\n", noun)
	fmt.Fprintf(&b, "This slice is one task of several on this %s in the project's tracker.\n", noun)
	fmt.Fprintf(&b, "The %s's text below is context — the brief above is the work. Do not try\n", noun)
	fmt.Fprintf(&b, "to act on the %s itself: no commenting on it, no closing it; nat keeps\n", noun)
	b.WriteString("the tracker in step.\n\n")
	fmt.Fprintf(&b, "%s\n", ct.Title)
	if ct.ExternalURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", ct.ExternalURL)
	}
	if ct.Prose != "" {
		fmt.Fprintf(&b, "\n%s\n", strings.TrimRight(ct.Prose, "\n"))
	}
	return b.String()
}

// repoPassage sends an agent whose slice has no repository yet (RepoUnknown)
// to find it before anything else: work it out from the container, ask the
// user where it cannot tell, record it with `nat slice-repo` so every later
// session, the review and the merge find it, and then cut the slice's worktree
// itself — by the very naming nat's own launch cuts one by, since a relaunch
// finds the worktree by arriving at the same path.
//
// The naming — actions.SliceBranch, worktree.pathSlug, git.CLI.Base — is
// spelled out here in prose and again in skills/next-slice/SKILL.md. Never
// deduplicate it: a prompt is read by an agent, not compiled, so it cannot
// call the Go, and both copies must independently say the same thing.
func repoPassage(c PromptContext) string {
	noun := "container"
	if c.Container != nil && c.Container.Noun != "" {
		noun = c.Container.Noun
	}
	var b strings.Builder
	b.WriteString("\n## First, find the repository\n\n")
	b.WriteString("This project has no repository of its own: each of its tasks is worked in\n")
	fmt.Fprintf(&b, "whichever repository its %s is about, and nobody has said which that is\n", noun)
	b.WriteString("for this one yet. That is your first job, before any other.\n\n")
	fmt.Fprintf(&b, "Work it out from the %s above and what is on this machine: its facts and\n", noun)
	b.WriteString("links — branches, pull requests, external links — its project and epic,\n")
	b.WriteString("and its description. If you cannot tell with confidence, ask the user\n")
	b.WriteString("here in the terminal, and do nothing else until they answer.\n\n")
	b.WriteString("Once you know, record it, so every later session on this slice, the\n")
	b.WriteString("review and the merge all find it:\n\n")
	fmt.Fprintf(&b, "    nat slice-repo %s --project %s --repo <absolute path to the checkout>\n\n", c.Slice.ID, c.ProjectID)
	b.WriteString("Then cut this slice's own worktree from that repository, exactly as nat\n")
	b.WriteString("cuts one — a later launch finds it by arriving at the same path, so the\n")
	b.WriteString("naming has to be followed to the letter:\n\n")
	b.WriteString("- The branch is `slice/` followed by the slice's name lowercased, with\n")
	b.WriteString("  every run of anything that is not an ASCII letter or digit collapsed\n")
	b.WriteString("  into a single hyphen and none left at either end: \"Fix the login page\"\n")
	b.WriteString("  is `slice/fix-the-login-page`.\n")
	b.WriteString("- The worktree goes in a sibling `<repo>.worktrees` directory, one entry\n")
	b.WriteString("  per branch, named by the branch with every run of anything that is not\n")
	b.WriteString("  a letter, a digit, a dot, a hyphen or an underscore collapsed into a\n")
	b.WriteString("  single hyphen — so `slice/fix-the-login-page` under a repository at\n")
	b.WriteString("  `/repos/app` is `/repos/app.worktrees/slice-fix-the-login-page`.\n")
	b.WriteString("  `<repo>` is the directory holding the git directory every worktree of\n")
	b.WriteString("  the repository shares: the parent of what `git rev-parse\n")
	b.WriteString("  --path-format=absolute --git-common-dir` names.\n")
	b.WriteString("- If `git worktree list --porcelain` already shows a worktree on that\n")
	b.WriteString("  branch, work there and cut nothing. If the branch exists with no\n")
	b.WriteString("  worktree, check it out: `git worktree add <repo>.worktrees/<path slug>\n")
	b.WriteString("  slice/<slug>`. Otherwise run `git fetch origin` (a fetch that fails is\n")
	b.WriteString("  no reason to stop) and cut it from the base:\n")
	b.WriteString("  `git worktree add <repo>.worktrees/<path slug> -b slice/<slug> <base>`.\n")
	b.WriteString("- The base is whatever `git symbolic-ref --short refs/remotes/origin/HEAD`\n")
	b.WriteString("  names (`origin/main`, `origin/master`); with no such ref, `origin/main`\n")
	b.WriteString("  if the repository has one; and only a repository with no origin at all\n")
	b.WriteString("  falls back to the local `main`.\n")
	return b.String()
}

// gitSnapshotSection is the "captured at launch" rendering [Prompt] (for a
// resume) and [fixPrompt] share: the branch's commits since base and its diff
// stat, framed so the agent knows both were already read and need not be
// re-run. Left out entirely when neither read came back — a gather that
// failed, or a first-time launch that never attempted one.
func gitSnapshotSection(c PromptContext) string {
	if c.GitLog == "" && c.GitDiffStat == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## What is already on the branch\n\n")
	fmt.Fprintf(&b, "Captured at launch — no need to re-run this. Commits on %s since %s:\n\n", c.Branch, c.GitBase)
	if c.GitLog != "" {
		fmt.Fprintf(&b, "```\n%s\n```\n\n", c.GitLog)
	} else {
		b.WriteString("_(could not be read at launch)_\n\n")
	}
	b.WriteString("Files changed:\n\n")
	if c.GitDiffStat != "" {
		fmt.Fprintf(&b, "```\n%s\n```\n", c.GitDiffStat)
	} else {
		b.WriteString("_(could not be read at launch)_\n")
	}
	return b.String()
}

// Prompt is the opening message for an agent session working one slice.
//
// The agent starts with no history, so the prompt has to carry the whole
// contract: which slice, its own brief and the project's conventions, and how
// to record the outcome. The slice is already claimed by the time the prompt
// is written — the launch claims it before starting the session — so there is
// nothing for the agent to run before it starts work; the brief comes with
// it, read once at launch rather than by a command the agent runs itself.
//
// A relaunch is told so: a session placed on the very branch the slice
// records is continuing work that is already there — see [resuming] — rather
// than left to be discovered, since an agent that read the ordinary prompt
// would take a branch full of commits for somebody else's.
//
// Every step that touches the tracker is a `nat` command. The agent is told
// nothing about Notion — not the data sources, not the properties, not even
// that Notion is what is behind the commands — because the commands are the
// only writes it is allowed to make, and a prompt that also explained the
// underlying pages would be describing a second way to do the same thing.
//
// The ending it is told to reach is a hand-back: the branch pushed and recorded
// with `complete-slice --branch`, the slice left in progress for the user to
// review on the board, which is where the pull request is opened from. So the
// prompt tells the agent not to run `gh` at all — an agent that opened its own
// pull request would put the work past the review the approve key answers,
// and `gh pr create` on a branch that already has one refuses anyway.
//
// Every one of those commands names the project it acts on with --project,
// which they require: a command given none is refused rather than falling back
// to the project the board is on, since the user changes that while an agent
// runs. The prompt pins them to the project of the launch, which is the one the
// slice is in and cannot change under a session, and says why, since the
// commands an agent runs of its own accord are the ones no template can spell
// out.
// A session sent at an open pull request rather than at the slice is told
// something else — see [fixPrompt] — so it is dispatched here rather than
// woven through the sections below: its brief is the review rather than the
// slice, and nothing about claiming one applies to work already published.
func Prompt(c PromptContext) string {
	if c.Fix {
		return fixPrompt(c)
	}
	var b strings.Builder

	fmt.Fprintf(&b, "You are a Claude Code agent working exactly one slice of the %q project.\n\n", c.Project.Name)
	b.WriteString(frontendNote(c.Frontend))

	b.WriteString("## The slice\n\n")
	fmt.Fprintf(&b, "- Name: %s\n", c.Slice.Name)
	fmt.Fprintf(&b, "- Slice ID: %s\n", c.Slice.ID)
	if c.Slice.URL != "" {
		fmt.Fprintf(&b, "- Slice URL: %s\n", c.Slice.URL)
	}
	if c.RepoUnknown {
		fmt.Fprintf(&b, "- Working directory: none yet — this session starts in %s, your home\n", c.WorkingDir)
		b.WriteString("  directory, and finding the repository is your first job (below)\n")
	} else {
		fmt.Fprintf(&b, "- Working directory: %s\n", c.WorkingDir)
	}
	if repoOverridden(c) {
		fmt.Fprintf(&b, "  (this slice overrides the project default of %s)\n", c.Project.WorkingDir)
	}
	if c.Branch != "" {
		fmt.Fprintf(&b, "- Branch: %s (the working directory is a worktree already on it)\n", c.Branch)
	}
	if Resuming(c) {
		b.WriteString("- There is work on that branch already: an earlier session pushed it and\n")
		b.WriteString("  handed it back. You are continuing that work, not starting again.\n")
	}

	fmt.Fprintf(&b, "\nThis slice is already claimed for %s: the board claims a slice as\n", c.AssigneeName)
	b.WriteString("it launches the agent for it, so there is nothing to run before starting\n")
	b.WriteString("work. What follows is your brief: the slice's own body and acceptance\n")
	b.WriteString("criteria, then the conventions that apply to every slice of the project.\n")
	b.WriteString("The body may end in `Note` sections that earlier sessions left for\n")
	b.WriteString("whoever worked the slice next: they are part of the brief.\n\n")
	if !Resuming(c) && !c.RepoUnknown {
		b.WriteString("Claude Code has already loaded git status into this session's context —\n")
		b.WriteString("branch, working-tree state, recent commits — so there is no need to run\n")
		b.WriteString("`git status` yourself.\n\n")
	}
	b.WriteString(BriefSections(c.Brief, c.MilestoneDigest, c.Conventions))
	b.WriteString(gitSnapshotSection(c))

	b.WriteString("\nEvery `nat` command below names the project this slice is in:\n\n")
	fmt.Fprintf(&b, "    --project %s\n\n", c.ProjectID)
	b.WriteString("Put it on any other one you run too.\n")
	b.WriteString("A command given no project is refused: there is nothing for it to fall\n")
	b.WriteString("back to, and in particular not the project the user's board is on,\n")
	b.WriteString("which they can switch while you work.\n")
	b.WriteString(containerSection(c))

	if c.RepoUnknown {
		b.WriteString(repoPassage(c))
	} else {
		b.WriteString("\n## Already in your context\n\n")
		b.WriteString("`CLAUDE.md` in the working directory — architecture and the verification\n")
		b.WriteString("gate — is auto-loaded by Claude Code; there is no need to read it again.\n")
	}

	b.WriteString("\n## Before you write code\n\n")
	b.WriteString("If this slice turns on an architecture question — a decision neither the\n")
	b.WriteString("brief nor a Done slice in the milestone actually settles — do not guess\n")
	b.WriteString("and start writing: raise it with the user right away, before code, not at\n")
	b.WriteString("hand-back where an hour of the wrong shape is already sunk. And never run\n")
	b.WriteString("`complete-slice` on work whose architecture is still unsettled that way —\n")
	b.WriteString("a hand-back is a claim the shape is right, not a place to flag that it\n")
	b.WriteString("might not be.\n")

	b.WriteString("\n## Do the work\n\n")
	if c.RepoUnknown {
		b.WriteString("Work in the worktree you cut above; this session did not start there,\n")
		b.WriteString("so use absolute paths or `git -C`. Read its `CLAUDE.md`, if it has one,\n")
		b.WriteString("before anything else — architecture and the verification gate. Honour\n")
		b.WriteString("the brief's acceptance criteria and that gate before calling it done.\n\n")
	} else {
		b.WriteString("Work in the working directory above; if that is not where this session\n")
		b.WriteString("started, use absolute paths or `git -C`. Honour the brief's acceptance\n")
		b.WriteString("criteria and the project's verification gate before calling it done.\n\n")
	}
	b.WriteString("Read files with the Read tool, not `cat`/`sed`/`head` — Read handles\n")
	b.WriteString("offsets for files too big to read whole. Edit files with Edit or Write,\n")
	b.WriteString("not a shell heredoc — a heredoc edit re-transmits the whole old block and\n")
	b.WriteString("the whole new one, and this codebase's house style is dense enough prose\n")
	b.WriteString("that doubling it on every touch adds up fast. The shell is for running\n")
	b.WriteString("things — tests, git, the verification gate — not for reading or editing\n")
	b.WriteString("files.\n\n")
	switch {
	case Resuming(c):
		b.WriteString("That directory is a git worktree cut for this slice alone, already on\n")
		fmt.Fprintf(&b, "the branch %s and shared with nobody, and the work an earlier\n", c.Branch)
		b.WriteString("session pushed is already on it. Read what is there before adding to\n")
		b.WriteString("it — the commits on the branch, and the summary the slice page carries\n")
		fmt.Fprintf(&b, "of what that session did. Commit your own work there and push %s\n", c.Branch)
		b.WriteString("again: the same branch, which is the one the review is against. Do not\n")
		b.WriteString("create a branch of your own and do not switch to another; this one is\n")
		b.WriteString("yours and is what you hand back. Do not run `gh`, and do not open a\n")
		b.WriteString("pull request: you hand the branch back and the user opens the pull\n")
		b.WriteString("request from the board once they have reviewed it.\n\n")
	case c.RepoUnknown:
		b.WriteString("Once the worktree above is cut, it is yours alone. If the work is code:\n")
		b.WriteString("commit there — exactly ONE change, this slice's — and push its branch.\n")
		b.WriteString("Do not create another branch and do not switch away; that one is what\n")
		b.WriteString("you hand back. Do not run `gh`, and do not open a pull request: you hand\n")
		b.WriteString("the branch back and the user opens the pull request from the board once\n")
		b.WriteString("they have reviewed it.\n\n")
	case c.Branch != "":
		fmt.Fprintf(&b, "That directory is a git worktree cut for this slice alone, already on\n")
		fmt.Fprintf(&b, "the branch %s and shared with nobody. If the work is code: commit\n", c.Branch)
		fmt.Fprintf(&b, "there — exactly ONE change, this slice's — and push %s. Do not\n", c.Branch)
		b.WriteString("create a branch of your own and do not switch to another; this one is\n")
		b.WriteString("yours and is what you hand back. Do not run `gh`, and do not open a\n")
		b.WriteString("pull request: you hand the branch back and the user opens the pull\n")
		b.WriteString("request from the board once they have reviewed it.\n\n")
	default:
		b.WriteString("If the work is code: branch for the slice — one branch, and exactly ONE\n")
		b.WriteString("change on it — commit, and push the branch. Do not run `gh`, and do not\n")
		b.WriteString("open a pull request: you hand the branch back and the user opens the\n")
		b.WriteString("pull request from the board once they have reviewed it.\n\n")
	}
	b.WriteString("If the work is not code — docs, research, written-up findings — produce\n")
	b.WriteString("the deliverable the brief asks for and link it in the summary below.\n")
	b.WriteString(checksPassage(c))
	b.WriteString(notesPassage(c))
	b.WriteString(namingPassage)
	b.WriteString(tmuxPassage)

	b.WriteString("\n## Finish\n\n")
	b.WriteString(followUpsPassage(c))
	b.WriteString(visualsPassage(c, "before `complete-slice`"))
	b.WriteString("On completion, record the outcome:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --project %s \\\n", c.Slice.ID, c.ProjectID)
	fmt.Fprintf(&b, "        --branch %s --summary '- <what changed>\\n- <key decision>' \\\n", branchArg(c))
	b.WriteString("        --pr-description '<title line>\n\n<what the PR does and why>'\n\n")
	b.WriteString("That records the branch you pushed and hands the slice back for review,\n")
	b.WriteString("writing the summary onto its page. `--summary` is quoted back to a future\n")
	b.WriteString("agent in its milestone's digest, not read by a person, so keep it a\n")
	if c.Frontend == FrontendGnat {
		b.WriteString("handful of terse bullet points — what changed and key decisions — never\n")
		b.WriteString("a narrative of the session. It leaves the slice in progress on purpose —\n")
		b.WriteString("approving it in the app's Diff tab is what opens the pull request and\n")
		b.WriteString("marks it Done.\n\n")
	} else {
		b.WriteString("handful of terse bullet points — what changed, key decisions, follow-ups\n")
		b.WriteString("worth queueing — never a narrative of the session. It leaves the slice in\n")
		b.WriteString("progress on purpose — approving it on the board is what opens the pull\n")
		b.WriteString("request and marks it Done.\n\n")
	}
	b.WriteString("`--pr-description` is what that pull request is opened with: its first\n")
	b.WriteString("line is the title and the rest the body, so write it ready to publish —\n")
	b.WriteString("what the change does and why, for whoever reviews it on GitHub, not a\n")
	b.WriteString("report of your session. Pass `--pr-description -` to read it from stdin\n")
	b.WriteString("when it is too long for an argument, and give `--summary` as a flag then.\n\n")
	b.WriteString("Make no unverifiable claims in either one: say only what you actually\n")
	b.WriteString("checked, never what you assume or expect to be true. \"nothing invented\"\n")
	b.WriteString("about a value you interpolated rather than read is exactly the kind of\n")
	b.WriteString("line that costs somebody else an hour redoing the work to find out it\n")
	b.WriteString("was wrong.\n\n")
	b.WriteString("Leave `--branch` off when the slice produced no branch — a docs or\n")
	b.WriteString("research slice — and it is marked Done there and then, with no pull\n")
	b.WriteString("request to describe. A summary too long for one argument can be piped in\n")
	b.WriteString("on stdin instead of passing `--summary`.\n\n")
	b.WriteString("If you cannot complete it, say what stopped you and leave the slice\n")
	b.WriteString("in progress, so nobody else picks it up on top of your work:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --project %s \\\n", c.Slice.ID, c.ProjectID)
	b.WriteString("        --blocked --summary '<what is blocking>'\n")

	b.WriteString("\n## Guardrails\n\n")
	b.WriteString("- One slice per session. Never pick up another when this one is done.\n")
	b.WriteString("- The `nat` commands are the only way to record anything about the slice.\n")
	fmt.Fprintf(&b, "- Every one of them carries `--project %s`.\n", c.ProjectID)
	b.WriteString("- Never touch other slices, other milestones, or the plan itself, beyond\n")
	b.WriteString("  a note on a later slice's brief.\n")
	b.WriteString("- Never open or merge a pull request, and never push to the main branch.\n")

	return b.String()
}

// PlanPrompt is the opening message for a planning agent session: an agent
// launched from the board to workshop the plan itself — milestones and slices —
// with the user, rather than to execute a slice.
//
// Like Prompt, it routes every write through the `nat` commands and says
// nothing about Notion: the planning commands are the only writes the agent is
// allowed, and they enforce the drafting rules themselves. The workflow lives
// in the /queue-work skill rather than being restated here, for the same
// reason the slice prompt does not restate the brief.
//
// request is what the user typed into the launch input: the thing they want to
// workshop, carried in the prompt so the agent starts on it rather than
// opening with a question. Empty means a plain planning session.
//
// projectID is the project's own page ID, which every command in the prompt
// names with --project: a planning session outlives the board's idea of which
// project is active, and a plan written into the project the user has since
// switched to is the one mistake none of the drafting rules would catch.
//
// plan is the launch's own read of the current plan, rendered the same way
// `nat info` prints it — see [actions.RenderedPlan] — and carried inline so
// the agent starts with it already in hand rather than being told to run
// that command itself. Empty when the read failed at launch, which falls
// the prompt back to naming the command instead.
//
// frontend says which surface launched the session — see [Frontend].
func PlanPrompt(projectID, projectName, workingDir, request, plan string, frontend Frontend) string {
	b := planBody(projectID, projectName, workingDir, plan, frontend)

	if request != "" {
		b.WriteString("\n## The request\n\n")
		b.WriteString("The user launched you with this in hand — start on it straight away,\n")
		b.WriteString("rather than asking what they want to work on:\n\n")
		b.WriteString(request + "\n")
	}

	return b.String()
}

// planBody is everything a planning session is told before the request it was
// launched on: the job, the plan itself, the workflow, the commands, the
// guardrails. PlanPrompt opens with it and adds only the request.
//
// plan is the launch's own read of the current plan, already rendered — see
// [PlanPrompt] — carried inline in place of an instruction to go and read it.
// Empty falls back to naming the command instead, which is what a gather
// that failed at launch leaves it as.
//
// frontend says which surface launched the session — see [Frontend].
func planBody(projectID, projectName, workingDir, plan string, frontend Frontend) *strings.Builder {
	b := &strings.Builder{}

	fmt.Fprintf(b, "You are a Claude Code planning agent for the %q project.\n\n", projectName)
	b.WriteString(frontendNote(frontend))
	b.WriteString("Your job is to workshop the plan itself with the user — reshape\n")
	b.WriteString("milestones, draft new slices — not to execute any slice.\n")

	b.WriteString("\n## The plan\n\n")
	if plan != "" {
		b.WriteString("This was read fresh at launch — the project's conventions, its\n")
		b.WriteString("milestones in plan order, and the slices under them:\n\n")
		b.WriteString(plan)
		if !strings.HasSuffix(plan, "\n") {
			b.WriteString("\n")
		}
		fmt.Fprintf(b, "\n`nat info --project %s` (`--json` to parse it) re-reads it, for when\n", projectID)
		b.WriteString("fresh state matters mid-session — the plan above is a snapshot from\n")
		b.WriteString("launch, and the user may add to it or change it while you work.\n")
	} else {
		b.WriteString("Could not be read at launch; run this to see it — the project's\n")
		b.WriteString("conventions, its milestones in plan order, and the slices under them\n")
		b.WriteString("(`--json` to parse it instead):\n\n")
		fmt.Fprintf(b, "    nat info --project %s\n", projectID)
	}

	b.WriteString("\n## How to work\n\n")
	b.WriteString("Follow the /queue-work skill: it is the planning workflow for this\n")
	b.WriteString("tracker.\n\n")
	b.WriteString("Every `nat` command you run names the project you are planning:\n\n")
	fmt.Fprintf(b, "    --project %s\n\n", projectID)
	b.WriteString("A command given no project is refused: there is nothing for it to fall\n")
	b.WriteString("back to, and in particular not the project the user's board is on,\n")
	b.WriteString("which they can switch while you work.\n")

	b.WriteString("\n## Applying changes\n\n")
	if frontend == FrontendGnat {
		b.WriteString("Present the draft in conversation too, but the workshop's Plan section\n")
		b.WriteString("in the app is what the user reads it from and accepts it in. As soon\n")
		b.WriteString("as you have a draft, and again on every revision, without being asked,\n")
		b.WriteString("pipe the plan JSON (the same document plan-apply reads) to this:\n\n")
		fmt.Fprintf(b, "    nat plan-propose --project %s\n\n", projectID)
		b.WriteString("Never run plan-apply, milestone-add or slice-add yourself — the\n")
		b.WriteString("user's Accept in the app is the one approval there is, and it is what\n")
		b.WriteString("applies the plan, not you. A revised proposal replaces whichever one\n")
		b.WriteString("is on screen, so send the whole plan again each time rather than a\n")
		b.WriteString("diff of it.\n")
	} else {
		b.WriteString("Draft in conversation first, and write only after the user explicitly\n")
		b.WriteString("approves. The `nat` planning commands are the only way to change the\n")
		b.WriteString("plan:\n\n")
		fmt.Fprintf(b, "- `nat plan-apply [FILE] --project %s` — a whole drafted\n", projectID)
		b.WriteString("  plan of milestones and slices at once, from a JSON document (stdin\n")
		b.WriteString("  without FILE)\n")
		fmt.Fprintf(b, "- `nat milestone-add <name> --project %s` — one new\n", projectID)
		b.WriteString("  milestone, Queued, at the end of the plan\n")
		fmt.Fprintf(b, "- `nat slice-add <title> --milestone <name> [--description -] --project %s`\n", projectID)
		b.WriteString("  — one new Todo slice, its brief read from stdin\n")
	}

	b.WriteString(namingPassage)
	b.WriteString(tmuxPassage)

	b.WriteString("\n## Guardrails\n\n")
	b.WriteString("- Plan only. Never claim, start, or complete a slice — launching work is\n")
	b.WriteString("  the board's job, not yours.\n")
	b.WriteString("- Never touch work in flight: slices in progress and Done slices, and the\n")
	b.WriteString("  milestones holding them, are records of what happened.\n")
	if frontend == FrontendGnat {
		b.WriteString("- plan-propose is the only way to change the plan; never run\n")
		b.WriteString("  plan-apply, milestone-add or slice-add yourself — applying a\n")
		b.WriteString("  proposal is the user's Accept, not something you do.\n")
	} else {
		b.WriteString("- The commands above are the only way to change the plan; write nothing\n")
		b.WriteString("  until the user has approved the draft.\n")
	}
	if frontend == FrontendGnat {
		fmt.Fprintf(b, "- Every `nat` command you run, plan-propose included, carries `--project %s`.\n", projectID)
	} else {
		fmt.Fprintf(b, "- Every one of them carries `--project %s`.\n", projectID)
	}
	fmt.Fprintf(b, "- This session starts in %s; ", workingDir)
	if frontend == FrontendGnat {
		b.WriteString("the macOS app picks up\n  your changes on its own — there is no refresh key to press.\n")
	} else {
		b.WriteString("the user's board picks up\n  your changes when you exit, or on its refresh key.\n")
	}

	return b
}

// Resuming reports whether the session is picking work up rather than
// starting it: the worktree it is placed in is on the very branch the slice
// records, so there are commits there already and an earlier session put
// them there.
//
// It is the branch matching that says so rather than the slice's status alone,
// because a released slice is back at Todo with its branch still recorded and
// its work still exactly what the next session wants, and because a launch that
// fell back to the shared checkout has no worktree to have found the work in.
//
// Exported for [actions.Launch]'s own use: whether a resume launch's git
// snapshot is worth gathering is the same question this prompt already asks.
func Resuming(c PromptContext) bool {
	return c.Branch != "" && c.Branch == strings.TrimSpace(c.Slice.Branch)
}

// visualsPassage tells a slice agent to hand in images of a visible change
// where the project already renders such things cheaply, and never to build a
// way to render where it does not; when says where in its ending that comes.
// It is told to every slice agent whatever launched it, unlike follow-ups,
// since nothing waits on a hand-in: the user's comments, if any, arrive as a
// message. skills/next-slice/SKILL.md says the same in its own words.
func visualsPassage(c PromptContext, when string) string {
	var b strings.Builder
	b.WriteString("If what you changed is visible — a pane, a page, a rendered component —\n")
	b.WriteString("and the project already has a cheap or usual way to render it (a gallery\n")
	b.WriteString("story, a screenshot script, a storybook), render the result and hand the\n")
	fmt.Fprintf(&b, "images in %s:\n\n", when)
	fmt.Fprintf(&b, "    nat slice-visuals %s --project %s \\\n", c.Slice.ID, c.ProjectID)
	b.WriteString("        --visual '<what it shows, one line>\n<absolute path to the image>'\n\n")
	b.WriteString("`--visual` repeats, one per image. Hand in the full set each time: a later\n")
	b.WriteString("hand-in replaces an earlier one. Do not build a way to render when the\n")
	b.WriteString("project has none — go on without images instead. The user reviews them\n")
	b.WriteString("in the app; their comments, if any, arrive here as a message.\n\n")
	return b.String()
}

// notesPassage tells a slice agent — a fresh one or a fix session — how to
// leave a note on a later slice's brief, with the agent's own slice as where it
// came from. skills/next-slice/SKILL.md says the same in its own words.
func notesPassage(c PromptContext) string {
	var b strings.Builder
	b.WriteString("\n## Notes for later slices\n\n")
	b.WriteString("If this session finds out something a *later* slice needs to know — a\n")
	b.WriteString("constraint, a seam that moved, an assumption in another slice's brief\n")
	b.WriteString("that is no longer true — leave a note on that slice, named by its name:\n\n")
	fmt.Fprintf(&b, "    nat slice-note '<slice name>' --from %s --project %s \\\n", c.Slice.ID, c.ProjectID)
	b.WriteString("        --note '<what it needs to know, and why>'\n\n")
	b.WriteString("Add `--milestone '<milestone name>'` where that name is filed under more\n")
	b.WriteString("than one milestone, and `--note -` to pipe a long note in. The note ends\n")
	b.WriteString("that slice's brief, with where it came from written by nat, so whoever\n")
	b.WriteString("works it next reads it as part of the brief. A note is never work to be\n")
	b.WriteString("done — that is a follow-up, not a note — and never goes on a Done slice.\n")
	return b.String()
}

// followUpsPassage tells an agent — a fresh one or a fix session — to hand in
// the work it noticed but did not do, and stop, before `complete-slice`. Only
// the app has anywhere to triage follow-ups, so only an agent it launched is
// told to hand them in and wait; one launched from the board would wait on a
// decision nothing there can make, and is told nothing.
func followUpsPassage(c PromptContext) string {
	if c.Frontend != FrontendGnat {
		return ""
	}
	var b strings.Builder
	b.WriteString("Work you noticed but did not do — a bug beside your change, a test gap\n")
	b.WriteString("in code you didn't touch, a refactor the brief didn't ask for — is not\n")
	b.WriteString("yours to do and not yours to lose. When the gate is green, before\n")
	b.WriteString("`complete-slice`, hand each one in and **stop**:\n\n")
	fmt.Fprintf(&b, "    nat slice-followups %s --project %s \\\n", c.Slice.ID, c.ProjectID)
	b.WriteString("        --follow-up '<title line>\n\n<the change: which file or function, what it does instead, and why>\nDone when: <how anyone checks it is finished>'\n\n")
	b.WriteString(followUpBriefPassage)
	b.WriteString("`--follow-up` repeats, one per follow-up. The user decides in the app —\n")
	b.WriteString("queue it as a slice, fold it into this one, or drop it — and the decision\n")
	b.WriteString("arrives here as a message naming what to fold in. Do that, then hand back\n")
	b.WriteString("as below. `complete-slice` refuses while the decision is outstanding.\n")
	b.WriteString("Never widen your branch to include a follow-up on your own, and never\n")
	b.WriteString("write them into the summary or the brief instead. No follow-ups: hand\n")
	b.WriteString("back straight away.\n\n")
	return b.String()
}

// checksPassage tells a slice agent how to read CI: the project's checks run
// on the pull request once the slice is approved, and `nat slice-checks` is
// the one way an agent reads them — never `gh`. skills/next-slice/SKILL.md
// says the same in its own words.
func checksPassage(c PromptContext) string {
	var b strings.Builder
	b.WriteString("\n## Reading CI\n\n")
	b.WriteString("The project's checks run on the pull request, once the slice is approved.\n")
	b.WriteString("If you are told they failed, read how they stand — each check, and each\n")
	b.WriteString("failed step's log — with:\n\n")
	fmt.Fprintf(&b, "    nat slice-checks %s --log --project %s\n\n", c.Slice.ID, c.ProjectID)
	b.WriteString("That is the one way to read CI: never `gh`.\n")
	return b.String()
}

// namingPassage is the rule every text handed to an agent that writes about
// slices carries — slice, fix, plan and new-project prompts, and every
// embedded skill in its own copy of the same words. A test walks each for it.
const namingPassage = "\n## Naming slices\n\n" +
	"Refer to another slice only by its name, adding its milestone's name\n" +
	"where the name alone is ambiguous — never by a number, an index, a\n" +
	"position in a list, a page ID, a URL, or any id of another tracker (a\n" +
	"card number, an issue key). Names are what every reading of the plan\n" +
	"shows; the rest is the tracker's own or a plugin's, which the next reader\n" +
	"may not have. This holds for everything you write: summaries, PR\n" +
	"descriptions, follow-up briefs, notes, proposal briefs.\n"

// tmuxPassage is the rule every agent nat launches carries, since every one of
// them runs in a pane of the user's own tmux server: never kill that server,
// and never kill a session that is not its own. An agent that wanted a clean
// tmux for an experiment once ran `tmux kill-server` under its own
// `TMUX_TMPDIR`, believing that isolated it — it does not, since `$TMUX` names
// the socket while set — and took down itself, every other agent and the
// user's own sessions, twice. A test walks every prompt for it; the next-slice
// skill carries the same rule in its own words.
const tmuxPassage = "\n## tmux\n\n" +
	"You are running in a tmux session nat opened for you, on the user's own\n" +
	"tmux server — the one the app, every other agent and the user's own\n" +
	"sessions live on; `$TMUX` in your environment names it. Never run `tmux\n" +
	"kill-server`, and never kill, detach or send keys to a session you did\n" +
	"not create: either takes this session and every other agent's down\n" +
	"mid-run, with no record of why. If the work needs a tmux of its own, give\n" +
	"it a private socket — `tmux -L <name>` on every one of its commands — and\n" +
	"kill only that socket's server when you are done. Setting `TMUX_TMPDIR`\n" +
	"does not isolate you: `$TMUX` wins while it is set.\n"

// followUpBriefPassage says how a follow-up is written: as the brief of the
// slice it becomes if queued, since slice-triage files it as one verbatim.
// skills/next-slice/SKILL.md carries the same words in its own copy.
const followUpBriefPassage = "Write each one as a slice brief: if the user queues it, this text is the\n" +
	"brief of a new slice, word for word, read by an agent with nothing else.\n" +
	"The title is an imperative action (\"Make the sidebar's post-write\n" +
	"refresh read the replica\"), not a symptom. The body is the change —\n" +
	"which file or function, what it does instead, and why — then a line\n" +
	"starting `Done when:` saying how anyone checks it is finished. Write a\n" +
	"decision, not a question: where there is a choice, pick one and name the\n" +
	"alternative rejected; no \"could\", \"might\", \"consider\" or \"worth looking\n" +
	"at\". If saying what to change needs a look at the code, take that look\n" +
	"now — it is usually one read; if it genuinely needs investigation, the\n" +
	"investigation is the deliverable and `Done when:` says what it produces.\n\n"

// branchArg is what the hand-back command names: the branch the session's
// worktree is already on, or the placeholder for an agent that will make one.
func branchArg(c PromptContext) string {
	if c.Branch != "" {
		return c.Branch
	}
	return "<branch>"
}

// repoOverridden reports whether the agent is being sent somewhere other than
// the project's default working directory, which is worth calling out in the
// prompt so the agent does not assume the default.
//
// The comparison is against the checkout rather than the session's own
// directory, because a worktree is never the default one: reporting every
// worktree launch as an override would say nothing about the slice.
func repoOverridden(c PromptContext) bool {
	dir := c.WorkingDir
	if c.Repo != "" {
		dir = c.Repo
	}
	return c.Project.WorkingDir != "" && dir != c.Project.WorkingDir
}
