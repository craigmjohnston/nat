import Foundation

/// Generates shell script text for a `.command` file that attaches to a tmux session.
///
/// This is used by the "Open in Terminal…" button: it writes a small executable script
/// to a temp directory and opens it with NSWorkspace, which passes it to Terminal.app
/// to run the same attach command the embedded terminal uses.
public struct AttachCommandScript {
    /// Generates the complete shell script text for a .command file.
    ///
    /// The script:
    /// - Sets up the environment (scrubs TMUX/TMUX_PANE, sets TERM)
    /// - Runs `tmux -T <features> attach-session -t <session>`
    /// - Waits for user input before exiting so the Terminal window doesn't close immediately
    ///
    /// - Parameter spec: The AttachSpec describing the tmux invocation
    /// - Returns: Shell script text suitable for writing to a .command file
    public static func generate(from spec: AttachSpec) -> String {
        let executable = AttachSpec.executable
        let features = AttachSpec.viewerFeatures
        let term = AttachSpec.viewerTerm
        let sessionName = spec.sessionName

        return """
        #!/bin/bash
        # Auto-generated tmux attach script

        # Scrub tmux environment variables so nested attach works
        unset TMUX
        unset TMUX_PANE

        # Set the terminal type
        export TERM="\(term)"

        # Attach to the session
        "\(executable)" -T "\(features)" attach-session -t "\(sessionName)"

        # Keep the window open after detach
        echo "Session ended. Press Enter to close this window..."
        read
        """
    }
}
