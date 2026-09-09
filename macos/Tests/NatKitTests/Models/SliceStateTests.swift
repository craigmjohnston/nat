import XCTest
@testable import NatKit

final class SliceStateTests: XCTestCase {
    func testAllKnownStates() {
        let states: [SliceState] = [
            .working,
            .waiting,
            .blocked,
            .readyToPush,
            .awaitingReview,
            .readyToMerge
        ]

        for state in states {
            let encoded = try! JSONEncoder().encode(state)
            let decoded = try! JSONDecoder().decode(SliceState.self, from: encoded)
            XCTAssertEqual(state, decoded)
        }
    }

    func testEncodingProducesCorrectStrings() {
        let testCases: [(SliceState, String)] = [
            (.working, "\"working\""),
            (.waiting, "\"waiting\""),
            (.blocked, "\"blocked\""),
            (.readyToPush, "\"ready to push\""),
            (.awaitingReview, "\"awaiting review\""),
            (.readyToMerge, "\"ready to merge\"")
        ]

        for (state, expectedJSON) in testCases {
            let encoded = try! JSONEncoder().encode(state)
            let json = String(data: encoded, encoding: .utf8)!
            XCTAssertEqual(json, expectedJSON)
        }
    }

    func testDecodingFromStrings() {
        let testCases: [(String, SliceState)] = [
            ("\"working\"", .working),
            ("\"waiting\"", .waiting),
            ("\"blocked\"", .blocked),
            ("\"ready to push\"", .readyToPush),
            ("\"awaiting review\"", .awaitingReview),
            ("\"ready to merge\"", .readyToMerge)
        ]

        for (json, expectedState) in testCases {
            let data = json.data(using: .utf8)!
            let decoded = try! JSONDecoder().decode(SliceState.self, from: data)
            XCTAssertEqual(decoded, expectedState)
        }
    }

    func testUnknownState() {
        let json = "\"some_unknown_state\"".data(using: .utf8)!
        let decoded = try! JSONDecoder().decode(SliceState.self, from: json)
        XCTAssertEqual(decoded, .unknown("some_unknown_state"))
    }

    func testUnknownStateEncoding() {
        let state = SliceState.unknown("custom_state")
        let encoded = try! JSONEncoder().encode(state)
        let json = String(data: encoded, encoding: .utf8)!
        XCTAssertEqual(json, "\"custom_state\"")
    }

    func testHashable() {
        let state1 = SliceState.working
        let state2 = SliceState.working
        XCTAssertEqual(state1.hashValue, state2.hashValue)

        var set: Set<SliceState> = [.working, .waiting, .blocked]
        XCTAssertEqual(set.count, 3)
        set.insert(.working)
        XCTAssertEqual(set.count, 3)
    }
}
