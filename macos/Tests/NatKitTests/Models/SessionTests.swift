import XCTest
@testable import NatKit

final class SessionTests: XCTestCase {
    func testDecodingALiveSession() throws {
        let json = """
        {
            "id": "sess-1",
            "tag": "session:proj-1:sess-1",
            "live": true,
            "session": "nat-session-abcd1234",
            "started_at": "2026-03-01T12:00:00Z",
            "dir": "/Users/craig/scratch",
            "branch": "session/abcd1234",
            "ended": false,
            "prs": [
                {"number": 12, "title": "Fixture", "url": "https://github.com/x/y/pull/12", "state": "OPEN"}
            ]
        }
        """
        let session = try JSONDecoder().decode(Session.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(session.id, "sess-1")
        XCTAssertEqual(session.tag, "session:proj-1:sess-1")
        XCTAssertTrue(session.live)
        XCTAssertEqual(session.session, "nat-session-abcd1234")
        XCTAssertEqual(session.dir, "/Users/craig/scratch")
        XCTAssertEqual(session.branch, "session/abcd1234")
        XCTAssertFalse(session.ended)
        XCTAssertEqual(session.prs.count, 1)
        XCTAssertEqual(session.prs[0].number, 12)
        XCTAssertTrue(session.prs[0].isOpen)
    }

    func testDecodingGoneSessionWithNoBranchOrPRs() throws {
        let json = """
        {
            "id": "sess-2",
            "tag": "session:proj-1:sess-2",
            "live": false,
            "started_at": "2026-03-01T12:00:00Z",
            "dir": "/Users/craig/scratch",
            "ended": true
        }
        """
        let session = try JSONDecoder().decode(Session.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(session.branch, "")
        XCTAssertEqual(session.session, "")
        XCTAssertTrue(session.prs.isEmpty)
        XCTAssertFalse(session.prsStale)
        XCTAssertTrue(session.ended)
    }

    func testDecodingRefusesAnUnreadableStartedAt() {
        let json = """
        {"id": "s", "tag": "t", "live": false, "started_at": "not-a-date", "dir": "/x", "ended": false}
        """
        XCTAssertThrowsError(try JSONDecoder().decode(Session.self, from: json.data(using: .utf8)!))
    }

    func testLabel_prefersBranchOverDirectory() {
        let withBranch = Session(
            id: "s", tag: "t", live: false, startedAt: Date(),
            dir: "/Users/craig/Projects/scratch", branch: "session/fixture"
        )
        XCTAssertEqual(withBranch.label, "session/fixture")

        let withoutBranch = Session(id: "s", tag: "t", live: false, startedAt: Date(), dir: "/Users/craig/Projects/scratch")
        XCTAssertEqual(withoutBranch.label, "scratch")
    }

    func testOpenPRs_filtersToOpenOnly() {
        let session = Session(
            id: "s", tag: "t", live: false, startedAt: Date(), dir: "/x",
            prs: [
                SessionPR(number: 1, title: "a", url: "https://a", state: "OPEN"),
                SessionPR(number: 2, title: "b", url: "https://b", state: "MERGED"),
                SessionPR(number: 3, title: "c", url: "https://c", state: "CLOSED"),
            ]
        )
        XCTAssertEqual(session.openPRs.map(\.number), [1])
    }

    func testSessionPR_decodesAnOptionalMergedAt() throws {
        let merged = try JSONDecoder().decode(
            SessionPR.self,
            from: #"{"number":1,"title":"x","url":"https://x","state":"MERGED","merged_at":"2026-03-01T12:00:00Z"}"#
                .data(using: .utf8)!
        )
        XCTAssertNotNil(merged.mergedAt)

        let open = try JSONDecoder().decode(
            SessionPR.self, from: #"{"number":1,"title":"x","url":"https://x","state":"OPEN"}"#.data(using: .utf8)!
        )
        XCTAssertNil(open.mergedAt)
    }

    // MARK: - SessionStatusDoc

    func testSessionStatusDoc_firstPRAcrossBranches() throws {
        let json = """
        {
            "id": "sess-1",
            "live": false,
            "ended": false,
            "dir": "/x",
            "branch": "session/two",
            "branches": [
                {"branch": "session/one", "prs": []},
                {"branch": "session/two", "prs": [
                    {"number": 7, "title": "y", "url": "https://y", "state": "OPEN"}
                ]}
            ]
        }
        """
        let doc = try JSONDecoder().decode(SessionStatusDoc.self, from: json.data(using: .utf8)!)
        XCTAssertEqual(doc.firstPR?.number, 7)
    }

    func testSessionStatusDoc_noPRAnywhereIsNil() throws {
        let json = """
        {"id": "s", "live": false, "ended": false, "dir": "/x", "branch": "b", "branches": [{"branch": "b", "prs": []}]}
        """
        let doc = try JSONDecoder().decode(SessionStatusDoc.self, from: json.data(using: .utf8)!)
        XCTAssertNil(doc.firstPR)
    }

    // MARK: - SessionLaunchResult

    func testSessionLaunchResult_decoding() throws {
        let json = """
        {"session": "nat-session-abcd", "tag": "session:p:1", "id": "1", "dir": "/x", "branch": "session/abcd"}
        """
        let result = try JSONDecoder().decode(SessionLaunchResult.self, from: json.data(using: .utf8)!)
        XCTAssertEqual(result.session, "nat-session-abcd")
        XCTAssertEqual(result.warning, "")
    }
}
