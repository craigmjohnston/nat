import XCTest
@testable import NatKit

final class LaunchPlanTests: XCTestCase {
    func testLaunchableSliceTodo() {
        let slice = Slice(
            id: "slice-1",
            name: "Test",
            status: "Todo",
            milestoneID: "m1",
            assignee: "",
            pr: "",
            url: "https://example.com",
            blocked: false,
            handedBack: false
        )

        let plan = LaunchPlan(for: slice, hasLiveAgent: false)
        XCTAssertTrue(plan.canLaunch)
        XCTAssertNil(plan.blockedBy)
    }

    func testLaunchableSliceInProgressNoAgent() {
        let slice = Slice(
            id: "slice-1",
            name: "Test",
            status: "In progress",
            milestoneID: "m1",
            assignee: "",
            pr: "",
            url: "https://example.com",
            blocked: false,
            handedBack: false
        )

        let plan = LaunchPlan(for: slice, hasLiveAgent: false)
        XCTAssertTrue(plan.canLaunch)
        XCTAssertNil(plan.blockedBy)
    }

    func testNotLaunchableSliceInProgressWithAgent() {
        let slice = Slice(
            id: "slice-1",
            name: "Test",
            status: "In progress",
            milestoneID: "m1",
            assignee: "",
            pr: "",
            url: "https://example.com",
            blocked: false,
            handedBack: false
        )

        let plan = LaunchPlan(for: slice, hasLiveAgent: true)
        XCTAssertFalse(plan.canLaunch)
        XCTAssertNil(plan.blockedBy)
    }

    func testNotLaunchableSliceBlocked() {
        let slice = Slice(
            id: "slice-1",
            name: "Test",
            status: "Todo",
            milestoneID: "m1",
            assignee: "",
            pr: "",
            url: "https://example.com",
            dependsOn: ["slice-0", "slice-5"],
            blocked: true,
            handedBack: false
        )

        let plan = LaunchPlan(for: slice, hasLiveAgent: false)
        XCTAssertFalse(plan.canLaunch)
        XCTAssertEqual(plan.blockedBy, ["slice-0", "slice-5"])
    }

    func testNotLaunchableSliceDone() {
        let slice = Slice(
            id: "slice-1",
            name: "Test",
            status: "Done",
            milestoneID: "m1",
            assignee: "",
            pr: "",
            url: "https://example.com",
            blocked: false,
            handedBack: false
        )

        let plan = LaunchPlan(for: slice, hasLiveAgent: false)
        XCTAssertFalse(plan.canLaunch)
        XCTAssertNil(plan.blockedBy)
    }

    func testBuildFlagsWithBoth() {
        let flags = LaunchPlan.buildFlags(model: "opus", effort: "high")
        XCTAssertEqual(flags, ["--model", "opus", "--effort", "high"])
    }

    func testBuildFlagsWithModelOnly() {
        let flags = LaunchPlan.buildFlags(model: "sonnet", effort: nil)
        XCTAssertEqual(flags, ["--model", "sonnet"])
    }

    func testBuildFlagsWithEffortOnly() {
        let flags = LaunchPlan.buildFlags(model: nil, effort: "low")
        XCTAssertEqual(flags, ["--effort", "low"])
    }

    func testBuildFlagsWithNeither() {
        let flags = LaunchPlan.buildFlags(model: nil, effort: nil)
        XCTAssertTrue(flags.isEmpty)
    }

    func testBuildFlagsIgnoresDefault() {
        let flags = LaunchPlan.buildFlags(model: "Default", effort: "Default")
        XCTAssertTrue(flags.isEmpty)
    }

    func testBuildFlagsIgnoresEmptyStrings() {
        let flags = LaunchPlan.buildFlags(model: "", effort: "")
        XCTAssertTrue(flags.isEmpty)
    }
}
