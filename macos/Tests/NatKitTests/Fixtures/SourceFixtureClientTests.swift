import XCTest
@testable import NatKit
@testable import NatFixtures

/// The canned source project and plugins, as the fixture client answers
/// them — and the protocol's defaults for a client with no source at all.
final class SourceFixtureClientTests: XCTestCase {
    func testTheSourceProjectReadsItsTreeWithDoneFoldedUntilExpanded() async throws {
        let client = FixtureNatClient()
        let folded = try await client.info(projectID: Fixtures.sourceProjectID)
        let source = try XCTUnwrap(folded.source)
        XCTAssertEqual(source.tag, "DM")
        XCTAssertEqual(source.groups.map(\.id), ["doing", "ready", "done"])
        XCTAssertEqual(source.group(withID: "done")?.containers, [])
        XCTAssertEqual(source.group(withID: "seg-mine")?.menu.map(\.input), [.text, .choice, .none])
        // The card listed under both segments is one container.
        XCTAssertEqual(source.allContainers.map(\.id), [
            Fixtures.sourceCardID, Fixtures.sourceSecondCardID, Fixtures.sourceMineCardID, Fixtures.sourceBoardCardID,
        ])
        XCTAssertEqual(Set(folded.slices.map(\.milestoneID)), [Fixtures.sourceCardID, Fixtures.sourceSecondCardID])

        let open = try await client.info(projectID: Fixtures.sourceProjectID, refresh: false, expand: ["done"])
        XCTAssertEqual(open.source?.group(withID: "done")?.containers.map(\.id), [Fixtures.sourceDoneCardID])

        // Every other project reads as it always has.
        let plain = try await client.info(projectID: Fixtures.projectID, refresh: true, expand: ["done"])
        XCTAssertEqual(plain, Fixtures.projectInfo)
        XCTAssertNil(plain.source)
    }

    func testContainerShowAnswersTheFirstCardInFullAndAnyOtherPlainly() async throws {
        let client = FixtureNatClient()
        let first = try await client.containerShow(projectID: Fixtures.sourceProjectID, containerID: Fixtures.sourceCardID)
        XCTAssertEqual(first.container, Fixtures.sourceCardDetail)
        XCTAssertEqual(first.container.sections.map(\.kind), [.prose, .comments, .links])
        XCTAssertEqual(first.tasks.map(\.id), [
            Fixtures.sourceTodoTaskID, Fixtures.sourceWorkingTaskID, Fixtures.sourceReviewTaskID,
        ])

        let second = try await client.containerShow(projectID: Fixtures.sourceProjectID, containerID: Fixtures.sourceSecondCardID)
        XCTAssertEqual(second.container.title, "Board mouse support")
        XCTAssertEqual(second.tasks.map(\.id), [Fixtures.sourceSecondCardTaskID])

        let unknown = try await client.containerShow(projectID: Fixtures.sourceProjectID, containerID: "nope")
        XCTAssertEqual(unknown.container.title, "nope")
        XCTAssertEqual(unknown.tasks, [])
    }

    func testEveryTaskShowsItsContainer() async throws {
        let client = FixtureNatClient()
        for task in Fixtures.sourceTasks {
            let detail = try await client.sliceShow(projectID: Fixtures.sourceProjectID, sliceRef: task.id)
            XCTAssertEqual(detail.container?.id, task.milestoneID, task.name)
        }
        let review = try await client.sliceShow(projectID: Fixtures.sourceProjectID, sliceRef: Fixtures.sourceReviewTaskID)
        XCTAssertEqual(review.container?.taskNote, Fixtures.sourceCardDetail.taskNote)
        XCTAssertEqual(review.state, "awaiting review")
        XCTAssertNotNil(review.pr)
    }

    func testSourceListAndActions() async throws {
        let client = FixtureNatClient()
        let plugins = try await client.sourceList()
        XCTAssertEqual(plugins.map(\.name), ["demo", "shortcut"])
        XCTAssertNotNil(plugins[1].error)

        let header = try await client.sourceAction(
            projectID: Fixtures.sourceProjectID, action: "refresh", group: nil, container: nil, input: nil)
        XCTAssertEqual(header.message, "Ran refresh.")
        _ = try await client.sourceAction(
            projectID: Fixtures.sourceProjectID, action: "segment-owner", group: "seg-mine", container: nil, input: "me")
        _ = try await client.sourceAction(
            projectID: Fixtures.sourceProjectID, action: "comment", group: nil, container: "4821", input: "Hi")
        XCTAssertEqual(client.writes, [
            "source-action refresh",
            "source-action segment-owner --group seg-mine",
            "source-action comment --container 4821",
        ])
    }

    func testARefusingClientRefusesTheSourceReads() async {
        let client = FixtureNatClient(behaviour: .refusing("down"))
        do {
            _ = try await client.sourceList()
            XCTFail("expected a refusal")
        } catch {}
        do {
            _ = try await client.containerShow(projectID: Fixtures.sourceProjectID, containerID: Fixtures.sourceCardID)
            XCTFail("expected a refusal")
        } catch {}
    }

    func testTheFailedSourceFixtureHoldsOnlyUnlisted() {
        XCTAssertNotNil(Fixtures.sourceInfoFailed.error)
        XCTAssertEqual(Fixtures.sourceInfoFailed.groups.map(\.id), [SourceGroup.unlistedID])
        XCTAssertEqual(Fixtures.sourceInfoFailed.allContainers.count, 2)
    }

    func testAClientWithNoSourceRefusesTheSourceCommands() async {
        let client = MockActivityClient(response: .agents([]))
        do {
            _ = try await client.sourceList()
            XCTFail("expected a refusal")
        } catch NatError.commandFailed(let message) {
            XCTAssertTrue(message.hasPrefix("source-list"))
        } catch { XCTFail("\(error)") }
        do {
            _ = try await client.containerShow(projectID: "p", containerID: "c")
            XCTFail("expected a refusal")
        } catch NatError.commandFailed(let message) {
            XCTAssertTrue(message.hasPrefix("container-show"))
        } catch { XCTFail("\(error)") }
        do {
            _ = try await client.sourceAction(projectID: "p", action: "a", group: nil, container: nil, input: nil)
            XCTFail("expected a refusal")
        } catch NatError.commandFailed(let message) {
            XCTAssertTrue(message.hasPrefix("source-action"))
        } catch { XCTFail("\(error)") }
        // A plan read with groups expanded is the plain read, which this
        // client refuses on its own terms.
        do {
            _ = try await client.info(projectID: "p", refresh: false, expand: ["done"])
            XCTFail("expected the plain read's refusal")
        } catch {
            XCTAssertEqual((error as NSError).domain, "test")
        }
    }
}
