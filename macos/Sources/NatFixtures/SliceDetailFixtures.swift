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
        repo: nil,
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
