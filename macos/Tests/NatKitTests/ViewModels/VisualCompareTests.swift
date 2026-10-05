import AppKit
import XCTest
@testable import NatKit

final class VisualCompareTests: XCTestCase {
    func testTheDividerIsClampedToTheFrame() {
        XCTAssertEqual(VisualCompare.clamp(-1), 0)
        XCTAssertEqual(VisualCompare.clamp(2), 1)
        XCTAssertEqual(VisualCompare.clamp(0.3), 0.3)
        XCTAssertEqual(VisualCompare.clamp(.nan), VisualCompare.middle)
        XCTAssertEqual(VisualCompare.divider(at: 50, width: 200), 0.25)
        XCTAssertEqual(VisualCompare.divider(at: -10, width: 200), 0)
        XCTAssertEqual(VisualCompare.divider(at: 500, width: 200), 1)
        XCTAssertEqual(VisualCompare.divider(at: 5, width: 0), VisualCompare.middle)
    }

    func testTheToggleFollowsTheDivider() {
        XCTAssertEqual(VisualCompare.divider(showing: .before), 1, "the before whole: the divider far right")
        XCTAssertEqual(VisualCompare.divider(showing: .after), 0)
        XCTAssertEqual(VisualCompare.side(showing: 1), .before)
        XCTAssertEqual(VisualCompare.side(showing: 0), .after)
        XCTAssertEqual(VisualCompare.side(showing: 3), .before)
        XCTAssertNil(VisualCompare.side(showing: 0.5))
        XCTAssertNil(VisualCompare.side(showing: 0.01))
    }

    func testTheFrameIsTheLargerOfEachDimension() {
        XCTAssertEqual(VisualCompare.frameSize(CGSize(width: 100, height: 20), CGSize(width: 50, height: 80)),
                       CGSize(width: 100, height: 80))
    }

    func testHighlightRefusals() {
        let image = NSImage(size: CGSize(width: 1, height: 1))
        let big = VisualImage.image(image, pixelSize: CGSize(width: 10, height: 10))
        let small = VisualImage.image(image, pixelSize: CGSize(width: 8, height: 10))
        XCTAssertNil(VisualCompare.highlightRefusal(after: big, before: big))
        XCTAssertEqual(VisualCompare.highlightRefusal(after: .unavailable, before: big), "The image couldn't be opened")
        XCTAssertEqual(VisualCompare.highlightRefusal(after: big, before: nil), "The before couldn't be opened")
        XCTAssertEqual(VisualCompare.highlightRefusal(after: big, before: small),
                       "The before is 8×10 and the after 10×10: only images of the same size can be compared pixel for pixel")
    }

    func testTheMaskMarksExactlyThePixelsThatDiffer() throws {
        let black = NSColor(deviceRed: 0, green: 0, blue: 0, alpha: 1)
        let white = NSColor(deviceRed: 1, green: 1, blue: 1, alpha: 1)
        let before = try image(width: 3, height: 2) { _, _ in black }
        let after = try image(width: 3, height: 2) { x, y in x == 1 && y == 0 ? white : black }
        let mask = try XCTUnwrap(VisualCompare.differenceMask(before: before, after: after, pixelSize: CGSize(width: 3, height: 2)))
        XCTAssertEqual(mask.count, 1)
        XCTAssertEqual(mask.image.width, 3)
        XCTAssertEqual(mask.image.height, 2)
        XCTAssertEqual(VisualCompare.differenceMask(before: before, after: before, pixelSize: CGSize(width: 3, height: 2))?.count, 0)
        XCTAssertNil(VisualCompare.differenceMask(before: before, after: after, pixelSize: .zero))
    }

    private func image(width: Int, height: Int, color: (Int, Int) -> NSColor) throws -> NSImage {
        let rep = try XCTUnwrap(NSBitmapImageRep(
            bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height, bitsPerSample: 8, samplesPerPixel: 4,
            hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0))
        for x in 0..<width {
            for y in 0..<height { rep.setColor(color(x, y), atX: x, y: y) }
        }
        let image = NSImage(size: NSSize(width: width, height: height))
        image.addRepresentation(rep)
        return image
    }
}
