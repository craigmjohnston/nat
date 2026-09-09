import XCTest
@testable import NatKit

final class AttachCommandScriptTests: XCTestCase {
    func testScriptGenerationIncludesCorrectSession() {
        let spec = AttachSpec(session: "nat-abc123de")
        let script = AttachCommandScript.generate(from: spec)

        XCTAssertTrue(script.contains("nat-abc123de"))
        XCTAssertTrue(script.contains("#!/bin/bash"))
    }

    func testScriptIncludesProperEnvironmentSetup() {
        let spec = AttachSpec(session: "nat-test")
        let script = AttachCommandScript.generate(from: spec)

        // Should unset TMUX and TMUX_PANE
        XCTAssertTrue(script.contains("unset TMUX"))
        XCTAssertTrue(script.contains("unset TMUX_PANE"))

        // Should set TERM to the viewer term
        XCTAssertTrue(script.contains("export TERM=\"xterm-256color\""))
    }

    func testScriptIncludesTmuxFeatures() {
        let spec = AttachSpec(session: "nat-test")
        let script = AttachCommandScript.generate(from: spec)

        // Should include the viewer features flags
        XCTAssertTrue(script.contains("256,RGB,extkeys,focus"))
    }

    func testScriptIsExecutable() {
        let spec = AttachSpec(session: "nat-test")
        let script = AttachCommandScript.generate(from: spec)

        // Must start with shebang
        XCTAssertTrue(script.starts(with: "#!/bin/bash"))
    }

    func testScriptIncludesInvocation() {
        let spec = AttachSpec(session: "nat-testfail")
        let script = AttachCommandScript.generate(from: spec)

        // Should include the tmux attach invocation
        XCTAssertTrue(script.contains("tmux"))
        XCTAssertTrue(script.contains("attach-session"))
        XCTAssertTrue(script.contains("nat-testfail"))
    }

    func testScriptIncludesWaitForUserInput() {
        let spec = AttachSpec(session: "nat-test")
        let script = AttachCommandScript.generate(from: spec)

        // Should include a prompt and read to keep window open
        XCTAssertTrue(script.contains("read"))
        XCTAssertTrue(script.contains("Press Enter"))
    }

    func testScriptWithDifferentSessions() {
        let spec1 = AttachSpec(session: "nat-session1")
        let spec2 = AttachSpec(session: "nat-session2")

        let script1 = AttachCommandScript.generate(from: spec1)
        let script2 = AttachCommandScript.generate(from: spec2)

        // Scripts should differ in their session names
        XCTAssertNotEqual(script1, script2)
        XCTAssertTrue(script1.contains("nat-session1"))
        XCTAssertTrue(script2.contains("nat-session2"))
    }
}
