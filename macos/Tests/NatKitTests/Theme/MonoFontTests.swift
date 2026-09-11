import AppKit
import CoreText
import SwiftUI
import XCTest
@testable import NatKit

/// The bundled face, and the rule that nothing breaks without it.
///
/// The acceptance this stands for is "a machine without JetBrains Mono
/// installed renders in JetBrains Mono", and the awkward part of asserting
/// it is that a machine *with* the family installed — every developer who
/// already uses it, and never the CI runner — resolves every face whether
/// anything shipped or not. So the two halves are asserted apart: that the
/// files ship and declare the faces (asked of the bytes, true anywhere), and
/// that registering leaves them resolvable (which only the bundle can be
/// making true on a runner that has never seen the family).
final class MonoFontTests: XCTestCase {
    // MARK: - What ships

    /// Four faces, and the names the call sites resolve them by. A face
    /// renamed upstream is a silent fall back to the system font, which is
    /// precisely the thing this test is here to fail over.
    func testTheFourFacesAreDeclared() {
        XCTAssertEqual(MonoFont.faces, [
            "JetBrainsMono-Regular",
            "JetBrainsMono-Bold",
            "JetBrainsMono-Italic",
            "JetBrainsMono-BoldItalic",
        ])
    }

    func testTheResourceBundleIsFound() {
        XCTAssertNotNil(MonoFont.resourceBundle)
    }

    /// All four TTFs are in the bundle, in the order they are declared —
    /// which is what says `.copy("Resources/Fonts")` kept the directory and
    /// nothing was left out of the checkout.
    func testEveryDeclaredFaceIsBundled() {
        let urls = MonoFont.bundledFontURLs
        XCTAssertEqual(urls.count, MonoFont.faces.count)
        XCTAssertEqual(urls.map { $0.deletingPathExtension().lastPathComponent }, MonoFont.faces)
        for url in urls {
            XCTAssertTrue(FileManager.default.fileExists(atPath: url.path), url.path)
        }
    }

    /// The licence ships beside the faces, because the OFL requires it to.
    func testTheLicenceShipsWithTheFaces() throws {
        let bundle = try XCTUnwrap(MonoFont.resourceBundle)
        let url = bundle.url(
            forResource: "OFL", withExtension: "txt", subdirectory: MonoFont.fontsDirectory)
        XCTAssertNotNil(url)
    }

    /// The bundled files themselves carry the faces the call sites name —
    /// asked of the bytes rather than of the text system, because a
    /// developer machine with JetBrains Mono already installed would resolve
    /// every name whether anything shipped or not, and this is the assertion
    /// that shipping is what makes it resolve.
    func testEachBundledFileDeclaresItsFace() throws {
        let urls = MonoFont.bundledFontURLs
        XCTAssertEqual(urls.count, MonoFont.faces.count)
        for (url, face) in zip(urls, MonoFont.faces) {
            let descriptors = try XCTUnwrap(
                CTFontManagerCreateFontDescriptorsFromURL(url as CFURL) as? [CTFontDescriptor])
            let names = descriptors.map {
                CTFontDescriptorCopyAttribute($0, kCTFontNameAttribute) as? String
            }
            XCTAssertEqual(names, [face], url.lastPathComponent)
        }
    }

    // MARK: - Registration

    /// Registering is what makes the face available on a machine that has
    /// never installed it, and it answers for the regular face because that
    /// is the one every fallback is decided on.
    func testRegisteringMakesTheFaceResolvable() {
        XCTAssertTrue(MonoFont.register())
        XCTAssertTrue(MonoFont.isResolvable(MonoFont.regularFace))
    }

    /// It is idempotent: the app calls it at launch and `Typo.mono` calls it
    /// on every use, and a second registration must not undo the first.
    func testRegisteringTwiceIsTheSameAnswer() {
        XCTAssertEqual(MonoFont.register(), MonoFont.register())
    }

    func testAFaceThatIsNotThereIsNotResolvable() {
        XCTAssertFalse(MonoFont.isResolvable("NoSuchFace-Regular"))
    }

    /// Every face the family declares resolves once registered — the bold
    /// and the italics included, since SwiftUI asks for those by name too.
    func testEveryFaceResolves() {
        MonoFont.register()
        for face in MonoFont.faces {
            XCTAssertTrue(MonoFont.isResolvable(face), face)
        }
    }

    /// The four corners of the family, each answering with its own face.
    func testFaceNamesTheWeightAndSlantAskedFor() {
        XCTAssertEqual(MonoFont.face(), MonoFont.regularFace)
        XCTAssertEqual(MonoFont.face(bold: true), MonoFont.boldFace)
        XCTAssertEqual(MonoFont.face(italic: true), MonoFont.italicFace)
        XCTAssertEqual(MonoFont.face(bold: true, italic: true), MonoFont.boldItalicFace)
    }

    /// The faces are the family's, which is what lets `NSFontManager` and
    /// SwiftUI's own `.italic()` find their way between them.
    func testTheFacesBelongToTheOneFamily() throws {
        MonoFont.register()
        let font = try XCTUnwrap(NSFont(name: MonoFont.regularFace, size: Typo.code))
        XCTAssertEqual(font.familyName, MonoFont.family)
    }

    /// A monospaced family, which is the whole reason it is the app's code
    /// face: a terminal column and a diff gutter are cells.
    func testTheFaceIsFixedPitch() throws {
        MonoFont.register()
        let font = try XCTUnwrap(NSFont(name: MonoFont.regularFace, size: Typo.code))
        XCTAssertTrue(font.isFixedPitch)
    }

    // MARK: - Typo.mono

    func testMonoIsTheBundledFaceAtTheSizeAskedFor() {
        XCTAssertEqual(Typo.mono(size: Typo.code), .custom(MonoFont.regularFace, fixedSize: Typo.code))
    }

    func testMonoTakesTheBoldFaceForAHeavyWeight() {
        XCTAssertEqual(Typo.mono(size: Typo.code, weight: .bold),
                       .custom(MonoFont.boldFace, fixedSize: Typo.code))
    }

    /// Without the face, the system's monospaced font — never its
    /// proportional one, which is what a bare `Font.custom` on a missing
    /// name would fall back to.
    func testMonoFallsBackToTheSystemMonospacedFont() {
        XCTAssertEqual(Typo.mono(size: 11, weight: .regular, face: nil),
                       .system(size: 11, weight: .regular, design: .monospaced))
    }

    func testMonoNSFontIsTheBundledFace() {
        let font = Typo.monoNSFont(size: Typo.code)
        XCTAssertEqual(font.fontName, MonoFont.regularFace)
        XCTAssertEqual(font.pointSize, Typo.code)
    }

    func testMonoNSFontTakesTheBoldFaceForAHeavyWeight() {
        XCTAssertEqual(Typo.monoNSFont(size: Typo.code, weight: .bold).fontName, MonoFont.boldFace)
    }

    func testMonoNSFontFallsBackWithNoFace() {
        XCTAssertEqual(Typo.monoNSFont(size: Typo.code, weight: .regular, face: nil),
                       NSFont.monospacedSystemFont(ofSize: Typo.code, weight: .regular))
    }

    /// A named face that will not resolve falls back too, rather than
    /// handing back a nil font nobody can draw with.
    func testMonoNSFontFallsBackWithAnUnresolvableFace() {
        XCTAssertEqual(
            Typo.monoNSFont(size: Typo.code, weight: .regular, face: "NoSuchFace-Regular"),
            NSFont.monospacedSystemFont(ofSize: Typo.code, weight: .regular))
    }

    /// Which weights the two bundled faces stand for: the heavy half is
    /// named, since `Font.Weight` cannot be compared.
    func testWeightsAboveMediumAreBold() {
        for weight in [Font.Weight.semibold, .bold, .heavy, .black] {
            XCTAssertTrue(Typo.isBold(weight))
        }
        for weight in [Font.Weight.ultraLight, .thin, .light, .regular, .medium] {
            XCTAssertFalse(Typo.isBold(weight))
        }
    }
}
