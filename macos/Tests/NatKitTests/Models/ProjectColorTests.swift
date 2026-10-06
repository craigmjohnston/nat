import XCTest
@testable import NatKit

/// A project's colour as each decoder of a config entry reads it: a known
/// name, an unknown one (nil, never a failed read) and none (nil).
final class ProjectColorTests: XCTestCase {
    func testTheEightNamesAreNatsInNatsOrder() {
        XCTAssertEqual(
            ProjectColor.allCases.map(\.rawValue),
            ["red", "orange", "yellow", "green", "teal", "blue", "purple", "pink"])
        XCTAssertEqual(ProjectColor(word: "teal"), .teal)
        XCTAssertNil(ProjectColor(word: "magenta"))
        XCTAssertNil(ProjectColor(word: nil))
    }

    private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
        try JSONDecoder().decode(T.self, from: Data(json.utf8))
    }

    func testAConfigFileEntryDecodesItsColour() throws {
        XCTAssertEqual(try decode(ProjectConfig.self, #"{"working_dir": "/w", "color": "blue"}"#).color, .blue)
        XCTAssertNil(try decode(ProjectConfig.self, #"{"working_dir": "/w", "color": "magenta"}"#).color)
        XCTAssertNil(try decode(ProjectConfig.self, #"{"working_dir": "/w", "color": 7}"#).color)
        XCTAssertNil(try decode(ProjectConfig.self, #"{"working_dir": "/w"}"#).color)
    }

    func testAConfigShowEntryDecodesItsColour() throws {
        let known = #"{"name": "n", "working_dir": "/w", "color": "pink"}"#
        XCTAssertEqual(try decode(ConfigDocProject.self, known).color, .pink)
        XCTAssertNil(try decode(ConfigDocProject.self, #"{"name": "n", "working_dir": "/w", "color": "x"}"#).color)
        XCTAssertNil(try decode(ConfigDocProject.self, #"{"name": "n", "working_dir": "/w"}"#).color)
    }

    func testACreatedOrOpenedProjectDecodesItsColour() throws {
        XCTAssertEqual(try decode(CreatedProject.self, #"{"id": "p", "name": "n", "color": "green"}"#).color, .green)
        XCTAssertNil(try decode(CreatedProject.self, #"{"id": "p", "name": "n", "color": "x"}"#).color)
        XCTAssertNil(try decode(CreatedProject.self, #"{"id": "p", "name": "n"}"#).color)
        XCTAssertEqual(try decode(ProjectEntry.self, #"{"id": "p", "name": "n", "color": "red"}"#).color, .red)
        XCTAssertNil(try decode(ProjectEntry.self, #"{"id": "p", "name": "n"}"#).color)
    }

    /// Written back as nat writes it: the name where there is one, no key
    /// where there is none.
    func testEachEntryRoundTripsItsColour() throws {
        let entry = ProjectConfig(name: "n", workingDir: "/w", color: .purple)
        let written = String(decoding: try JSONEncoder().encode(entry), as: UTF8.self)
        XCTAssertTrue(written.contains(#""color":"purple""#), written)
        XCTAssertEqual(try decode(ProjectConfig.self, written), entry)
        let bare = String(decoding: try JSONEncoder().encode(ProjectConfig(name: "n", workingDir: "/w")), as: UTF8.self)
        XCTAssertFalse(bare.contains("color"), bare)

        let doc = ConfigDocProject(name: "n", workingDir: "/w", color: .yellow)
        let docWritten = String(decoding: try JSONEncoder().encode(doc), as: UTF8.self)
        XCTAssertEqual(try decode(ConfigDocProject.self, docWritten), doc)
        let docBare = String(decoding: try JSONEncoder().encode(ConfigDocProject(name: "n", workingDir: "/w")), as: UTF8.self)
        XCTAssertFalse(docBare.contains("color"), docBare)
    }
}
