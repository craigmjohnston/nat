import Foundation
import NatKit

// MARK: - The plan

extension Fixtures {
    /// The project every plan fixture belongs to.
    public static let projectID = "f1x70000-0000-4000-8000-000000000001"

    public static let project = Project(
        id: projectID,
        name: "notion-agent-tracker",
        conventions: """
        A Go TUI over Notion for tracking project work executed by Claude Code agents.

        - A **slice** is a unit of work completable in one agent session.
        - Branch per slice; never push to main.
        - Gate before claiming done: `go vet ./... && go test -race ./...`.
        """
    )

    public static let milestones: [Milestone] = [
        Milestone(id: "M1: Foundations", name: "M1: Foundations", order: 0, status: "Done"),
        Milestone(id: "M2: Review flow", name: "M2: Review flow", order: 1, status: "Active"),
        Milestone(id: "M3: View gallery", name: "M3: View gallery", order: 2, status: "Queued"),
    ]

    // Slice IDs, named rather than spelled out at each use so a fixture that
    // wants to say something about one slice — a live agent, a diff, a pull
    // request — names the same slice the plan does.

    /// A finished slice of the finished milestone.
    public static let shellSliceID = "f1x75111-0000-4000-8000-000000000001"
    /// A finished slice of the milestone still moving.
    public static let railSliceID = "f1x75111-0000-4000-8000-000000000002"
    /// In progress with an agent working on it.
    public static let diffPaneSliceID = "f1x75111-0000-4000-8000-000000000003"
    /// In progress with an agent stopped for input.
    public static let activitySliceID = "f1x75111-0000-4000-8000-000000000004"
    /// In progress, no agent, waiting on a dependency.
    public static let commentsSliceID = "f1x75111-0000-4000-8000-000000000005"
    /// In progress, no agent, nothing in its way.
    public static let expandSliceID = "f1x75111-0000-4000-8000-000000000006"
    /// Handed back on a branch — the slice every diff fixture is about.
    public static let mergeBoxSliceID = "f1x75111-0000-4000-8000-000000000007"
    /// Done with its pull request still open — the slice every PR fixture is
    /// about.
    public static let approveSliceID = "f1x75111-0000-4000-8000-000000000008"
    /// Todo and blocked on a dependency.
    public static let cacheSliceID = "f1x75111-0000-4000-8000-000000000009"
    /// Todo, in the milestone nothing has started in.
    public static let fixturesSliceID = "f1x75111-0000-4000-8000-000000000010"
    /// Todo and blocked, in the milestone nothing has started in.
    public static let gallerySliceID = "f1x75111-0000-4000-8000-000000000011"

    /// The plan's slices: every status the board draws, and every shape the
    /// rail sorts them into — two finished milestones' worth of Done, a slice
    /// in each of the four ACTIVE readings, one handed back and one waiting on
    /// its merge in NEEDS REVIEW, and two blocked rows.
    public static let slices: [Slice] = [
        Slice(
            id: shellSliceID,
            name: "Bootstrap the SwiftUI shell",
            status: "Done",
            milestoneID: "M1: Foundations",
            assignee: "Craig Johnston",
            pr: "https://github.com/craigmjohnston/notion-agent-tracker/pull/101",
            url: "https://notion.so/\(shellSliceID)",
            branch: "slice/bootstrap-the-swiftui-shell",
            blocked: false,
            handedBack: false
        ),
        Slice(
            id: railSliceID,
            name: "Read the plan through nat info",
            status: "Done",
            milestoneID: "M1: Foundations",
            assignee: "Craig Johnston",
            pr: "https://github.com/craigmjohnston/notion-agent-tracker/pull/102",
            url: "https://notion.so/\(railSliceID)",
            branch: "slice/read-the-plan-through-nat-info",
            blocked: false,
            handedBack: false
        ),
        Slice(
            id: diffPaneSliceID,
            name: "Wire the diff pane to the store",
            status: "In progress",
            milestoneID: "M2: Review flow",
            assignee: "Craig Johnston",
            pr: "",
            url: "https://notion.so/\(diffPaneSliceID)",
            branch: nil,
            blocked: false,
            handedBack: false
        ),
        Slice(
            id: activitySliceID,
            name: "Poll tmux for agent activity",
            status: "In progress",
            milestoneID: "M2: Review flow",
            assignee: "Craig Johnston",
            pr: "",
            url: "https://notion.so/\(activitySliceID)",
            branch: nil,
            blocked: false,
            handedBack: false
        ),
        Slice(
            id: commentsSliceID,
            name: "Send review comments to the agent",
            status: "In progress",
            milestoneID: "M2: Review flow",
            assignee: "Craig Johnston",
            pr: "",
            url: "https://notion.so/\(commentsSliceID)",
            branch: nil,
            dependsOn: [diffPaneSliceID],
            blocked: true,
            handedBack: false
        ),
        Slice(
            id: expandSliceID,
            name: "Expand the context around a hunk",
            status: "In progress",
            milestoneID: "M2: Review flow",
            assignee: "Craig Johnston",
            pr: "",
            url: "https://notion.so/\(expandSliceID)",
            branch: nil,
            blocked: false,
            handedBack: false
        ),
        Slice(
            id: mergeBoxSliceID,
            name: "Draw the merge box on the PR tab",
            status: "In progress",
            milestoneID: "M2: Review flow",
            assignee: "Craig Johnston",
            pr: "",
            url: "https://notion.so/\(mergeBoxSliceID)",
            branch: diffBranch,
            repo: "/Users/craig/Projects/notion-agent-tracker",
            blocked: false,
            handedBack: true
        ),
        Slice(
            id: approveSliceID,
            name: "Approve opens the pull request",
            status: "Done",
            milestoneID: "M2: Review flow",
            assignee: "Craig Johnston",
            pr: prURL,
            url: "https://notion.so/\(approveSliceID)",
            branch: "slice/approve-opens-the-pull-request",
            repo: "/Users/craig/Projects/notion-agent-tracker",
            blocked: false,
            handedBack: false
        ),
        Slice(
            id: cacheSliceID,
            name: "Cache the plan on disk",
            status: "Todo",
            milestoneID: "M2: Review flow",
            assignee: "",
            pr: "",
            url: "https://notion.so/\(cacheSliceID)",
            dependsOn: [commentsSliceID],
            blocked: true,
            handedBack: false
        ),
        Slice(
            id: fixturesSliceID,
            name: "Build a fixture library of canned app states",
            status: "Todo",
            milestoneID: "M3: View gallery",
            assignee: "",
            pr: "",
            url: "https://notion.so/\(fixturesSliceID)",
            blocked: false,
            handedBack: false
        ),
        Slice(
            id: gallerySliceID,
            name: "Run the gallery from the fixtures",
            status: "Todo",
            milestoneID: "M3: View gallery",
            assignee: "",
            pr: "",
            url: "https://notion.so/\(gallerySliceID)",
            dependsOn: [fixturesSliceID],
            blocked: true,
            handedBack: false
        ),
    ]

    /// One slice of the plan by ID, for a view that takes the slice rather
    /// than reading it off a selection. It traps on an ID the plan does not
    /// hold, which is a fixture naming a slice that was renamed out from
    /// under it — a story that draws the wrong slice silently is worse.
    public static func slice(_ id: String) -> Slice {
        guard let slice = slices.first(where: { $0.id == id }) else {
            preconditionFailure("no fixture slice with id \(id)")
        }
        return slice
    }

    /// A realistic loaded project: the plan above, mid-flight.
    public static let projectInfo = ProjectInfo(
        project: project,
        milestones: milestones,
        slices: slices
    )

    /// A project with a plan and nothing in it — what a freshly created
    /// project looks like before anything has been queued into it.
    public static let emptyProjectInfo = ProjectInfo(
        project: Project(id: projectID, name: "notion-agent-tracker", conventions: ""),
        milestones: [],
        slices: []
    )
}

// MARK: - What the live readings say about it

extension Fixtures {
    /// The activity poll's reading: one agent working, one stopped for input.
    /// Keyed by slice ID, exactly as `ActivityStore.agents` is.
    public static let agentStatuses: [AgentStatus] = [
        AgentStatus(
            sliceID: diffPaneSliceID,
            session: TmuxSession.name(forSlicePageID: diffPaneSliceID),
            activity: .working
        ),
        AgentStatus(
            sliceID: activitySliceID,
            session: TmuxSession.name(forSlicePageID: activitySliceID),
            activity: .waiting
        ),
    ]

    /// The same reading in the words `buildRailModel` takes it in.
    public static var liveAgents: [String: AgentActivity] { [
        diffPaneSliceID: .working,
        activitySliceID: .waiting,
    ] }

    /// When each of those agents was first seen, so an ACTIVE row has an
    /// elapsed time to draw.
    public static let agentStarts: [String: Date] = [
        diffPaneSliceID: minutesAgo(74),
        activitySliceID: minutesAgo(6),
    ]

    /// `ReviewStatsStore.stats`: the handed-back branch's own diff tally.
    /// Counted off `sliceDiff` rather than typed out beside it, since that is
    /// exactly what the store does with the very same read — a number written
    /// here by hand would be one the diff fixture could drift away from.
    public static var reviewStats: [String: String] {
        [mergeBoxSliceID: "+\(diffAdds) \u{2212}\(diffDels)"]
    }

    /// `ReviewStatsStore.fileCounts`, from the same read as `reviewStats`.
    public static var reviewFileCounts: [String: Int] {
        [mergeBoxSliceID: sliceDiff.files.count]
    }

    /// The handed-back branch's own totals, off the diff fixture itself.
    public static var diffAdds: Int { sliceDiff.files.reduce(0) { $0 + $1.adds } }
    public static var diffDels: Int { sliceDiff.files.reduce(0) { $0 + $1.dels } }

    /// `ReviewStatsStore.prReadiness`: the slices whose pull request gh
    /// positively reads as open. The approved slice is Done and still waiting
    /// on its merge, which is what keeps it in NEEDS REVIEW.
    public static let prReadiness: [String: String] = [
        approveSliceID: "ready to merge",
    ]

    /// `nat pr-status --json` saying the same thing.
    public static let prStatusDoc = PRStatusDoc(slices: [
        PRStatusSlice(
            sliceID: approveSliceID,
            name: "Approve opens the pull request",
            pr: prURL,
            readiness: "ready to merge"
        ),
        PRStatusSlice(
            sliceID: shellSliceID,
            name: "Bootstrap the SwiftUI shell",
            pr: "https://github.com/craigmjohnston/notion-agent-tracker/pull/101",
            readiness: "unread"
        ),
    ])
}

// MARK: - Rail models

extension Fixtures {
    /// The rail as it is drawn for the loaded plan — built through
    /// `buildRailModel` itself rather than written out by hand, so the fixture
    /// can never drift from the rule the app draws by.
    public static var railModel: RailModel {
        buildRailModel(
            from: projectInfo,
            liveAgents: liveAgents,
            reviewStats: reviewStats,
            reviewFileCounts: reviewFileCounts,
            prReadiness: prReadiness,
            agentStarts: agentStarts,
            now: now
        )
    }

    /// The rail of a project with nothing in it: every section empty.
    public static var emptyRailModel: RailModel {
        buildRailModel(from: emptyProjectInfo, liveAgents: [:], now: now)
    }

    /// The rail with no live reading behind it at all — the board a second
    /// after it opened, or one on a machine with no tmux and no gh: every
    /// ACTIVE row reads by its page alone, and the Done slice awaiting its
    /// merge is out of NEEDS REVIEW rather than in it.
    public static var unreadRailModel: RailModel {
        buildRailModel(from: projectInfo, liveAgents: [:], now: now)
    }

    /// The WORKSHOP row with a planning agent working on it.
    public static var workshopEntry: WorkshopEntry? {
        buildWorkshopEntry(activity: .working, isLaunching: false, firstSeen: minutesAgo(12), now: now)
    }
}

// MARK: - Load states

extension Fixtures {
    /// Every state the plan can be read in, so a view that draws one can be
    /// shown in all of them.
    public static var loadStateIdle: LoadState { .idle }
    public static var loadStateLoading: LoadState { .loading }
    public static var loadStateLoaded: LoadState { .loaded(projectInfo) }
    public static var loadStateEmpty: LoadState { .loaded(emptyProjectInfo) }
    /// A read that failed with nothing behind it — the only case with an
    /// empty board to show.
    public static var loadStateFailed: LoadState { .failed(loadErrorMessage, previous: nil) }
    /// A read that failed over a plan already on screen, which is what the
    /// board keeps drawing under the error.
    public static var loadStateStale: LoadState {
        .failed(loadErrorMessage, previous: projectInfo)
    }

    /// What a failed read says — nat's own refusal, not a sentence invented
    /// for a fixture.
    public static let loadErrorMessage =
        "nat info: Notion API: 502 Bad Gateway (request 0c3f1f2a-7b21-4a5e-9a0d-2f6b8c1d4e77)"
}
