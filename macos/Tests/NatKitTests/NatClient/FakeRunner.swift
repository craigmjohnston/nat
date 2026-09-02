import Foundation
@testable import NatKit

/// A fake CommandRunning for testing that returns pre-configured fixture data.
final class FakeRunner: CommandRunning, @unchecked Sendable {
    enum Fixture {
        case infoWithAllFields
        case infoMinimal
        case paths
        case commandError
        case nonJSON
        case status
        case sliceShow
        case sliceDiff
        case sliceLaunchSuccess
        case sliceLaunchWithWarning
        case sliceLaunchFailure
        case agentInterruptSuccess
        case agentInterruptNoSession
        case agentSendSuccess
        case agentSendNoSession
        case sliceApproveSuccess
        case sliceApproveFailure
        case prViewFull
        case prViewMinimal
        case prViewFailure
        case prMergeSuccess
        case prMergeFailure
        case prCommentSuccess
        case prCommentFailure
    }

    private var fixture: Fixture
    private var shouldFailWith: Error?

    /// The `standardInput` the most recent call was given, so a test can
    /// check what a command sent over stdin (`agent-send`'s prompt).
    private(set) var lastArguments: [String]?
    private(set) var lastStandardInput: Data?

    init(fixture: Fixture = .infoWithAllFields) {
        self.fixture = fixture
    }

    func run(
        executable: String,
        arguments: [String],
        workingDirectory: String?,
        standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        lastArguments = arguments
        lastStandardInput = standardInput

        if let error = shouldFailWith {
            throw error
        }

        switch fixture {
        case .infoWithAllFields:
            return (fixtureInfoFull.data(using: .utf8)!, Data(), 0)
        case .infoMinimal:
            return (fixtureInfoMinimal.data(using: .utf8)!, Data(), 0)
        case .paths:
            return (fixturePaths.data(using: .utf8)!, Data(), 0)
        case .commandError:
            return (Data(), "Error: project not found".data(using: .utf8)!, 1)
        case .nonJSON:
            return ("This is not JSON".data(using: .utf8)!, Data(), 0)
        case .status:
            return (fixtureStatus.data(using: .utf8)!, Data(), 0)
        case .sliceShow:
            return (fixtureSliceShow.data(using: .utf8)!, Data(), 0)
        case .sliceDiff:
            return (fixtureSliceDiff.data(using: .utf8)!, Data(), 0)
        case .sliceLaunchSuccess:
            return (fixtureSliceLaunchSuccess.data(using: .utf8)!, Data(), 0)
        case .sliceLaunchWithWarning:
            return (fixtureSliceLaunchWithWarning.data(using: .utf8)!, Data(), 0)
        case .sliceLaunchFailure:
            return (Data(), "slice is blocked by incomplete dependencies".data(using: .utf8)!, 1)
        case .agentInterruptSuccess:
            return ("success".data(using: .utf8)!, Data(), 0)
        case .agentInterruptNoSession:
            return (Data(), "no live session for slice-id".data(using: .utf8)!, 1)
        case .agentSendSuccess:
            // agent-send says nothing at all on success.
            return (Data(), Data(), 0)
        case .agentSendNoSession:
            return (Data(), "no live session for slice-id".data(using: .utf8)!, 1)
        case .sliceApproveSuccess:
            return (fixtureSliceApprove.data(using: .utf8)!, Data(), 0)
        case .sliceApproveFailure:
            return (Data(), "\"Write the UI\" is not handed back".data(using: .utf8)!, 1)
        case .prViewFull:
            return (fixturePRViewFull.data(using: .utf8)!, Data(), 0)
        case .prViewMinimal:
            return (fixturePRViewMinimal.data(using: .utf8)!, Data(), 0)
        case .prViewFailure:
            return (Data(), "\"Write the UI\" has no pull request recorded: nothing to view".data(using: .utf8)!, 1)
        case .prMergeSuccess:
            return (fixtureMerged.data(using: .utf8)!, Data(), 0)
        case .prMergeFailure:
            return (Data(), "cannot merge #12 — checks: 1 failing".data(using: .utf8)!, 1)
        case .prCommentSuccess:
            // pr-comment says nothing at all on success, exactly as agent-send.
            return (Data(), Data(), 0)
        case .prCommentFailure:
            return (Data(), "\"Write the UI\" has no pull request recorded: nothing to comment on".data(using: .utf8)!, 1)
        }
    }

    func setFixture(_ fixture: Fixture) {
        self.fixture = fixture
    }

    func setError(_ error: Error) {
        self.shouldFailWith = error
    }
}

// MARK: - Fixtures

let fixtureInfoFull = """
{
  "project": {
    "id": "proj-abc123",
    "name": "Example Project",
    "conventions": "Use TDD for all features"
  },
  "milestones": [
    {
      "id": "m1",
      "name": "Phase 1",
      "order": 1.0,
      "status": "Active"
    },
    {
      "id": "m2",
      "name": "Phase 2",
      "order": 2.0,
      "status": "Queued"
    }
  ],
  "slices": [
    {
      "id": "slice-1",
      "name": "Implement core API",
      "status": "In progress",
      "milestone_id": "m1",
      "assignee": "Alice",
      "pr": "https://github.com/org/repo/pull/42",
      "url": "https://notion.so/slice-1",
      "branch": "feature/api",
      "repo": "/path/to/repo",
      "depends_on": [],
      "blocked": false,
      "handed_back": true,
      "state": "awaiting review"
    },
    {
      "id": "slice-2",
      "name": "Add tests",
      "status": "Todo",
      "milestone_id": "m1",
      "assignee": "",
      "pr": "",
      "url": "https://notion.so/slice-2",
      "depends_on": ["slice-1"],
      "blocked": true,
      "handed_back": false
    },
    {
      "id": "slice-3",
      "name": "Future work",
      "status": "Todo",
      "milestone_id": "m2",
      "assignee": "",
      "pr": "",
      "url": "https://notion.so/slice-3",
      "depends_on": [],
      "blocked": false,
      "handed_back": false
    }
  ]
}
"""

let fixtureInfoMinimal = """
{
  "project": {
    "id": "proj-def456",
    "name": "Minimal Project",
    "conventions": ""
  },
  "milestones": [],
  "slices": []
}
"""

let fixturePaths = """
{
  "config": "/Users/alice/.config/notion-agent-tracker/config.json",
  "log_dir": "/Users/alice/Library/Logs/notion-agent-tracker",
  "nudge": "/Users/alice/Library/Logs/notion-agent-tracker/nudge"
}
"""

let fixtureStatus = """
{
  "agents": [
    {
      "slice_id": "slice-1",
      "session": "nat-abc123def",
      "activity": "working"
    },
    {
      "slice_id": "slice-2",
      "session": "nat-def456ghi",
      "activity": "waiting"
    }
  ]
}
"""

let fixtureSliceShow = """
{
  "id": "slice-1",
  "name": "Implement API",
  "url": "https://notion.so/slice-1",
  "status": "In progress",
  "milestone": "Phase 1",
  "assignee": "Alice",
  "branch": "feature/api",
  "repo": "/path/to/repo",
  "pr": "https://github.com/org/repo/pull/42",
  "depends_on": [],
  "blocked": false,
  "handed_back": true,
  "state": "awaiting_review",
  "brief": "# API Implementation\\nBuild the REST API endpoints"
}
"""

let fixtureSliceApprove = """
{
  "url": "https://github.test/craig/nat/pull/42"
}
"""

// fixturePRViewFull is a full-shaped nat pr-view --json reading: two checks
// (one done, one still going), a review that approved and a second still
// PENDING (submitted_at is Go's zero-time marker, which should decode as no
// time at all and drop it from the conversation), one comment, and the
// additions/deletions/changed_files/commits tally a newer nat carries.
let fixturePRViewFull = """
{
  "number": 42,
  "title": "Add the PR tab",
  "body": "## What\\n\\nAdds the PR tab.",
  "state": "OPEN",
  "is_draft": false,
  "author": "craigmjohnston",
  "base_ref_name": "main",
  "head_ref_name": "slice/add-the-pr-tab",
  "url": "https://github.test/craig/nat/pull/42",
  "checks": [
    {"name": "build", "state": "SUCCESS", "link": "https://github.test/craig/nat/actions/1"},
    {"name": "lint", "state": "IN_PROGRESS", "link": "https://github.test/craig/nat/actions/2"}
  ],
  "reviews": [
    {"author": "reviewer", "state": "APPROVED", "body": "", "submitted_at": "2026-03-01T12:00:00Z"},
    {"author": "reviewer2", "state": "PENDING", "body": "", "submitted_at": "0001-01-01T00:00:00Z"}
  ],
  "comments": [
    {"author": "craigmjohnston", "body": "Ready for a look.", "created_at": "2026-03-01T10:00:00Z",
     "url": "https://github.test/craig/nat/pull/42#issuecomment-1"}
  ],
  "review_decision": "APPROVED",
  "mergeable": "MERGEABLE",
  "merge_state_status": "CLEAN",
  "additions": 120,
  "deletions": 8,
  "changed_files": 5,
  "commits": 3
}
"""

// fixturePRViewMinimal is what an older nat pr-view sends: no checks, reviews
// or comments, and none of the additions/deletions/changed_files/commits
// keys at all — every one of those should decode as nil rather than fail
// the whole read.
let fixturePRViewMinimal = """
{
  "number": 7,
  "title": "Small fix",
  "body": "",
  "state": "OPEN",
  "is_draft": true,
  "author": "",
  "base_ref_name": "main",
  "head_ref_name": "slice/small-fix",
  "url": "https://github.test/craig/nat/pull/7",
  "checks": [],
  "reviews": [],
  "comments": [],
  "review_decision": "",
  "mergeable": "UNKNOWN",
  "merge_state_status": "UNKNOWN"
}
"""

let fixtureMerged = """
{
  "merged": true
}
"""

let fixtureSliceLaunchSuccess = """
{
  "session": "nat-abc123def",
  "workdir": "/path/to/worktree",
  "branch": "slice/test-slice"
}
"""

let fixtureSliceLaunchWithWarning = """
{
  "session": "nat-xyz789uvw",
  "workdir": "/path/to/worktree",
  "branch": "slice/test-slice",
  "warning": "worktrunk not installed; using shared checkout instead"
}
"""

// fixtureSliceDiff is a real-shaped diff (git-generated, then hand-annotated
// with adds/dels/described the way writeDiffJSON in internal/cli/slicediff.go
// would): two files with multiple hunks between them (board.go has two, one
// with a gap between them so the second hunk header becomes a break), an
// add-only new file, a rename with content changes of its own, and a binary
// file git described rather than diffed.
let fixtureSliceDiff = """
{
  "base": "main",
  "branch": "nat/diff-tab-fixture",
  "files": [
    {
      "path": "internal/tui/board.go",
      "old_path": "internal/tui/board.go",
      "adds": 2,
      "dels": 1,
      "described": false,
      "lines": [
        "diff --git a/internal/tui/board.go b/internal/tui/board.go",
        "index abfa861..79d4510 100644",
        "--- a/internal/tui/board.go",
        "+++ b/internal/tui/board.go",
        "@@ -4,6 +4,7 @@ import \\"fmt\\"",
        " ",
        " func Render(width int) string {",
        " \\tfmt.Println(\\"start\\")",
        "+\\tfmt.Println(\\"inserted near top\\")",
        " \\tfmt.Println(\\"line two\\")",
        " \\tfmt.Println(\\"line three\\")",
        " \\tfmt.Println(\\"line four\\")",
        "@@ -12,7 +13,7 @@ func Render(width int) string {",
        " \\tfmt.Println(\\"line seven\\")",
        " \\tfmt.Println(\\"line eight\\")",
        " \\tfmt.Println(\\"line nine\\")",
        "-\\tfmt.Println(\\"line ten\\")",
        "+\\tfmt.Println(\\"line ten (edited)\\")",
        " \\tfmt.Println(\\"end\\")",
        " \\treturn \\"done\\"",
        " }"
      ]
    },
    {
      "path": "internal/tui/newfile.go",
      "old_path": "internal/tui/newfile.go",
      "adds": 6,
      "dels": 0,
      "described": false,
      "lines": [
        "diff --git a/internal/tui/newfile.go b/internal/tui/newfile.go",
        "new file mode 100644",
        "index 0000000..3a1f1d4",
        "--- /dev/null",
        "+++ b/internal/tui/newfile.go",
        "@@ -0,0 +1,6 @@",
        "+package tui",
        "+",
        "+// NewFile is created whole, so its diff is nothing but additions.",
        "+func NewFile() string {",
        "+\\treturn \\"new\\"",
        "+}"
      ]
    },
    {
      "path": "new_name.go",
      "old_path": "old_name.go",
      "adds": 1,
      "dels": 1,
      "described": false,
      "lines": [
        "diff --git a/old_name.go b/new_name.go",
        "similarity index 66%",
        "rename from old_name.go",
        "rename to new_name.go",
        "index 686ae1c..1a1a054 100644",
        "--- a/old_name.go",
        "+++ b/new_name.go",
        "@@ -1,5 +1,5 @@",
        " package tui",
        " ",
        " func Old() string {",
        "-\\treturn \\"old\\"",
        "+\\treturn \\"renamed\\"",
        " }"
      ]
    },
    {
      "path": "docs/shot.png",
      "old_path": "docs/shot.png",
      "adds": 0,
      "dels": 0,
      "described": true,
      "lines": [
        "diff --git a/docs/shot.png b/docs/shot.png",
        "index 5555555..6666666 100644",
        "Binary files a/docs/shot.png and b/docs/shot.png differ"
      ]
    }
  ]
}
"""
