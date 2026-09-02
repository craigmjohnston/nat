import XCTest
@testable import NatKit

final class TmuxSessionNameTests: XCTestCase {
    // Mirrors internal/agent/tmux.go's TestSessionName table exactly, so a
    // divergence between the Go and Swift implementations of SessionName
    // fails a test rather than showing up as a session the Go side would
    // never have produced.
    func testSessionNameCases() {
        let cases: [(id: String, want: String)] = [
            ("3b738308-f654-8170-8c99-eccab4463d8f", "nat-b4463d8f"),
            ("3b738308f65481708c99eccab4463d8f", "nat-b4463d8f"),
            ("3B738308-F654-8170-8C99-ECCAB4463D8F", "nat-b4463d8f"),
            ("zz3b-73g83h08f654", "nat-8308f654"),
            ("3b73", "nat-3b73"),
            ("", "nat-"),
        ]
        for testCase in cases {
            XCTAssertEqual(
                TmuxSession.name(forSlicePageID: testCase.id),
                testCase.want,
                "SessionName(\(testCase.id))"
            )
        }
    }

    func testSessionNamePlanSentinel() {
        XCTAssertEqual(TmuxSession.name(forSlicePageID: TmuxSession.planSentinel), TmuxSession.planSession)
        XCTAssertEqual(TmuxSession.planSession, "nat-plan")
    }

    // The bug this replaced on the Go side: page IDs from one Notion
    // workspace share a long leading prefix, so a name taken off the front
    // is the same name for every slice of a project.
    func testSessionNameDistinguishesIDsSharingAPrefix() {
        let first = TmuxSession.name(forSlicePageID: "3b738308-f654-8170-8c99-eccab4463d8f")
        let second = TmuxSession.name(forSlicePageID: "3b738308-f654-812d-ac8d-d4c80dfecb09")
        XCTAssertNotEqual(first, second)
    }
}

final class AttachSpecTests: XCTestCase {
    // wantAttachArgs in internal/agent/tmux_test.go, minus the leading "tmux"
    // argv[0] the Go *exec.Cmd carries and this type does not.
    private let wantArguments = ["-T", "256,RGB,extkeys,focus", "attach-session", "-t", "nat-3b738308"]

    func testArguments() {
        let spec = AttachSpec(session: "nat-3b738308")
        XCTAssertEqual(spec.sessionName, "nat-3b738308")
        XCTAssertEqual(spec.arguments, wantArguments)
        XCTAssertEqual(AttachSpec.executable, "tmux")
    }

    func testViewerFeaturesMatchesGoConstant() {
        // internal/agent/tmux.go: const ViewerFeatures = "256,RGB,extkeys,focus"
        XCTAssertEqual(AttachSpec.viewerFeatures, "256,RGB,extkeys,focus")
    }

    func testEnvironmentDropsTmuxSessionAndPaneVariables() {
        let base = [
            "TMUX": "/private/tmp/tmux-501/default,1234,0",
            "TMUX_PANE": "%7",
            "PATH": "/usr/bin:/bin",
        ]

        let scrubbed = AttachSpec.environment(from: base)

        XCTAssertNil(scrubbed["TMUX"])
        XCTAssertNil(scrubbed["TMUX_PANE"])
        XCTAssertEqual(scrubbed["PATH"], "/usr/bin:/bin")
    }

    func testEnvironmentReplacesExistingTerm() {
        let base = ["TERM": "xterm-ghostty"]

        let scrubbed = AttachSpec.environment(from: base)

        XCTAssertEqual(scrubbed["TERM"], "xterm-256color")
    }

    func testEnvironmentSetsTermWhenAbsent() {
        let scrubbed = AttachSpec.environment(from: [:])

        XCTAssertEqual(scrubbed["TERM"], "xterm-256color")
        XCTAssertEqual(scrubbed.count, 1)
    }

    func testEnvironmentLeavesUnrelatedVariablesAlone() {
        let base = ["HOME": "/Users/craig", "SHELL": "/bin/zsh"]

        let scrubbed = AttachSpec.environment(from: base)

        XCTAssertEqual(scrubbed["HOME"], "/Users/craig")
        XCTAssertEqual(scrubbed["SHELL"], "/bin/zsh")
    }
}
