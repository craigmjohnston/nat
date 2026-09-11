import AppKit
import CoreText
import SwiftUI

/// The app's monospaced face: JetBrains Mono, shipped inside the app rather
/// than asked of the Mac it runs on.
///
/// It is bundled for the same reason the palette is stated rather than taken
/// from the system: a terminal, a diff and a code span are the surfaces this
/// app is read on, and leaving their face to `design: .monospaced` means
/// whatever SF Mono the OS release happens to carry — a different rhythm on
/// a different machine, and one nobody here chose. Four static faces are
/// carried (regular, bold, and both italics, ~1.1MB, OFL — the licence sits
/// beside them in `Resources/Fonts`), which is every combination SwiftUI can
/// ask this family for; the variable font is not, because a face registered
/// per weight is a face that resolves by name.
///
/// Nothing installs anything: `register` hands the files to CoreText for
/// this process alone, so a machine without JetBrains Mono has it for as
/// long as gnat is running and no longer. Everything that draws in it goes
/// through `Typo.mono`, which falls back to the monospaced system font
/// wherever the face is not there to be had — a registration that failed, a
/// bundle built without the resource — so the app is readable either way.
public enum MonoFont {
    /// The family, as the four faces name themselves.
    public static let family = "JetBrains Mono"

    /// The PostScript names the faces resolve by. They are what `NSFont` and
    /// `Font.custom` are given: a family name leaves the choice of face to
    /// the text system, and the point of registering four is to say which.
    public static let regularFace = "JetBrainsMono-Regular"
    public static let boldFace = "JetBrainsMono-Bold"
    public static let italicFace = "JetBrainsMono-Italic"
    public static let boldItalicFace = "JetBrainsMono-BoldItalic"

    /// Every face this bundles, in the order they are registered.
    public static let faces = [regularFace, boldFace, italicFace, boldItalicFace]

    /// Registers the bundled faces with CoreText for this process.
    ///
    /// Idempotent and safe to call from anywhere: the work happens once, on
    /// the first call, and every later one reads the answer. `Typo.mono`
    /// calls it itself, so a preview or a test that draws code gets the face
    /// without a launch having happened; the app calls it at launch anyway,
    /// so the cost is paid before the first frame rather than during it.
    ///
    /// The answer is whether the regular face can now be resolved by name,
    /// which is the only thing a caller could act on — a registration that
    /// reported success and left nothing resolvable would be worse than one
    /// that said so.
    @discardableResult
    public static func register() -> Bool { registration }

    /// The face to draw in, or `nil` where the family is not available and
    /// the caller should fall back to the system's own monospaced font.
    ///
    /// Availability is asked of the text system rather than of the
    /// registration, because those are different questions: the face may be
    /// installed on the Mac already, and a registration that failed against
    /// an identical font already registered is not a font that is missing.
    public static func face(bold: Bool = false, italic: Bool = false) -> String? {
        register()
        let name: String
        switch (bold, italic) {
        case (false, false): name = regularFace
        case (true, false): name = boldFace
        case (false, true): name = italicFace
        case (true, true): name = boldItalicFace
        }
        return isResolvable(name) ? name : nil
    }

    /// Whether a face resolves by name — which is the whole of what the call
    /// sites need to know, since a name that resolves to nothing would draw
    /// in the system's proportional font and silently un-align every column
    /// this family exists to line up.
    static func isResolvable(_ name: String) -> Bool {
        NSFont(name: name, size: NSFont.systemFontSize) != nil
    }

    // MARK: - Registration

    /// The one registration, run on first access. A `static let` is how this
    /// stays once-only and thread-safe without a lock of its own.
    private static let registration: Bool = {
        for url in bundledFontURLs {
            var error: Unmanaged<CFError>?
            // `.process` rather than `.persistent`: the face is gnat's for
            // as long as gnat runs, and an app that quietly installs fonts
            // on the Mac is an app that leaves something behind.
            if !CTFontManagerRegisterFontsForURL(url as CFURL, .process, &error) {
                // Nothing is raised: the commonest failure by far is the
                // face already being registered — installed on the Mac, or
                // a second call in a process that has already done this —
                // and in both of those the font is there to be used. The
                // resolvable check below is what actually answers.
                error?.release()
            }
        }
        return isResolvable(regularFace)
    }()

    /// The four TTFs inside the resource bundle, in `faces` order so a
    /// registration reads in the order the faces are declared.
    static var bundledFontURLs: [URL] {
        guard let bundle = resourceBundle else { return [] }
        return faces.compactMap {
            bundle.url(forResource: $0, withExtension: "ttf", subdirectory: fontsDirectory)
        }
    }

    /// The subdirectory `Package.swift` copies the fonts in as, kept whole
    /// by `.copy` rather than flattened by `.process`.
    static let fontsDirectory = "Fonts"

    /// SwiftPM's resource bundle for this target, found by hand rather than
    /// through the generated `Bundle.module`, whose accessor traps when the
    /// bundle is missing — and missing is not fatal here: an app that cannot
    /// find its fonts falls back to the system's monospaced face and goes on
    /// running, which is what `Typo.mono` is written to do.
    static var resourceBundle: Bundle? {
        let name = "nat_NatKit.bundle"
        let kit = Bundle(for: BundleToken.self)
        let candidates = [
            // A framework build: inside NatKit's own bundle.
            kit.resourceURL,
            kit.bundleURL,
            // A test run: `.xctest` is its own bundle, and SwiftPM leaves
            // the resource bundles of the targets under test beside it
            // rather than inside it.
            kit.bundleURL.deletingLastPathComponent(),
            // The bundled app, where make-app.sh copies it.
            Bundle.main.resourceURL,
            // A bare executable, where SwiftPM leaves it beside the binary.
            Bundle.main.bundleURL,
            Bundle.main.bundleURL.deletingLastPathComponent(),
        ].compactMap { $0?.appendingPathComponent(name) }
        return candidates.lazy.compactMap { Bundle(url: $0) }.first
    }

    /// Only ever a handle on the bundle NatKit's code was loaded from.
    private final class BundleToken {}
}
