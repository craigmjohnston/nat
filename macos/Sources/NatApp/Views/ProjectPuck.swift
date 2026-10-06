import SwiftUI
import NatKit

/// A project's colour as a mark: a small vertical capsule, no wider than
/// 3pt and about the row's text height, filled in the project's tint on the
/// ground it is drawn over. One mark wherever a project is named — its
/// Active rows, its PROJECTS row, the titlebar breadcrumb — so the eye reads
/// them as one. The scratch and source projects take no colour, so none.
struct ProjectPuck: View {
    let color: ProjectColor
    var ground: Ground = .window

    static let width: CGFloat = 3
    /// The room kept between the puck and whatever follows it in a row.
    static let gap: CGFloat = 6
    static var height: CGFloat { GnatMetrics.body - 1 }

    var body: some View {
        Capsule()
            .fill(DesignTokens.projectInk(color, on: ground))
            .frame(width: Self.width, height: Self.height)
            .accessibilityLabel("\(color.rawValue.capitalized) project")
    }
}

extension View {
    /// The puck drawn in space already there — a row's leading padding,
    /// `inset` wide — never added to its stack, so nothing else in the row
    /// moves by a point. It ends `gap` short of the row's first glyph, and
    /// sits `drop` below the row's centre — `StateDot.drop` beside a state
    /// dot or a project tag, so the three share a line. No colour, no puck.
    func projectPuck(
        _ color: ProjectColor?, inset: CGFloat, gap: CGFloat = ProjectPuck.gap, drop: CGFloat = 0,
        ground: Ground = .window
    ) -> some View {
        overlay(alignment: .leading) {
            if let color {
                ProjectPuck(color: color, ground: ground)
                    .offset(y: drop)
                    .padding(.leading, inset - gap - ProjectPuck.width)
                    .allowsHitTesting(false)
            }
        }
    }
}
