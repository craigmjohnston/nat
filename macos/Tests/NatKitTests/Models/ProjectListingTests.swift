import XCTest
@testable import NatKit

final class ProjectListingTests: XCTestCase {
    private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
        try JSONDecoder().decode(type, from: json.data(using: .utf8)!)
    }

    func testListingDecodesBothHalves() throws {
        let listing = try decode(ProjectListing.self, """
        {
          "projects": [
            {"id": "p1", "name": "Tracked", "configured": true, "working_dir": "/src/p1"},
            {"id": "p2", "name": "Untracked", "configured": false}
          ]
        }
        """)

        XCTAssertNil(listing.note)
        XCTAssertEqual(listing.projects.map(\.id), ["p1", "p2"])
        XCTAssertEqual(listing.projects[0].workingDir, "/src/p1")
        XCTAssertFalse(listing.projects[1].configured)
        XCTAssertEqual(listing.projects[1].workingDir, "")
    }

    func testListingDecodesANoteWithNoProjects() throws {
        let listing = try decode(ProjectListing.self, """
        {"note": "the projects database could not be read"}
        """)

        XCTAssertTrue(listing.projects.isEmpty)
        XCTAssertEqual(listing.note, "the projects database could not be read")
    }

    func testListingRoundTrips() throws {
        let listing = ProjectListing(
            projects: [ProjectListingEntry(id: "p1", name: "Tracked", configured: true, workingDir: "/src")],
            note: "a note"
        )

        let data = try JSONEncoder().encode(listing)
        XCTAssertEqual(try JSONDecoder().decode(ProjectListing.self, from: data), listing)
    }

    func testEntryIsIdentifiedByItsPageID() {
        let entry = ProjectListingEntry(id: "p1", name: "Tracked", configured: false)
        XCTAssertEqual(entry.id, "p1")
        XCTAssertNotEqual(entry, ProjectListingEntry(id: "p2", name: "Tracked", configured: false))
    }

    func testProjectEntryDecodesAndTakesEmptyForAbsentFields() throws {
        let full = try decode(ProjectEntry.self, """
        {"id": "p2", "name": "Untracked", "slices_ds_id": "ds-2", "working_dir": "/src/p2"}
        """)
        XCTAssertEqual(full, ProjectEntry(id: "p2", name: "Untracked", slicesDSID: "ds-2", workingDir: "/src/p2"))

        let bare = try decode(ProjectEntry.self, """
        {"id": "p2", "name": "Untracked"}
        """)
        XCTAssertEqual(bare.slicesDSID, "")
        XCTAssertEqual(bare.workingDir, "")
    }

    func testCreatedProjectDecodesTheWholeReading() throws {
        let project = try decode(CreatedProject.self, """
        {
          "id": "p9",
          "name": "Fresh",
          "url": "https://notion.so/p9",
          "slices_db_id": "db-9",
          "slices_ds_id": "ds-9",
          "working_dir": "/src/fresh",
          "assignee": true
        }
        """)

        XCTAssertEqual(project, CreatedProject(
            id: "p9", name: "Fresh", url: "https://notion.so/p9",
            slicesDBID: "db-9", slicesDSID: "ds-9",
            workingDir: "/src/fresh", assignee: true
        ))
    }

    func testCreatedProjectTakesDefaultsForAbsentFields() throws {
        let project = try decode(CreatedProject.self, """
        {"id": "p9", "name": "Fresh"}
        """)

        XCTAssertEqual(project.url, "")
        XCTAssertEqual(project.slicesDBID, "")
        XCTAssertEqual(project.slicesDSID, "")
        XCTAssertEqual(project.workingDir, "")
        XCTAssertFalse(project.assignee)
    }

    func testAnEntryWithoutAnIDIsNotAProject() {
        XCTAssertThrowsError(try decode(ProjectListingEntry.self, #"{"name": "Nameless"}"#))
        XCTAssertThrowsError(try decode(ProjectEntry.self, #"{"name": "Nameless"}"#))
        XCTAssertThrowsError(try decode(CreatedProject.self, #"{"name": "Nameless"}"#))
    }
}
