import SwiftUI
import NatKit

/// The Notion mark, drawn from the design's own SVG (`NotionMark` in
/// `docs/design/nat-new-project/ui-newproject.jsx`, a 100×100 viewBox): the
/// cube's face and, over it, the outline and the N cut out of it. Two shapes
/// rather than one image so each takes a theme token — the face the primary
/// ink, the cut-outs the card it sits on — and the mark reads in both themes
/// without a colour of its own.
struct NotionMark: View {
    var size: CGFloat = 20

    var body: some View {
        ZStack {
            NotionMarkFace().fill(DesignTokens.label)
            NotionMarkDetail().fill(DesignTokens.fill(.card))
        }
        .frame(width: size, height: size)
    }
}

private func P(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: x, y: y) }

/// Scales a path drawn in the SVG's 100×100 box to the rect it is given.
private func scaled(_ path: Path, to rect: CGRect) -> Path {
    path.applying(CGAffineTransform(scaleX: rect.width / 100, y: rect.height / 100))
}

private struct NotionMarkFace: Shape {
    func path(in rect: CGRect) -> Path {
        var p = Path()
        p.move(to: P(6.4, 4.3))
        p.addLine(to: P(61.4, 0.2))
        p.addCurve(to: P(74.2, 3.1), control1: P(68.2, -0.4), control2: P(69.9, 0))
        p.addLine(to: P(91.8, 15.5))
        p.addCurve(to: P(95.7, 20.5), control1: P(94.7, 17.6), control2: P(95.7, 18.2))
        p.addLine(to: P(95.7, 88.4))
        p.addCurve(to: P(88.7, 95.6), control1: P(95.7, 92.7), control2: P(94.1, 95.2))
        p.addLine(to: P(24.8, 99.5))
        p.addCurve(to: P(16.6, 96.4), control1: P(20.7, 99.7), control2: P(18.8, 99.1))
        p.addLine(to: P(3.7, 79.7))
        p.addCurve(to: P(0.3, 71.3), control1: P(1.3, 76.5), control2: P(0.3, 74.1))
        p.addLine(to: P(0.3, 11.1))
        p.addCurve(to: P(6.4, 4.3), control1: P(0.3, 7.6), control2: P(1.9, 4.7))
        p.closeSubpath()
        return scaled(p, to: rect)
    }
}

private struct NotionMarkDetail: Shape {
    func path(in rect: CGRect) -> Path {
        var p = Path()
        p.move(to: P(61.4, 0.2))
        p.addLine(to: P(6.4, 4.3))
        p.addCurve(to: P(0.3, 11.1), control1: P(1.9, 4.7), control2: P(0.3, 7.6))
        p.addLine(to: P(0.3, 71.3))
        p.addCurve(to: P(3.7, 79.7), control1: P(0.3, 74.1), control2: P(1.3, 76.5))
        p.addLine(to: P(16.6, 96.4))
        p.addCurve(to: P(24.8, 99.5), control1: P(18.8, 99.1), control2: P(20.7, 99.7))
        p.addLine(to: P(88.7, 95.6))
        p.addCurve(to: P(95.7, 88.4), control1: P(94.1, 95.2), control2: P(95.7, 92.7))
        p.addLine(to: P(95.7, 20.5))
        p.addCurve(to: P(92.2, 15.7), control1: P(95.7, 18.3), control2: P(94.8, 17.6))
        p.addLine(to: P(74.2, 3.1))
        p.addCurve(to: P(61.4, 0.2), control1: P(69.9, 0), control2: P(68.2, -0.4))
        p.closeSubpath()
        p.move(to: P(26.2, 19.6))
        p.addCurve(to: P(16.8, 17.6), control1: P(21, 20), control2: P(19.8, 20))
        p.addLine(to: P(9.3, 11.7))
        p.addCurve(to: P(10.8, 9.8), control1: P(8.5, 10.9), control2: P(8.9, 10))
        p.addLine(to: P(63.7, 5.9))
        p.addCurve(to: P(72.2, 8.4), control1: P(68.2, 5.5), control2: P(70.5, 7.1))
        p.addLine(to: P(81.3, 15))
        p.addCurve(to: P(81.5, 16.3), control1: P(81.7, 15.2), control2: P(82.7, 16.3))
        p.addLine(to: P(26.9, 19.6))
        p.addLine(to: P(26.2, 19.6))
        p.closeSubpath()
        p.move(to: P(20.1, 88.3))
        p.addLine(to: P(20.1, 30.8))
        p.addCurve(to: P(23.2, 26.9), control1: P(20.1, 28.3), control2: P(20.9, 27.1))
        p.addLine(to: P(85.9, 23.2))
        p.addCurve(to: P(89, 26.9), control1: P(88, 23), control2: P(89, 24.4))
        p.addLine(to: P(89, 84))
        p.addCurve(to: P(85.1, 88.8), control1: P(89, 86.5), control2: P(88.6, 88.6))
        p.addLine(to: P(25.1, 92.3))
        p.addCurve(to: P(20.1, 88.3), control1: P(21.6, 92.5), control2: P(20.1, 91.3))
        p.closeSubpath()
        p.move(to: P(79.3, 33.9))
        p.addCurve(to: P(77.5, 37.6), control1: P(79.7, 35.6), control2: P(79.3, 37.4))
        p.addLine(to: P(74.6, 38.2))
        p.addLine(to: P(74.6, 80.7))
        p.addCurve(to: P(67.8, 82.8), control1: P(72.1, 82), control2: P(69.8, 82.8))
        p.addCurve(to: P(61.6, 78.9), control1: P(64.7, 82.8), control2: P(63.9, 81.8))
        p.addLine(to: P(42.7, 49.2))
        p.addLine(to: P(42.7, 78))
        p.addLine(to: P(48.7, 79.4))
        p.addCurve(to: P(43.9, 82.9), control1: P(48.7, 79.4), control2: P(48.7, 82.9))
        p.addLine(to: P(30.5, 83.7))
        p.addCurve(to: P(31.9, 80.6), control1: P(30.1, 82.9), control2: P(30.5, 81))
        p.addLine(to: P(35.4, 79.6))
        p.addLine(to: P(35.4, 41.5))
        p.addLine(to: P(30.6, 41.1))
        p.addCurve(to: P(33.9, 36.6), control1: P(30.2, 39.4), control2: P(31.2, 36.8))
        p.addLine(to: P(48.3, 35.6))
        p.addLine(to: P(68, 65.8))
        p.addLine(to: P(68, 39.1))
        p.addLine(to: P(63, 38.5))
        p.addCurve(to: P(66.1, 34.6), control1: P(62.6, 36.4), control2: P(64.2, 34.8))
        p.addLine(to: P(79.3, 33.9))
        p.closeSubpath()
        return scaled(p, to: rect)
    }
}
