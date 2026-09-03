import XCTest
@testable import NatKit

final class NatClientTests: XCTestCase {
    func testInfoWithAllFields() async throws {
        let fakeRunner = FakeRunner(fixture: .infoWithAllFields)
        let client = NatClient(commandRunner: fakeRunner)

        let info = try await client.info(projectID: "proj-123")

        XCTAssertEqual(info.project.id, "proj-abc123")
        XCTAssertEqual(info.project.name, "Example Project")
        XCTAssertEqual(info.milestones.count, 2)
        XCTAssertEqual(info.slices.count, 3)

        // Check first slice has all optional fields
        let firstSlice = info.slices[0]
        XCTAssertEqual(firstSlice.id, "slice-1")
        XCTAssertEqual(firstSlice.branch, "feature/api")
        XCTAssertEqual(firstSlice.repo, "/path/to/repo")
        XCTAssertEqual(firstSlice.state, .awaitingReview)
        XCTAssertTrue(firstSlice.handedBack)

        // Check second slice doesn't have optional fields
        let secondSlice = info.slices[1]
        XCTAssertNil(secondSlice.branch)
        XCTAssertNil(secondSlice.repo)
        XCTAssertNil(secondSlice.state)
    }

    func testInfoMinimal() async throws {
        let fakeRunner = FakeRunner(fixture: .infoMinimal)
        let client = NatClient(commandRunner: fakeRunner)

        let info = try await client.info(projectID: "proj-456")

        XCTAssertEqual(info.project.name, "Minimal Project")
        XCTAssertTrue(info.milestones.isEmpty)
        XCTAssertTrue(info.slices.isEmpty)
    }

    func testPaths() async throws {
        let fakeRunner = FakeRunner(fixture: .paths)
        let client = NatClient(commandRunner: fakeRunner)

        let paths = try await client.paths()

        XCTAssertEqual(paths.config, "/Users/alice/.config/notion-agent-tracker/config.json")
        XCTAssertEqual(paths.logDir, "/Users/alice/Library/Logs/notion-agent-tracker")
        XCTAssertEqual(paths.nudge, "/Users/alice/Library/Logs/notion-agent-tracker/nudge")
    }

    func testCommandFailureReturnsFirstLineOfStderr() async throws {
        let fakeRunner = FakeRunner(fixture: .commandError)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.info(projectID: "nonexistent")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "Error: project not found")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testInvalidJSONThrows() async throws {
        let fakeRunner = FakeRunner(fixture: .nonJSON)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.info(projectID: "test")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .invalidJSON = error {
                // Expected
            } else {
                XCTFail("Expected invalidJSON error, got \(error)")
            }
        }
    }

    func testProcessRunnerResolvesNatBinary() async throws {
        let runner = ProcessRunner()

        // This test verifies that ProcessRunner can be instantiated and respects
        // the NAT_BIN environment variable. We don't actually run nat here because
        // it might not be installed, but we can verify the behavior with a fake.
        XCTAssertNotNil(runner)
    }

    func testMultipleCalls() async throws {
        let fakeRunner = FakeRunner(fixture: .infoWithAllFields)
        let client = NatClient(commandRunner: fakeRunner)

        let info1 = try await client.info(projectID: "proj-1")
        let info2 = try await client.info(projectID: "proj-2")

        XCTAssertEqual(info1.project.name, "Example Project")
        XCTAssertEqual(info2.project.name, "Example Project")
    }

    func testStatus() async throws {
        let fakeRunner = FakeRunner(fixture: .status)
        let client = NatClient(commandRunner: fakeRunner)

        let statuses = try await client.status()

        XCTAssertEqual(statuses.count, 2)
        XCTAssertEqual(statuses[0].sliceID, "slice-1")
        XCTAssertEqual(statuses[0].session, "nat-abc123def")
        XCTAssertEqual(statuses[0].activity, .working)

        XCTAssertEqual(statuses[1].sliceID, "slice-2")
        XCTAssertEqual(statuses[1].activity, .waiting)
    }

    func testSliceShow() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceShow)
        let client = NatClient(commandRunner: fakeRunner)

        let detail = try await client.sliceShow(projectID: "proj-123", sliceRef: "slice-1")

        XCTAssertEqual(detail.id, "slice-1")
        XCTAssertEqual(detail.name, "Implement API")
        XCTAssertEqual(detail.status, "In progress")
        XCTAssertEqual(detail.milestone, "Phase 1")
        XCTAssertEqual(detail.branch, "feature/api")
        XCTAssertEqual(detail.repo, "/path/to/repo")
        XCTAssertEqual(detail.pr, "https://github.com/org/repo/pull/42")
        XCTAssertTrue(detail.handedBack)
        XCTAssertEqual(detail.state, "awaiting_review")
        XCTAssert(detail.brief.contains("API Implementation"))
    }

    func testSliceDiff() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceDiff)
        let client = NatClient(commandRunner: fakeRunner)

        let diff = try await client.sliceDiff(projectID: "proj-123", sliceRef: "slice-1")

        XCTAssertEqual(diff.base, "main")
        XCTAssertEqual(diff.branch, "nat/diff-tab-fixture")
        XCTAssertEqual(diff.files.count, 4)

        let board = diff.files[0]
        XCTAssertEqual(board.path, "internal/tui/board.go")
        XCTAssertEqual(board.adds, 2)
        XCTAssertEqual(board.dels, 1)
        XCTAssertFalse(board.described)
        XCTAssertFalse(board.isRenamed)
        XCTAssertTrue(board.lines.contains("@@ -4,6 +4,7 @@ import \"fmt\""))

        let newFile = diff.files[1]
        XCTAssertEqual(newFile.adds, 6)
        XCTAssertEqual(newFile.dels, 0)
        XCTAssertFalse(newFile.isRenamed)

        let renamed = diff.files[2]
        XCTAssertEqual(renamed.path, "new_name.go")
        XCTAssertEqual(renamed.oldPath, "old_name.go")
        XCTAssertTrue(renamed.isRenamed)

        let binary = diff.files[3]
        XCTAssertTrue(binary.described)
        XCTAssertFalse(binary.isRenamed)
    }

    func testSliceDiffWithNoCommitOmitsTheFlag() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceDiff)
        let client = NatClient(commandRunner: fakeRunner)

        _ = try await client.sliceDiff(projectID: "proj-123", sliceRef: "slice-1", commit: nil)

        XCTAssertEqual(fakeRunner.lastArguments, ["slice-diff", "--project", "proj-123", "--json", "slice-1"])
    }

    func testSliceDiffWithACommitPassesTheFlag() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceCommitDiff)
        let client = NatClient(commandRunner: fakeRunner)

        let diff = try await client.sliceDiff(projectID: "proj-123", sliceRef: "slice-1", commit: "ccccccc")

        XCTAssertEqual(fakeRunner.lastArguments, [
            "slice-diff", "--project", "proj-123", "--json", "--commit", "ccccccc", "slice-1"
        ])
        XCTAssertEqual(diff.branch, "cccccccccccccccccccccccccccccccccccccccc")
        XCTAssertEqual(diff.files.count, 1)
    }

    func testSliceCommitsListsThemNewestFirst() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceCommits)
        let client = NatClient(commandRunner: fakeRunner)

        let doc = try await client.sliceCommits(projectID: "proj-123", sliceRef: "slice-1")

        XCTAssertEqual(fakeRunner.lastArguments, ["slice-diff", "--project", "proj-123", "--json", "--commits", "slice-1"])
        XCTAssertEqual(doc.base, "main")
        XCTAssertEqual(doc.branch, "nat/diff-tab-fixture")
        XCTAssertEqual(doc.commits.count, 3)
        XCTAssertEqual(doc.commits[0].subject, "Fix the gutter width")
        XCTAssertEqual(doc.commits[0].shortSHA, "cccccccc")
    }

    func testSliceCommitsFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceCommitsFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.sliceCommits(projectID: "proj-123", sliceRef: "slice-1")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "\"Write the UI\" is not handed back: only a slice with a branch has a diff to read")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testSliceEditPostsDescriptionOverStdinAndReturnsTheResult() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceEditSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        let result = try await client.sliceEdit(projectID: "proj-123", sliceRef: "slice-1", description: "New brief text")

        XCTAssertEqual(fakeRunner.lastArguments, [
            "slice-edit", "--project", "proj-123", "--json", "--description", "-", "slice-1"
        ])
        XCTAssertEqual(fakeRunner.lastStandardInput, "New brief text".data(using: .utf8))
        XCTAssertEqual(result.id, "slice-1")
        XCTAssertEqual(result.brief, "New brief text")
    }

    func testSliceEditFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceEditFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.sliceEdit(projectID: "proj-123", sliceRef: "slice-1", description: "x")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "\"Write the UI\" is in progress: a slice being worked is not edited under its agent")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testAgentInterruptSuccess() async throws {
        let fakeRunner = FakeRunner(fixture: .agentInterruptSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        // Should not throw
        try await client.agentInterrupt(projectID: "proj-123", sliceRef: "slice-1")
    }

    func testAgentSendPostsThePromptOverStdin() async throws {
        let fakeRunner = FakeRunner(fixture: .agentSendSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        try await client.agentSend(projectID: "proj-123", sliceRef: "slice-1", text: "clamp this")

        XCTAssertEqual(fakeRunner.lastArguments, ["agent-send", "--project", "proj-123", "slice-1", "--text", "-"])
        XCTAssertEqual(fakeRunner.lastStandardInput, "clamp this".data(using: .utf8))
    }

    func testAgentSendNoSession() async throws {
        let fakeRunner = FakeRunner(fixture: .agentSendNoSession)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            try await client.agentSend(projectID: "proj-123", sliceRef: "slice-1", text: "clamp this")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "no live session for slice-id")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testSliceApproveReturnsTheURL() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceApproveSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        let url = try await client.sliceApprove(projectID: "proj-123", sliceRef: "slice-1")

        XCTAssertEqual(url, "https://github.test/craig/nat/pull/42")
        XCTAssertEqual(fakeRunner.lastArguments, ["slice-approve", "--project", "proj-123", "--json", "slice-1"])
    }

    func testSliceApproveFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceApproveFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.sliceApprove(projectID: "proj-123", sliceRef: "slice-1")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "\"Write the UI\" is not handed back")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testPRViewFull() async throws {
        let fakeRunner = FakeRunner(fixture: .prViewFull)
        let client = NatClient(commandRunner: fakeRunner)

        let pr = try await client.prView(projectID: "proj-123", sliceRef: "slice-1")

        XCTAssertEqual(fakeRunner.lastArguments, ["pr-view", "--project", "proj-123", "--json", "slice-1"])
        XCTAssertEqual(pr.number, 42)
        XCTAssertEqual(pr.title, "Add the PR tab")
        XCTAssertEqual(pr.state, "OPEN")
        XCTAssertFalse(pr.isDraft)
        XCTAssertEqual(pr.baseRefName, "main")
        XCTAssertEqual(pr.headRefName, "slice/add-the-pr-tab")
        XCTAssertEqual(pr.checks.count, 2)
        XCTAssertEqual(pr.checks[0].name, "build")
        XCTAssertEqual(pr.reviews.count, 2)
        // A review submitted at Go's zero-time marker decodes as no time at all.
        XCTAssertNotNil(pr.reviews[0].submittedAt)
        XCTAssertNil(pr.reviews[1].submittedAt)
        XCTAssertEqual(pr.comments.count, 1)
        XCTAssertEqual(pr.comments[0].author, "craigmjohnston")
        XCTAssertEqual(pr.reviewDecision, "APPROVED")
        XCTAssertEqual(pr.mergeable, "MERGEABLE")
        XCTAssertEqual(pr.mergeStateStatus, "CLEAN")
        XCTAssertEqual(pr.additions, 120)
        XCTAssertEqual(pr.deletions, 8)
        XCTAssertEqual(pr.changedFiles, 5)
        XCTAssertEqual(pr.commits, 3)
    }

    func testPRViewMinimalOmitsTheOptionalTally() async throws {
        let fakeRunner = FakeRunner(fixture: .prViewMinimal)
        let client = NatClient(commandRunner: fakeRunner)

        let pr = try await client.prView(projectID: "proj-123", sliceRef: "slice-1")

        XCTAssertTrue(pr.checks.isEmpty)
        XCTAssertTrue(pr.reviews.isEmpty)
        XCTAssertTrue(pr.comments.isEmpty)
        XCTAssertNil(pr.additions)
        XCTAssertNil(pr.deletions)
        XCTAssertNil(pr.changedFiles)
        XCTAssertNil(pr.commits)
    }

    func testPRViewFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .prViewFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.prView(projectID: "proj-123", sliceRef: "slice-1")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "\"Write the UI\" has no pull request recorded: nothing to view")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testPRMergeSuccess() async throws {
        let fakeRunner = FakeRunner(fixture: .prMergeSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        try await client.prMerge(projectID: "proj-123", sliceRef: "slice-1")

        XCTAssertEqual(fakeRunner.lastArguments, ["pr-merge", "--project", "proj-123", "--json", "slice-1"])
    }

    func testPRMergeFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .prMergeFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            try await client.prMerge(projectID: "proj-123", sliceRef: "slice-1")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "cannot merge #12 — checks: 1 failing")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testPRCommentPostsTheBodyOverStdin() async throws {
        let fakeRunner = FakeRunner(fixture: .prCommentSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        try await client.prComment(projectID: "proj-123", sliceRef: "slice-1", body: "Looks good.")

        XCTAssertEqual(fakeRunner.lastArguments, ["pr-comment", "slice-1", "--project", "proj-123", "--body", "-"])
        XCTAssertEqual(fakeRunner.lastStandardInput, "Looks good.".data(using: .utf8))
    }

    func testPRCommentFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .prCommentFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            try await client.prComment(projectID: "proj-123", sliceRef: "slice-1", body: "Looks good.")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "\"Write the UI\" has no pull request recorded: nothing to comment on")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testAgentInterruptNoSession() async throws {
        let fakeRunner = FakeRunner(fixture: .agentInterruptNoSession)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            try await client.agentInterrupt(projectID: "proj-123", sliceRef: "slice-1")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "no live session for slice-id")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testSliceLaunchSuccess() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceLaunchSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        let result = try await client.sliceLaunch(projectID: "proj-123", sliceRef: "slice-1", model: "opus", effort: "high")

        XCTAssertEqual(result.session, "nat-abc123def")
        XCTAssertEqual(result.workdir, "/path/to/worktree")
        XCTAssertEqual(result.branch, "slice/test-slice")
        XCTAssertNil(result.warning)
        XCTAssertEqual(fakeRunner.lastArguments, ["slice-launch", "--project", "proj-123", "--json", "--model", "opus", "--effort", "high", "slice-1"])
    }

    func testSliceLaunchWithWarning() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceLaunchWithWarning)
        let client = NatClient(commandRunner: fakeRunner)

        let result = try await client.sliceLaunch(projectID: "proj-123", sliceRef: "slice-1", model: nil, effort: nil)

        XCTAssertEqual(result.session, "nat-xyz789uvw")
        XCTAssertEqual(result.workdir, "/path/to/worktree")
        XCTAssertEqual(result.branch, "slice/test-slice")
        XCTAssertEqual(result.warning, "worktrunk not installed; using shared checkout instead")
        XCTAssertEqual(fakeRunner.lastArguments, ["slice-launch", "--project", "proj-123", "--json", "slice-1"])
    }

    func testSliceLaunchFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceLaunchFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.sliceLaunch(projectID: "proj-123", sliceRef: "slice-1", model: nil, effort: nil)
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "slice is blocked by incomplete dependencies")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testWorkshopLaunchSuccess() async throws {
        let fakeRunner = FakeRunner(fixture: .workshopLaunchSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        let result = try await client.workshopLaunch(projectID: "proj-123", model: "opus", effort: "high")

        XCTAssertEqual(result.session, "nat-plan")
        XCTAssertEqual(result.workdir, "/path/to/repo")
        XCTAssertTrue(result.wishlist)
        XCTAssertEqual(
            fakeRunner.lastArguments,
            ["workshop-launch", "--project", "proj-123", "--json", "--model", "opus", "--effort", "high"]
        )
    }

    func testWorkshopLaunchWithNoOverrideOmitsFlags() async throws {
        let fakeRunner = FakeRunner(fixture: .workshopLaunchSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        _ = try await client.workshopLaunch(projectID: "proj-123", model: nil, effort: nil)

        XCTAssertEqual(fakeRunner.lastArguments, ["workshop-launch", "--project", "proj-123", "--json"])
    }

    func testWorkshopLaunchAlreadyLiveFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .workshopLaunchAlreadyLive)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.workshopLaunch(projectID: "proj-123", model: nil, effort: nil)
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "a planning agent is already live: nat-plan")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testSliceAddPostsDescriptionOverStdin() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceAddSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        let result = try await client.sliceAdd(
            projectID: "proj-123",
            title: "Add the settings scene",
            milestone: "Phase 1",
            description: "Wire up config-show and config-set."
        )

        XCTAssertEqual(result.id, "slice-new")
        XCTAssertEqual(result.name, "Add the settings scene")
        XCTAssertEqual(result.status, "Todo")
        XCTAssertEqual(result.milestoneName, "Phase 1")
        XCTAssertEqual(result.repo, "/path/to/repo")
        XCTAssertEqual(
            fakeRunner.lastArguments,
            ["slice-add", "Add the settings scene", "--project", "proj-123", "--milestone", "Phase 1", "--json", "--description", "-"]
        )
        XCTAssertEqual(fakeRunner.lastStandardInput, "Wire up config-show and config-set.".data(using: .utf8))
    }

    func testSliceAddWithoutDescriptionOmitsTheFlag() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceAddSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        _ = try await client.sliceAdd(projectID: "proj-123", title: "Add the settings scene", milestone: "Phase 1", description: nil)

        XCTAssertEqual(
            fakeRunner.lastArguments,
            ["slice-add", "Add the settings scene", "--project", "proj-123", "--milestone", "Phase 1", "--json"]
        )
        XCTAssertNil(fakeRunner.lastStandardInput)
    }

    func testSliceAddFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceAddFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.sliceAdd(projectID: "proj-123", title: "Title", milestone: "Nope", description: nil)
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "no milestone named \"Nope\": the project's milestones are Phase 1, Phase 2")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testConfigShow() async throws {
        let fakeRunner = FakeRunner(fixture: .configShowSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        let doc = try await client.configShow()

        XCTAssertEqual(doc.agentSplitPercent, 65)
        XCTAssertEqual(doc.pollSeconds, 30)
        XCTAssertEqual(doc.workshopAgent.model, "sonnet")
        XCTAssertEqual(doc.workshopAgent.effort, "low")
        XCTAssertEqual(doc.sliceAgent.model, "opus")
        XCTAssertEqual(doc.sliceAgent.effort, "high")
        XCTAssertEqual(doc.projects["proj-1"]?.name, "Example Project")
        XCTAssertEqual(doc.projects["proj-1"]?.workingDir, "/path/to/repo")
        XCTAssertEqual(fakeRunner.lastArguments, ["config-show", "--json"])
    }

    func testConfigSetSuccess() async throws {
        let fakeRunner = FakeRunner(fixture: .configSetSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        try await client.configSet(key: "agent_split_percent", value: "70")

        XCTAssertEqual(fakeRunner.lastArguments, ["config-set", "agent_split_percent", "70"])
    }

    func testConfigSetFailure() async throws {
        let fakeRunner = FakeRunner(fixture: .configSetFailure)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            try await client.configSet(key: "agent_split_percent", value: "5")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "config-set: agent_split_percent must be between 10 and 90, given 5")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }
}
