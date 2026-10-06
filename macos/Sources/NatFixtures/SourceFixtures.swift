import Foundation
import NatKit

// MARK: - A source project

/// A project whose milestones are a task-source plugin's containers — "Work",
/// made over the demo plugin (`examples/nat-source-demo/`), as `info`,
/// `container-show`, `slice-show` and `source-list` report it. The shapes
/// follow `docs/design/task-sources/README.md` field for field.
extension Fixtures {
    /// The Shortcut plugin's icon as its `describe` sends it — the SVG mark
    /// (`plugins/shortcut`'s `iconSVG`), the symbol its fallback — for the
    /// stories that draw a Shortcut ("SC") source.
    public static let shortcutIcon = SourceIcon(
        symbol: "rectangle.on.rectangle.angled",
        svg: #"<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><path fill="currentColor" fill-rule="evenodd" clip-rule="evenodd" d="M18.2765 8.46875H39.8392L30.0769 19.183L39.652 28.7301L29.7873 39.5561L8.15918 39.5506L17.9624 28.7915L8.42517 19.2828L18.2765 8.46875ZM19.7228 30.5467L13.8141 37.0315L26.2301 37.0346L19.7228 30.5467ZM29.2139 36.498L21.3993 28.7067L28.4005 21.0229L36.2151 28.8147L29.2139 36.498ZM26.6401 19.2677L19.6388 26.9516L11.8619 19.1979L18.8627 11.5129L26.6401 19.2677ZM28.3166 17.4277L34.183 10.9893H21.8593L28.3166 17.4277Z"/></svg>"#)

    /// Sorts ahead of the other fixture projects, so a board started over
    /// `sourceConfig` opens on Work — the shell's own start activates the
    /// first project, and a story's selection there has to survive it.
    public static let sourceProjectID = "f1x6500c-0000-4000-8000-000000000001"

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

    // A card's one badge is its Shortcut-style project, as the plugin sends
    // it; a card with no project has none (never its team's).
    public static let sourceMobileApp = SourceBadge(text: "MOB", color: "#e5732a", title: "Mobile App")
    static let sourceWeb = SourceBadge(text: "WE", color: "#8e8e93", title: "Web")

    // The workspace's choices a filter offers, as the Shortcut plugin sends
    // them: teams by mention name, projects and epics by id, labels by name.
    static let sourceTeamOptions = [
        SourceFilterOption(id: "board", label: "Board", color: "#2a9d8f"),
        SourceFilterOption(id: "native-app", label: "Native App", color: "#4f6bd8"),
    ]
    static let sourceProjectOptions = [
        SourceFilterOption(id: "30", label: "Mobile App", color: "#e5732a"),
        SourceFilterOption(id: "31", label: "Web", color: "#8e8e93"),
    ]
    static let sourceEpicOptions = [
        SourceFilterOption(id: "10", label: "Native app parity"),
        SourceFilterOption(id: "12", label: "Review pane v3"),
    ]
    /// A segment's states, one workflow's, so named alone.
    static let sourceStateOptions = [
        SourceFilterOption(id: "501", label: "Backlog"),
        SourceFilterOption(id: "505", label: "Done"),
        SourceFilterOption(id: "503", label: "In Development"),
        SourceFilterOption(id: "504", label: "In Review"),
        SourceFilterOption(id: "502", label: "Ready for Dev"),
    ]
    static let sourceLabelOptions = [
        SourceFilterOption(id: "agent", label: "agent"),
        SourceFilterOption(id: "diff", label: "diff", color: "#d64545"),
        SourceFilterOption(id: "sidebar", label: "sidebar"),
    ]

    /// The section's own filter: every list narrowed to the Mobile App
    /// project.
    static let sourceSectionFilter = ["project": ["30"]]

    /// A filter action's fields over `selected`; `section` (a segment's)
    /// names what each "Any" falls through to, and adds the State field a
    /// segment's editor alone has; `epicsLoading` is the plugin still
    /// fetching its epic list.
    public static func sourceFilterAction(
        _ selected: [String: [String]], section: [String: [String]]? = nil, epicsLoading: Bool = false
    ) -> SourceAction {
        func named(_ options: [SourceFilterOption], _ field: String) -> String? {
            guard let ids = section?[field], !ids.isEmpty else { return nil }
            return ids.map { id in options.first { $0.id == id }?.label ?? id }.joined(separator: ", ")
        }
        let state = section == nil ? [] : [
            SourceFilterField(id: "state", label: "State", options: sourceStateOptions, value: selected["state"] ?? []),
        ]
        return SourceAction(id: "filter", label: "Filter…", input: .filter, fields: [
            SourceFilterField(id: "team", label: "Team", options: sourceTeamOptions, value: selected["team"] ?? [],
                              inherited: named(sourceTeamOptions, "team")),
            SourceFilterField(id: "project", label: "Project", options: sourceProjectOptions,
                              value: selected["project"] ?? [], inherited: named(sourceProjectOptions, "project")),
        ] + state + [
            SourceFilterField(id: "epic", label: "Epic", options: epicsLoading ? [] : sourceEpicOptions,
                              value: selected["epic"] ?? [], inherited: named(sourceEpicOptions, "epic"),
                              loading: epicsLoading),
            SourceFilterField(id: "labels", label: "Labels", multi: true, options: sourceLabelOptions,
                              value: selected["labels"] ?? [], inherited: named(sourceLabelOptions, "labels")),
        ])
    }

    /// A segment's menu: Rename, its filter over the section's, Remove.
    public static func sourceSegmentMenu(_ filter: [String: [String]], epicsLoading: Bool = false) -> [SourceAction] {
        [
            SourceAction(id: "rename", label: "Rename…", input: .text),
            sourceFilterAction(filter, section: sourceSectionFilter, epicsLoading: epicsLoading),
            SourceAction(id: "remove", label: "Remove Segment", input: .none, destructive: true),
        ]
    }

    static let sourceCardMenu: [SourceAction] = [
        SourceAction(id: "unassign-me", label: "Remove Me as Owner"),
    ]

    static let sourceDoingCards: [SourceContainer] = [
        SourceContainer(
            id: sourceCardID, title: "Improve diff review ergonomics",
            externalURL: "https://demo.example/cards/\(sourceCardID)",
            badges: [sourceMobileApp], meta: "3", menu: sourceCardMenu),
        SourceContainer(
            id: sourceSecondCardID, title: "Board mouse support",
            externalURL: "https://demo.example/cards/\(sourceSecondCardID)",
            badges: [], meta: "2", menu: sourceCardMenu),
    ]

    static let sourceMineCard = SourceContainer(
        id: sourceMineCardID, title: "Kanban column view",
        externalURL: "https://demo.example/cards/\(sourceMineCardID)",
        badges: [], meta: "3", menu: sourceCardMenu)

    static let sourceBoardCard = SourceContainer(
        id: sourceBoardCardID, title: "Wheel scrolling in the Active panel",
        externalURL: "https://demo.example/cards/\(sourceBoardCardID)",
        badges: [sourceWeb], meta: "1", menu: sourceCardMenu)

    static let sourceDoneCards: [SourceContainer] = [
        SourceContainer(
            id: sourceDoneCardID, title: "Pluggable plan storage",
            externalURL: "https://demo.example/cards/\(sourceDoneCardID)",
            badges: [sourceMobileApp], meta: "5"),
    ]

    /// The plugin's tree, with the lazy Done group listing its cards only
    /// where `expand` names it — as `info --expand done` reads it. Each saved
    /// segment is a top-level group between Doing and Done, and the same card
    /// sits under both, as segments are filters over one board.
    public static func sourceGroups(expand: [String] = [], epicsLoading: Bool = false) -> [SourceGroup] {
        [
            SourceGroup(id: "doing", label: "Doing", count: 2, containers: sourceDoingCards),
            SourceGroup(id: "ready/mine", label: "Mine", count: 1,
                        menu: sourceSegmentMenu(["team": ["board"]], epicsLoading: epicsLoading),
                        containers: [sourceMineCard]),
            SourceGroup(id: "ready/board", label: "Board", count: 2,
                        menu: sourceSegmentMenu(["labels": ["sidebar"]], epicsLoading: epicsLoading),
                        containers: [sourceBoardCard, sourceMineCard]),
            SourceGroup(id: "done", label: "Done", count: 4, lazy: true,
                        containers: expand.contains("done") ? sourceDoneCards : []),
        ]
    }

    /// `info --json`'s `source` for the Work project: the sidebar's own
    /// header menu, with the section's filter, in place of describe's.
    public static func sourceInfo(expand: [String] = [], epicsLoading: Bool = false) -> SourceInfo {
        SourceInfo(
            name: "demo", title: "Demo source", tag: "DM",
            // Drawn with the Shortcut mark, as the plugin it stands for is.
            iconSymbol: "rectangle.on.rectangle.angled", iconSVG: shortcutIcon.svg,
            containerNoun: "card", taskNoun: "task",
            menu: [
                SourceAction(id: "refresh", label: "Refresh"),
                SourceAction(id: "new-segment", label: "New Segment…", input: .text),
                sourceFilterAction(sourceSectionFilter, epicsLoading: epicsLoading),
            ],
            groups: sourceGroups(expand: expand, epicsLoading: epicsLoading)
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
            // #214, the number the fixture client's `pr-view` answers with,
            // so the PR view reads it as this task's.
            pr: "https://github.com/acme/app/pull/214",
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
        SourceFact(label: "project", value: "Mobile App", color: "#e5732a", badge: sourceMobileApp),
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
                    SourceLink(label: "PR #214", text: "Jump between comment threads", state: "open",
                               url: "https://github.com/acme/app/pull/214"),
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

    /// `twoProjectConfig` with the Work source project beside the two, so the
    /// sidebar has a source fold under Projects.
    public static var sourceConfig: NatProjectConfig {
        var projects = twoProjectConfig.projects
        projects[sourceProjectID] = ProjectConfig(
            name: sourceProject.name, workingDir: sourceRepo, backend: .source, source: "demo")
        return NatProjectConfig(
            projects: projects, agentSplitPercent: 45, pollSeconds: 3600,
            workshopAgent: AgentModel(model: "sonnet", effort: nil),
            sliceAgent: AgentModel(model: "opus", effort: "high"),
            assigneeUserName: "Craig Johnston")
    }

    /// A client whose Work project's plugin cannot be read: `source.error`
    /// set and the tree nat's `_unlisted` group alone.
    public static func failedSourceClient() -> FixtureNatClient {
        FixtureNatClient(otherPlans: [
            secondProjectID: secondProjectInfo,
            sourceProjectID: ProjectInfo(
                project: sourceProject, milestones: sourceMilestones, slices: sourceTasks,
                source: sourceInfoFailed),
        ])
    }

    /// `source-list --json`: the demo plugin, described, and one that
    /// refused to describe.
    public static let sourcePlugins: [SourcePlugin] = [
        SourcePlugin(
            name: "demo",
            path: "/Users/craig/.config/notion-agent-tracker/plugins/demo/nat-source-demo",
            // Not connected — its token unset — so a board started over a
            // config with no demo project makes none (AppModel.ensureSourceProjects).
            describe: SourceDescribe(
                name: "demo", title: "Demo source", tag: "DM",
                iconSymbol: "rectangle.on.rectangle.angled",
                containerNoun: "card", taskNoun: "task",
                menu: [SourceAction(id: "refresh", label: "Refresh")],
                setup: [PluginSetupField(id: "token", label: "API token", input: "secret", set: false)]
            )
        ),
        SourcePlugin(
            name: "shortcut",
            path: "/opt/homebrew/bin/nat-source-shortcut",
            error: "source plugin shortcut speaks protocol 2; this nat speaks protocol 1"
        ),
    ]
}
