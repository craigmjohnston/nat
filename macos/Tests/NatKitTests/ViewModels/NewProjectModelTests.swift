import XCTest
@testable import NatKit

final class NewProjectModelTests: XCTestCase {
    private let listing = ProjectListing(projects: [
        ProjectListingEntry(id: "p1", name: "Tracked", configured: true, workingDir: "/src/p1"),
        ProjectListingEntry(id: "p2", name: "Untracked", configured: false),
        ProjectListingEntry(id: "p3", name: "Also Untracked", configured: false),
    ])

    func testOpenableIsTheUnconfiguredHalfInTheListingsOwnOrder() {
        XCTAssertEqual(NewProjectModel.openable(listing).map(\.id), ["p2", "p3"])
    }

    func testOpenableIsEmptyWhenEveryProjectIsAlreadyTracked() {
        let all = ProjectListing(projects: [
            ProjectListingEntry(id: "p1", name: "Tracked", configured: true),
        ])
        XCTAssertTrue(NewProjectModel.openable(all).isEmpty)
    }

    func testCanOpenOnlyAPickTheListingStillOffers() {
        XCTAssertTrue(NewProjectModel.canOpen(selection: "p2", in: listing))
        // Nothing picked.
        XCTAssertFalse(NewProjectModel.canOpen(selection: "", in: listing))
        // The configured half is not the picker's to offer.
        XCTAssertFalse(NewProjectModel.canOpen(selection: "p1", in: listing))
        // A pick the listing no longer holds.
        XCTAssertFalse(NewProjectModel.canOpen(selection: "p9", in: listing))
    }

    func testCanCreateWantsBothANameAndADirectory() {
        XCTAssertTrue(NewProjectModel.canCreate(name: "nat", directory: "/src/nat"))
        XCTAssertFalse(NewProjectModel.canCreate(name: "", directory: "/src/nat"))
        XCTAssertFalse(NewProjectModel.canCreate(name: "nat", directory: ""))
        // Whitespace is not an answer to either.
        XCTAssertFalse(NewProjectModel.canCreate(name: "   ", directory: "/src/nat"))
        XCTAssertFalse(NewProjectModel.canCreate(name: "nat", directory: "  \n "))
    }

    func testMessageIsNatsOwnWordsForACommandThatRefused() {
        let message = NewProjectModel.message(from: NatError.commandFailed("no such page"))
        XCTAssertEqual(message, "no such page")
    }

    func testMessageFallsBackToTheErrorsOwnDescription() {
        let other = NSError(domain: "test", code: 7, userInfo: [NSLocalizedDescriptionKey: "something else"])
        XCTAssertEqual(NewProjectModel.message(from: other), "something else")
        // A NatError that is not a refusal takes the same fallback.
        XCTAssertEqual(
            NewProjectModel.message(from: NatError.missingOutput),
            NatError.missingOutput.localizedDescription
        )
    }
}

final class EmptyProjectNoteTests: XCTestCase {
    func testSubtitleNamesSettingsOnlyWhileTheWorkingDirectoryIsUnset() {
        let unset = EmptyProjectNote.subtitle(needsWorkingDir: true)
        XCTAssertTrue(unset.contains("Settings"))

        let set = EmptyProjectNote.subtitle(needsWorkingDir: false)
        XCTAssertFalse(set.contains("Settings"))
        XCTAssertNotEqual(set, unset)
    }

    func testTitleSaysThePlanIsEmptyRatherThanUnread() {
        XCTAssertEqual(EmptyProjectNote.title, "No slices yet")
    }
}
