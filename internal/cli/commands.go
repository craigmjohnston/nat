package cli

import (
	"context"
	"strings"
)

// command is one word of a command line: a command nat runs, a group of
// commands under it, or — `slice checks` — both, the group's own command
// run when the next word names none of its subcommands.
type command struct {
	name string
	run  func(ctx context.Context, args []string, env Env) error
	subs []*command
	// was is the hyphenated name the command was typed as before every
	// command became nested words, refused naming the new form.
	was string
}

// noCtx adapts a command that needs no context.
func noCtx(f func([]string, Env) error) func(context.Context, []string, Env) error {
	return func(_ context.Context, args []string, env Env) error { return f(args, env) }
}

// group is a command that only gathers others.
func group(name string, subs ...*command) *command { return &command{name: name, subs: subs} }

// leaf is a command with no subcommands.
func leaf(name, was string, run func(context.Context, []string, Env) error) *command {
	return &command{name: name, run: run, was: was}
}

// commands is every command nat runs, thing first and action last; the
// machine-wide ones stay one word.
var commands = []*command{
	leaf("setup", "", noCtx(setup)),
	leaf("paths", "", noCtx(paths)),
	leaf("status", "", noCtx(status)),
	leaf("usage", "", noCtx(usage)),
	leaf("info", "", info),
	leaf("run", "", runCmd),
	group("storage",
		leaf("usage", "storage-usage", storageUsage)),
	group("claude",
		leaf("version", "claude-version", claudeVersion),
		leaf("update", "claude-update", claudeUpdate)),
	group("slice",
		leaf("show", "slice-show", sliceShow),
		leaf("status", "slice-status", sliceStatus),
		leaf("launch", "slice-launch", sliceLaunch),
		leaf("approve", "slice-approve", sliceApprove),
		leaf("rework", "slice-rework", sliceRework),
		leaf("resume", "slice-resume", sliceResume),
		leaf("cancel", "slice-cancel", sliceCancel),
		leaf("diff", "slice-diff", sliceDiff),
		leaf("file", "slice-file", sliceFile),
		leaf("edit", "slice-edit", sliceEdit),
		leaf("move", "slice-move", sliceMove),
		leaf("reorder", "slice-reorder", sliceReorder),
		leaf("delete", "slice-delete", sliceDelete),
		leaf("add", "slice-add", sliceAdd),
		leaf("depends", "slice-depends", sliceDepends),
		leaf("followups", "slice-followups", sliceFollowUps),
		leaf("triage", "slice-triage", sliceTriage),
		leaf("visuals", "slice-visuals", sliceVisuals),
		leaf("repo", "slice-repo", sliceRepoCmd),
		leaf("worktree", "slice-worktree", sliceWorktreeCmd),
		leaf("note", "slice-note", sliceNote),
		&command{name: "checks", run: sliceChecks, was: "slice-checks", subs: []*command{
			leaf("rerun", "slice-checks-rerun", sliceChecksRerun),
			leaf("cancel", "slice-checks-cancel", sliceChecksCancel),
		}},
		leaf("next", "next-slice", nextSlice),
		leaf("start", "start-slice", startSlice),
		leaf("complete", "complete-slice", completeSlice),
		leaf("release", "release-slice", releaseSlice)),
	group("session",
		leaf("launch", "session-launch", sessionLaunch),
		leaf("list", "session-list", sessionList),
		leaf("status", "session-status", sessionStatus),
		leaf("diff", "session-diff", sessionDiff)),
	group("agent",
		leaf("send", "agent-send", agentSend),
		leaf("interrupt", "agent-interrupt", agentInterrupt),
		leaf("kill", "agent-kill", agentKill),
		leaf("waiting", "agent-waiting", noCtx(agentWaiting)),
		leaf("working", "agent-working", noCtx(agentWorking))),
	group("project",
		leaf("create", "project-create", projectCreate),
		leaf("mirror", "project-mirror", projectMirror),
		group("open",
			leaf("folder", "project-open-folder", projectOpenFolder)),
		group("done",
			leaf("clear", "done-clear", doneClear))),
	group("milestone",
		leaf("add", "milestone-add", milestoneAdd),
		leaf("rename", "milestone-rename", milestoneRename),
		leaf("remove", "milestone-remove", milestoneRemove),
		leaf("move", "milestone-move", milestoneMove)),
	group("plan",
		leaf("apply", "plan-apply", planApply),
		leaf("propose", "plan-propose", planPropose),
		leaf("proposal", "plan-proposal", planProposal),
		leaf("accept", "plan-accept", planAccept),
		leaf("withdraw", "plan-withdraw", planWithdraw)),
	group("pr",
		leaf("view", "pr-view", prView),
		leaf("comment", "pr-comment", prComment),
		leaf("edit", "pr-edit", prEdit),
		leaf("reviewers", "pr-reviewers", prReviewers),
		leaf("merge", "pr-merge", prMerge),
		leaf("status", "pr-status", prStatus)),
	group("config",
		leaf("show", "config-show", noCtx(configShow)),
		leaf("set", "config-set", noCtx(configSet))),
	group("source",
		leaf("list", "source-list", sourceList),
		leaf("setup", "source-setup", sourceSetup),
		leaf("action", "source-action", sourceAction)),
	group("container",
		leaf("show", "container-show", containerShow)),
	group("plugin",
		leaf("list", "plugin-list", pluginList),
		leaf("install", "plugin-install", pluginInstall),
		leaf("uninstall", "plugin-uninstall", pluginUninstall),
		group("source",
			leaf("add", "plugin-source-add", noCtx(pluginSourceAdd)),
			leaf("remove", "plugin-source-remove", noCtx(pluginSourceRemove)))),
	group("workshop",
		leaf("launch", "workshop-launch", workshopLaunch)),
	group("scratch",
		leaf("open", "scratch-open", scratchOpen)),
	group("notion",
		leaf("search", "notion-search", notionSearch)),
}

// find is the command of cmds called name, or nil.
func find(cmds []*command, name string) *command {
	for _, c := range cmds {
		if c.name == name {
			return c
		}
	}
	return nil
}

// renamed maps each hyphenated name nat used to take to the words it is
// typed as now.
func renamed() map[string]string {
	m := map[string]string{}
	var walk func(cmds []*command, path []string)
	walk = func(cmds []*command, path []string) {
		for _, c := range cmds {
			p := append(append([]string{}, path...), c.name)
			if c.was != "" {
				m[c.was] = strings.Join(p, " ")
			}
			walk(c.subs, p)
		}
	}
	walk(commands, nil)
	return m
}

// groupUsage is the part of [Usage] for the commands under path: every entry
// whose command line opens on those words, under a heading naming the group.
func groupUsage(path []string) string {
	prefix := "  nat " + strings.Join(path, " ") + " "
	var b strings.Builder
	b.WriteString("usage of nat " + strings.Join(path, " ") + ":\n")
	in := false
	for _, line := range strings.Split(Usage, "\n") {
		switch {
		case strings.HasPrefix(line, "  nat"):
			in = strings.HasPrefix(line+" ", prefix)
		case !strings.HasPrefix(line, "   "):
			in = false
		}
		if in {
			b.WriteString(line + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}
