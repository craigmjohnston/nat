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
            batch: 1, index: 1,
            title: "Persist the conversation split width per project",
            brief: "The split's width lives under one AppStorage key, so every project shares it. "
                + "The PR sidebar has the same problem. Store both per project the way pane widths already are."
        ),
        FollowUp(
            batch: 1, index: 2,
            title: "Render the emoji picker open in a gallery story",
            brief: "The picker is open-state only and no story draws it, so StoryNamesTests can't guard it. "
                + "Add pr-composer-emoji with the picker open over the composer."
        ),
        FollowUp(
            batch: 1, index: 3,
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
        followUps: proposedFollowUps,
        // The proposal on the page, undecided, stamped 25 minutes before `now`.
        events: [TaskLogEvent(
            .followUps, at: now.addingTimeInterval(-25 * 60), batch: 1,
            followUps: proposedFollowUps.map { TaskFollowUp(index: $0.index, title: $0.title, brief: $0.brief) })]
    )

    /// `sliceDetails` with the activity slice's follow-ups added.
    public static var followUpsSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([activitySliceID: followUpsSliceDetail]) { _, new in new }
    }

    /// The two follow-ups the activity slice's agent handed in as a second
    /// batch, after talking to the user about the first: each pending item's
    /// index counts on from the first batch's, as `slice-triage` takes it.
    public static func secondBatchFollowUps(from first: Int) -> [FollowUp] {
        [
            FollowUp(
                batch: 2, index: first,
                title: "Name the pane's activity states in one enum",
                brief: "Activity is a string compared in three places. Make it an enum in NatKit and switch on it."),
            FollowUp(
                batch: 2, index: first + 1,
                title: "Log a capture tmux refuses",
                brief: "A failed capture-pane is dropped silently. Log it once per pane, with tmux's own error."),
        ]
    }

    /// A batch's proposals as its log event names them, each still pending.
    private static func undecided(_ followUps: [FollowUp]) -> [TaskFollowUp] {
        followUps.enumerated().map { TaskFollowUp(index: $0.offset + 1, title: $0.element.title, brief: $0.element.brief) }
    }

    /// The activity slice with two batches of follow-ups pending at once:
    /// three proposed 50 minutes before `now`, then — the agent handed back
    /// between them — two more 10 minutes before it.
    public static let twoBatchesSliceDetail = SliceDetail(
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
        followUps: proposedFollowUps + secondBatchFollowUps(from: 4),
        events: [
            TaskLogEvent(
                .followUps, at: now.addingTimeInterval(-50 * 60), batch: 1, followUps: undecided(proposedFollowUps)),
            TaskLogEvent(.note, note: "Asked about the split width: per project, as the pane widths are.",
                         by: "Craig Johnston", at: now.addingTimeInterval(-30 * 60)),
            TaskLogEvent(
                .followUps, at: now.addingTimeInterval(-10 * 60), batch: 2,
                followUps: undecided(secondBatchFollowUps(from: 4))),
        ]
    )

    /// The activity slice with its first batch decided — one queued, one
    /// folded in, one dropped, 20 minutes before `now` — and its second still
    /// pending, its items' indexes counting from 1 again now nothing is pending
    /// before them.
    public static let decidedAndPendingSliceDetail = SliceDetail(
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
        followUps: secondBatchFollowUps(from: 1),
        events: [
            TaskLogEvent(
                .followUps, at: now.addingTimeInterval(-50 * 60), batch: 1,
                followUps: zip(proposedFollowUps, [TaskFollowUp.Decision.queued, .folded, .dropped]).map {
                    TaskFollowUp(
                        index: $0.0.index, title: $0.0.title, brief: $0.0.brief, decision: $0.1,
                        link: $0.1 == .queued ? "https://notion.so/\(cacheSliceID)" : nil,
                        decidedAt: now.addingTimeInterval(-20 * 60))
                }),
            TaskLogEvent(
                .followUps, at: now.addingTimeInterval(-10 * 60), batch: 2,
                followUps: undecided(secondBatchFollowUps(from: 1))),
        ]
    )

    /// A merged slice with a history: handed back three times, sent back
    /// with comments between them twice, three follow-ups triaged on the way
    /// (one queued, one folded in, one dropped),
    /// then approved and merged — `slice-show`'s `events` for it, in order.
    /// Each section is stamped a day apart, the launch nine days before `now`,
    /// the first hand-back eight and the proposal five; two of its decisions
    /// a day after it, the dropped one's predating stamps.
    public static let taskLogEvents: [TaskLogEvent] = [
        TaskLogEvent(.launched, at: now.addingTimeInterval(-9 * 86_400)),
        TaskLogEvent(
            .handedBack, note: "The shell window and its three panes, empty states in each.",
            at: now.addingTimeInterval(-8 * 86_400)),
        TaskLogEvent(
            .sentBack, note: "Sources/NatApp/NatApp.swift, line 42: the window should remember its frame.",
            at: now.addingTimeInterval(-7 * 86_400)),
        TaskLogEvent(
            .handedBack, note: "Window frame autosaved under the scene's id.", at: now.addingTimeInterval(-6 * 86_400)),
        TaskLogEvent(.followUps, at: now.addingTimeInterval(-5 * 86_400), followUps: [
            TaskFollowUp(
                index: 1, title: "Cache the plan on disk",
                brief: "Write each `nat info` reading to the app's caches directory and draw it at launch, "
                    + "before the first read lands, so the window never opens empty.\n\n"
                    + "Done when: a relaunch with no network draws the last plan read.",
                decision: .queued, link: "https://notion.so/\(cacheSliceID)",
                decidedAt: now.addingTimeInterval(-4 * 86_400)),
            TaskFollowUp(
                index: 2, title: "Remember the window's frame between launches",
                brief: "Autosave the main window's frame under the scene's id so it opens where it was left.\n\n"
                    + "Done when: a moved and resized window reopens at that frame.",
                decision: .folded, decidedAt: now.addingTimeInterval(-4 * 86_400)),
            TaskFollowUp(
                index: 3, title: "Drop the unused toolbar style",
                brief: "Remove `ShellToolbarStyle`, which nothing applies since the titlebar became the header band.\n\n"
                    + "Done when: the type is gone and the app builds.",
                decision: .dropped),
        ]),
        TaskLogEvent(
            .sentBack, note: "Sources/NatApp/Views/ShellView.swift, lines 10-14: use the design's 32pt titlebar.",
            at: now.addingTimeInterval(-3 * 86_400)),
        TaskLogEvent(
            .handedBack, note: "Titlebar at 32pt, traffic lights recentred.", at: now.addingTimeInterval(-2 * 86_400)),
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

    /// An in-progress slice with notes left on its brief: one from the shell
    /// slice (a task on the plan) before it was launched, last year; its
    /// launch, a hand-back and a send-back earlier this year; and one from
    /// Craig (no task) today — `slice-show`'s `events` for it, in order. The times are
    /// measured from `now`, the clock the gallery draws at, so a story of it
    /// says "today", "this year" and "last year" the same way every run.
    public static var notedTaskLogEvents: [TaskLogEvent] {
        let calendar = Calendar.current
        let today = max(calendar.startOfDay(for: now), now.addingTimeInterval(-40 * 60))
        let startOfYear = calendar.dateInterval(of: .year, for: now)?.start ?? now
        let thisYear = max(startOfYear, calendar.date(byAdding: .day, value: -12, to: now) ?? now)
        let lastYear = calendar.date(byAdding: .year, value: -1, to: now) ?? now
        return [
            TaskLogEvent(.note, note: "The window's frame is autosaved under the scene's id now: read it from there rather than adding a key of your own.",
                         by: "\"Bootstrap the SwiftUI shell\" (M1: Foundations)",
                         fromSlice: NoteSource(name: "Bootstrap the SwiftUI shell", milestone: "M1: Foundations"),
                         at: lastYear),
            TaskLogEvent(.launched, at: thisYear.addingTimeInterval(-2 * 3600)),
            TaskLogEvent(.handedBack, note: "Polls every second; each row draws its agent's activity.", at: thisYear),
            TaskLogEvent(.sentBack, note: "Sources/NatKit/Activity.swift, line 30: a failed capture is unread, not gone.",
                         at: thisYear.addingTimeInterval(3 * 3600)),
            TaskLogEvent(.note, note: "tmux 3.5 renamed the pane activity format; check `tmux -V` before trusting it.",
                         by: "Craig Johnston", at: today),
        ]
    }

    /// The activity slice, read with those notes in its history.
    public static var notedSliceDetail: SliceDetail {
        SliceDetail(
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
            events: notedTaskLogEvents
        )
    }

    /// `sliceDetails` with the activity slice's notes added.
    public static var notedSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([activitySliceID: notedSliceDetail]) { _, new in new }
    }

    /// An in-progress slice whose log has gone quiet in the middle: launched
    /// and handed back early this year, then three notes, a blocked hand-in
    /// and a proposal of follow-ups already triaged (its count line and one
    /// decided follow-up) in a row — six quiet items, which fold into one
    /// group — then a send-back and one more note today, folded alone. Times
    /// measured from `now`, as `notedTaskLogEvents`' are.
    public static var groupedTaskLogEvents: [TaskLogEvent] {
        let calendar = Calendar.current
        let today = max(calendar.startOfDay(for: now), now.addingTimeInterval(-40 * 60))
        let startOfYear = calendar.dateInterval(of: .year, for: now)?.start ?? now
        let thisYear = max(startOfYear, calendar.date(byAdding: .day, value: -12, to: now) ?? now)
        func day(_ n: Int) -> Date { calendar.date(byAdding: .day, value: n, to: thisYear) ?? thisYear }
        return [
            TaskLogEvent(.launched, at: thisYear),
            TaskLogEvent(.handedBack, note: "Polls every second; each row draws its agent's activity.", at: day(1)),
            TaskLogEvent(.note, note: "tmux 3.5 renamed the pane activity format; check `tmux -V` before trusting it.",
                         by: "Craig Johnston", at: day(2)),
            TaskLogEvent(.note, note: "The window's frame is autosaved under the scene's id now: read it from there rather than adding a key of your own.",
                         by: "\"Bootstrap the SwiftUI shell\" (M1: Foundations)",
                         fromSlice: NoteSource(name: "Bootstrap the SwiftUI shell", milestone: "M1: Foundations"),
                         at: day(3)),
            TaskLogEvent(.note, note: "The capture can take 200ms on a busy server; don't block the main actor on it.",
                         by: "Craig Johnston", at: day(4)),
            TaskLogEvent(.blocked, note: "No tmux on the CI runner to test against.", at: day(5)),
            TaskLogEvent(.followUps, at: day(6), followUps: [
                TaskFollowUp(
                    index: 1, title: "Install tmux on the CI runner",
                    brief: "Add tmux to the runner image so the activity poll can be tested end to end.",
                    decision: .folded, decidedAt: day(7)),
            ]),
            TaskLogEvent(.sentBack, note: "Sources/NatKit/Activity.swift, line 30: a failed capture is unread, not gone.",
                         at: today),
            TaskLogEvent(.note, note: "The poll can drop to every two seconds while the window is in the background.",
                         by: "Craig Johnston", at: today.addingTimeInterval(20 * 60)),
        ]
    }

    /// The activity slice, read with that quiet run in its history.
    public static var groupedSliceDetail: SliceDetail {
        SliceDetail(
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
            events: groupedTaskLogEvents
        )
    }

    /// `sliceDetails` with the activity slice's quiet run added.
    public static var groupedSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([activitySliceID: groupedSliceDetail]) { _, new in new }
    }

    /// The fixtures slice, never launched, with one note on its brief from the
    /// shell slice — its whole history, today.
    public static var notedTodoSliceDetail: SliceDetail {
        SliceDetail(
            id: fixturesSliceID,
            name: "Build a fixture library of canned app states",
            url: "https://notion.so/\(fixturesSliceID)",
            status: "Todo",
            milestone: "M3: View gallery",
            assignee: "",
            blocked: false,
            handedBack: false,
            brief: "Build a library of canned app states for the gallery to draw.",
            events: [
                TaskLogEvent(.note, note: "The shell's window reads its frame from the scene's id: a fixture window needs one too.",
                             by: "\"Bootstrap the SwiftUI shell\" (M1: Foundations)",
                             fromSlice: NoteSource(name: "Bootstrap the SwiftUI shell", milestone: "M1: Foundations"),
                             at: max(Calendar.current.startOfDay(for: now), now.addingTimeInterval(-25 * 60))),
            ]
        )
    }

    /// `sliceDetails` with the fixtures slice's note added.
    public static var notedTodoSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([fixturesSliceID: notedTodoSliceDetail]) { _, new in new }
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
                    workingDir: "/Users/craig/Projects/notion-agent-tracker",
                    color: .teal
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
                    workingDir: "/Users/craig/Projects/notion-agent-tracker",
                    color: .teal
                ),
                scratchProjectID: ProjectConfig(
                    name: "Scratch",
                    slicesDSID: "",
                    workingDir: "/Users/craig",
                    color: .purple
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
                    workingDir: "/Users/craig/Projects/notion-agent-tracker",
                    color: .teal
                ),
                secondProjectID: ProjectConfig(
                    name: "gnat",
                    slicesDSID: "f1x70000-0000-4000-8000-0000000000d6",
                    workingDir: "/Users/craig/Projects/gnat",
                    color: .orange
                ),
            ],
            agentSplitPercent: 45,
            pollSeconds: 3600,
            workshopAgent: AgentModel(model: "sonnet", effort: nil),
            sliceAgent: AgentModel(model: "opus", effort: "high"),
            assigneeUserName: "Craig Johnston"
        )
    }

    /// The fixture project's run commands: Play for both places, and Board
    /// with a scoped pair — something different in a slice's worktree — so
    /// the titlebar offers Play then Board, and a handed-back slice the same
    /// labels with its own Board.
    public static let runs: [RunCommand] = [
        RunCommand(label: "Play", command: "./scripts/play.sh --windowed"),
        RunCommand(label: "Board", command: "go run .", scope: .global),
        RunCommand(label: "Board", command: "go run . --sandbox", scope: .slice),
    ]

    /// The second project's one run.
    public static let secondProjectRuns = [RunCommand(label: "Unity", command: "open -a Unity --args -projectPath .")]

    /// `twoProjectConfig` with `runs` on both projects.
    public static var runsConfig: NatProjectConfig {
        var projects = twoProjectConfig.projects
        let p = projects[projectID]!
        projects[projectID] = ProjectConfig(
            name: p.name, slicesDSID: p.slicesDSID, workingDir: p.workingDir, runs: runs, color: p.color)
        let second = projects[secondProjectID]!
        projects[secondProjectID] = ProjectConfig(
            name: second.name, slicesDSID: second.slicesDSID, workingDir: second.workingDir, runs: secondProjectRuns,
            color: second.color)
        return NatProjectConfig(
            projects: projects, agentSplitPercent: 45, pollSeconds: 3600,
            workshopAgent: AgentModel(model: "sonnet", effort: nil),
            sliceAgent: AgentModel(model: "opus", effort: "high"),
            assigneeUserName: "Craig Johnston")
    }

    /// The plan with the handed-back slice merged: Done, its pull request
    /// recorded — what greys its Task header's run button.
    public static var mergedReviewProjectInfo: ProjectInfo {
        ProjectInfo(
            project: projectInfo.project,
            milestones: projectInfo.milestones,
            slices: projectInfo.slices.map { s in
                guard s.id == mergeBoxSliceID else { return s }
                return Slice(
                    id: s.id, name: s.name, status: "Done", milestoneID: s.milestoneID, assignee: s.assignee,
                    pr: "https://github.com/craigmjohnston/notion-agent-tracker/pull/120", url: s.url,
                    branch: s.branch, repo: s.repo, dependsOn: s.dependsOn, blocked: s.blocked,
                    handedBack: s.handedBack, state: s.state)
            })
    }

    /// The plan with the working slice's branch recorded while its agent is
    /// still at it — Changes live, nothing handed back to approve yet.
    public static var branchedWorkingProjectInfo: ProjectInfo {
        ProjectInfo(
            project: projectInfo.project,
            milestones: projectInfo.milestones,
            slices: projectInfo.slices.map { s in
                guard s.id == diffPaneSliceID else { return s }
                return Slice(
                    id: s.id, name: s.name, status: s.status, milestoneID: s.milestoneID, assignee: s.assignee,
                    pr: s.pr, url: s.url, branch: diffBranch, repo: s.repo, dependsOn: s.dependsOn,
                    blocked: s.blocked, handedBack: false, state: s.state)
            })
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
