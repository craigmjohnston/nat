import XCTest
@testable import NatKit
@testable import NatFixtures

/// The filter editor's choices: opened on what is saved, changed field by
/// field, and handed back as the action's JSON input.
final class SourceFilterDraftTests: XCTestCase {
    private var action: SourceAction { Fixtures.sourceFilterAction(["team": ["board"]], section: ["project": ["30"], "labels": ["diff", "x"]]) }
    private func field(_ id: String, in action: SourceAction) -> SourceFilterField { action.fields.first { $0.id == id }! }

    func testItOpensOnWhatIsSaved() {
        let draft = SourceFilterDraft(action: action)
        XCTAssertEqual(draft.choice(field("team", in: action)), "board")
        XCTAssertNil(draft.choice(field("epic", in: action)))
        XCTAssertEqual(draft.summary(field("team", in: action)), "Board")
        XCTAssertFalse(draft.differs(from: action))
        XCTAssertEqual(draft.input(for: action), #"{"epic":[],"labels":[],"project":[],"team":["board"]}"#)
    }

    func testAnyNamesWhatItFallsThroughTo() {
        XCTAssertEqual(SourceFilterDraft.anyLabel(field("project", in: action)), "Any (section\u{2019}s: Mobile App)")
        XCTAssertEqual(SourceFilterDraft.anyLabel(field("labels", in: action)), "Any (section\u{2019}s: diff, x)")
        XCTAssertEqual(SourceFilterDraft.anyLabel(field("team", in: action)), "Any")
        XCTAssertEqual(SourceFilterDraft.anyLabel(SourceFilterField(id: "t", label: "T", inherited: "")), "Any")
        let draft = SourceFilterDraft(action: action)
        XCTAssertEqual(draft.summary(field("project", in: action)), "Any (section\u{2019}s: Mobile App)")
    }

    func testChoosingAndTogglingChangeTheAnswer() {
        var draft = SourceFilterDraft(action: action)
        draft.choose(nil, in: field("team", in: action))
        draft.choose("30", in: field("project", in: action))
        draft.toggle("diff", in: field("labels", in: action))
        draft.toggle("agent", in: field("labels", in: action))
        draft.toggle("diff", in: field("labels", in: action))
        draft.toggle("gone", in: field("labels", in: action))
        XCTAssertTrue(draft.isChosen("agent", in: field("labels", in: action)))
        XCTAssertFalse(draft.isChosen("diff", in: field("labels", in: action)))
        XCTAssertEqual(draft.summary(field("labels", in: action)), "agent, gone", "an id offered nowhere names itself")
        XCTAssertTrue(draft.differs(from: action))
        XCTAssertEqual(draft.input(for: action), #"{"epic":[],"labels":["agent","gone"],"project":["30"],"team":[]}"#)
    }

    func testAFieldTheDraftHasNotSeenReadsAsSaved() {
        let draft = SourceFilterDraft(action: SourceAction(id: "f", label: "F", input: .filter))
        let late = SourceFilterField(id: "late", label: "Late", value: ["a"])
        XCTAssertEqual(draft.selected(late), ["a"])
    }

    func testLoading() {
        XCTAssertFalse(SourceFilterDraft.isLoading(action))
        XCTAssertTrue(SourceFilterDraft.isLoading(Fixtures.sourceFilterAction([:], epicsLoading: true)))
    }
}
