import XCTest
@testable import NatKit

final class ChecksControlsTests: XCTestCase {
    private func check(_ name: String, _ state: String, run: String? = "1") -> PRCheck {
        PRCheck(name: name, state: state, link: "", rerunnable: run != nil, run: run)
    }

    /// Two runs and another service: run 1 finished, one job failed; run 2
    /// going, a job running and a sibling queued.
    private var mixed: [PRCheck] {
        [
            check("Gate", "SUCCESS"), check("test", "FAILURE"),
            check("mac", "IN_PROGRESS", run: "2"), check("lint", "QUEUED", run: "2"),
            check("Vercel", "PENDING", run: nil),
        ]
    }

    func testRowControls() {
        let controls = ChecksControls(checks: mixed)
        let byName = Dictionary(uniqueKeysWithValues: mixed.map { ($0.name, $0) })
        XCTAssertEqual(controls.rerun(byName["Gate"]!), CheckControl(enabled: true))
        XCTAssertEqual(controls.cancel(byName["Gate"]!), CheckControl(enabled: false))
        XCTAssertEqual(controls.rerun(byName["mac"]!), CheckControl(enabled: true, stops: ["lint"]))
        XCTAssertEqual(controls.cancel(byName["mac"]!), CheckControl(enabled: true, stops: ["lint"]))
        XCTAssertEqual(controls.rerun(byName["lint"]!), CheckControl(enabled: false), "a queued job has not started")
        XCTAssertEqual(controls.cancel(byName["lint"]!), CheckControl(enabled: true, stops: ["mac"]))
        XCTAssertEqual(controls.rerun(byName["Vercel"]!), CheckControl(enabled: false))
        XCTAssertEqual(controls.cancel(byName["Vercel"]!), CheckControl(enabled: false))
    }

    func testHeadingControls() {
        let controls = ChecksControls(checks: mixed)
        XCTAssertTrue(controls.hasControls)
        XCTAssertTrue(controls.rerunAll)
        XCTAssertTrue(controls.rerunFailed)
        XCTAssertTrue(controls.cancelAll)

        let queued = ChecksControls(checks: [check("Gate", "QUEUED"), check("Vercel", "FAILURE", run: nil)])
        XCTAssertFalse(queued.rerunAll, "nothing has run")
        XCTAssertFalse(queued.rerunFailed, "only a check no run is behind failed")
        XCTAssertTrue(queued.cancelAll)

        let done = ChecksControls(checks: [check("Gate", "SUCCESS")])
        XCTAssertTrue(done.rerunAll)
        XCTAssertFalse(done.cancelAll)

        XCTAssertFalse(ChecksControls(checks: [check("Vercel", "SUCCESS", run: nil)]).hasControls)
    }

    func testTooltipsNameTheChecksStopped() {
        let controls = ChecksControls(checks: mixed)
        let byName = Dictionary(uniqueKeysWithValues: mixed.map { ($0.name, $0) })
        XCTAssertEqual(controls.rerunHelp(byName["Gate"]!), "Re-run Gate")
        XCTAssertEqual(controls.rerunHelp(byName["mac"]!), "Cancel and re-run mac — also stops lint, which share its run")
        XCTAssertEqual(controls.rerunHelp(byName["lint"]!), "lint has not started")
        XCTAssertEqual(controls.rerunHelp(byName["Vercel"]!), "Vercel has no GitHub Actions run to re-run")
        XCTAssertEqual(controls.cancelHelp(byName["lint"]!), "Cancel lint — also stops mac, which share its run")
        XCTAssertEqual(controls.cancelHelp(byName["Gate"]!), "Gate is not running")
        XCTAssertEqual(controls.cancelHelp(byName["Vercel"]!), "Vercel has no GitHub Actions run to cancel")

        let alone = ChecksControls(checks: [check("solo", "IN_PROGRESS")])
        XCTAssertEqual(alone.rerunHelp(alone.checks[0]), "Cancel and re-run solo")
        XCTAssertEqual(alone.cancelHelp(alone.checks[0]), "Cancel solo")
    }

    func testNotice() {
        XCTAssertEqual(checksActionNotice(ChecksActionResult(cancelled: ["a", "b"], rerun: ["a", "b", "c"])),
                       "Cancelled a and b, then re-ran a, b and c.")
        XCTAssertEqual(checksActionNotice(ChecksActionResult(rerun: ["a"])), "Re-ran a.")
        XCTAssertEqual(checksActionNotice(ChecksActionResult(cancelled: ["a"])), "Cancelled a.")
        XCTAssertEqual(checksActionNotice(ChecksActionResult(rerun: ["a"], skipped: ["V"])),
                       "Re-ran a; skipped V, which GitHub Actions does not run.")
        XCTAssertEqual(checksActionNotice(ChecksActionResult(skipped: ["V"])), "Skipped V, which GitHub Actions does not run.")
        XCTAssertEqual(checksActionNotice(ChecksActionResult()), "Nothing to do.")
        XCTAssertEqual(listed([]), "")
    }

    func testDecoding() throws {
        let check = try JSONDecoder().decode(PRCheck.self, from: Data(
            #"{"name":"test","state":"FAILURE","link":"l","rerunnable":true,"run":"11"}"#.utf8))
        XCTAssertEqual(check, PRCheck(name: "test", state: "FAILURE", link: "l", rerunnable: true, run: "11"))
        let old = try JSONDecoder().decode(PRCheck.self, from: Data(#"{"name":"test","state":"FAILURE","link":"l"}"#.utf8))
        XCTAssertFalse(old.rerunnable)
        XCTAssertNil(old.run)

        let result = try JSONDecoder().decode(ChecksActionResult.self, from: Data(#"{"cancelled":["a"],"skipped":[]}"#.utf8))
        XCTAssertEqual(result, ChecksActionResult(cancelled: ["a"]))
    }

    func testArguments() {
        XCTAssertEqual(NatClient.checksRerunArguments(projectID: "p", sliceRef: "s", mode: .all),
                       ["slice-checks-rerun", "s", "--project", "p", "--json", "--all"])
        XCTAssertEqual(NatClient.checksRerunArguments(projectID: "p", sliceRef: "s", mode: .failed),
                       ["slice-checks-rerun", "s", "--project", "p", "--json", "--failed"])
        XCTAssertEqual(NatClient.checksRerunArguments(projectID: "p", sliceRef: "s", mode: .checks(["a", "b"])),
                       ["slice-checks-rerun", "s", "--project", "p", "--json", "--check", "a", "--check", "b"])
        XCTAssertEqual(NatClient.checksCancelArguments(projectID: "p", sliceRef: "s", checks: []),
                       ["slice-checks-cancel", "s", "--project", "p", "--json"])
        XCTAssertEqual(NatClient.checksCancelArguments(projectID: "p", sliceRef: "s", checks: ["a"]),
                       ["slice-checks-cancel", "s", "--project", "p", "--json", "--check", "a"])
    }
}
