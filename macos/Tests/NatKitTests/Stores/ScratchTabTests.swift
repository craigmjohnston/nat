import XCTest
import NatFixtures
@testable import NatKit

/// The scratch tab as `AppModel` sees it: opened and cleared once per launch
/// before its first read, pinned first, never closable, and asking for a
/// folder before a New Session.
@MainActor
final class ScratchTabTests: XCTestCase {
    private let scratchID = Fixtures.scratchProjectID
    private let projectID = Fixtures.projectID

    private func model(
        client: FixtureNatClient = FixtureNatClient(),
        config: NatProjectConfig = Fixtures.scratchConfig
    ) -> AppModel {
        Fixtures.appModel(client: client, config: config)
    }

    // MARK: - Launch

    func testStartOpensThenClearsTheScratchProjectOnce() async {
        let client = FixtureNatClient()
        let appModel = model(client: client)

        await appModel.start()
        await appModel.start()

        XCTAssertEqual(client.writes, ["scratch-open", "done-clear \(scratchID)"],
                       "once per launch: onboarding finishing must not clear a second time")
    }

    func testAFailedClearIsLoggedAndTheLaunchGoesOn() async {
        let client = FixtureNatClient()
        client.armDoneClearFailure("boom")
        let appModel = model(client: client)

        await appModel.start()

        XCTAssertEqual(appModel.projectTabs.first?.id, scratchID)
        XCTAssertNotNil(appModel.projectStore)
    }

    func testAScratchOpenThatFailsLeavesNoClearAndTriesAgain() async {
        let refusing = FixtureNatClient(behaviour: .refusing("no such command"))
        let appModel = model(client: refusing)

        await appModel.start()
        await appModel.start()

        XCTAssertEqual(refusing.writes, [], "a refused scratch-open records nothing and clears nothing")
    }

    // MARK: - Tabs

    func testTheScratchTabIsPinnedFirstAheadOfTheIDSort() async {
        let appModel = model()
        await appModel.start(configPath: "/c", nudgePath: "/n")

        XCTAssertEqual(appModel.projectTabs.map(\.id), [scratchID, projectID])
        XCTAssertEqual(appModel.scratchProjectID, scratchID)
        XCTAssertTrue(appModel.isScratchTab(scratchID))
        XCTAssertFalse(appModel.isScratchTab(projectID))
    }

    func testLaunchActivatesTheFirstRealProjectNotTheScratchTab() async {
        let appModel = model()
        await appModel.start(configPath: "/c", nudgePath: "/n")

        XCTAssertEqual(appModel.activeProjectID, projectID)
        XCTAssertFalse(appModel.activeTabIsScratch)
    }

    func testTheScratchTabAloneIsActivated() async {
        let only = NatProjectConfig(
            projects: [scratchID: ProjectConfig(name: "Scratch", slicesDSID: "", workingDir: "/Users/craig")],
            scratchProject: scratchID
        )
        let appModel = model(config: only)
        await appModel.start(configPath: "/c", nudgePath: "/n")

        XCTAssertEqual(appModel.activeProjectID, scratchID)
        XCTAssertTrue(appModel.activeTabIsScratch)
    }

    func testAScratchIDConfigDoesNotTrackIsNotPinned() async {
        var config = Fixtures.config
        config = NatProjectConfig(projects: config.projects, scratchProject: "not-a-project")
        let appModel = model(config: config)
        await appModel.start(configPath: "/c", nudgePath: "/n")

        XCTAssertNil(appModel.scratchProjectID)
        XCTAssertEqual(appModel.projectTabs.map(\.id), [projectID])
    }

    // MARK: - Closing

    func testCloseRefusesTheScratchTab() async {
        let appModel = model()
        await appModel.start(configPath: "/c", nudgePath: "/n")

        await appModel.closeProject(scratchID)

        XCTAssertEqual(appModel.projectTabs.map(\.id), [scratchID, projectID])
    }

    func testTheLastRealTabIsNotClosableBesideScratch() async {
        let appModel = model()
        await appModel.start(configPath: "/c", nudgePath: "/n")

        XCTAssertEqual(appModel.closableTabCount, 1)
        await appModel.closeProject(projectID)

        XCTAssertEqual(appModel.projectTabs.map(\.id), [scratchID, projectID],
                       "the scratch tab is not counted, so this is the last tab standing")
    }

    func testARealTabClosesOnceThereIsAnotherRealOne() async {
        let appModel = model(config: Fixtures.scratchConfigWithSecondProject)
        await appModel.start(configPath: "/c", nudgePath: "/n")

        XCTAssertEqual(appModel.closableTabCount, 2)
        await appModel.closeProject(Fixtures.secondProjectID)

        XCTAssertEqual(appModel.projectTabs.map(\.id), [scratchID, projectID])
    }

    // MARK: - New Session

    func testTheNewSessionButtonAsksForAFolderOnlyOnTheScratchTab() async {
        let appModel = model()
        await appModel.start(configPath: "/c", nudgePath: "/n")
        XCTAssertFalse(appModel.newSessionNeedsFolder)

        await appModel.activateProject(scratchID)
        XCTAssertTrue(appModel.newSessionNeedsFolder)
    }

    func testAScratchSessionPassesItsFolderAndRemembersIt() async {
        let client = FixtureNatClient()
        let appModel = model(client: client)
        await appModel.start(configPath: "/c", nudgePath: "/n")
        await appModel.activateProject(scratchID)
        XCTAssertNil(appModel.lastSessionFolder)

        await appModel.launchSession(dir: "/Users/craig/Projects/somewhere")

        XCTAssertTrue(client.writes.contains("session-launch /Users/craig/Projects/somewhere"))
        XCTAssertEqual(appModel.lastSessionFolder, "/Users/craig/Projects/somewhere")
    }

    func testAProjectSessionLaunchesStraightAwayAndRemembersNoFolder() async {
        let client = FixtureNatClient()
        let appModel = model(client: client)
        await appModel.start(configPath: "/c", nudgePath: "/n")

        await appModel.launchSession()

        XCTAssertTrue(client.writes.contains("session-launch "), "no --dir: the project's own directory")
        XCTAssertNil(appModel.lastSessionFolder)
    }

    func testAFailedScratchLaunchDoesNotRememberItsFolder() async {
        let appModel = model(client: FixtureNatClient(behaviour: .refusing("no tmux")))
        await appModel.start(configPath: "/c", nudgePath: "/n")
        await appModel.activateProject(scratchID)

        await appModel.launchSession(dir: "/x")

        XCTAssertNil(appModel.lastSessionFolder)
        XCTAssertEqual(appModel.newSessionError, "no tmux")
    }
}
