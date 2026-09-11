import XCTest

/// The rules the view layer is held to, read off the source.
///
/// Wrapper types were the obvious way to make the original bug uncompilable,
/// and they are the wrong instrument: ink drawn as a *mark* is correct — an
/// icon, a glyph, a rule are all ink — and it was ink drawn as *ground* that
/// put `text` on `overlay0`. A type that forbade one would forbid the other.
///
/// So the rule is stated where it is true: a ground is painted through the
/// surface vocabulary and nowhere else. `DesignTokens.fill(_:)` already takes
/// a `Ground` and nothing else, so the only way past it is handing an ink
/// straight to `.background`, and that is what this reads the source for.
final class ViewLayerRulesTests: XCTestCase {
    /// Swift sources of the view layer, paired with their names.
    private func viewSources() throws -> [(String, String)] {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()   // Components
            .deletingLastPathComponent()   // NatKitTests
            .deletingLastPathComponent()   // Tests
            .deletingLastPathComponent()   // the package root
            .appendingPathComponent("Sources")
        var found: [(String, String)] = []
        let keys: [URLResourceKey] = [.isRegularFileKey]
        guard let walk = FileManager.default.enumerator(at: root, includingPropertiesForKeys: keys) else {
            throw XCTSkip("no Sources directory beside the tests")
        }
        for case let url as URL in walk where url.pathExtension == "swift" {
            // The theme and the components are where colour is *defined*; the
            // rules below are about where it is used.
            let path = url.path
            guard !path.contains("/Theme/"), !path.contains("/Components/") else { continue }
            found.append((url.lastPathComponent, try String(contentsOf: url, encoding: .utf8)))
        }
        // A walk that found nothing would make every rule below vacuously
        // true, which is the one way a test like this fails silently.
        XCTAssertGreaterThan(found.count, 20, "the view layer should have been found and read")
        return found
    }

    /// A ground is never an ink. This is the bug the whole theme change came
    /// out of — the hover fill, the tab's live-count badge and the loading
    /// skeleton all filled with `labelQuaternary` — and the one shape of it
    /// the type system cannot refuse.
    func testNoInkIsPaintedAsAGround() throws {
        let inks = ["label", "labelSecondary", "labelTertiary", "labelQuaternary", "accentText"]
        for (name, source) in try viewSources() {
            for ink in inks {
                for painter in ["background", "surface"] {
                    XCTAssertFalse(
                        source.contains(".\(painter)(DesignTokens.\(ink))"),
                        "\(name): \(ink) is ink and cannot be a ground — paint it with surface(_:) or a wash"
                    )
                }
            }
            XCTAssertFalse(
                source.contains(".background(DesignTokens.ink("),
                "\(name): ink(_:on:) is for marks and text, not for grounds"
            )
        }
    }

    /// Text says what it is for. A view naming a label tier directly has
    /// skipped the ground its colour depends on, which is how one `separator`
    /// came to render as five different colours.
    func testTextIsColouredByRoleRatherThanByToken() throws {
        for (name, source) in try viewSources() {
            for ink in ["label", "labelSecondary", "labelTertiary", "labelQuaternary"] {
                XCTAssertFalse(
                    source.contains(".foregroundStyle(DesignTokens.\(ink))"),
                    "\(name): use ink(.primary/.secondary/.tertiary) rather than naming \(ink)"
                )
            }
        }
    }

    /// No view invents a colour. Every one of them comes from the palette, by
    /// a name or by a rule over names — a hex typed into a view is the one
    /// thing no theme can follow.
    func testNoViewTypesAColour() throws {
        for (name, source) in try viewSources() {
            let hex = try NSRegularExpression(pattern: #"Color\(hex:\s*"|#[0-9a-fA-F]{6}""#)
            let range = NSRange(source.startIndex..., in: source)
            XCTAssertEqual(
                hex.numberOfMatches(in: source, range: range), 0,
                "\(name): a colour typed into a view belongs in the palette"
            )
        }
    }
}
