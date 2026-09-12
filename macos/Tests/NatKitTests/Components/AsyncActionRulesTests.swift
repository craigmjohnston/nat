import XCTest

/// What a spinner is allowed to do to a button, read off the source the way
/// `ViewLayerRulesTests` reads the colour rules — the views live in the app
/// target, which has no tests of its own, and the rule is about how a call
/// site is written rather than about anything a value could carry.
///
/// The rule: a button that can spin sits at its text's own width when idle,
/// grows by the spinner while the work runs, and keeps its label visible
/// throughout. So the wrapper shows the spinner only while busy, and no call
/// site reserves the room back with a `frame(width:)` of its own or hides
/// its label to make space.
final class AsyncActionRulesTests: XCTestCase {
    private func packageRoot() -> URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()   // Components
            .deletingLastPathComponent()   // NatKitTests
            .deletingLastPathComponent()   // Tests
            .deletingLastPathComponent()   // the package root
    }

    private func source(_ relative: String) throws -> String {
        let url = packageRoot().appendingPathComponent(relative)
        guard let text = try? String(contentsOf: url, encoding: .utf8) else {
            throw XCTSkip("\(relative) was not found beside the tests")
        }
        return text
    }

    /// Every Swift source of the view layer, paired with its name.
    private func viewSources() throws -> [(String, String)] {
        let root = packageRoot().appendingPathComponent("Sources")
        var found: [(String, String)] = []
        guard let walk = FileManager.default.enumerator(
            at: root, includingPropertiesForKeys: [.isRegularFileKey]
        ) else {
            throw XCTSkip("no Sources directory beside the tests")
        }
        for case let url as URL in walk where url.pathExtension == "swift" {
            found.append((url.lastPathComponent, try String(contentsOf: url, encoding: .utf8)))
        }
        XCTAssertGreaterThan(found.count, 20, "the view layer should have been found and read")
        return found
    }

    /// The label block of every `AsyncActionLabel(isBusy:)` call — the
    /// trailing closure's contents, brace-matched, so a modifier applied to
    /// the wrapper after it is not read as part of the label.
    private func labelBlocks(in source: String) -> [String] {
        var blocks: [String] = []
        var search = source.startIndex..<source.endIndex
        while let call = source.range(of: "AsyncActionLabel(isBusy:", range: search) {
            search = call.upperBound..<source.endIndex
            guard let open = source[call.upperBound...].firstIndex(of: "{") else { break }
            var depth = 0
            var index = open
            while index < source.endIndex {
                if source[index] == "{" { depth += 1 }
                if source[index] == "}" {
                    depth -= 1
                    if depth == 0 { break }
                }
                index = source.index(after: index)
            }
            guard index < source.endIndex else { break }
            blocks.append(String(source[source.index(after: open)..<index]))
            search = index..<source.endIndex
        }
        return blocks
    }

    /// The wrapper takes room for the spinner rather than holding it: idle,
    /// there is nothing in the `HStack` but the label. `BusySlot` is the
    /// other reading — a slot held whatever happens — and a button using it
    /// would be back to a blank column beside every idle label.
    func testTheWrapperShowsTheSpinnerOnlyWhileBusy() throws {
        let helpers = try source("Sources/NatApp/Views/ViewHelpers.swift")
        guard let body = helpers.range(of: "struct AsyncActionLabel"),
              let end = helpers.range(of: "struct BusySpinner") else {
            return XCTFail("AsyncActionLabel and BusySpinner should both be in ViewHelpers.swift")
        }
        let wrapper = String(helpers[body.lowerBound..<end.lowerBound])
        XCTAssertTrue(
            wrapper.contains("if isBusy {"),
            "AsyncActionLabel should draw the spinner only while the work is in flight"
        )
        XCTAssertFalse(
            wrapper.contains("BusySlot"),
            "AsyncActionLabel grows by its spinner; a held slot is RefreshingMark's rule, not a button's"
        )
    }

    /// No call site reserves the width back. A `frame(width:)` or a
    /// `minWidth` around the label is exactly the padding the held slot used
    /// to need, and it puts the idle button back at more than its text.
    func testNoSpinnerCapableButtonReservesAWidth() throws {
        var seen = 0
        for (name, source) in try viewSources() {
            for block in labelBlocks(in: source) {
                seen += 1
                for reservation in [".frame(width:", ".frame(minWidth:", "minWidth:"] {
                    XCTAssertFalse(
                        block.contains(reservation),
                        "\(name): a spinner-capable button sits at its text's width — drop the \(reservation)"
                    )
                }
            }
        }
        XCTAssertGreaterThan(seen, 5, "the AsyncActionLabel call sites should have been found")
    }

    /// The label stays visible while the work runs: the spinner joins it
    /// rather than replacing it, so nothing inside the block draws a spinner
    /// of its own or fades itself out to make room for one.
    func testNoSpinnerCapableButtonHidesItsLabel() throws {
        for (name, source) in try viewSources() {
            for block in labelBlocks(in: source) {
                for swap in ["ProgressView", ".opacity(", ".hidden()"] {
                    XCTAssertFalse(
                        block.contains(swap),
                        "\(name): the label stays visible while busy — \(swap) swaps it out"
                    )
                }
            }
        }
    }
}
