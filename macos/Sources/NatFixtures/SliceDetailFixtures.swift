import Foundation
import NatKit

extension Fixtures {
    /// The brief of the handed-back slice — the markdown the Brief tab
    /// renders, with the shapes a real brief has: headings, a list, a fenced
    /// block and inline code.
    public static let mergeBoxBrief = """
    Draw GitHub's merge box on the PR tab: three verdicts answering "can this
    merge" — the review decision, the checks, and the branch itself.

    ## Acceptance

    - Each verdict is one line with a mark and a colour.
    - The heading says what the three come to, coloured by the worst of them.
    - The checks line is `checkRollup` itself rather than a second reading.
    - A merged or closed pull request replaces the section with that ending.

    ```
    swift test --package-path macos
    ```
    """

    /// The handed-back slice in full, as `nat slice-show --json` reports it.
    public static let sliceDetail = SliceDetail(
        id: mergeBoxSliceID,
        name: "Draw the merge box on the PR tab",
        url: "https://notion.so/\(mergeBoxSliceID)",
        status: "In progress",
        milestone: "M2: Review flow",
        assignee: "Craig Johnston",
        branch: diffBranch,
        repo: "/Users/craig/Projects/notion-agent-tracker",
        base: "origin/main",
        pr: nil,
        dependsOn: nil,
        blocked: false,
        handedBack: true,
        state: "awaiting review",
        brief: mergeBoxBrief
    )

    /// A slice that cannot start yet, so the Brief tab has a blocked one to
    /// draw with the slices it waits on named.
    public static let blockedSliceDetail = SliceDetail(
        id: cacheSliceID,
        name: "Cache the plan on disk",
        url: "https://notion.so/\(cacheSliceID)",
        status: "Todo",
        milestone: "M2: Review flow",
        assignee: "",
        branch: nil,
        repo: "/Users/craig/Projects/notion-agent-tracker",
        base: "origin/main",
        pr: nil,
        dependsOn: [commentsSliceID],
        blocked: true,
        handedBack: false,
        state: "blocked",
        brief: "Keep each project's last-good plan on disk so the board draws from it while the fresh read is in flight."
    )

    /// A slice nobody has written a brief for — the empty state the Brief tab
    /// draws its own note in place of.
    public static let brieflessSliceDetail = SliceDetail(
        id: gallerySliceID,
        name: "Run the gallery from the fixtures",
        url: "https://notion.so/\(gallerySliceID)",
        status: "Todo",
        milestone: "M3: View gallery",
        assignee: "",
        blocked: true,
        handedBack: false,
        brief: ""
    )

    /// The three follow-ups the activity slice's agent proposed before
    /// handing back — the mock's own: one to queue, one to fold in, one to
    /// drop.
    public static let proposedFollowUps: [FollowUp] = [
        FollowUp(
            index: 1,
            title: "Persist the conversation split width per project",
            brief: "The split's width lives under one AppStorage key, so every project shares it. "
                + "The PR sidebar has the same problem. Store both per project the way pane widths already are."
        ),
        FollowUp(
            index: 2,
            title: "Render the emoji picker open in a gallery story",
            brief: "The picker is open-state only and no story draws it, so StoryNamesTests can't guard it. "
                + "Add pr-composer-emoji with the picker open over the composer."
        ),
        FollowUp(
            index: 3,
            title: "Remove the dead reply-threading code in PRConversationView",
            brief: "replyTargets and its two helpers haven't been read since the inline composer landed. "
                + "Delete them and the fake in FakeRunner that feeds them."
        ),
    ]

    /// The activity slice with its agent waiting on a decision about those
    /// follow-ups — not in `sliceDetails`, so only a client built with
    /// `followUpsSliceDetails` draws the sidebar.
    public static let followUpsSliceDetail = SliceDetail(
        id: activitySliceID,
        name: "Poll tmux for agent activity",
        url: "https://notion.so/\(activitySliceID)",
        status: "In progress",
        milestone: "M2: Review flow",
        assignee: "Craig Johnston",
        blocked: false,
        handedBack: false,
        state: "in progress",
        brief: "Poll tmux every second for each agent's activity.",
        followUps: proposedFollowUps
    )

    /// `sliceDetails` with the activity slice's follow-ups added.
    public static var followUpsSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([activitySliceID: followUpsSliceDetail]) { _, new in new }
    }

    /// A merged slice with a history: handed back three times, sent back
    /// with comments between them twice, two follow-ups triaged on the way,
    /// then approved and merged — `slice-show`'s `events` for it, in order.
    public static let taskLogEvents: [TaskLogEvent] = [
        TaskLogEvent(.handedBack, note: "The shell window and its three panes, empty states in each."),
        TaskLogEvent(.sentBack, note: "Sources/NatApp/NatApp.swift, line 42: the window should remember its frame."),
        TaskLogEvent(.handedBack, note: "Window frame autosaved under the scene's id."),
        TaskLogEvent(.followUps, followUps: [
            TaskFollowUp(index: 1, title: "Restore the last selected project on launch", decision: .queued,
                         link: "https://notion.so/f1x7queued"),
            TaskFollowUp(index: 2, title: "Drop the unused toolbar style", decision: .dropped),
        ]),
        TaskLogEvent(.sentBack, note: "Sources/NatApp/Views/ShellView.swift, lines 10-14: use the design's 32pt titlebar."),
        TaskLogEvent(.handedBack, note: "Titlebar at 32pt, traffic lights recentred."),
        TaskLogEvent(.approved, pr: "https://github.com/craigmjohnston/notion-agent-tracker/pull/101"),
        TaskLogEvent(.merged),
    ]

    /// The finished shell slice, read with that history.
    public static let taskLogSliceDetail = SliceDetail(
        id: shellSliceID,
        name: "Bootstrap the SwiftUI shell",
        url: "https://notion.so/\(shellSliceID)",
        status: "Done",
        milestone: "M1: Foundations",
        assignee: "Craig Johnston",
        branch: "slice/bootstrap-the-swiftui-shell",
        pr: "https://github.com/craigmjohnston/notion-agent-tracker/pull/101",
        blocked: false,
        handedBack: false,
        state: "done",
        brief: "Stand up the SwiftUI shell: the window, its three panes and their empty states.",
        events: taskLogEvents
    )

    /// `sliceDetails` with the shell slice's history added.
    public static var taskLogSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([shellSliceID: taskLogSliceDetail]) { _, new in new }
    }

    /// Every slice a fixture has a detail for, keyed the way
    /// `SliceDetailStore` asks for one.
    public static var sliceDetails: [String: SliceDetail] {
        [
            mergeBoxSliceID: sliceDetail,
            cacheSliceID: blockedSliceDetail,
            gallerySliceID: brieflessSliceDetail,
        ]
    }

    // MARK: - Load states

    public static let sliceDetailStateIdle: SliceDetailLoadState = .idle
    public static let sliceDetailStateLoading: SliceDetailLoadState = .loading(stale: nil)
    public static let sliceDetailStateLoaded: SliceDetailLoadState = .loaded(sliceDetail)
    public static let sliceDetailStateFailed: SliceDetailLoadState =
        .failed(sliceDetailErrorMessage, previous: nil)
    public static let sliceDetailStateStale: SliceDetailLoadState =
        .failed(sliceDetailErrorMessage, previous: sliceDetail)

    public static let sliceDetailErrorMessage =
        "nat slice-show: Notion API: 404 could not find page with ID"
}

// MARK: - Config

extension Fixtures {
    /// The local config the fixture board runs on: one project, pointed at a
    /// working directory, with a poll cadence long enough that nothing a
    /// preview draws refetches under it.
    public static var config: NatProjectConfig {
        NatProjectConfig(
            projects: [
                projectID: ProjectConfig(
                    name: "notion-agent-tracker",
                    slicesDSID: "f1x70000-0000-4000-8000-0000000000d5",
                    workingDir: "/Users/craig/Projects/notion-agent-tracker"
                ),
            ],
            agentSplitPercent: 45,
            pollSeconds: 3600,
            workshopAgent: AgentModel(model: "sonnet", effort: nil),
            sliceAgent: AgentModel(model: "opus", effort: "high"),
            assigneeUserName: "Craig Johnston"
        )
    }

    /// The same config with a second project on it. One project draws no
    /// close button at all (`ProjectTabRules.showsClose`), so this is what a
    /// story needs to show the tab strip as the user usually has it: a ✕ on
    /// the active tab, with the count pill seated against it.
    /// The scratch project's ID in `scratchConfig`. Sorted after the fixture
    /// project's ID on purpose: pinning it first is the strip's doing, not the
    /// sort's.
    public static let scratchProjectID = "f1x7ffff-0000-4000-8000-0000000000aa"

    /// The fixture project plus the reserved scratch project — a local one,
    /// as it is in real config — so the strip draws the scratch tab first.
    public static var scratchConfig: NatProjectConfig {
        NatProjectConfig(
            projects: [
                projectID: ProjectConfig(
                    name: "notion-agent-tracker",
                    slicesDSID: "f1x70000-0000-4000-8000-0000000000d5",
                    workingDir: "/Users/craig/Projects/notion-agent-tracker"
                ),
                scratchProjectID: ProjectConfig(
                    name: "Scratch",
                    slicesDSID: "",
                    workingDir: "/Users/craig"
                ),
            ],
            agentSplitPercent: 45,
            pollSeconds: 3600,
            workshopAgent: AgentModel(model: "sonnet", effort: nil),
            sliceAgent: AgentModel(model: "opus", effort: "high"),
            assigneeUserName: "Craig Johnston",
            scratchProject: scratchProjectID
        )
    }

    /// `scratchConfig` with the second fixture project as well, so there are two
    /// tabs besides the scratch one and one of them can be closed.
    public static var scratchConfigWithSecondProject: NatProjectConfig {
        var projects = twoProjectConfig.projects
        projects[scratchProjectID] = scratchConfig.projects[scratchProjectID]
        return NatProjectConfig(
            projects: projects,
            agentSplitPercent: 45,
            pollSeconds: 3600,
            assigneeUserName: "Craig Johnston",
            scratchProject: scratchProjectID
        )
    }

    public static var twoProjectConfig: NatProjectConfig {
        NatProjectConfig(
            projects: [
                projectID: ProjectConfig(
                    name: "notion-agent-tracker",
                    slicesDSID: "f1x70000-0000-4000-8000-0000000000d5",
                    workingDir: "/Users/craig/Projects/notion-agent-tracker"
                ),
                secondProjectID: ProjectConfig(
                    name: "gnat",
                    slicesDSID: "f1x70000-0000-4000-8000-0000000000d6",
                    workingDir: "/Users/craig/Projects/gnat"
                ),
            ],
            agentSplitPercent: 45,
            pollSeconds: 3600,
            workshopAgent: AgentModel(model: "sonnet", effort: nil),
            sliceAgent: AgentModel(model: "opus", effort: "high"),
            assigneeUserName: "Craig Johnston"
        )
    }

    /// The second project's page ID, sorting after `projectID` so the
    /// fixture project stays the active tab.
    public static let secondProjectID = "f1x70000-0000-4000-8000-000000000002"

    /// Config naming no project at all — what a first run reads, and what
    /// leaves the app on its onboarding screen.
    public static let emptyConfig = NatProjectConfig(projects: [:])

    /// Where a fixture board would read its config and nudge file from. No
    /// file is ever written to either path: the fixture config reader answers
    /// without touching the disk, and a nudge file that never appears simply
    /// never nudges.
    public static var paths: NatPaths {
        NatPaths(
            config: "/dev/null/fixtures/config.json",
            logDir: "/dev/null/fixtures/logs",
            nudge: "/dev/null/fixtures/nudge"
        )
    }
}
