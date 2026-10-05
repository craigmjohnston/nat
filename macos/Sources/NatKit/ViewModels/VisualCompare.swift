import AppKit
import CoreGraphics

/// Which image of a pair the compare view shows whole.
public enum VisualCompareSide: Equatable, Sendable {
    case before, after
}

/// Where the pixels of a pair's two images differ: an image the pair's pixel
/// size, opaque where they differ and clear where they agree, for the pane to
/// tint through.
///
/// `@unchecked Sendable` because `CGImage` is not declared so: a mask is only
/// read after the computation that made it has handed it over, and never
/// mutated.
public struct VisualMask: @unchecked Sendable, Equatable {
    public let image: CGImage
    /// How many pixels differ.
    public let count: Int

    public static func == (a: VisualMask, b: VisualMask) -> Bool {
        a.image === b.image && a.count == b.count
    }
}

/// The compare view's rules, kept here where they are tested: the divider
/// across a pair is a fraction of the frame's width — 0 at its leading edge,
/// showing the after whole, 1 at its trailing edge, showing the before whole —
/// the before drawn to its left and the after to its right.
public enum VisualCompare {
    /// Where a pair's divider starts: the middle.
    public static let middle: CGFloat = 0.5

    /// A divider kept within the frame.
    public static func clamp(_ divider: CGFloat) -> CGFloat {
        guard divider.isFinite else { return middle }
        return min(max(divider, 0), 1)
    }

    /// The divider under a point `x` along a frame `width` wide, clamped.
    public static func divider(at x: CGFloat, width: CGFloat) -> CGFloat {
        guard width > 0 else { return middle }
        return clamp(x / width)
    }

    /// The divider that shows `side` whole: at the far end from it.
    public static func divider(showing side: VisualCompareSide) -> CGFloat {
        side == .before ? 1 : 0
    }

    /// The side the Before / After toggle shows as selected: one only while
    /// the divider sits at its end, neither in between.
    public static func side(showing divider: CGFloat) -> VisualCompareSide? {
        switch clamp(divider) {
        case 1: return .before
        case 0: return .after
        default: return nil
        }
    }

    /// The frame a pair is drawn in: both images at one scale from its top
    /// leading corner, so it is the larger of each dimension.
    public static func frameSize(_ a: CGSize, _ b: CGSize) -> CGSize {
        CGSize(width: max(a.width, b.width), height: max(a.height, b.height))
    }

    /// Why a pair's differences cannot be highlighted, nil where they can:
    /// both images must be open and the same size in pixels.
    public static func highlightRefusal(after: VisualImage?, before: VisualImage?) -> String? {
        guard let afterSize = after?.pixelSize else { return "The image couldn't be opened" }
        guard let beforeSize = before?.pixelSize else { return "The before couldn't be opened" }
        guard afterSize == beforeSize else {
            return "The before is \(Self.describe(beforeSize)) and the after \(Self.describe(afterSize)): "
                + "only images of the same size can be compared pixel for pixel"
        }
        return nil
    }

    private static func describe(_ size: CGSize) -> String {
        "\(Int(size.width))×\(Int(size.height))"
    }

    /// The pixels where `before` and `after` differ, both drawn at
    /// `pixelSize`; nil where either cannot be drawn. Slow on a large image,
    /// so run it off the main actor.
    public nonisolated static func differenceMask(before: NSImage, after: NSImage, pixelSize: CGSize) -> VisualMask? {
        let width = Int(pixelSize.width), height = Int(pixelSize.height)
        guard width > 0, height > 0,
              let a = rgba(before, width: width, height: height),
              let b = rgba(after, width: width, height: height)
        else { return nil }
        var mask = [UInt8](repeating: 0, count: width * height * 4)
        var count = 0
        for pixel in 0..<(width * height) {
            let o = pixel * 4
            if a[o] != b[o] || a[o + 1] != b[o + 1] || a[o + 2] != b[o + 2] || a[o + 3] != b[o + 3] {
                mask[o] = 255; mask[o + 1] = 255; mask[o + 2] = 255; mask[o + 3] = 255
                count += 1
            }
        }
        guard let provider = CGDataProvider(data: Data(mask) as CFData),
              let image = CGImage(
                width: width, height: height, bitsPerComponent: 8, bitsPerPixel: 32, bytesPerRow: width * 4,
                space: CGColorSpace(name: CGColorSpace.sRGB)!,
                bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedLast.rawValue),
                provider: provider, decode: nil, shouldInterpolate: false, intent: .defaultIntent)
        else { return nil }
        return VisualMask(image: image, count: count)
    }

    /// An image's pixels drawn at a size, as RGBA bytes.
    private nonisolated static func rgba(_ image: NSImage, width: Int, height: Int) -> [UInt8]? {
        var rect = CGRect(x: 0, y: 0, width: width, height: height)
        guard let cg = image.cgImage(forProposedRect: &rect, context: nil, hints: nil) else { return nil }
        var bytes = [UInt8](repeating: 0, count: width * height * 4)
        let drawn = bytes.withUnsafeMutableBytes { buffer -> Bool in
            guard let context = CGContext(
                data: buffer.baseAddress, width: width, height: height, bitsPerComponent: 8, bytesPerRow: width * 4,
                space: CGColorSpace(name: CGColorSpace.sRGB)!,
                bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
            else { return false }
            context.interpolationQuality = .none
            context.draw(cg, in: CGRect(x: 0, y: 0, width: width, height: height))
            return true
        }
        return drawn ? bytes : nil
    }
}
