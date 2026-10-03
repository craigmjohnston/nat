import Foundation
import NatKit

// MARK: - A source project

/// A project whose milestones are a task-source plugin's containers — "Work",
/// made over the demo plugin (`examples/nat-source-demo/`), as `info`,
/// `container-show`, `slice-show` and `source-list` report it. The shapes
/// follow `docs/design/task-sources/README.md` field for field.
extension Fixtures {
    public static let sourceProjectID = "f1x7500c-0000-4000-8000-000000000001"

    public static let sourceProject = Project(
        id: sourceProjectID,
        name: "Work",
        conventions: "Work tracked as cards on the demo board; each card's tasks are ordinary nat slices."
    )

    // Container ids, as the plugin names them.

    /// The first Doing card — the one with a detail, and three tasks.
    public static let sourceCardID = "4821"
    /// The second Doing card, with one task.
    public static let sourceSecondCardID = "4790"
    public static let sourceMineCardID = "4802"
    public static let sourceBoardCardID = "4811"
    public static let sourceDoneCardID = "4756"

    // Task ids.

    /// Todo, under the first card.
    public static let sourceTodoTaskID = "f1x7500c-0000-4000-8000-000000000011"
    /// In progress with an agent working, on a branch.
    public static let sourceWorkingTaskID = "f1x7500c-0000-4000-8000-000000000012"
    /// Handed back, its pull request opened.
    public static let sourceReviewTaskID = "f1x7500c-0000-4000-8000-000000000013"
    /// Todo, under the second card.
    public static let sourceSecondCardTaskID = "f1x7500c-0000-4000-8000-000000000014"

    static let sourceNativeApp = SourceBadge(text: "NA", color: "#4f6bd8", title: "Native App")
    static let sourceBoard = SourceBadge(text: "BD", color: "#2a9d8f", title: "Board")

    /// The menu every segment (a Ready child group) carries: one action of
    /// each input kind, the last destructive.
    public static let sourceSegmentMenu: [SourceAction] = [
        SourceAction(id: "rename-segment", label: "Rename…", input: .text),
        SourceAction(id: "segment-owner", label: "Owner", input: .choice, options: ["any", "me", "unassigned"]),
        SourceAction(id: "remove-segment", label: "Remove Segment", input: .none, destructive: true),
    ]

    static let sourceCardMenu: [SourceAction] = [
        SourceAction(id: "unassign-me", label: "Remove Me as Owner"),
    ]

    static let sourceDoingCards: [SourceContainer] = [
        SourceContainer(
            id: sourceCardID, title: "Improve diff review ergonomics",
            externalURL: "https://demo.example/cards/\(sourceCardID)",
            badges: [sourceNativeApp], meta: "3", menu: sourceCardMenu),
        SourceContainer(
            id: sourceSecondCardID, title: "Board mouse support",
            externalURL: "https://demo.example/cards/\(sourceSecondCardID)",
            badges: [sourceBoard], meta: "2", menu: sourceCardMenu),
    ]

    static let sourceMineCard = SourceContainer(
        id: sourceMineCardID, title: "Kanban column view",
        externalURL: "https://demo.example/cards/\(sourceMineCardID)",
        badges: [sourceBoard], meta: "3", menu: sourceCardMenu)

    static let sourceBoardCard = SourceContainer(
        id: sourceBoardCardID, title: "Wheel scrolling in the Active panel",
        externalURL: "https://demo.example/cards/\(sourceBoardCardID)",
        badges: [sourceBoard], meta: "1", menu: sourceCardMenu)

    static let sourceDoneCards: [SourceContainer] = [
        SourceContainer(
            id: sourceDoneCardID, title: "Pluggable plan storage",
            externalURL: "https://demo.example/cards/\(sourceDoneCardID)",
            badges: [sourceNativeApp], meta: "5"),
    ]

    /// The plugin's tree, with the lazy Done group listing its cards only
    /// where `expand` names it — as `info --expand done` reads it. The same
    /// card sits under both segments, as segments are filters over one board.
    public static func sourceGroups(expand: [String] = []) -> [SourceGroup] {
        [
            SourceGroup(id: "doing", label: "Doing", count: 2, containers: sourceDoingCards),
            SourceGroup(id: "ready", label: "Ready", count: 3, children: [
                SourceGroup(id: "seg-mine", label: "Mine", count: 1, menu: sourceSegmentMenu,
                            containers: [sourceMineCard]),
                SourceGroup(id: "seg-board", label: "Board", count: 2, menu: sourceSegmentMenu,
                            containers: [sourceBoardCard, sourceMineCard]),
            ]),
            SourceGroup(id: "done", label: "Done", count: 36, lazy: true,
                        containers: expand.contains("done") ? sourceDoneCards : []),
        ]
    }

    /// `info --json`'s `source` for the Work project.
    public static func sourceInfo(expand: [String] = []) -> SourceInfo {
        SourceInfo(
            name: "demo", title: "Demo source", tag: "DM",
            iconSymbol: "rectangle.on.rectangle.angled",
            containerNoun: "card", taskNoun: "task",
            menu: [
                SourceAction(id: "refresh", label: "Refresh"),
                SourceAction(id: "new-segment", label: "New Segment…", input: .text),
            ],
            groups: sourceGroups(expand: expand)
        )
    }

    /// The same project with the plugin unreadable: `error` set, the fields
    /// it could not read empty (the name is the configured one, the nouns
    /// nat's defaults), and the tree nat's `_unlisted` group alone — every
    /// card with tasks, titled from nat's cache.
    public static let sourceInfoFailed = SourceInfo(
        name: "demo", title: "", tag: "", iconSymbol: "", containerNoun: "container", taskNoun: "task",
        groups: [
            SourceGroup(id: SourceGroup.unlistedID, label: "Other containers", count: 2, containers: [
                SourceContainer(id: sourceCardID, title: "Improve diff review ergonomics"),
                SourceContainer(id: sourceSecondCardID, title: "Board mouse support"),
            ]),
        ],
        error: "nat-source-demo describe: demo token missing — run nat-source-demo login"
    )

    /// The cards with tasks, as `milestones` — a container is a milestone
    /// whose id is the container id, named by nat's cached title.
    public static let sourceMilestones: [Milestone] = [
        Milestone(id: sourceCardID, name: "Improve diff review ergonomics", order: 0, status: "Active"),
        Milestone(id: sourceSecondCardID, name: "Board mouse support", order: 1, status: "Queued"),
    ]

    static let sourceRepo = "/Users/craig/work/app"

    public static let sourceTasks: [Slice] = [
        Slice(
            id: sourceTodoTaskID, name: "Syntax highlighting in review comments", status: "Todo",
            milestoneID: sourceCardID, assignee: "", pr: "",
            url: "nat://\(sourceTodoTaskID)", blocked: false, handedBack: false
        ),
        Slice(
            id: sourceWorkingTaskID, name: "Diff comments reach the agent", status: "In progress",
            milestoneID: sourceCardID, assignee: "Craig Johnston", pr: "",
            url: "nat://\(sourceWorkingTaskID)",
            branch: "slice/diff-comments-reach-the-agent", repo: sourceRepo,
            blocked: false, handedBack: false, state: .working
        ),
        Slice(
            id: sourceReviewTaskID, name: "Jump between comment threads", status: "In progress",
            milestoneID: sourceCardID, assignee: "Craig Johnston",
            pr: "https://github.com/acme/app/pull/418",
            url: "nat://\(sourceReviewTaskID)",
            branch: "slice/jump-between-comment-threads", repo: sourceRepo,
            blocked: false, handedBack: true, state: .awaitingReview
        ),
        Slice(
            id: sourceSecondCardTaskID, name: "Wheel events reach the board", status: "Todo",
            milestoneID: sourceSecondCardID, assignee: "", pr: "",
            url: "nat://\(sourceSecondCardTaskID)", blocked: false, handedBack: false
        ),
    ]

    /// `nat info --project <Work> --json`.
    public static func sourceProjectInfo(expand: [String] = []) -> ProjectInfo {
        ProjectInfo(
            project: sourceProject, milestones: sourceMilestones, slices: sourceTasks,
            source: sourceInfo(expand: expand))
    }

    static let sourceCardFacts: [SourceFact] = [
        SourceFact(label: "id", value: "dm-4821"),
        SourceFact(label: "project", value: "NA · Native App", color: "#4f6bd8"),
        SourceFact(label: "state", value: "In Development"),
        SourceFact(label: "type", value: "feature"),
        SourceFact(label: "epic", value: "Native app parity"),
        SourceFact(label: "labels", value: "diff, agent"),
        SourceFact(label: "owner", value: "Craig"),
        SourceFact(label: "requester", value: "Dana Wolfe"),
    ]

    static let sourceCardTaskNote =
        "Linked to dm-4821. Merging moves the card to Done when it's the last open task."

    /// The plugin's `container` response for the first Doing card.
    public static let sourceCardDetail = ContainerDetail(
        id: sourceCardID,
        title: "Improve diff review ergonomics",
        externalURL: "https://demo.example/cards/\(sourceCardID)",
        facts: sourceCardFacts,
        sections: [
            SourceSection(id: "story", title: "Story", kind: .prose, body: """
            Comments left on a diff in the app never reach the working agent: the \
            reviewer types into a box the agent cannot see, and the round trip is a \
            copy and paste.

            Send each comment to the agent's session as it is posted, with the file \
            and line it was left on, and show the agent's reply in the thread.

            **Acceptance:** a comment posted mid-session shows up in the transcript \
            within a second, and a reply lands under the comment it answers.
            """),
            SourceSection(
                id: "comments", title: "Comments", kind: .comments,
                comments: [
                    SourceComment(by: "Dana Wolfe", when: "3d ago", text: "Pairs with the syntax highlighting work."),
                    SourceComment(by: "Craig", when: "2d ago", text: "Agreed. Splitting into three tasks."),
                    SourceComment(by: "Sam Ortiz", when: "5h ago", text: "Thread jumping would help the review pass a lot."),
                ],
                composer: SourceAction(id: "comment", label: "Comment", input: .text)
            ),
            SourceSection(
                id: "links", title: "Links", kind: .links,
                links: [
                    SourceLink(label: "PR #418", text: "Jump between comment threads", state: "open",
                               url: "https://github.com/acme/app/pull/418"),
                    SourceLink(label: "PR #402", text: "Comment anchors survive a rebase", state: "merged",
                               url: "https://github.com/acme/app/pull/402"),
                    SourceLink(label: "Design", text: "Figma · Review pane v3",
                               url: "https://www.figma.com/file/abc/review-pane-v3"),
                ]
            ),
        ],
        menu: sourceCardMenu,
        taskNote: sourceCardTaskNote
    )

    /// `container-show` for any card: the first card's full detail, else a
    /// plain one built from the sidebar row, with the plan's tasks under it.
    public static func sourceContainerShow(id: String) -> ContainerShow {
        let tasks = sourceTasks.filter { $0.milestoneID == id }
        if id == sourceCardID {
            return ContainerShow(container: sourceCardDetail, tasks: tasks)
        }
        let row = sourceInfo(expand: ["done"]).allContainers.first { $0.id == id }
        return ContainerShow(
            container: ContainerDetail(
                id: id, title: row?.title ?? id, externalURL: row?.externalURL,
                facts: [SourceFact(label: "id", value: "dm-\(id)")],
                sections: [SourceSection(id: "story", title: "Story", kind: .prose, body: "No description yet.")],
                menu: row?.menu ?? []
            ),
            tasks: tasks
        )
    }

    /// `slice-show`'s `state` word for the tasks that have one.
    static let sourceTaskStates: [String: String] = [
        sourceWorkingTaskID: "working",
        sourceReviewTaskID: "awaiting review",
    ]

    /// `slice-show` for each task of the Work project, `container` filled.
    public static var sourceSliceDetails: [String: SliceDetail] {
        let firstCard = SliceContainer(
            id: sourceCardID, title: sourceCardDetail.title, externalURL: sourceCardDetail.externalURL,
            taskNote: sourceCardTaskNote, facts: sourceCardFacts)
        let secondCard = SliceContainer(
            id: sourceSecondCardID, title: "Board mouse support",
            externalURL: "https://demo.example/cards/\(sourceSecondCardID)",
            facts: [SourceFact(label: "id", value: "dm-\(sourceSecondCardID)")])
        var details: [String: SliceDetail] = [:]
        for task in sourceTasks {
            let card = task.milestoneID == sourceCardID ? firstCard : secondCard
            details[task.id] = SliceDetail(
                id: task.id, name: task.name, url: task.url, status: task.status,
                milestone: card.title, assignee: task.assignee,
                branch: task.branch, repo: task.repo, base: task.repo == nil ? nil : "origin/main",
                pr: task.pr.isEmpty ? nil : task.pr,
                blocked: false, handedBack: task.handedBack,
                state: sourceTaskStates[task.id],
                brief: "Part of \(card.title).",
                container: card
            )
        }
        return details
    }

    /// `source-list --json`: the demo plugin, described, and one that
    /// refused to describe.
    public static let sourcePlugins: [SourcePlugin] = [
        SourcePlugin(
            name: "demo",
            path: "/Users/craig/.config/notion-agent-tracker/plugins/demo/nat-source-demo",
            describe: SourceDescribe(
                name: "demo", title: "Demo source", tag: "DM",
                iconSymbol: "rectangle.on.rectangle.angled",
                containerNoun: "card", taskNoun: "task",
                menu: [SourceAction(id: "refresh", label: "Refresh")]
            )
        ),
        SourcePlugin(
            name: "shortcut",
            path: "/opt/homebrew/bin/nat-source-shortcut",
            error: "source plugin shortcut speaks protocol 2; this nat speaks protocol 1"
        ),
    ]
}
