import XCTest
@testable import NatKit
import NatFixtures

final class ClaudeVersionTests: XCTestCase {
    /// `nat claude-version --json`, both sides read.
    func testDecodesBothSides() throws {
        let json = #"{"installed": "2.1.294", "latest": "2.1.295", "update_available": true}"#
        let version = try JSONDecoder().decode(ClaudeVersion.self, from: Data(json.utf8))
        XCTAssertEqual(version, ClaudeVersion(installed: "2.1.294", latest: "2.1.295", updateAvailable: true))
    }

    /// A side nat could not read is absent from the wire, and nil here.
    func testDecodesAnAbsentSide() throws {
        let json = #"{"installed": "2.1.294", "update_available": false}"#
        let version = try JSONDecoder().decode(ClaudeVersion.self, from: Data(json.utf8))
        XCTAssertEqual(version.installed, "2.1.294")
        XCTAssertNil(version.latest)
        XCTAssertFalse(version.updateAvailable)
    }

    func testDecodesTheUpdateOutput() throws {
        let json = #"{"output": "Successfully updated\n"}"#
        XCTAssertEqual(try JSONDecoder().decode(ClaudeUpdateResult.self, from: Data(json.utf8)).output,
                       "Successfully updated\n")
    }

    /// The notice is there only where nat says an update is available.
    func testNoticeOnlyWithAnUpdateAvailable() {
        XCTAssertEqual(Fixtures.claudeVersionBehind.notice, "Claude Code 2.1.295 available")
        XCTAssertNil(Fixtures.claudeVersionCurrent.notice)
        XCTAssertNil(ClaudeVersion(installed: nil, latest: "2.1.295", updateAvailable: false).notice)
        XCTAssertNil(ClaudeVersion(installed: "2.1.294", latest: nil, updateAvailable: true).notice)
    }
}

@MainActor
final class ClaudeVersionStoreTests: XCTestCase {
    func testStartReadsTheVersion() async {
        let store = ClaudeVersionStore(client: FixtureNatClient(claudeVersion: Fixtures.claudeVersionBehind))
        XCTAssertNil(store.notice)
        await store.start()
        store.stop()
        XCTAssertEqual(store.notice, "Claude Code 2.1.295 available")
    }

    func testUpToDateDrawsNoNotice() async {
        let store = ClaudeVersionStore(client: FixtureNatClient(claudeVersion: Fixtures.claudeVersionCurrent))
        await store.refresh()
        XCTAssertNotNil(store.version)
        XCTAssertNil(store.notice)
    }

    /// A read that fails concludes nothing: no notice, nothing thrown.
    func testAFailedReadDrawsNothing() async {
        let store = ClaudeVersionStore(client: FixtureNatClient(behaviour: .refusing("boom")))
        await store.refresh()
        XCTAssertNil(store.version)
    }

    /// The update runs through nat's `claude-update`, its output the sheet's.
    func testUpdateRecordsClaudeUpdateAndShowsItsOutput() async {
        let client = FixtureNatClient(claudeVersion: Fixtures.claudeVersionBehind)
        let store = ClaudeVersionStore(client: client)
        await store.runUpdate()
        XCTAssertEqual(client.writes, ["claude-update"])
        XCTAssertEqual(store.update, .finished(output: Fixtures.claudeUpdateOutput))
        store.dismissUpdate()
        XCTAssertNil(store.update)
    }

    /// A failed update shows nat's refusal, claude's own words in it.
    func testAFailedUpdateShowsWhy() async {
        let client = FixtureNatClient(claudeUpdateOutput: nil)
        let store = ClaudeVersionStore(client: client)
        await store.runUpdate()
        guard case .failed(let message) = store.update else {
            return XCTFail("update = \(String(describing: store.update)), want failed")
        }
        XCTAssertTrue(message.contains("could not write to the install directory"), message)
    }

    /// While one runs, a second click does nothing and the sheet stays.
    func testOneUpdateAtATime() async {
        let store = ClaudeVersionStore(client: FixtureNatClient(behaviour: .hanging))
        let first = Task { await store.runUpdate() }
        while store.update != .running { await Task.yield() }
        await store.runUpdate()
        store.dismissUpdate()
        XCTAssertEqual(store.update, .running)
        first.cancel()
    }

    /// A client that never learned the command refuses it, as every test
    /// double does.
    func testDefaultConformanceRefuses() async {
        let client = SequencedUsageClient([.reading(.empty)])
        do {
            _ = try await client.claudeVersion()
            XCTFail("want a refusal")
        } catch {}
        do {
            _ = try await client.claudeUpdate()
            XCTFail("want a refusal")
        } catch {}
    }
}
