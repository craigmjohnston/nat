import XCTest
@testable import NatKit

/// `replyBody`: a reply written as GitHub's own Quote reply writes one.
final class ReplyBodyTests: XCTestCase {
    private func entry(author: String = "octocat", body: String) -> ConvoEntry {
        ConvoEntry(
            author: author, verb: "commented", at: Date(timeIntervalSince1970: 100), body: body,
            tone: .neutral, isReview: false)
    }

    func testOneLineParent() {
        XCTAssertEqual(
            replyBody(to: entry(body: "Does it wrap?"), text: "  It does.\n"),
            "@octocat\n> Does it wrap?\n\nIt does.")
    }

    func testMultiLineParent() {
        XCTAssertEqual(
            replyBody(to: entry(body: "First line.\nSecond line."), text: "Both fixed."),
            "@octocat\n> First line.\n> Second line.\n\nBoth fixed.")
    }

    func testBlankLinesInTheParentAreQuotedAlone() {
        XCTAssertEqual(
            replyBody(to: entry(body: "One.\r\n\r\n  \r\nTwo."), text: "Yes."),
            "@octocat\n> One.\n>\n>\n> Two.\n\nYes.")
    }

    func testAVerdictWithNoWordsHasNoQuoteBlock() {
        XCTAssertEqual(replyBody(to: entry(body: ""), text: "Thanks."), "@octocat\n\nThanks.")
    }

    func testSomeoneIsNotMentioned() {
        XCTAssertEqual(
            replyBody(to: entry(author: convoAuthor(""), body: "Gone now."), text: "Noted."),
            "> Gone now.\n\nNoted.")
        XCTAssertEqual(replyBody(to: entry(author: "someone", body: ""), text: "Noted."), "Noted.")
    }

    func testReplyKeyTellsEntriesApartAndHoldsAcrossReadings() {
        let comment = entry(body: "a")
        XCTAssertEqual(comment.replyKey, entry(body: "edited").replyKey, "a later reading keeps its key")
        let review = ConvoEntry(
            author: "octocat", verb: "approved", at: comment.at, body: "", tone: .approved, isReview: true)
        XCTAssertNotEqual(comment.replyKey, review.replyKey)
        XCTAssertNotEqual(comment.replyKey, entry(author: "mona", body: "a").replyKey)
    }
}
