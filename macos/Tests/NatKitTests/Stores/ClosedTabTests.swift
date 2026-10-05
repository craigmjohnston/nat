import XCTest
@testable import NatKit

/// A closed project's tab stays closed across launches: `closeProject`
/// records it in `ClosedTabMemory`, `start` leaves it out of the strip, and
/// opening it again from the "+" tab forgets the close. Every model here is
/// given an in-memory `ClosedTabMemory` (or one over a throwaway suite),
/// never the real defaults.
@MainActor
final class ClosedTabTests: XCTestCase {
    private func config(_ ids: [String], scratch: String? = nil) -> NatProjectConfig {
        NatProjectConfig(
            projects: Dictionary(uniqueKeysWithValues: ids.map {
                ($0, ProjectConfig(name: "\($0) name", slicesDSID: "ds-\($0)", workingDir: "/path/\($0)"))
            }),
            scratchProject: scratch)
    }

    /// One launch of the app over `config` and `memory` — a relaunch is
    /// another call over the same memory.
    private func launch(_ config: NatProjectConfig, memory: ClosedTabMemory) async -> AppModel {
        let appModel = AppModel(configReader: MockConfigReader(response: .success(config)), closedTabMemory: memory)
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        return appModel
    }

    // MARK: - Recording a close

    func testAClosedTabIsRecordedAndStaysClosedAtTheNextLaunch() async {
        let memory = ClosedTabMemory.inMemory()
        let first = await launch(config(["proj-a", "proj-b", "proj-c"]), memory: memory)

        await first.closeProject("proj-b")

        XCTAssertEqual(memory.closed, ["proj-b"])
        let relaunched = await launch(config(["proj-a", "proj-b", "proj-c"]), memory: memory)
        XCTAssertEqual(relaunched.projectTabs.map(\.id), ["proj-a", "proj-c"],
                       "every other configured project still has its tab")
        XCTAssertNotNil(relaunched.config?.projects["proj-b"], "the config entry is untouched")
    }

    func testARefusedCloseRecordsNothing() async {
        let memory = ClosedTabMemory.inMemory()
        let appModel = await launch(config(["proj-a"]), memory: memory)

        await appModel.closeProject("proj-a")

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-a"])
        XCTAssertEqual(memory.closed, [])
    }

    func testTheScratchTabIsNeverClosedOrRecorded() async {
        let memory = ClosedTabMemory.inMemory()
        let appModel = await launch(config(["proj-a", "proj-b", "scratch"], scratch: "scratch"), memory: memory)

        await appModel.closeProject("scratch")

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["scratch", "proj-a", "proj-b"])
        XCTAssertEqual(memory.closed, [])
    }

    func testAnUntitledTabIsNeverRecorded() async {
        let memory = ClosedTabMemory.inMemory()
        let appModel = await launch(config(["proj-a"]), memory: memory)
        let untitled = appModel.openUntitledTab()

        await appModel.closeProject(untitled)

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-a"])
        XCTAssertEqual(memory.closed, [])
    }

    // MARK: - Honouring it at start

    func testACloseOfAProjectConfigNoLongerNamesIsDroppedAtStart() async {
        let memory = ClosedTabMemory.inMemory()
        memory.close("proj-gone")
        memory.close("proj-b")

        let appModel = await launch(config(["proj-a", "proj-b"]), memory: memory)

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-a"])
        XCTAssertEqual(memory.closed, ["proj-b"])
    }

    /// Close every closable project but one, then that one's config entry
    /// goes: the closes are ignored for the launch rather than opening on an
    /// empty board — and kept, so a later launch with more open honours them.
    func testClosesThatWouldEmptyTheStripAreIgnoredForThatLaunch() async {
        let memory = ClosedTabMemory.inMemory()
        let first = await launch(config(["proj-a", "proj-b"]), memory: memory)
        await first.closeProject("proj-a")

        let relaunched = await launch(config(["proj-a"]), memory: memory)

        XCTAssertEqual(relaunched.projectTabs.map(\.id), ["proj-a"])
        XCTAssertEqual(relaunched.activeProjectID, "proj-a")
        XCTAssertEqual(memory.closed, ["proj-a"])
    }

    func testTheScratchTabAloneDoesNotCountAsSomethingToOpenOnto() async {
        let memory = ClosedTabMemory.inMemory()
        memory.close("proj-a")

        let appModel = await launch(config(["proj-a", "scratch"], scratch: "scratch"), memory: memory)

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["scratch", "proj-a"])
    }

    func testTheScratchProjectIsNeverFilteredOut() async {
        let memory = ClosedTabMemory.inMemory()
        memory.close("scratch")
        memory.close("proj-a")

        let appModel = await launch(config(["proj-a", "proj-b", "scratch"], scratch: "scratch"), memory: memory)

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["scratch", "proj-b"])
    }

    // MARK: - Reopening

    func testClosedProjectsAreTheConfiguredOnesWithNoTabButScratch() async {
        let memory = ClosedTabMemory.inMemory()
        let appModel = await launch(config(["proj-a", "proj-b", "proj-c", "scratch"], scratch: "scratch"),
                                    memory: memory)
        XCTAssertEqual(appModel.closedProjects, [])

        await appModel.closeProject("proj-c")
        await appModel.closeProject("proj-a")

        XCTAssertEqual(appModel.closedProjects, [
            ProjectListingEntry(id: "proj-a", name: "proj-a name", configured: true, workingDir: "/path/proj-a"),
            ProjectListingEntry(id: "proj-c", name: "proj-c name", configured: true, workingDir: "/path/proj-c"),
        ])
    }

    func testReopeningFromTheStarterClearsTheCloseAndSurvivesARelaunch() async {
        let memory = ClosedTabMemory.inMemory()
        let first = await launch(config(["proj-a", "proj-b"]), memory: memory)
        await first.closeProject("proj-b")
        let second = await launch(config(["proj-a", "proj-b"]), memory: memory)
        let untitled = second.openUntitledTab()

        await second.addProject(id: "proj-b", name: "proj-b name", replacing: untitled)

        XCTAssertEqual(second.projectTabs.map(\.id), ["proj-a", "proj-b"])
        XCTAssertEqual(second.activeProjectID, "proj-b")
        XCTAssertEqual(memory.closed, [])
        let third = await launch(config(["proj-a", "proj-b"]), memory: memory)
        XCTAssertEqual(third.projectTabs.map(\.id), ["proj-a", "proj-b"])
    }
}

final class ClosedTabMemoryTests: XCTestCase {
    func testClosesSurviveANewInstanceOverTheSameDefaults() throws {
        let suite = "nat.tests.closedtabs.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }

        let first = ClosedTabMemory(defaults: defaults)
        XCTAssertEqual(first.closed, [])
        first.close("a")
        first.close("a")
        first.close("b")
        first.close("c")
        XCTAssertEqual(ClosedTabMemory(defaults: defaults).closed, ["a", "b", "c"], "a relaunch still has them")

        first.reopen("a")
        first.reopen("never-closed")
        XCTAssertEqual(ClosedTabMemory(defaults: defaults).closed, ["b", "c"])

        first.prune(keeping: ["b", "c", "d"])
        XCTAssertEqual(ClosedTabMemory(defaults: defaults).closed, ["b", "c"])
        first.prune(keeping: ["c"])
        XCTAssertEqual(ClosedTabMemory(defaults: defaults).closed, ["c"])
    }

    func testAnInMemoryMemoryIsItsOwn() {
        let one = ClosedTabMemory.inMemory()
        one.close("a")
        XCTAssertEqual(one.closed, ["a"])
        XCTAssertEqual(ClosedTabMemory.inMemory().closed, [])
    }
}
