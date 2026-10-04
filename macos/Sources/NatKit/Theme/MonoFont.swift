import AppKit
import CoreText
import SwiftUI

/// The app's monospaced face: Fira Code, shipped inside the app rather than
/// asked of the Mac it runs on.
///
/// It is bundled for the same reason the palette is stated rather than taken
/// from the system: a terminal, a diff and a code span are the surfaces this
/// app is read on, and leaving their face to `design: .monospaced` means
/// whatever SF Mono the OS release happens to carry — a different rhythm on
/// a different machine, and one nobody here chose. Three static faces are
/// carried (regular, medium, bold, ~870KB, OFL — the licence sits beside
/// them in `Resources/Fonts`): the three-step weight ramp the app draws
/// with. There is no italic, because Fira Code has none, and nothing in the
/// app sets code in one. The variable font is not carried either, because a
/// face registered per weight is a face that resolves by name.
///
/// Nothing installs anything: `register` hands the files to CoreText for
/// this process alone, so a machine without Fira Code has it for as long
/// as gnat is running and no longer. Everything that draws in it goes
/// through `Typo.mono`, which falls back to the monospaced system font
/// wherever the face is not there to be had — a registration that failed, a
/// bundle built without the resource — so the app is readable either way.
/// The three weights the bundled family carries.
public enum MonoWeight: Equatable, Sendable {
    case regular, medium, bold
}

public enum MonoFont {
    /// The family, as the faces name themselves.
    public static let family = "Fira Code"

    /// The PostScript names the faces resolve by. They are what `NSFont` and
    /// `Font.custom` are given: a family name leaves the choice of face to
    /// the text system, and the point of registering each is to say which.
    public static let regularFace = "FiraCode-Regular"
    public static let mediumFace = "FiraCode-Medium"
    public static let boldFace = "FiraCode-Bold"

    /// Every face this bundles, in the order they are registered.
    public static let faces = [regularFace, mediumFace, boldFace]

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

    /// The face for a step of the weight ramp, or `nil` where the family is
    /// not available and the caller should fall back to the system's own
    /// monospaced font.
    ///
    /// Availability is asked of the text system rather than of the
    /// registration, because those are different questions: the face may be
    /// installed on the Mac already, and a registration that failed against
    /// an identical font already registered is not a font that is missing.
    public static func face(weight: MonoWeight) -> String? {
        face(weight: weight, trial: trial)
    }

    /// The above with the trial named rather than read off the environment
    /// — the seam the tests drive it through, since the environment of a test
    /// run is not one a test can set.
    static func face(weight: MonoWeight, trial: Trial?) -> String? {
        register()
        if let trial, let name = trial.face(weight: weight) {
            return name
        }
        let name: String
        switch weight {
        case .regular: name = regularFace
        case .medium: name = mediumFace
        case .bold: name = boldFace
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

    // MARK: - Trying another face on

    /// A family tried on in place of the bundled one, read off the
    /// environment once: `NAT_MONO_FAMILY=<family>` draws every monospaced
    /// surface — the terminal, the diff, code spans, every input — in that
    /// family at the weight asked for, and `NAT_MONO_FONT_DIR=<dir>`
    /// registers every font file in that directory for the process first, so
    /// a family need not be installed on the Mac to be tried. A dev and
    /// gallery affordance beside `NAT_SNAPSHOT`: unset, nothing here runs.
    ///
    /// A family that is not there — misspelt, or its files not where the
    /// directory said — falls back to the bundled face, never to the
    /// system's, so a trial that failed draws the app as it ships rather than
    /// as something nobody asked for.
    public struct Trial: Equatable, Sendable {
        public let family: String
        public let fontDirectory: URL?

        /// `nil` where the environment names no family, which is every run
        /// but a trial.
        public init?(environment: [String: String]) {
            guard let family = environment["NAT_MONO_FAMILY"], !family.isEmpty else { return nil }
            self.init(
                family: family,
                fontDirectory: environment["NAT_MONO_FONT_DIR"].map {
                    URL(fileURLWithPath: $0, isDirectory: true)
                })
        }

        init(family: String, fontDirectory: URL? = nil) {
            self.family = family
            self.fontDirectory = fontDirectory
        }

        /// The PostScript name of the family's face nearest the weight asked
        /// for, or `nil` where the family is not there to be had.
        /// `NSFontManager` does the nearest-match, which is what lets a
        /// family with no medium answer with its regular, and a variable
        /// font answer with a named instance.
        func face(weight: MonoWeight) -> String? {
            let step: Int
            switch weight {
            case .regular: step = 5
            case .medium: step = 6
            case .bold: step = 9
            }
            return NSFontManager.shared
                .font(withFamily: family, traits: [], weight: step, size: NSFont.systemFontSize)?
                .fontName
        }

        /// The font files in the directory, sorted by name so a registration
        /// runs in one order; empty where there is no directory, or none
        /// that can be read.
        var fontURLs: [URL] {
            guard let fontDirectory,
                  let names = try? FileManager.default.contentsOfDirectory(atPath: fontDirectory.path)
            else { return [] }
            return names.sorted()
                .filter { ["ttf", "otf", "ttc"].contains(($0 as NSString).pathExtension.lowercased()) }
                .map { fontDirectory.appendingPathComponent($0) }
        }
    }

    /// The trial this process runs under, if any.
    static let trial = Trial(environment: ProcessInfo.processInfo.environment)

    // MARK: - Registration

    /// The one registration, run on first access. A `static let` is how this
    /// stays once-only and thread-safe without a lock of its own.
    private static let registration: Bool = {
        registerFiles(bundledFontURLs)
        registerFiles(trial?.fontURLs ?? [])
        return isResolvable(regularFace)
    }()

    /// Hands each file to CoreText for this process.
    static func registerFiles(_ urls: [URL]) {
        for url in urls {
            var error: Unmanaged<CFError>?
            // `.process` rather than `.persistent`: the face is gnat's for
            // as long as gnat runs, and an app that quietly installs fonts
            // on the Mac is an app that leaves something behind.
            if !CTFontManagerRegisterFontsForURL(url as CFURL, .process, &error) {
                // Nothing is raised: the commonest failure by far is the
                // face already being registered — installed on the Mac, or
                // a second call in a process that has already done this —
                // and in both of those the font is there to be used. The
                // resolvable check in `registration` is what actually answers.
                error?.release()
            }
        }
    }

    /// The bundled TTFs inside the resource bundle, in `faces` order so a
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
