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
// Brief and Conventions are the slice's own page body and the project's
// conventions, read by the launch after the claim succeeds and written
// straight into the prompt — the same document `nat start-slice` prints, so
// the agent needs no command of its own to see it.
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
// and read it with `nat info` itself. Empty for a slice filed under no
// milestone.
//
// Frontend says which surface launched the session — see [Frontend] — so the
// prompt's user-facing guidance about picking up changes and approving or
// merging work names the right one. The zero value is unspecified, which
// reads exactly as every template did before this field existed.
//
// GitBase, GitLog and GitDiffStat are a resume launch's read of the
// worktree, taken by [actions.Launch] right after PlaceAgent resolves it:
// the base the branch is measured against, `git log --oneline <base>..HEAD`
// and `git diff --stat <base>...HEAD` — separating an earlier session's
// commits from whatever the base has moved on by since, and naming the files
// already touched, neither of which Claude Code's own injected git status
// snapshot carries. A first-time launch never gathers them — there is
// nothing yet on the branch worth reading — and a gather that fails leaves
// whichever of GitLog/GitDiffStat failed empty, which is what tells [Prompt]
// to leave the section out rather than print half of it.
//
// ReviewComments and ReviewChecks are a launch's read of the review on a
// slice with a pull request recorded, taken the same way: `gh pr view <url>
// --comments` and `gh pr checks <url>` — the first the one `gh` read the
// prompt lets the agent run again itself ([pullRequestPassage]), the second
// re-read with `nat slice-checks`, the way every agent reads CI. Each is
// independently left empty on a failed read, the project's usual
// reads-conclude-nothing posture — a launch never fails over missing context.
//
// HandedBack says the slice's task log holds a hand-back, read off the brief
// by [actions.Launch]: work an earlier session pushed and gave up for review,
// whether its Branch is still recorded or was cleared when it was sent back.
// It is one of the ways [Resuming] knows a session is picking work up.
//
// ConflictBase is the base a handed-back branch with no pull request was found
// conflicting with at launch ([git.CLI.ConflictsWithBase], run by
// [actions.Launch]): set, the prompt tasks the agent with bringing the branch
// up to date with it first ([conflictPassage]). Empty for a clean branch, one
// that could not be tested, and every slice with a pull request or never
// handed back. ConflictRebase is how far the launch itself got with that
// rebase, and ConflictPaths the files it left conflicted.
//
// Container is the container a source project's slice hangs off, read off the
// plugin at launch by [actions.Launch]; nil for every other project, and where
// the read failed — the prompt then simply has no section for it.
//
// RepoUnknown says the slice has no repository to work in yet: a source
// project, which has no working directory of its own, on a task none has been
// recorded for. The session starts in the home directory (WorkingDir), with no
// worktree, and the prompt sends it to work the repository out from the
// container and record it with `nat slice-repo`, which cuts its worktree —
// see [repoPassage].
type PromptContext struct {
	Slice           domain.Slice
	Project         config.ProjectConfig
	ProjectID       string
	WorkingDir      string
	Branch          string
	Repo            string
	AssigneeName    string
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
	HandedBack      bool
	ConflictBase    string
	ConflictRebase  ConflictRebase
	ConflictPaths   []string
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
// user where it cannot tell, and record it with `nat slice-repo` — which also
// cuts the slice's worktree there by nat's own naming and prints its path, so
// every later session, the review and the merge find the same one. The naming
// itself is the Go's alone (actions.EnsureWorktree): nothing here spells it
// out, and a test keeps it that way.
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
	b.WriteString("That also cuts this slice's own worktree in that repository, exactly as\n")
	b.WriteString("nat cuts one for a launch, and prints its path on the `Worktree:` line:\n")
	b.WriteString("work there and nowhere else. Cut no worktree or branch of your own. If it\n")
	b.WriteString("says the worktree could not be cut, the repository is recorded but git\n")
	b.WriteString("refused, in its own words — tell the user what it said and stop.\n")
	return b.String()
}

// gitSnapshotSection is the "captured at launch" rendering [Prompt] gives a
// resume: the branch's commits since base and its diff
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
//
// A slice with a pull request recorded is relaunched like any other, and told
// besides that its work is out — see [pullRequestPassage].
func Prompt(c PromptContext) string {
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
	b.WriteString(pullRequestPassage(c))
	b.WriteString(conflictPassage(c))
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

	b.WriteString(questionPassage)

	b.WriteString("\n## Do the work\n\n")
	if c.RepoUnknown {
		b.WriteString("Work in the worktree slice-repo printed; this session did not start there,\n")
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
	b.WriteString(testingPassage("immediately before you hand back"))
	b.WriteString("\n")
	switch {
	case Resuming(c):
		b.WriteString("That directory is a git worktree cut for this slice alone, already on\n")
		fmt.Fprintf(&b, "the branch %s and shared with nobody, and the work an earlier\n", c.Branch)
		b.WriteString("session pushed is already on it. Read what is there before adding to\n")
		b.WriteString("it — the commits on the branch, and the summary the slice page carries\n")
		b.WriteString("of what that session did. Commit your own work there, on the same\n")
		b.WriteString("branch, which is the one the review is against; the hand-back below\n")
		b.WriteString("pushes it, so do not push it yourself. Do not create a branch of your\n")
		b.WriteString("own and do not switch to another; this one is yours and is what you\n")
		if c.Slice.PRURL != "" {
			b.WriteString("hand back. Its pull request is open already, and the hand-back's push\n")
			b.WriteString("updates it: there is no other to open.\n\n")
		} else {
			b.WriteString("hand back. Do not run `gh`, and do not open a pull request: you hand\n")
			b.WriteString("the branch back and the user opens the pull request from the board\n")
			b.WriteString("once they have reviewed it.\n\n")
		}
	case c.RepoUnknown:
		b.WriteString("Once the worktree above is cut, it is yours alone. If the work is code:\n")
		b.WriteString("commit there — exactly ONE change, this slice's. The hand-back below\n")
		b.WriteString("pushes its branch: do not push it yourself. Do not create another branch\n")
		b.WriteString("and do not switch away; that one is what you hand back. Do not run\n")
		b.WriteString("`gh`, and do not open a pull request: you hand the branch back and the\n")
		b.WriteString("user opens the pull request from the board once they have reviewed it.\n\n")
	case c.Branch != "":
		fmt.Fprintf(&b, "That directory is a git worktree cut for this slice alone, already on\n")
		fmt.Fprintf(&b, "the branch %s and shared with nobody. If the work is code: commit\n", c.Branch)
		b.WriteString("there — exactly ONE change, this slice's. The hand-back below pushes\n")
		b.WriteString("the branch, so do not push it yourself. Do not create a branch of your\n")
		b.WriteString("own and do not switch to another; this one is yours and is what you\n")
		b.WriteString("hand back. Do not run `gh`, and do not open a pull request: you hand\n")
		b.WriteString("the branch back and the user opens the pull request from the board\n")
		b.WriteString("once they have reviewed it.\n\n")
	default:
		b.WriteString("If the work is code: branch for the slice — one branch, and exactly ONE\n")
		b.WriteString("change on it — and commit; the hand-back below pushes the branch, so do\n")
		b.WriteString("not push it yourself. Do not run `gh`, and do not open a pull request:\n")
		b.WriteString("you hand the branch back and the user opens the pull request from the\n")
		b.WriteString("board once they have reviewed it.\n\n")
	}
	b.WriteString("If the work is not code — docs, research, written-up findings — produce\n")
	b.WriteString("the deliverable the brief asks for and link it in the summary below.\n")
	b.WriteString(checksPassage(c))
	b.WriteString(notesPassage(c))
	b.WriteString(namingPassage)
	b.WriteString(writingPassage)
	b.WriteString(tmuxPassage)
	b.WriteString(waitingPassage(true))

	b.WriteString("\n## Finish\n\n")
	b.WriteString(followUpsPassage(c))
	b.WriteString(visualsPassage(c, "before `complete-slice`"))
	b.WriteString("On completion, record the outcome:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --project %s \\\n", c.Slice.ID, c.ProjectID)
	if c.Branch == "" && !c.RepoUnknown {
		// No worktree for nat to read the branch off: the agent names it.
		b.WriteString("        --branch <branch> \\\n")
	}
	b.WriteString("        --summary '- <what now works, in the user's words>\\n- <a decision you made>' \\\n")
	b.WriteString("        --pr-description '<title line>\n\n<what the PR does and why>'\n\n")
	b.WriteString("That refuses while the worktree holds anything uncommitted — commit it\n")
	b.WriteString("first — then pushes the branch, records it and hands the slice back for\n")
	b.WriteString("review, writing the summary onto its page; a push it reports refused is\n")
	b.WriteString("yours to sort out before handing back again. It leaves the slice in\n")
	if c.Frontend == FrontendGnat {
		b.WriteString("progress on purpose — approving it in the app's Diff tab is what opens\n")
		b.WriteString("the pull request and marks it Done.\n\n")
	} else {
		b.WriteString("progress on purpose — approving it on the board is what opens the pull\n")
		b.WriteString("request and marks it Done.\n\n")
	}
	b.WriteString(summaryPassage)
	if c.Frontend != FrontendGnat {
		b.WriteString("Follow-ups worth queueing go in a last bullet of their own.\n")
	}
	b.WriteString("\n")
	b.WriteString(prDescriptionPassage)
	b.WriteString("Pass `--pr-description -` to read it from stdin when it is too long for\n")
	b.WriteString("an argument, and give `--summary` as a flag then.\n\n")
	b.WriteString("Handing the same slice back a second time, leave `--pr-description` off\n")
	b.WriteString("where the one already filed still describes the change: the last one\n")
	b.WriteString("filed is what the pull request opens with, so restating it unchanged only\n")
	b.WriteString("sends it again. Where the change has moved, amend the one you filed and\n")
	b.WriteString("pass it whole — it replaces the earlier one, it is not added to it.\n\n")
	b.WriteString("Make no unverifiable claims in either one: say only what you actually\n")
	b.WriteString("checked, never what you assume or expect to be true. \"nothing invented\"\n")
	b.WriteString("about a value you interpolated rather than read is exactly the kind of\n")
	b.WriteString("line that costs somebody else an hour redoing the work to find out it\n")
	b.WriteString("was wrong.\n\n")
	b.WriteString("Pass `--no-branch` when the slice produced no branch — a docs or\n")
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

	b.WriteString("\n## Superseded work\n\n")
	b.WriteString("The plan document changes Todo slices already on the board as well as\n")
	b.WriteString("creating new ones: a top-level `remove` list of titles sends each to the\n")
	b.WriteString("trash, `move` (`[{\"slice\": <title>, \"milestone\": <name>}]`) refiles\n")
	b.WriteString("each under a milestone the project has or the document creates, and\n")
	b.WriteString("`edit` (`[{\"slice\": <title>, \"title\": <new title>, \"description\":\n")
	b.WriteString("<brief>}]`) renames each, replaces its brief whole, or both — give at\n")
	b.WriteString("least one. Each names a Todo slice by title, as `depends_on` does. A\n")
	b.WriteString("Todo slice the new plan supersedes is removed in the same document that\n")
	b.WriteString("replaces it — never left as a list for the user to delete or move by\n")
	b.WriteString("hand. They apply in the order edits, moves, removals, then creations, so\n")
	b.WriteString("a replacement may take the title of the slice it removes.\n")

	b.WriteString("\n## Applying changes\n\n")
	if frontend == FrontendGnat {
		b.WriteString("Present the draft in conversation too, but the workshop's Plan section\n")
		b.WriteString("in the app is what the user reads it from and accepts it in. As soon\n")
		b.WriteString("as you have a draft, and again on every revision, without being asked,\n")
		b.WriteString("pipe the plan JSON (the same document plan-apply reads) to this:\n\n")
		fmt.Fprintf(b, "    nat plan-propose --project %s\n\n", projectID)
		b.WriteString("Never run plan-apply, milestone-add or slice-add yourself — the\n")
		b.WriteString("user's Accept in the app is the one approval there is, and it is what\n")
		b.WriteString("applies the plan, not you.\n\n")
		b.WriteString(acceptedProposalPassage(projectID))
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
	b.WriteString("\n" + SliceTitleRule + "\n")
	b.WriteString(briefShapePassage)

	b.WriteString(namingPassage)
	b.WriteString(writingPassage)
	b.WriteString(tmuxPassage)
	b.WriteString(waitingPassage(true))

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
// records, or the slice has a pull request recorded — work handed back,
// approved, then resumed, its branch cleared until the next hand-back — or
// its task log holds a hand-back (HandedBack) — a review sent back before any
// pull request, its branch cleared the same way — so there are commits there
// already and an earlier session put them there.
//
// It is the branch matching that says so rather than the slice's status alone,
// because a released slice is back at Todo with its branch still recorded and
// its work still exactly what the next session wants, and because a launch that
// fell back to the shared checkout has no worktree to have found the work in.
//
// Exported for [actions.Launch]'s own use: whether a resume launch's git
// snapshot is worth gathering is the same question this prompt already asks.
func Resuming(c PromptContext) bool {
	return c.Branch != "" && (c.Branch == strings.TrimSpace(c.Slice.Branch) || c.Slice.PRURL != "" || c.HandedBack)
}

// pullRequestPassage tells an agent launched on a slice with a pull request
// recorded that its work is out: the pull request is open, the hand-back's
// push updates it, and the review as it stood at launch is carried inline. It
// relaxes the standing ban on `gh` for the one read the review needs and no
// other — the checks are read with `nat slice-checks`, as every agent reads
// CI — and keeps every pull request write the user's. Empty for a slice with
// none recorded.
func pullRequestPassage(c PromptContext) string {
	pr := c.Slice.PRURL
	if pr == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## The pull request\n\n")
	b.WriteString("This slice's work was handed back and approved, and its pull request is\n")
	fmt.Fprintf(&b, "open: %s. Handing the branch back pushes it, which updates it. The\n", pr)
	b.WriteString("user has taken the work back up — the slice's task log ends in why — so\n")
	b.WriteString("finish what they asked for and hand it back, as below; it returns to its\n")
	b.WriteString("pull request.\n\n")
	if c.ReviewComments != "" || c.ReviewChecks != "" {
		b.WriteString("Captured at launch — no need to re-run this to see where it stood then:\n\n")
		if c.ReviewComments != "" {
			fmt.Fprintf(&b, "`gh pr view %s --comments`:\n\n```\n%s\n```\n\n", pr, c.ReviewComments)
		}
		if c.ReviewChecks != "" {
			fmt.Fprintf(&b, "The pull request's checks:\n\n```\n%s\n```\n\n", c.ReviewChecks)
		}
	}
	b.WriteString("Re-read the review before you hand back, since it can have moved since launch:\n\n")
	fmt.Fprintf(&b, "    gh pr view %s --comments\n\n", pr)
	b.WriteString("That is the only `gh` you may run — read CI with the `slice-checks` command\nbelow.\n")
	b.WriteString("Never open, merge, close or reopen a pull request: merging this one is\n")
	if c.Frontend == FrontendGnat {
		b.WriteString("a button in the app's PR tab, pressed once the user is satisfied.\n")
	} else {
		b.WriteString("the user's, once they are satisfied.\n")
	}
	return b.String()
}

// ConflictRebase is how far [actions.Launch] got rebasing a conflicted
// hand-back onto its base before the agent started: the fetch and the rebase
// are mechanical, and only resolving what conflicts needs an agent.
type ConflictRebase int

const (
	// RebaseLeftToAgent is a rebase the launch did not make — it could not
	// tell whether one was under way, or its own failed and was aborted — so
	// the agent is told to make it. The zero value, the passage that asks the
	// most of the agent, for any launch that set nothing.
	RebaseLeftToAgent ConflictRebase = iota
	// RebasedAtLaunch is a rebase that went through with no conflict.
	RebasedAtLaunch
	// RebaseStoppedAtLaunch is the launch's own rebase, stopped on the first
	// commit that conflicts.
	RebaseStoppedAtLaunch
	// RebaseUnderWay is a rebase the launch found already in progress in the
	// worktree, and so started none of its own.
	RebaseUnderWay
)

// conflictPassage tells an agent relaunched on a handed-back branch that no
// longer merges into its base — found by the launch's own test, there being no
// pull request for GitHub to say so of — that bringing the branch up to date
// comes first, from wherever the launch's own rebase left it
// ([PromptContext.ConflictRebase]): run the gate on a branch it rebased
// cleanly; resolve the files it stopped on (or found a rebase already stopped
// on), keeping both sides' meaning, and continue; or, where it made no rebase,
// make one. Every ending is a hand-back, whose own push is the lease form —
// what lets a rebased branch go over its old self while never overwriting
// anything it has not seen. Empty where the launch found no conflict.
func conflictPassage(c PromptContext) string {
	base := c.ConflictBase
	if base == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n## The branch conflicts with %s\n\n", base)
	fmt.Fprintf(&b, "This slice was handed back on %s, and %s has moved on\n", c.Branch, base)
	b.WriteString("since: the launch tested the merge, and the branch no longer merged into\n")
	b.WriteString("it cleanly. Bringing it up to date comes before anything else — that is\n")
	b.WriteString("what this session was launched for, along with anything the task log's\n")
	b.WriteString("last entry asks.\n\n")
	switch c.ConflictRebase {
	case RebasedAtLaunch:
		fmt.Fprintf(&b, "The launch fetched origin and rebased %s onto %s, and the\n", c.Branch, base)
		b.WriteString("rebase went through with no conflict. What is left:\n\n")
		b.WriteString("1. Run the project's verification gate on the rebased branch.\n")
		b.WriteString("2. Hand the slice back with `complete-slice`, as below.\n")
		return b.String()
	case RebaseStoppedAtLaunch:
		fmt.Fprintf(&b, "The launch fetched origin and rebased %s onto %s. The rebase\n", c.Branch, base)
		b.WriteString("is stopped on the first commit that conflicts")
	case RebaseUnderWay:
		b.WriteString("A rebase is already under way in the worktree, so the launch started\n")
		b.WriteString("none of its own. It is stopped")
	default:
		fmt.Fprintf(&b, "1. `git fetch origin`, then, on %s:\n   `git rebase %s`.\n", c.Branch, base)
		fmt.Fprintf(&b, "2. Resolve every conflict, keeping what both sides meant: %s's side\n", base)
		b.WriteString("   is merged work, never to be undone to make the branch fit.\n")
		b.WriteString("3. Run the project's verification gate on the result.\n")
		b.WriteString("4. Hand the slice back with `complete-slice`, as below. Do not push\n")
		b.WriteString("   yourself: the hand-back pushes the rebased branch with a lease.\n")
		return b.String()
	}
	if len(c.ConflictPaths) == 0 {
		b.WriteString(", with no file left conflicted.\n\n")
	} else {
		b.WriteString(", with these files conflicted:\n\n")
		for _, p := range c.ConflictPaths {
			fmt.Fprintf(&b, "- `%s`\n", p)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "1. Resolve each conflict, keeping what both sides meant: %s's side\n", base)
	b.WriteString("   is merged work, never to be undone to make the branch fit. `git add`\n")
	b.WriteString("   each file as it is resolved.\n")
	b.WriteString("2. `git rebase --continue`, resolving the same way through any later\n")
	b.WriteString("   commit that conflicts, until the rebase is finished.\n")
	b.WriteString("3. Run the project's verification gate on the result.\n")
	b.WriteString("4. Hand the slice back with `complete-slice`, as below.\n")
	return b.String()
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
	b.WriteString("`--visual` repeats, one per image. Hand in only what is new or\n")
	b.WriteString("re-rendered: an image handed in under a name already filed replaces that\n")
	b.WriteString("one, so never remove an image you are updating. Drop one that no longer\n")
	b.WriteString("shows anything relevant to the change — a view the work no longer\n")
	b.WriteString("touches, a render superseded by a differently named one — with\n")
	b.WriteString("`--remove '<its name>'`. Where a change is best judged against what was\n")
	b.WriteString("there, hand in a before too, with `--before '<the visual's name>\n")
	b.WriteString("<absolute path to the before>'`, rendered to a different file than the\n")
	b.WriteString("after: a before the after's render overwrote is no before.\n\n")
	b.WriteString("Do not build a way to render when the project has none — go on without\n")
	b.WriteString("images instead. The user reviews them in the app; their comments, if\n")
	b.WriteString("any, arrive here as a message.\n\n")
	return b.String()
}

// testingPassage holds a slice agent to
// targeted tests while it iterates and one full gate at the end, where end is
// when that is: most of the test time across audited sessions went on full-suite
// runs mid-loop. CLAUDE.md's conventions and skills/next-slice/SKILL.md say the
// same in their own words. A test walks each slice prompt for it.
func testingPassage(end string) string {
	return "While you iterate, run only the tests for what you are touching — one\n" +
		"package, `go test -run <Name>`, `swift test --filter <Name>` — never the\n" +
		"full suite or the coverage gate mid-loop. Run the full verification gate\n" +
		"once, " + end + "; if it fails, fix it with targeted runs\n" +
		"and run the gate once more. Batch a stage's edits and build once per\n" +
		"batch, not once per edit.\n"
}

// notesPassage tells a slice agent how to
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
	b.WriteString("works it next reads it as part of the brief; where that slice has a live\n")
	b.WriteString("agent, the note reaches it in its session too. A note is never work to be\n")
	b.WriteString("done — that is a follow-up, not a note — and never goes on a Done slice.\n")
	return b.String()
}

// followUpsPassage tells a slice agent to hand in
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
	b.WriteString("        --follow-up '<title line>\n\n<the problem, as the user sees it, and what leaving it costs>\n<what you recommend, and why>\n<the change: what, where, instead of what>\nDone when: <how anyone checks it is finished>'\n\n")
	b.WriteString(followUpBriefPassage)
	b.WriteString(SliceTitleRule + "\n\n")
	b.WriteString("`--follow-up` repeats, one per follow-up. A later hand-in carries only\n")
	b.WriteString("what is new — never a repeat of a follow-up already handed in. The user\n")
	b.WriteString("decides in the app — queue it as a slice, fold it into this one, or drop\n")
	b.WriteString("it — and the decision arrives here as a message naming what to fold in.\n")
	b.WriteString("Do that, then hand back as below. `complete-slice` refuses while the\n")
	b.WriteString("decision is outstanding.\n")
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
	b.WriteString(runningChecksSentence)
	b.WriteString("That is the one way to read CI: never `gh`.\n\n")
	b.WriteString(rerunPassage(c.Slice.ID, c.ProjectID))
	return b.String()
}

// runningChecksSentence is what the slice prompt and the checks nudge each
// say after the slice-checks command: it also reads a check
// still running, which is where a stalled one is looked into.
const runningChecksSentence = "The same command shows what a check still running is doing — the step\n" +
	"it is on and for how long — so a check that has sat pending far longer\n" +
	"than its siblings or its usual run is read there, never with `gh`.\n\n"

// rerunPassage is how the slice prompt and the checks nudge tell an agent to re-run CI: `nat slice-checks-rerun`, for a failure that is
// not the change's, never as a retry of a real one — and never `gh`.
// skills/next-slice/SKILL.md says the same in its own words.
func rerunPassage(sliceID, projectID string) string {
	var b strings.Builder
	b.WriteString("Where a check failed or stalled for a reason that is not the change's —\n")
	b.WriteString("a flake, a runner that died, a job that has stalled — re-run it with:\n\n")
	fmt.Fprintf(&b, "    nat slice-checks-rerun %s --check '<check name>' --project %s\n\n", sliceID, projectID)
	b.WriteString("(`--failed` instead of `--check` re-runs every failed job.) A check still\n")
	b.WriteString("running is cancelled first by the re-run itself, and a cancel stops every\n")
	b.WriteString("job of that run — the output says which. It is never a way to retry a\n")
	b.WriteString("real failure without a fix: a pushed commit re-runs CI by itself. Never\n")
	b.WriteString("`gh` for this either.\n")
	return b.String()
}

// SliceTitleRule is the one sentence every text that tells an agent how to
// name slices carries — the workshop, new-project and slice (follow-ups)
// prompts, and the queue-work, queue-project and next-slice skills in their
// own copies — with the cap read from [domain.MaxSliceTitleLen], the one place
// the number is kept. Tests walk each template and skill for it.
var SliceTitleRule = fmt.Sprintf("A slice title names one change in eight words or fewer, in the "+
	"words of the person who asked for it — what they get, not how it is built. No file, type or "+
	"command names; no colon, dash or \"and\" joining several changes; no \"N fixes in one pass\". "+
	"At most %d characters. The list of what it covers goes in the brief.", domain.MaxSliceTitleLen)

// ProposalWithdrawnRule is what both gnat planning prompts — a project's
// workshop and a new project's — say of a proposal the user writes after: the
// app withdraws it (`nat plan-withdraw`) the moment they send, so the agent
// proposes again, whole, on every turn that answers them. Tests walk both
// templates for it.
const ProposalWithdrawnRule = "Once you have proposed, a message from the user withdraws the proposal —\n" +
	"the app takes it off screen the moment they send it. So every turn that\n" +
	"answers the user ends with plan-propose run again, the whole plan, even\n" +
	"unchanged: never assume your last proposal is still on screen.\n"

// acceptedProposalPassage tells a gnat-launched planning agent what an
// unaccepted proposal and an accepted one each are to its next revision: the
// first is replaced whole, the second is on the board and is changed only
// through the document's edit, move and remove lists — and that the plan is
// re-read before every revision. Nothing tells the agent of an accept: a
// slice on the board under one of its titles is how it learns of one, and a
// document creating that title again is refused whole. skills/queue-work/
// SKILL.md says the same in its own words.
func acceptedProposalPassage(projectID string) string {
	var b strings.Builder
	b.WriteString("While a proposal is unaccepted, a revised one replaces whichever is on\n")
	b.WriteString("screen, so send the whole plan again each time rather than a diff of it.\n")
	b.WriteString(ProposalWithdrawnRule)
	b.WriteString("Nothing tells you when the user accepts it. Re-read the plan before\n")
	b.WriteString("every revision, since the board may have moved while you worked:\n\n")
	fmt.Fprintf(&b, "    nat info --project %s\n", projectID)
	b.WriteString("\nA slice already on the board under one of your titles is there because\n")
	b.WriteString("the user accepted it: change it only through `edit`, `move` and `remove`\n")
	b.WriteString("by title, never by creating it again.\n")
	return b.String()
}

// namingPassage is the rule every text handed to an agent that writes about
// slices carries — slice, plan and new-project prompts, and every
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

// waitingPassage tells every agent nat launches to say when it has stopped on
// the user, since nothing else does: [Tmux.Activity] reads a live agent as
// working until it runs `nat agent-waiting`, and as working again once it runs
// `nat agent-working`. The two act on the caller's own pane and take no
// --project, which a pinned prompt's passage says outright, since every such
// prompt also says an unpinned command is refused; the new-project prompt,
// which has no project and never names the flag, is given it without that. A
// test walks every prompt for it; the skills do not carry it, since an agent
// run by hand is not in a pane nat launched.
func waitingPassage(pinned bool) string {
	s := "\n## Waiting on the user\n\n" +
		"Before you end a turn on a question, a decision or anything else only the\n" +
		"user can supply, run `nat agent-waiting`: without it the user sees an\n" +
		"agent still at work and will not look. On the next turn, once the user has\n" +
		"answered, run `nat agent-working` before doing anything else. They are not\n" +
		"for hand-back, follow-ups or a blocked note: those have their own commands\n" +
		"and their own place in the app.\n"
	if pinned {
		s += "\nBoth act on this session's own tmux pane and take no `--project`: they\n" +
			"are the one exception to pinning it.\n"
	}
	return s
}

// followUpBriefPassage says how a follow-up is written: as the brief of the
// slice it becomes if queued, since slice-triage files it as one verbatim.
// skills/next-slice/SKILL.md carries the same words in its own copy.
const followUpBriefPassage = "A follow-up is work the user has not asked for that you noticed and did\n" +
	"not do; anything this slice's own `Done when:` covers is this slice's\n" +
	"work, so do it rather than file it. Write each one as a slice brief: if\n" +
	"the user queues it, this text is the brief of a new slice, word for\n" +
	"word, read by an agent with nothing else. The title is an imperative\n" +
	"action in plain words, eight words or fewer, naming no file or type. The\n" +
	"body opens with one line saying the problem as the user would see it and\n" +
	"what it costs to leave it, then one line saying what you recommend\n" +
	"(queue it, fold it in now, or drop it) and why. Then the change — what,\n" +
	"where, instead of what — and a line starting `Done when:` saying how\n" +
	"anyone checks it is finished, as something they can see or run. Write a\n" +
	"decision, not a question: where there is a choice, pick one and name the\n" +
	"alternative rejected; no \"could\", \"might\", \"consider\" or \"worth looking\n" +
	"at\". Before filing one, check the plan for a later slice that already\n" +
	"covers it, and fold housekeeping you noticed — a flaky test, lint drift,\n" +
	"dead code — into one item or fix it now. If saying what to change needs\n" +
	"a look at the code, take that look now; if it genuinely needs\n" +
	"investigation, the investigation is the deliverable and `Done when:`\n" +
	"says what it produces.\n\n"

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

// writingPassage is how every agent nat launches writes what a person reads:
// for someone who set the goals but has not followed the code, the point
// first, in their words, with code kept out of prose. It is in every slice,
// plan and new-project prompt, beside namingPassage; every embedded skill
// carries the same words in its own copy. Tests walk each for it.
const writingPassage = "\n" +
	"## Writing for the user\n" +
	"\n" +
	"Everything you write that a person reads — a slice title or brief, a\n" +
	"question, a hand-back summary, a pull request description, a follow-up,\n" +
	"a note — is read by someone who set the goals and follows the progress\n" +
	"but has not followed the code, and may not read English as a first\n" +
	"language. They decide from your first sentence whether to read on, so\n" +
	"write for them, not for the engineer who will review the diff.\n" +
	"\n" +
	"- Lead with the point. The first sentence says what changes for them, or\n" +
	"  what you need from them. Detail comes after, never before.\n" +
	"- Use their words. Say things the way the brief and the user say them.\n" +
	"  Do not coin a name for something; where a new thing needs one, name it\n" +
	"  by what it does and say what it is the first time, in half a sentence.\n" +
	"  Use one name per thing throughout.\n" +
	"- Keep code out of prose. File paths, function and type names, flags,\n" +
	"  environment variables and identifiers go in a later detail section or\n" +
	"  the pull request body, never in a title, a question or an opening\n" +
	"  sentence. A command the user runs themselves is the exception.\n" +
	"- Write short, plain sentences: one idea each, about twenty words,\n" +
	"  common words (use, not utilise; show, not surface), no idioms. Say\n" +
	"  what something does, not how it is wired.\n" +
	"- Say what the reader gets. A fix is \"a link click no longer opens two\n" +
	"  tabs\", not the names of the two handlers that overlapped. A warning\n" +
	"  says what breaks for the user, not the mechanism.\n" +
	"\n" +
	"Before you send anything, check it: could someone who has never opened\n" +
	"the code say what this is about from the first sentence? If not, rewrite\n" +
	"the first sentence.\n" +
	"\n" +
	"For example. A title: not \"Catch the modified enters with a key monitor\n" +
	"— performKeyEquivalent never sees them\" but \"Make shift+enter insert a\n" +
	"newline in the agent terminal\". A summary line: not\n" +
	"\"DiffStore.sendComments now always sends the complete-slice --branch\n" +
	"instruction and runs slice-rework after agent-send succeeds\" but \"Review\n" +
	"comments sent to an agent now always ask it to hand the work back again,\n" +
	"so a slice cannot get stuck in review\". A question: not \"Where the\n" +
	"'already there' baseline comes from: a comment counts as new when no\n" +
	"`Sent back` names its URL …\" but \"Say a pull request already has five\n" +
	"comments when its agent starts. Should the agent be told about those\n" +
	"five, or only about new ones from now on? I recommend only new ones,\n" +
	"because you have already seen the five.\"\n"

// questionPassage is how a slice agent decides what to ask the user and how
// to shape a question a non-implementer can answer — and raises an
// architecture question before code rather than at hand-back. It is in every
// slice prompt; /next-slice carries the same words, less the closing
// sentence about `nat agent-waiting`, which it does not teach.
const questionPassage = "\n" +
	"## Asking the user\n" +
	"\n" +
	"Decide what you can, ask what you cannot. Ask only where different\n" +
	"answers would change the work materially, or before something\n" +
	"destructive or hard to undo; otherwise take the reading the brief and\n" +
	"the code best support, say so in one line, and carry on. A failing check\n" +
	"on your pull request, a flaky test you hit, or a loose end in code you\n" +
	"touched is part of this slice: fix it and say what you did, rather than\n" +
	"asking whether to. Do everything that does not depend on the answer\n" +
	"before you ask.\n" +
	"\n" +
	"Raise an architecture question — a decision neither the brief nor a Done\n" +
	"slice in the milestone settles — before you write code, not at\n" +
	"hand-back, where an hour of the wrong shape is already sunk. Never run\n" +
	"`complete-slice` on work whose shape is still unsettled that way: a\n" +
	"hand-back is a claim the shape is right.\n" +
	"\n" +
	"Shape every question so it can be answered without opening the code:\n" +
	"\n" +
	"- The first line is the decision, in one plain sentence, and why it\n" +
	"  matters to the user.\n" +
	"- Then the options, two to four, each with what the user gets and gives\n" +
	"  up, in a line. Put the one you recommend first and say why. Letter or\n" +
	"  number them so the reply can be \"1b\".\n" +
	"- No code identifiers, file paths or flags, unless the user must choose\n" +
	"  between them. No term the brief or the user has not used, unless you\n" +
	"  say what it means in the same sentence.\n" +
	"- One decision per question, under about 120 words, and the question\n" +
	"  before any report, never buried after one.\n" +
	"- Never ask the user to observe what they cannot (what a run printed,\n" +
	"  which input device failed): find out yourself, or say exactly what to\n" +
	"  click and what each outcome would mean.\n" +
	"- Never ask what is already decided: by the brief, a design it cites,\n" +
	"  the project's rules, or an answer earlier in this session. Read what\n" +
	"  the brief cites before you choose an approach; if it names something\n" +
	"  that does not exist, say so in your first message, before building\n" +
	"  anything.\n" +
	"\n" +
	"Do not end a turn on a status line (\"waiting on CI\", \"I'll check again\n" +
	"in five\"): wait inside the turn, or end with what you are waiting for\n" +
	"and when you will report. `nat agent-waiting` is for a question on\n" +
	"screen.\n"

// briefShapePassage is how a planning agent shapes a brief: a summary
// paragraph of its own first — the one gnat shows as the task's description,
// and the one [domain.CheckBriefOpening] caps — then what is settled, what is
// out of scope, a hint where to look and Done when. It is in every planning
// prompt after the title rule; queue-work and queue-project carry the same
// words. Tests walk each for it.
const briefShapePassage = "\n" +
	"## Writing a brief\n" +
	"\n" +
	"A brief opens with a summary: one or two sentences, in a paragraph of\n" +
	"its own, saying what changes for the user and why. The app shows this\n" +
	"paragraph as the task's description, so it must stand alone, and nothing\n" +
	"in it names a file, a function, a type or a language. A brief whose\n" +
	"first paragraph runs past sixty words is refused.\n" +
	"\n" +
	"Then, in short paragraphs or bullets:\n" +
	"\n" +
	"- What is settled: decisions already made — the user's, a design\n" +
	"  document's, an earlier slice's — stated as rules, with any term the\n" +
	"  user may not know explained in a clause. Where a rule rests on\n" +
	"  something the user has not confirmed, or on a number from a survey or\n" +
	"  an earlier slice, say so and keep the number out of Done when.\n" +
	"- What is out of scope.\n" +
	"- Where to look, if it helps: a starting point in the code, labelled as\n" +
	"  a hint (\"probably in …\"), never a line number and never a mechanism to\n" +
	"  use. Say what the code must do, not how to write it: a prescribed\n" +
	"  approach goes stale, and when it does it sends the agent down the\n" +
	"  wrong path. Name a method only where it is a real requirement, and say\n" +
	"  why. Do not fix a visual choice — a glyph, a colour, a badge rather\n" +
	"  than a line — unless the user chose it: say what it must tell the user\n" +
	"  and let the review of the rendered result settle the rest.\n" +
	"- Done when: how anyone checks it is finished, as things the user can\n" +
	"  see or run.\n" +
	"\n" +
	"Keep a brief under about 400 words. Leave out anecdotes, statistics and\n" +
	"history; one clause of why is enough. Before you name a slice, command,\n" +
	"file or feature as existing, check that it exists: read the plan, grep\n" +
	"the code. A slice is a change the user can see working on its own: do\n" +
	"not split work by layer, test surface or to allow parallel agents, and\n" +
	"put plumbing in the slice that uses it. When in doubt, fewer slices.\n"

// summaryPassage says who reads a hand-back's --summary — the user on the
// task log, and later agents in the milestone's digest — and so how it
// opens. /next-slice carries the same words.
const summaryPassage = "`--summary` is shown to the user on the task log and quoted to later\n" +
	"agents in the milestone's digest. Its first bullet says what now works\n" +
	"or what changed, in the user's words, as one sentence they could read\n" +
	"alone. Then at most three bullets: decisions you made and anything the\n" +
	"reviewer must check. Identifiers only where a reviewer needs them to\n" +
	"find the place. Never a narrative of the session.\n"

// prDescriptionPassage says how a hand-back's --pr-description is written:
// ready to publish, the plain why first. /next-slice carries the same words.
const prDescriptionPassage = "`--pr-description` is what the pull request is opened with: its first\n" +
	"line is the title and the rest the body, ready to publish. The first\n" +
	"paragraph says what the change does and why, in plain words, for whoever\n" +
	"reviews it on GitHub. Technical detail follows under its own heading. No\n" +
	"test counts, no list of files touched, no report of your session.\n"
