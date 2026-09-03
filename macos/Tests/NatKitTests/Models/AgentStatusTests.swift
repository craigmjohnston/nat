import XCTest
@testable import NatKit

final class AgentStatusTests: XCTestCase {
    func testAgentStatusDecoding() throws {
        let json = """
        {
            "slice_id": "page-123",
            "session": "nat-abc123def",
            "activity": "working"
        }
        """

        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let status = try decoder.decode(AgentStatus.self, from: data)

        XCTAssertEqual(status.sliceID, "page-123")
        XCTAssertEqual(status.session, "nat-abc123def")
        XCTAssertEqual(status.activity, .working)
    }

    func testAgentStatusDecodingWaiting() throws {
        let json = """
        {
            "slice_id": "page-456",
            "session": "nat-def456ghi",
            "activity": "waiting"
        }
        """

        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let status = try decoder.decode(AgentStatus.self, from: data)

        XCTAssertEqual(status.activity, .waiting)
    }

    func testAgentStatusDecodingUnknown() throws {
        let json = """
        {
            "slice_id": "page-789",
            "session": "nat-ghi789jkl",
            "activity": "unknown"
        }
        """

        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let status = try decoder.decode(AgentStatus.self, from: data)

        XCTAssertEqual(status.activity, .unknown)
    }

    func testAgentStatusDecodingInvalidActivityDefaultsToUnknown() throws {
        let json = """
        {
            "slice_id": "page-999",
            "session": "nat-jkl999mno",
            "activity": "invalid_state"
        }
        """

        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let status = try decoder.decode(AgentStatus.self, from: data)

        XCTAssertEqual(status.activity, .unknown)
    }

    func testAgentStatusIdentifiable() {
        let status = AgentStatus(sliceID: "slice-id", session: "session-name", activity: .working)
        XCTAssertEqual(status.id, "slice-id")
    }

    func testAgentStatusEquality() {
        let status1 = AgentStatus(sliceID: "slice-1", session: "session-1", activity: .working)
        let status2 = AgentStatus(sliceID: "slice-1", session: "session-1", activity: .working)
        let status3 = AgentStatus(sliceID: "slice-2", session: "session-1", activity: .working)

        XCTAssertEqual(status1, status2)
        XCTAssertNotEqual(status1, status3)
    }

    func testAgentActivityStateEncodingDecoding() throws {
        for activity: AgentActivityState in [.working, .waiting, .unknown] {
            let encoder = JSONEncoder()
            let data = try encoder.encode(activity)
            let decoder = JSONDecoder()
            let decoded = try decoder.decode(AgentActivityState.self, from: data)
            XCTAssertEqual(activity, decoded)
        }
    }
}
