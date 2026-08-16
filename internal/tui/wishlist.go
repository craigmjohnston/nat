package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// wishlistLoadedMsg carries the pending wishlist of the active project's page,
// or the read that failed instead. A failure is carried here rather than as a
// notionErrMsg because the wishlist is an indicator beside the plan, not the
// plan: a page nat cannot read the wishlist off is a board with no count on it,
// not a board replaced by an error.
type wishlistLoadedMsg struct {
	items []notion.WishlistItem
	err   error
}

// fetchWishlist reads a project page's pending wishlist items. It rides the
// load pipeline rather than being asked for separately, so the count refreshes
// wherever the plan does — the refresh key today, the background poll once
// there is one — and never on its own.
func (a *App) fetchWishlist(pageID string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		items, err := client.Wishlist(context.Background(), pageID)
		return wishlistLoadedMsg{items: items, err: err}
	}
}

// wishlistLoaded takes what the indicator counts, and what the workshop key
// launches an agent on, from a finished read. A
// failure leaves no indicator and is logged rather than shown: the board it
// sits beside has loaded, and the wishlist is not what the user asked for.
func (a *App) wishlistLoaded(msg wishlistLoadedMsg) {
	if msg.err != nil {
		logging.Error("could not read the project's wishlist", "error", msg.err)
		a.wishlist = nil
		return
	}
	a.wishlist = msg.items
}

// wishlistIndicator is what the status line says about the wishlist: how many
// items are waiting on it, and the key that opens a workshop session on them.
// Nothing at all when the wishlist is empty or could not be read — the count is
// a nudge, and a zero sitting on the bar is one more thing to read past.
func (a *App) wishlistIndicator() string {
	if len(a.wishlist) == 0 {
		return ""
	}
	items := "items"
	if len(a.wishlist) == 1 {
		items = "item"
	}
	help := a.keys.Workshop.Help()
	return a.styles.StatusDesc.Render(fmt.Sprintf("%d wishlist %s", len(a.wishlist), items)) +
		a.styles.HintSep.Render(" · ") +
		a.styles.StatusKey.Render(help.Key) + " " + a.styles.StatusDesc.Render(help.Desc)
}

// workshopFlow is what W does: launches a planning agent on the wishlist
// items, with no form between the key and the session — the items are the
// request, so there is nothing to ask. The pane is attached on launch, as a
// typed planning launch is.
//
// It does nothing at all — no note, no toast — when there is nothing to
// workshop or nowhere to put the session: an empty wishlist has no indicator
// on the bar naming the key, and a planning agent already running is the one
// holding the plan in its head, which W neither replaces nor toggles. Toggling
// is w's job, and that key is still there.
func (a *App) workshopFlow() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || a.launcher == nil || a.busy || len(a.wishlist) == 0 {
		return nil
	}
	if a.live[agent.PlanSentinel] != "" {
		return nil
	}
	return launchWishlistAgent(a.launcher, project.Name, expandHome(project.WorkingDir), a.wishlist,
		trimModel(a.cfg.WorkshopAgent))
}

// launchWishlistAgent writes the planning prompt out with the wishlist folded
// into it and starts the detached session that reads it. It is the wishlist's
// half of launchPlanAgent, and comes back as the same message, so the pane and
// the failure reporting are handled in one place.
func launchWishlistAgent(l AgentLauncher, projectName, workdir string, items []notion.WishlistItem, m config.AgentModel) tea.Cmd {
	return func() tea.Msg {
		file, err := agent.WritePromptFile(agent.PlanSession, agent.WishlistPrompt(projectName, workdir, items))
		if err != nil {
			return agentLaunchedMsg{err: fmt.Errorf("launch planning agent: %w", err)}
		}
		if err := l.Launch(agent.PlanSession, workdir, file, agent.PlanSentinel, m); err != nil {
			return agentLaunchedMsg{err: err}
		}
		return agentLaunchedMsg{slice: planSlice(), session: agent.PlanSession, attach: true}
	}
}
