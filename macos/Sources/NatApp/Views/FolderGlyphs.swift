import SwiftUI
import NatKit

/// A folder drawn closed or open: the outline while it holds its contents
/// back, the filled silhouette with its flap swung out once they are on the
/// tree. Drawn rather than an SF Symbol, since the system set has no open
/// folder to pair with `folder`.
struct FolderGlyph: View {
    let open: Bool
    let color: Color
    var size = CGSize(width: 13, height: 10.5)

    var body: some View {
        Group {
            if open {
                FolderGlyphShape(open: true).fill(color)
            } else {
                FolderGlyphShape(open: false).stroke(color, lineWidth: FolderGlyphShape.strokeWidth)
            }
        }
        .frame(width: size.width, height: size.height)
    }
}

/// The Done folder: a folder with a checkmark badged on its lower right,
/// open or closed alike. Around the check a mask a point wide, in the
/// check's own shape, cuts the folder away: the same glyph, in the ground's
/// colour, shifted a point around a circle behind it.
struct DoneFolderGlyph: View {
    @Environment(\.ground) private var ground
    let open: Bool
    let color: Color

    /// The mask's copies, on a circle a point out rather than a square of
    /// eight: a square dilation leaves square corners at the check's ends.
    private static let halo: [CGSize] = (0..<16).map { step in
        let angle = Double(step) * .pi / 8
        return CGSize(width: cos(angle), height: sin(angle))
    }

    var body: some View {
        FolderGlyph(open: open, color: color)
            .overlay(alignment: .bottomTrailing) {
                ZStack {
                    ForEach(Array(Self.halo.enumerated()), id: \.offset) { _, nudge in
                        check.foregroundStyle(DesignTokens.fill(ground)).offset(nudge)
                    }
                    check.foregroundStyle(color)
                }
                .padding(1.5)
                .offset(x: 4, y: 3)
            }
    }

    private var check: some View {
        Image(systemName: "checkmark").font(.system(size: 7, weight: .heavy))
    }
}

/// A folder of folders: a closed folder standing behind the project's own,
/// offset up and to the right — what a project is, a folder of milestones.
/// The front folder is knocked out of the back one's lines with the ground
/// it sits on, so the two read as stacked rather than overlapping.
struct StackedFolderGlyph: View {
    @Environment(\.ground) private var ground
    let open: Bool
    let color: Color
    let backColor: Color
    var size = CGSize(width: 15, height: 12)

    var body: some View {
        let front = CGSize(width: size.width * 0.84, height: size.height * 0.84)
        ZStack(alignment: .bottomLeading) {
            FolderGlyphShape(open: false)
                .stroke(backColor, lineWidth: FolderGlyphShape.strokeWidth)
                .frame(width: front.width, height: front.height)
                .offset(x: size.width - front.width, y: -(size.height - front.height))
            FolderGlyphShape(open: false)
                .fill(DesignTokens.fill(ground))
                .frame(width: front.width, height: front.height)
            FolderGlyph(open: open, color: color, size: front)
        }
        .frame(width: size.width, height: size.height, alignment: .bottomLeading)
    }
}

/// The tree's folder pictograms, drawn because SF Symbols has no open-folder
/// glyph to pair with `folder`. Both states are one folder: the same tabbed
/// body, the same corner radius, the same bounds. Closed is that body as an
/// outline (inset by half the stroke so it lands on the same bounds the
/// fill does); open is it filled, with the body's lower part swapped for a
/// front flap swung out to the right and a hairline gap between flap and
/// back panel — the closed folder with its flap opened and the body filled.
struct FolderGlyphShape: Shape {
    let open: Bool

    /// The closed outline's line width, which the shape insets by half of.
    static let strokeWidth: CGFloat = 1.1

    /// Where the flap's top edge sits, as a fraction of the height, and the
    /// gap the back panel stops short of it by.
    private static let flapTop: CGFloat = 0.50
    private static let gap: CGFloat = 0.06

    func path(in rect: CGRect) -> Path {
        open ? openPath(in: rect) : closedPath(in: rect)
    }

    private func closedPath(in rect: CGRect) -> Path {
        let inset = Self.strokeWidth / 2
        return body(in: rect.insetBy(dx: inset, dy: inset), bottom: nil)
    }

    private func openPath(in rect: CGRect) -> Path {
        let w = rect.width
        let h = rect.height
        let r = 0.14 * h
        var p = body(in: rect, bottom: rect.minY + (Self.flapTop - Self.gap) * h)
        // The flap: a parallelogram leaning left at the foot, its top edge
        // running out to the body's right bound.
        let lean = 0.14 * w
        let top = rect.minY + Self.flapTop * h
        let pts = [
            CGPoint(x: rect.minX + lean, y: top),
            CGPoint(x: rect.maxX, y: top),
            CGPoint(x: rect.maxX - lean, y: rect.maxY),
            CGPoint(x: rect.minX, y: rect.maxY),
        ]
        p.addPath(rounded(pts, radius: r))
        return p
    }

    /// The tabbed folder body: rounded corners, the tab across the top left,
    /// a short slant joining tab to top edge. `bottom` lifts the lower edge
    /// (still rounded) for the open folder's back panel.
    private func body(in rect: CGRect, bottom: CGFloat?) -> Path {
        let h = rect.height
        let r = 0.14 * h
        let tabW = 0.36 * rect.width
        let slant = 0.10 * rect.width
        let tabH = 0.22 * h
        let x0 = rect.minX
        let y0 = rect.minY
        let right = rect.maxX
        let foot = bottom ?? rect.maxY
        // The open folder's back panel is a short strip, so its cut edge
        // takes a tighter corner than the full body's.
        let rb = bottom == nil ? r : 0.07 * h

        var p = Path()
        p.move(to: CGPoint(x: x0, y: y0 + r))
        p.addArc(center: CGPoint(x: x0 + r, y: y0 + r), radius: r,
                 startAngle: .degrees(180), endAngle: .degrees(270), clockwise: false)
        p.addLine(to: CGPoint(x: x0 + tabW, y: y0))
        p.addLine(to: CGPoint(x: x0 + tabW + slant, y: y0 + tabH))
        p.addLine(to: CGPoint(x: right - r, y: y0 + tabH))
        p.addArc(center: CGPoint(x: right - r, y: y0 + tabH + r), radius: r,
                 startAngle: .degrees(270), endAngle: .degrees(0), clockwise: false)
        p.addLine(to: CGPoint(x: right, y: foot - rb))
        p.addArc(center: CGPoint(x: right - rb, y: foot - rb), radius: rb,
                 startAngle: .degrees(0), endAngle: .degrees(90), clockwise: false)
        p.addLine(to: CGPoint(x: x0 + rb, y: foot))
        p.addArc(center: CGPoint(x: x0 + rb, y: foot - rb), radius: rb,
                 startAngle: .degrees(90), endAngle: .degrees(180), clockwise: false)
        p.closeSubpath()
        return p
    }

    /// A closed polygon with every corner rounded to `radius`.
    private func rounded(_ pts: [CGPoint], radius: CGFloat) -> Path {
        var p = Path()
        let n = pts.count
        p.move(to: CGPoint(x: (pts[0].x + pts[n - 1].x) / 2, y: (pts[0].y + pts[n - 1].y) / 2))
        for i in 0..<n {
            p.addArc(tangent1End: pts[i], tangent2End: pts[(i + 1) % n], radius: radius)
        }
        p.closeSubpath()
        return p
    }
}
