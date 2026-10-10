import XCTest
@testable import NatKit
import NatFixtures

/// The app menu's About gnat leaves a request for the Settings window, which
/// takes it once — so Settings opened any other way keeps its section.
@MainActor
final class SettingsAboutRequestTests: XCTestCase {
    func testAboutIsRequestedThenTakenOnce() async {
        let appModel = await Fixtures.startedAppModel(client: FixtureNatClient(agents: []))
        XCTAssertFalse(appModel.takeSettingsAboutRequest(), "nothing asked: ⌘, opens as it does")
        appModel.requestSettingsAbout()
        XCTAssertTrue(appModel.settingsAboutRequested)
        XCTAssertTrue(appModel.takeSettingsAboutRequest())
        XCTAssertFalse(appModel.takeSettingsAboutRequest(), "answered once")
        XCTAssertFalse(appModel.settingsAboutRequested)
    }
}
