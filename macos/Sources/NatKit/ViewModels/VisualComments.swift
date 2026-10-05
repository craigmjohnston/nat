import Foundation

/// A review comment left on one handed-in image, held only in the session —
/// never written anywhere, and sent to the agent as one prompt with every
/// other pending comment on the slice's images, as diff comments are.
///
/// `point` is where on the image it was left, in the image's own pixels
/// (origin top-left) — independent of how far the pane was zoomed when it was
/// — and nil for a comment on the image as a whole. `imageSize` is the
/// image's pixel size, sent with the point so the agent can place it.
public struct PendingVisualComment: Identifiable, Equatable, Sendable {
    public let id: UUID
    public let index: Int
    public let name: String
    public let uri: String
    /// The hash of the image it was left on, which with `uri` names that
    /// image: a re-render at the same path is another image.
    public let hash: String?
    public let point: CGPoint?
    public let imageSize: CGSize
    public var text: String

    public init(
        id: UUID = UUID(), index: Int, name: String, uri: String, hash: String? = nil,
        point: CGPoint?, imageSize: CGSize, text: String
    ) {
        self.id = id
        self.index = index
        self.name = name
        self.uri = uri
        self.hash = hash
        self.point = point
        self.imageSize = imageSize
        self.text = text
    }

    /// Where on the image the comment sits, in the words a card's meta line
    /// and the prompt name it by.
    public var placement: String {
        guard let point else { return "whole image" }
        return "at (\(Int(point.x.rounded())), \(Int(point.y.rounded())))"
    }
}

/// The comments a send carried, as the slice's page keeps them under `Sent
/// back` — `commentsRecord`'s shape: one paragraph per comment, its image and
/// where on it, then what it says.
public func visualCommentsRecord(_ comments: [PendingVisualComment]) -> String {
    comments.map { "\($0.name), \($0.placement): \($0.text)" }.joined(separator: "\n\n")
}

/// Builds the one turn every pending visual comment is delivered to the agent
/// as, in `commentsPrompt`'s shape: what was reviewed, then each image by
/// name and URI with its comments under it, each saying where on the image it
/// was left. With `handBack`, it ends with the `complete-slice --branch`
/// hand-back `commentsPrompt` ends with — given only where the slice is
/// handed back, since only then is it taken out of review by the send.
public func visualCommentsPrompt(
    _ comments: [PendingVisualComment], branch: String?, handBack: HandBackInstruction?
) -> String {
    var out = "I have reviewed the visual changes you handed in and left \(comments.count) " +
        "\(plural(comments.count, "comment", "comments")) on them. " +
        "Address every one of them, then push the branch again and tell me it is ready.\n"
    var heading: (Int, String)?
    for comment in comments {
        if heading == nil || heading! != (comment.index, comment.uri) {
            heading = (comment.index, comment.uri)
            out += "\n## \(comment.name) (\(comment.uri))\n"
        }
        if let point = comment.point {
            out += "\nAt (\(Int(point.x.rounded())), \(Int(point.y.rounded()))) in the " +
                "\(Int(comment.imageSize.width.rounded()))×\(Int(comment.imageSize.height.rounded())) image:\n"
        } else {
            out += "\nOn the image as a whole:\n"
        }
        out += "\(comment.text)\n"
    }
    if let handBack {
        out += "\nWhen every comment is addressed and the branch is pushed, hand the slice " +
            "back for review by running exactly:\n\n" +
            "nat complete-slice \(handBack.sliceRef) --project \(handBack.projectID) " +
            "--branch \(branch ?? "<branch>") --summary '<what you changed for these comments>'\n"
    }
    return out
}
