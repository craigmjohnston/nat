import AppKit
import XCTest
@testable import NatKit

/// Trying another monospaced family on: `NAT_MONO_FAMILY` and
/// `NAT_MONO_FONT_DIR`, and the rule that a trial that fails draws the
/// bundled face.
///
/// Menlo stands in for the family under trial: it ships with every macOS and
/// carries a regular and a bold but no medium, so what the font manager
/// answers for it is the same on every machine.
final class MonoFontTrialTests: XCTestCase {
    // MARK: - Reading the environment

    func testNoFamilyInTheEnvironmentIsNoTrial() {
        XCTAssertNil(MonoFont.Trial(environment: [:]))
        XCTAssertNil(MonoFont.Trial(environment: ["NAT_MONO_FAMILY": ""]))
        XCTAssertNil(MonoFont.Trial(environment: ["NAT_MONO_FONT_DIR": "/tmp/fonts"]))
    }

    func testTheEnvironmentNamesTheFamilyAlone() {
        let trial = MonoFont.Trial(environment: ["NAT_MONO_FAMILY": "Menlo"])
        XCTAssertEqual(trial, MonoFont.Trial(family: "Menlo"))
        XCTAssertNil(trial?.fontDirectory)
    }

    func testTheEnvironmentNamesTheFamilyAndItsDirectory() {
        let trial = MonoFont.Trial(environment: [
            "NAT_MONO_FAMILY": "Fira Code", "NAT_MONO_FONT_DIR": "/tmp/fonts/fira",
        ])
        XCTAssertEqual(trial?.family, "Fira Code")
        XCTAssertEqual(trial?.fontDirectory, URL(fileURLWithPath: "/tmp/fonts/fira", isDirectory: true))
    }

    // MARK: - Resolving a face

    func testTheEndsOfTheRampEachAnswerWithTheirOwnFace() {
        let trial = MonoFont.Trial(family: "Menlo")
        XCTAssertEqual(trial.face(weight: .regular), "Menlo-Regular")
        XCTAssertEqual(trial.face(weight: .bold), "Menlo-Bold")
    }

    /// A family with no medium answers with its nearest face rather than
    /// nothing — the step is a preference, the family the requirement.
    func testAStepTheFamilyLacksIsItsNearestFace() throws {
        let name = try XCTUnwrap(MonoFont.Trial(family: "Menlo").face(weight: .medium))
        XCTAssertEqual(NSFont(name: name, size: 12)?.familyName, "Menlo")
    }

    func testAFamilyThatIsNotThereIsNoFace() {
        XCTAssertNil(MonoFont.Trial(family: "No Such Family").face(weight: .regular))
    }

    // MARK: - Through MonoFont.face

    func testFaceDrawsInTheTrialFamily() {
        let trial = MonoFont.Trial(family: "Menlo")
        XCTAssertEqual(MonoFont.face(weight: .regular, trial: trial), "Menlo-Regular")
        XCTAssertEqual(MonoFont.face(weight: .bold, trial: trial), "Menlo-Bold")
    }

    /// The fallback is the bundled face, not the system's: a trial that
    /// failed draws the app as it ships.
    func testFaceFallsBackToTheBundledFaceWhereTheTrialFamilyIsMissing() {
        let trial = MonoFont.Trial(family: "No Such Family")
        XCTAssertEqual(MonoFont.face(weight: .regular, trial: trial), MonoFont.regularFace)
        XCTAssertEqual(MonoFont.face(weight: .bold, trial: trial), MonoFont.boldFace)
    }

    func testFaceWithNoTrialIsTheBundledFace() {
        XCTAssertEqual(MonoFont.face(weight: .medium, trial: nil), MonoFont.mediumFace)
    }

    // MARK: - The directory

    func testFontURLsAreTheFontFilesInTheDirectorySortedByName() throws {
        let dir = try makeDirectory(files: ["b.ttf", "OFL.txt", "a.otf", "c.TTC", "notes.md"])
        let trial = MonoFont.Trial(family: "Any", fontDirectory: dir)
        XCTAssertEqual(trial.fontURLs.map(\.lastPathComponent), ["a.otf", "b.ttf", "c.TTC"])
    }

    func testFontURLsOfNoDirectoryAreNone() {
        XCTAssertEqual(MonoFont.Trial(family: "Any").fontURLs, [])
    }

    func testFontURLsOfAMissingDirectoryAreNone() {
        let dir = URL(fileURLWithPath: "/no/such/directory", isDirectory: true)
        XCTAssertEqual(MonoFont.Trial(family: "Any", fontDirectory: dir).fontURLs, [])
    }

    /// Registering the directory's files is the same call the bundled faces
    /// go through: a file that is a font registers (or is already there,
    /// which is the same outcome), and one that is not is passed over
    /// without raising.
    func testRegisterFilesTakesFontsAndPassesOverWhatIsNot() throws {
        let bundled = try XCTUnwrap(MonoFont.bundledFontURLs.first)
        let dir = try makeDirectory(files: ["junk.ttf"])
        let copy = dir.appendingPathComponent("FiraCode-Regular.ttf")
        try FileManager.default.copyItem(at: bundled, to: copy)
        MonoFont.registerFiles([copy, dir.appendingPathComponent("junk.ttf")])
        XCTAssertTrue(MonoFont.isResolvable(MonoFont.regularFace))
    }

    private func makeDirectory(files: [String]) throws -> URL {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent("mono-trial-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        for name in files {
            try Data("not a font".utf8).write(to: dir.appendingPathComponent(name))
        }
        return dir
    }
}
