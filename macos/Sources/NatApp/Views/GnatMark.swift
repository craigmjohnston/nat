import SwiftUI

/// The gnat mark on its own — the icon's cycloid "g", its head dot and the
/// teardrop counter, with no plate — for the titlebar. Its geometry is the
/// paper icon's own (`Resources/gnat-paper.svg`), normalised to a unit square
/// around the mark: a stroked line for the loop, filled shapes for the dot and
/// the teardrop.
struct GnatMark: View {
    var color: Color

    var body: some View {
        GeometryReader { proxy in
            let side = min(proxy.size.width, proxy.size.height)
            ZStack {
                GnatMarkLoop()
                    .stroke(color, style: StrokeStyle(lineWidth: side * GnatMarkGeometry.strokeWidth, lineCap: .round, lineJoin: .round))
                GnatMarkFills().fill(color)
            }
            .frame(width: side, height: side)
        }
        .aspectRatio(1, contentMode: .fit)
    }
}

private enum GnatMarkGeometry {
    /// The loop's stroke, as a share of the mark's side (78 of 572).
    static let strokeWidth: CGFloat = 78.0 / 572.0
    /// The head dot: its centre and radius, in the unit square.
    static let dot = (x: 0.9266, y: 0.0682, r: 39.0 / 572.0)
    static let loop: [(CGFloat, CGFloat)] = [
        (0.0680, 0.9323), (0.1016, 0.9276), (0.1351, 0.9208), (0.1685, 0.9119), (0.2016, 0.9007), (0.2339, 0.8874),
        (0.2656, 0.8722), (0.2960, 0.8549), (0.3252, 0.8357), (0.3530, 0.8149), (0.3790, 0.7923), (0.4035, 0.7682),
        (0.4259, 0.7427), (0.4462, 0.7161), (0.4643, 0.6883), (0.4801, 0.6596), (0.4935, 0.6304), (0.5045, 0.6005),
        (0.5131, 0.5705), (0.5191, 0.5402), (0.5226, 0.5101), (0.5236, 0.4802), (0.5222, 0.4509), (0.5184, 0.4222),
        (0.5121, 0.3942), (0.5037, 0.3673), (0.4932, 0.3416), (0.4808, 0.3173), (0.4664, 0.2944), (0.4505, 0.2733),
        (0.4330, 0.2538), (0.4143, 0.2362), (0.3946, 0.2206), (0.3740, 0.2070), (0.3526, 0.1955), (0.3308, 0.1860),
        (0.3087, 0.1788), (0.2867, 0.1738), (0.2650, 0.1708), (0.2437, 0.1699), (0.2231, 0.1713), (0.2035, 0.1747),
        (0.1848, 0.1799), (0.1675, 0.1871), (0.1517, 0.1960), (0.1376, 0.2065), (0.1253, 0.2184), (0.1150, 0.2316),
        (0.1068, 0.2462), (0.1009, 0.2615), (0.0972, 0.2780), (0.0960, 0.2949), (0.0972, 0.3122), (0.1009, 0.3299),
        (0.1072, 0.3476), (0.1159, 0.3652), (0.1271, 0.3825), (0.1407, 0.3991), (0.1568, 0.4152), (0.1752, 0.4304),
        (0.1956, 0.4444), (0.2182, 0.4573), (0.2427, 0.4687), (0.2691, 0.4787), (0.2970, 0.4869), (0.3262, 0.4934),
        (0.3568, 0.4979), (0.3885, 0.5005), (0.4210, 0.5012), (0.4540, 0.4997), (0.4874, 0.4960), (0.5210, 0.4900),
        (0.5545, 0.4822), (0.5878, 0.4719), (0.6205, 0.4596), (0.6523, 0.4453), (0.6834, 0.4290), (0.7131, 0.4107),
        (0.7416, 0.3906), (0.7685, 0.3687), (0.7937, 0.3453), (0.8171, 0.3205), (0.8385, 0.2944), (0.8575, 0.2671),
        (0.8745, 0.2390), (0.8890, 0.2100), (0.9012, 0.1802), (0.9108, 0.1503), (0.9180, 0.1201), (0.9227, 0.0899),
    ]
    static let teardrop: [(CGFloat, CGFloat)] = [
        (0.8990, 0.8998), (0.8995, 0.8867), (0.8995, 0.8736), (0.8986, 0.8607), (0.8967, 0.8476), (0.8941, 0.8350),
        (0.8906, 0.8222), (0.8865, 0.8098), (0.8815, 0.7976), (0.8760, 0.7855), (0.8699, 0.7740), (0.8635, 0.7628),
        (0.8561, 0.7519), (0.8486, 0.7411), (0.8402, 0.7309), (0.8316, 0.7210), (0.8226, 0.7117), (0.8131, 0.7026),
        (0.8031, 0.6939), (0.7928, 0.6858), (0.7823, 0.6781), (0.7712, 0.6710), (0.7600, 0.6642), (0.7484, 0.6580),
        (0.7369, 0.6523), (0.7247, 0.6470), (0.7126, 0.6421), (0.7002, 0.6379), (0.6878, 0.6341), (0.6748, 0.6309),
        (0.6621, 0.6295), (0.6589, 0.6414), (0.6580, 0.6545), (0.6580, 0.6677), (0.6580, 0.6808), (0.6580, 0.6939),
        (0.6589, 0.7070), (0.6600, 0.7201), (0.6612, 0.7330), (0.6629, 0.7462), (0.6650, 0.7591), (0.6673, 0.7720),
        (0.6703, 0.7848), (0.6731, 0.7976), (0.6767, 0.8101), (0.6804, 0.8227), (0.6846, 0.8351), (0.6895, 0.8474),
        (0.6944, 0.8594), (0.7000, 0.8713), (0.7058, 0.8830), (0.7124, 0.8944), (0.7194, 0.9054), (0.7267, 0.9163),
        (0.7350, 0.9267), (0.7434, 0.9367), (0.7524, 0.9462), (0.7622, 0.9549), (0.7727, 0.9628), (0.7836, 0.9698),
        (0.7955, 0.9757), (0.8077, 0.9802), (0.8203, 0.9832), (0.8334, 0.9837), (0.8463, 0.9813), (0.8584, 0.9760),
        (0.8689, 0.9682), (0.8776, 0.9586), (0.8846, 0.9477), (0.8902, 0.9357), (0.8944, 0.9234), (0.8972, 0.9105),
    ]
}

private struct GnatMarkLoop: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        for (index, point) in GnatMarkGeometry.loop.enumerated() {
            let p = CGPoint(x: rect.minX + point.0 * rect.width, y: rect.minY + point.1 * rect.height)
            if index == 0 { path.move(to: p) } else { path.addLine(to: p) }
        }
        return path
    }
}

private struct GnatMarkFills: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        for (index, point) in GnatMarkGeometry.teardrop.enumerated() {
            let p = CGPoint(x: rect.minX + point.0 * rect.width, y: rect.minY + point.1 * rect.height)
            if index == 0 { path.move(to: p) } else { path.addLine(to: p) }
        }
        path.closeSubpath()
        let dot = GnatMarkGeometry.dot
        let r = dot.r * rect.width
        path.addEllipse(in: CGRect(
            x: rect.minX + dot.x * rect.width - r, y: rect.minY + dot.y * rect.height - r,
            width: r * 2, height: r * 2))
        return path
    }
}
