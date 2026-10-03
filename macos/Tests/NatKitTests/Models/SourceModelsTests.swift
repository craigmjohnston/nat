import XCTest
@testable import NatKit

/// The task-source shapes, decoded from the JSON
/// `docs/design/task-sources/README.md` gives for each — every field, the
/// least a plugin may send, and the words this build does not know.
final class SourceModelsTests: XCTestCase {
    private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
        try JSONDecoder().decode(T.self, from: Data(json.utf8))
    }

    // MARK: - PlanBackend

    func testPlanBackendWordTable() {
        let table: [(String?, PlanBackend)] = [
            (nil, .notion), ("", .notion), ("notion", .notion), ("local", .local),
            ("source", .source), ("Source", .notion), ("postgres", .notion),
        ]
        for (word, backend) in table {
            XCTAssertEqual(PlanBackend(word: word), backend, "word \(word ?? "nil")")
        }
    }

    func testProjectConfigCarriesItsSourceAndWritesItBack() throws {
        let json = #"{"name":"Work","working_dir":"/w","backend":"source","source":"shortcut","plan_dir":"/p"}"#
        let config = try decode(ProjectConfig.self, json)
        XCTAssertEqual(config.backend, .source)
        XCTAssertEqual(config.source, "shortcut")

        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(config)) as? [String: String])
        XCTAssertEqual(object, ["name": "Work", "working_dir": "/w", "backend": "source", "source": "shortcut", "plan_dir": "/p"])
        XCTAssertEqual(try decode(ProjectConfig.self, String(decoding: JSONEncoder().encode(config), as: UTF8.self)), config)
    }

    func testLocalProjectConfigStillWritesNoSource() throws {
        let config = ProjectConfig(name: "L", workingDir: "/w", backend: .local)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(config)) as? [String: String])
        XCTAssertEqual(object, ["name": "L", "working_dir": "/w", "backend": "local"])
    }

    func testConfigDocProjectCarriesItsSource() throws {
        let project = try decode(
            ConfigDocProject.self, #"{"name":"Work","working_dir":"/w","backend":"source","source":"demo"}"#)
        XCTAssertEqual(project.backend, .source)
        XCTAssertEqual(project.source, "demo")
        let plain = try decode(ConfigDocProject.self, #"{"name":"N","working_dir":"/n"}"#)
        XCTAssertNil(plain.source)
        let again = try JSONDecoder().decode(ConfigDocProject.self, from: JSONEncoder().encode(project))
        XCTAssertEqual(again, project)
    }

    func testCreatedProjectReadsItsSource() throws {
        let created = try decode(CreatedProject.self, #"{"id":"p","name":"Work","source":"demo"}"#)
        XCTAssertEqual(created.source, "demo")
        XCTAssertNil(try decode(CreatedProject.self, #"{"id":"p","name":"N"}"#).source)
    }

    // MARK: - Actions

    func testActionInputWords() throws {
        let actions = try decode([SourceAction].self, """
        [{"id": "a", "label": "A", "input": "none"},
         {"id": "b", "label": "B", "input": "text"},
         {"id": "c", "label": "C", "input": "choice", "options": ["x", "y"], "destructive": true},
         {"id": "d", "label": "D", "input": "slider"},
         {"id": "e", "label": "E"}]
        """)
        XCTAssertEqual(actions.map(\.input), [.none, .text, .choice, .unknown("slider"), .none])
        XCTAssertEqual(actions[2].options, ["x", "y"])
        XCTAssertTrue(actions[2].destructive)
        XCTAssertFalse(actions[0].destructive)
        XCTAssertEqual(actions.map(\.input.word), ["none", "text", "choice", "slider", "none"])

        let again = try JSONDecoder().decode([SourceAction].self, from: JSONEncoder().encode(actions))
        XCTAssertEqual(again, actions)
    }

    func testAnActionWithFieldsOfTheWrongTypeStillReads() throws {
        let action = try decode(SourceAction.self, #"{"id": "a", "label": 3, "input": 7, "options": "no", "destructive": "yes"}"#)
        XCTAssertEqual(action, SourceAction(id: "a", label: ""))
    }

    // MARK: - info's source

    private let sourceJSON = """
    {
      "name": "shortcut", "title": "Shortcut", "tag": "SC",
      "icon_symbol": "rectangle.stack", "icon_svg": "<svg/>",
      "container_noun": "card", "task_noun": "task",
      "menu": [ { "id": "refresh", "label": "Refresh", "input": "none" } ],
      "groups": [
        { "id": "doing", "label": "Doing", "count": 2,
          "containers": [
            { "id": "4821", "title": "Improve diff review ergonomics",
              "external_url": "https://app.shortcut.com/acme/story/4821",
              "badges": [ { "text": "NA", "color": "#4f6bd8", "title": "Native App" } ],
              "meta": "3 pts" },
            { "id": "4790", "title": "Board mouse support" }
          ] },
        { "id": "ready", "label": "Ready", "count": 3,
          "children": [
            { "id": "seg-mine", "label": "Mine", "count": 1,
              "menu": [ { "id": "segment-owner", "label": "Owner", "input": "choice", "options": ["any", "me"] } ],
              "containers": [ { "id": "4802", "title": "Kanban column view" }, { "id": "4821", "title": "dup" } ] }
          ] },
        { "id": "done", "label": "Done", "count": 36, "lazy": true },
        { "id": "_unlisted", "label": "Other cards", "count": 1,
          "containers": [ { "id": "4611", "title": "Search result ranking" } ] }
      ],
      "error": ""
    }
    """

    func testSourceInfoDecodesEveryField() throws {
        let info = try decode(SourceInfo.self, sourceJSON)
        XCTAssertEqual(info.name, "shortcut")
        XCTAssertEqual(info.title, "Shortcut")
        XCTAssertEqual(info.tag, "SC")
        XCTAssertEqual(info.iconSymbol, "rectangle.stack")
        XCTAssertEqual(info.iconSVG, "<svg/>")
        XCTAssertEqual(info.containerNoun, "card")
        XCTAssertEqual(info.taskNoun, "task")
        XCTAssertEqual(info.menu, [SourceAction(id: "refresh", label: "Refresh")])
        XCTAssertNil(info.error, "an empty error is no error")
        XCTAssertEqual(info.groups.map(\.id), ["doing", "ready", "done", "_unlisted"])

        let first = info.groups[0].containers[0]
        XCTAssertEqual(first.externalURL, "https://app.shortcut.com/acme/story/4821")
        XCTAssertEqual(first.badges, [SourceBadge(text: "NA", color: "#4f6bd8", title: "Native App")])
        XCTAssertEqual(first.meta, "3 pts")
        let bare = info.groups[0].containers[1]
        XCTAssertNil(bare.externalURL)
        XCTAssertEqual(bare.badges, [])
        XCTAssertNil(bare.meta)
        XCTAssertEqual(bare.menu, [])

        let done = info.groups[2]
        XCTAssertTrue(done.lazy)
        XCTAssertEqual(done.count, 36)
        XCTAssertEqual(done.containers, [])
        XCTAssertFalse(info.groups[0].lazy)

        let again = try JSONDecoder().decode(SourceInfo.self, from: JSONEncoder().encode(info))
        XCTAssertEqual(again, info)
    }

    func testAllContainersIsDepthFirstAndEachOnce() throws {
        let info = try decode(SourceInfo.self, sourceJSON)
        XCTAssertEqual(info.allContainers.map(\.id), ["4821", "4790", "4802", "4611"])
        // The first appearance wins.
        XCTAssertEqual(info.allContainers[0].title, "Improve diff review ergonomics")
    }

    func testGroupWithIDFindsAtAnyDepth() throws {
        let info = try decode(SourceInfo.self, sourceJSON)
        XCTAssertEqual(info.group(withID: "doing")?.label, "Doing")
        XCTAssertEqual(info.group(withID: "seg-mine")?.menu.first?.input, .choice)
        XCTAssertNil(info.group(withID: "nowhere"))
    }

    func testAFailedSourceReadsWithItsError() throws {
        let info = try decode(SourceInfo.self, """
        {"name": "shortcut", "error": "source plugin shortcut: token missing",
         "groups": [{"id": "_unlisted", "label": "Other cards", "containers": [{"id": "1", "title": "T"}]}]}
        """)
        XCTAssertEqual(info.error, "source plugin shortcut: token missing")
        XCTAssertEqual(info.title, "")
        XCTAssertNil(info.iconSVG)
        XCTAssertEqual(info.menu, [])
        XCTAssertEqual(info.groups.first?.id, SourceGroup.unlistedID)
        XCTAssertNil(info.groups.first?.count)
    }

    func testInfoWithAndWithoutSource() throws {
        let base = #""project": {"id": "p", "name": "Work", "conventions": ""}, "milestones": [], "slices": []"#
        XCTAssertNil(try decode(ProjectInfo.self, "{\(base)}").source)
        let info = try decode(ProjectInfo.self, "{\(base), \"source\": \(sourceJSON)}")
        XCTAssertEqual(info.source?.tag, "SC")
        let again = try JSONDecoder().decode(ProjectInfo.self, from: JSONEncoder().encode(info))
        XCTAssertEqual(again, info)
    }

    // MARK: - container-show

    private let containerJSON = """
    {
      "id": "4821",
      "title": "Improve diff review ergonomics",
      "external_url": "https://app.shortcut.com/acme/story/4821",
      "facts": [
        { "label": "id", "value": "sc-4821" },
        { "label": "project", "value": "NA · Native App", "color": "#4f6bd8" }
      ],
      "sections": [
        { "id": "story", "title": "Story", "kind": "prose", "body": "Comments **never** arrive." },
        { "id": "comments", "title": "Comments", "kind": "comments",
          "comments": [ { "by": "Dana Wolfe", "when": "3d ago", "text": "Pairs with highlighting." } ],
          "composer": { "id": "comment", "label": "Comment", "input": "text" } },
        { "id": "links", "title": "Links", "kind": "links",
          "links": [
            { "label": "PR #418", "text": "Diff comments reach the agent", "state": "open", "url": "https://github.com/acme/app/pull/418" },
            { "label": "Design", "text": "Figma" }
          ] },
        { "id": "chart", "title": "Burndown", "kind": "chart" }
      ],
      "menu": [ { "id": "unassign-me", "label": "Remove Me as Owner", "input": "none" } ],
      "task_note": "Linked to sc-4821."
    }
    """

    func testContainerDetailDecodesEveryField() throws {
        let detail = try decode(ContainerDetail.self, containerJSON)
        XCTAssertEqual(detail.id, "4821")
        XCTAssertEqual(detail.externalURL, "https://app.shortcut.com/acme/story/4821")
        XCTAssertEqual(detail.facts, [
            SourceFact(label: "id", value: "sc-4821"),
            SourceFact(label: "project", value: "NA · Native App", color: "#4f6bd8"),
        ])
        XCTAssertEqual(detail.sections.map(\.kind), [.prose, .comments, .links, .unknown("chart")])
        XCTAssertEqual(detail.sections.map(\.kind.word), ["prose", "comments", "links", "chart"])
        XCTAssertEqual(detail.sections[0].body, "Comments **never** arrive.")
        XCTAssertEqual(detail.sections[1].comments, [SourceComment(by: "Dana Wolfe", when: "3d ago", text: "Pairs with highlighting.")])
        XCTAssertEqual(detail.sections[1].composer, SourceAction(id: "comment", label: "Comment", input: .text))
        XCTAssertEqual(detail.sections[2].links[0].state, "open")
        XCTAssertEqual(detail.sections[2].links[1], SourceLink(label: "Design", text: "Figma"))
        XCTAssertNil(detail.sections[0].composer)
        XCTAssertEqual(detail.menu.map(\.id), ["unassign-me"])
        XCTAssertEqual(detail.taskNote, "Linked to sc-4821.")

        let again = try JSONDecoder().decode(ContainerDetail.self, from: JSONEncoder().encode(detail))
        XCTAssertEqual(again, detail)
    }

    func testMinimalContainerDetail() throws {
        let detail = try decode(ContainerDetail.self, #"{"id": "1", "title": "T"}"#)
        XCTAssertEqual(detail, ContainerDetail(id: "1", title: "T"))
    }

    func testContainerShowCarriesTheTasksInInfosSliceShape() throws {
        let show = try decode(ContainerShow.self, """
        {"container": \(containerJSON),
         "tasks": [{"id": "t1", "name": "Task", "status": "Todo", "milestone_id": "4821", "assignee": "",
                    "pr": "", "url": "", "blocked": false, "handed_back": false}]}
        """)
        XCTAssertEqual(show.container.title, "Improve diff review ergonomics")
        XCTAssertEqual(show.tasks.map(\.milestoneID), ["4821"])
        XCTAssertEqual(try decode(ContainerShow.self, #"{"container": {"id": "1", "title": "T"}}"#).tasks, [])
    }

    // MARK: - source-list

    func testSourceListRows() throws {
        let plugins = try decode([SourcePlugin].self, """
        [
          { "name": "demo", "path": "/p/nat-source-demo",
            "describe": { "protocol": 1, "name": "demo", "title": "Demo source", "tag": "DM",
                          "icon_symbol": "rectangle.on.rectangle.angled", "icon_svg": "<svg/>",
                          "container_noun": "card", "task_noun": "task",
                          "menu": [ { "id": "refresh", "label": "Refresh", "input": "none" } ] } },
          { "name": "shortcut", "path": "/opt/homebrew/bin/nat-source-shortcut",
            "error": "source plugin shortcut speaks protocol 2; this nat speaks protocol 1" }
        ]
        """)
        XCTAssertEqual(plugins.map(\.id), ["demo", "shortcut"])
        let demo = try XCTUnwrap(plugins[0].describe)
        XCTAssertEqual(demo.protocol, 1)
        XCTAssertEqual(demo.iconSVG, "<svg/>")
        XCTAssertEqual(demo.containerNoun, "card")
        XCTAssertEqual(demo.taskNoun, "task")
        XCTAssertEqual(demo.menu.count, 1)
        XCTAssertNil(plugins[0].error)
        XCTAssertEqual(plugins[0].displayTitle, "Demo source")
        XCTAssertEqual(plugins[0].iconSymbol, "rectangle.on.rectangle.angled")
        XCTAssertEqual(plugins[0].executableName, "nat-source-demo")

        XCTAssertNil(plugins[1].describe)
        XCTAssertEqual(plugins[1].error, "source plugin shortcut speaks protocol 2; this nat speaks protocol 1")
        XCTAssertEqual(plugins[1].displayTitle, "shortcut", "a plugin that would not describe is named by its name")
        XCTAssertEqual(plugins[1].iconSymbol, "puzzlepiece.extension")

        let again = try JSONDecoder().decode([SourcePlugin].self, from: JSONEncoder().encode(plugins))
        XCTAssertEqual(again, plugins)
    }

    func testADescribeWithNoProtocolReadsAsZero() throws {
        let describe = try decode(SourceDescribe.self, #"{"name": "x"}"#)
        XCTAssertEqual(describe.protocol, 0)
        XCTAssertEqual(SourcePlugin(name: "x", path: "", describe: describe).displayTitle, "x")
    }

    // MARK: - slice-show's container

    func testSliceShowWithAndWithoutContainer() throws {
        let base = """
        "id": "t1", "name": "Task", "url": "", "status": "Todo", "milestone": "Card",
        "assignee": "", "blocked": false, "handed_back": false, "brief": ""
        """
        XCTAssertNil(try decode(SliceDetail.self, "{\(base)}").container)

        let detail = try decode(SliceDetail.self, """
        {\(base), "container": {"id": "4821", "title": "Improve diff review ergonomics",
          "external_url": "https://x/4821", "task_note": "Linked.",
          "facts": [{"label": "id", "value": "sc-4821"}]}}
        """)
        XCTAssertEqual(detail.container, SliceContainer(
            id: "4821", title: "Improve diff review ergonomics", externalURL: "https://x/4821",
            taskNote: "Linked.", facts: [SourceFact(label: "id", value: "sc-4821")]))

        // A failed container read fills only the id and the cached title.
        let failed = try decode(SliceDetail.self, #"{\#(base), "container": {"id": "4821", "title": "Cached"}}"#)
        XCTAssertEqual(failed.container, SliceContainer(id: "4821", title: "Cached"))
    }

    // MARK: - source-action

    func testSourceActionResult() throws {
        XCTAssertEqual(try decode(SourceActionResult.self, #"{"message": "Done"}"#).message, "Done")
        XCTAssertNil(try decode(SourceActionResult.self, "{}").message)
    }
}
