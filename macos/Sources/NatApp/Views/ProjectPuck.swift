import SwiftUI
import NatKit

/// A project's colour as a mark: a small vertical capsule, no wider than
/// 3pt and about the row's text height, filled in the project's tint on the
/// ground it is drawn over. One mark wherever a project is named — its
/// Active rows, its PROJECTS row (and a source or Scratch fold's heading),
/// the titlebar breadcrumb — so the eye reads them as one.
struct ProjectPuck: View {
    let color: ProjectColor
    var ground: Ground = .window

    static let width: CGFloat = 3
    /// The room kept between the puck and whatever follows it in the row.
    static let gap: CGFloat = 4
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
    /// moves by a point. It ends `ProjectPuck.gap` short of the row's first
    /// glyph. No colour, no puck.
    func projectPuck(_ color: ProjectColor?, inset: CGFloat, ground: Ground = .window) -> some View {
        overlay(alignment: .leading) {
            if let color {
                ProjectPuck(color: color, ground: ground)
                    .padding(.leading, inset - ProjectPuck.gap - ProjectPuck.width)
                    .allowsHitTesting(false)
            }
        }
    }
}
