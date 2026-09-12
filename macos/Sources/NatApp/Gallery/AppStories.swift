import SwiftUI
import NatKit
import NatFixtures

/// The stories the gallery ships with.
///
/// It lives in `NatApp` rather than in `NatKit` for the same reason the views
/// do: a story is a view, and the views are here. What it draws is entirely
/// `NatFixtures` — a pinned clock, a canned plan, a client that answers from
/// memory — so a run reaches no Notion, no `nat` and no tmux, and renders the
/// same pixels on any machine.
///
/// Three to begin with, one per surface with a state worth looking at: the
/// whole window loaded, the pane a review is read in, and the pane a pull
/// request is read in. Adding a fourth is a `Story` in this array and nothing
/// else.
/// `@MainActor` because a story's content is: every one of these builds a
/// view over a fixture app model, and both are the main actor's.
@MainActor
enum AppStories {
    static let catalog = StoryCatalog([
        // The mock's own canvas size — every metric in the shell was chosen
        // at 1360×840, so this is the one size the whole window is worth
        // drawing at.
        Story(name: "window-shell", size: CGSize(width: 1360, height: 840)) {
            WindowShellView(appModel: await Fixtures.startedAppModel())
        },

        // A pane rather than a window, so the size is the pane's: wide
        // enough for the diff's sidebar and its widest fixture line, tall
        // enough for more than one file box.
        Story(name: "diff-handed-back", size: CGSize(width: 1040, height: 680)) {
            let appModel = await Fixtures.startedAppModel()
            let slice = Fixtures.slices.first { $0.id == Fixtures.mergeBoxSliceID }!
            return DiffTabView(appModel: appModel, slice: slice)
            .surface(.window)
        },

        // The pull request the merge box says yes to — the reading with
        // something in every one of the pane's sections.
        Story(name: "pr-ready-to-merge", size: CGSize(width: 1040, height: 680)) {
            let appModel = await Fixtures.startedAppModel()
            let slice = Fixtures.slices.first { $0.id == Fixtures.approveSliceID }!
            return PRTabView(appModel: appModel, slice: slice)
            .surface(.window)
        },
    ])
}
