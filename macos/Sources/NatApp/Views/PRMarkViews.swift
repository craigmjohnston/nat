import SwiftUI
import NatKit

/// A branch that conflicts with its base, as a row marks it: gnat's merge
/// icon (the merge button's) in the danger ink, the base named under the pointer. A piece of its own, so a
/// conflicting branch with no pull request can carry the same mark.
struct ConflictMark: View {
    let conflict: BranchConflict

    var body: some View {
        MergeIcon(size: 11, lineWidth: 1.4)
            .ink(.danger)
            .help(conflict.help)
            .accessibilityLabel(conflict.help)
    }
}

/// A pull request as a sidebar row marks it, at its trailing edge: the
/// failing checks' danger mark, and the conflict mark — both where both are
/// wrong — or, in the same slot, the passing checks' success mark where
/// `prMarks(_:for:agent:)` kept it; nothing where none is.
struct PRMarksView: View {
    let marks: PRMarks

    var body: some View {
        if !marks.isEmpty {
            HStack(spacing: 4) {
                if let help = marks.checksHelp {
                    Image(systemName: "xmark.octagon.fill")
                        .font(.system(size: 10, weight: .medium))
                        .ink(.danger)
                        .help(help)
                        .accessibilityLabel(help)
                }
                if let conflict = marks.conflict {
                    ConflictMark(conflict: conflict)
                }
                if let help = marks.passingHelp {
                    Image(systemName: "checkmark.circle.fill")
                        .font(.system(size: 10, weight: .medium))
                        .ink(.success)
                        .help(help)
                        .accessibilityLabel(help)
                }
            }
        }
    }
}
