import SwiftUI

// MARK: - Rect Edge Set

struct RectEdgeSet: OptionSet {
    let rawValue: Int

    static let top = RectEdgeSet(rawValue: 1)
    static let leading = RectEdgeSet(rawValue: 2)
    static let bottom = RectEdgeSet(rawValue: 4)
    static let trailing = RectEdgeSet(rawValue: 8)
    static let all: RectEdgeSet = [.top, .leading, .bottom, .trailing]
}

// MARK: - View Extensions

extension View {
    func rectBorder(width: CGFloat, edges: RectEdgeSet, color: Color) -> some View {
        overlay(alignment: .topLeading) {
            VStack(spacing: 0) {
                if edges.contains(.top) {
                    color.frame(height: width)
                }
                HStack(spacing: 0) {
                    if edges.contains(.leading) {
                        color.frame(width: width)
                    }
                    Spacer()
                    if edges.contains(.trailing) {
                        color.frame(width: width)
                    }
                }
                if edges.contains(.bottom) {
                    color.frame(height: width)
                }
            }
        }
    }

    func rectBorderTrailing(width: CGFloat, color: Color) -> some View {
        self.rectBorder(width: width, edges: [.trailing], color: color)
    }
}
