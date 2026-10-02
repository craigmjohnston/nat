import XCTest
@testable import NatKit

final class AgentReadoutTests: XCTestCase {
    private func agent(
        model: String? = nil, effort: String? = nil, context: Double? = nil, tokens: Int? = nil
    ) -> AgentStatus {
        AgentStatus(sliceID: "s", session: "nat-s", activity: .working,
                    model: model, effort: effort, contextPercent: context, contextTokens: tokens)
    }

    func testNoAgentDrawsNothing() {
        XCTAssertNil(buildAgentReadout(from: nil))
    }

    func testNoReadingYetDrawsNothingNotZeros() {
        XCTAssertNil(buildAgentReadout(from: agent()))
        XCTAssertNil(buildAgentReadout(from: agent(model: "", effort: "")))
    }

    func testFullReading() {
        let readout = buildAgentReadout(from: agent(model: "Sonnet 5", effort: "high", context: 41.6, tokens: 83_200))
        XCTAssertEqual(readout?.label, "Sonnet 5 / high")
        XCTAssertEqual(readout?.context, AgentReadout.Context(text: "context 42% (83k tokens)", warning: false))
    }

    func testContextWithoutTokensDrawsThePercentAlone() {
        XCTAssertEqual(buildAgentReadout(from: agent(context: 41.6))?.context?.text, "context 42%")
        XCTAssertNil(buildAgentReadout(from: agent(tokens: 500)))
    }

    func testTokenCounts() {
        XCTAssertEqual(formatTokenCount(0), "0")
        XCTAssertEqual(formatTokenCount(999), "999")
        XCTAssertEqual(formatTokenCount(1000), "1k")
        XCTAssertEqual(formatTokenCount(325_840), "326k")
        XCTAssertEqual(formatTokenCount(999_499), "999k")
        XCTAssertEqual(formatTokenCount(999_500), "1.0m")
        XCTAssertEqual(formatTokenCount(1_234_567), "1.2m")
    }

    func testPartialReadings() {
        XCTAssertEqual(buildAgentReadout(from: agent(model: "Sonnet 5"))?.label, "Sonnet 5")
        XCTAssertNil(buildAgentReadout(from: agent(model: "Sonnet 5"))?.context)
        XCTAssertNil(buildAgentReadout(from: agent(context: 10))?.label)
        XCTAssertEqual(buildAgentReadout(from: agent(context: 0))?.context?.text, "context 0%")
    }

    func testHighContextWarnsAtThreshold() {
        XCTAssertFalse(buildAgentReadout(from: agent(context: 79.9))!.context!.warning)
        XCTAssertTrue(buildAgentReadout(from: agent(context: 80))!.context!.warning)
    }

    func testStatusDecodesReadoutFields() throws {
        let json = #"{"slice_id":"a","session":"b","activity":"working","model":"Sonnet 5","effort":"high","context_percent":12.5,"context_tokens":125000}"#
        let status = try JSONDecoder().decode(AgentStatus.self, from: Data(json.utf8))
        XCTAssertEqual(status.model, "Sonnet 5")
        XCTAssertEqual(status.effort, "high")
        XCTAssertEqual(status.contextPercent, 12.5)
        XCTAssertEqual(status.contextTokens, 125_000)
    }

    /// A launch's model and effort are written as a live agent's are, each
    /// half dropped when unset, and nothing at all with neither.
    func testModelEffortLabel() {
        XCTAssertEqual(modelEffortLabel(model: "opus", effort: "high"), "opus / high")
        XCTAssertEqual(modelEffortLabel(model: "opus", effort: ""), "opus")
        XCTAssertEqual(modelEffortLabel(model: "", effort: "max"), "max")
        XCTAssertNil(modelEffortLabel(model: "", effort: ""))
    }
}
