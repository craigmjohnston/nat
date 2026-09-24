import XCTest
@testable import NatKit

final class AgentReadoutTests: XCTestCase {
    private func agent(model: String? = nil, effort: String? = nil, context: Double? = nil) -> AgentStatus {
        AgentStatus(sliceID: "s", session: "nat-s", activity: .working,
                    model: model, effort: effort, contextPercent: context)
    }

    func testNoAgentDrawsNothing() {
        XCTAssertNil(buildAgentReadout(from: nil))
    }

    func testNoReadingYetDrawsNothingNotZeros() {
        XCTAssertNil(buildAgentReadout(from: agent()))
        XCTAssertNil(buildAgentReadout(from: agent(model: "", effort: "")))
    }

    func testFullReading() {
        let readout = buildAgentReadout(from: agent(model: "Sonnet 5", effort: "high", context: 41.6))
        XCTAssertEqual(readout?.label, "Sonnet 5 · high")
        XCTAssertEqual(readout?.context, AgentReadout.Context(text: "42%", warning: false))
    }

    func testPartialReadings() {
        XCTAssertEqual(buildAgentReadout(from: agent(model: "Sonnet 5"))?.label, "Sonnet 5")
        XCTAssertNil(buildAgentReadout(from: agent(model: "Sonnet 5"))?.context)
        XCTAssertNil(buildAgentReadout(from: agent(context: 10))?.label)
        XCTAssertEqual(buildAgentReadout(from: agent(context: 0))?.context?.text, "0%")
    }

    func testHighContextWarnsAtThreshold() {
        XCTAssertFalse(buildAgentReadout(from: agent(context: 79.9))!.context!.warning)
        XCTAssertTrue(buildAgentReadout(from: agent(context: 80))!.context!.warning)
    }

    func testStatusDecodesReadoutFields() throws {
        let json = #"{"slice_id":"a","session":"b","activity":"working","model":"Sonnet 5","effort":"high","context_percent":12.5}"#
        let status = try JSONDecoder().decode(AgentStatus.self, from: Data(json.utf8))
        XCTAssertEqual(status.model, "Sonnet 5")
        XCTAssertEqual(status.effort, "high")
        XCTAssertEqual(status.contextPercent, 12.5)
    }
}
