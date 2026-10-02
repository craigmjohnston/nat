import XCTest
@testable import NatKit

final class VisualCommentsTests: XCTestCase {
    private let comments = [
        PendingVisualComment(
            index: 1, name: "Settings, dark", uri: "/tmp/dark.png",
            point: nil, imageSize: CGSize(width: 1440, height: 900), text: "Too much contrast overall."),
        PendingVisualComment(
            index: 1, name: "Settings, dark", uri: "/tmp/dark.png",
            point: CGPoint(x: 412, y: 88), imageSize: CGSize(width: 1440, height: 900), text: "This label is clipped."),
        PendingVisualComment(
            index: 2, name: "Docs", uri: "https://example.com/docs.png",
            point: nil, imageSize: .zero, text: "Fine."),
    ]

    func testThePromptNamesEachImageOnceAndWhereEachCommentSits() {
        XCTAssertEqual(visualCommentsPrompt(comments, branch: "slice/x", handBack: nil), """
        I have reviewed the visual changes you handed in and left 3 comments on them. \
        Address every one of them, then push the branch again and tell me it is ready.

        ## Settings, dark (/tmp/dark.png)

        On the image as a whole:
        Too much contrast overall.

        At (412, 88) in the 1440×900 image:
        This label is clipped.

        ## Docs (https://example.com/docs.png)

        On the image as a whole:
        Fine.

        """)
    }

    func testAHandBackEndsWithTheCompleteSliceCommand() {
        let one = [comments[1]]
        let handBack = HandBackInstruction(projectID: "proj", sliceRef: "slice-1")
        XCTAssertEqual(visualCommentsPrompt(one, branch: "slice/x", handBack: handBack), """
        I have reviewed the visual changes you handed in and left 1 comment on them. \
        Address every one of them, then push the branch again and tell me it is ready.

        ## Settings, dark (/tmp/dark.png)

        At (412, 88) in the 1440×900 image:
        This label is clipped.

        When every comment is addressed and the branch is pushed, hand the slice back for review by running exactly:

        nat complete-slice slice-1 --project proj --branch slice/x --summary '<what you changed for these comments>'

        """)
        XCTAssertTrue(visualCommentsPrompt(one, branch: nil, handBack: handBack).contains("--branch <branch>"))
    }
}
