import Foundation

/// The tmux session name an agent's pane runs in, mirroring
/// `SessionName` in `internal/agent/tmux.go`.
///
/// The Go TUI identifies a running agent by its pane's `@nat_slice` option
/// rather than by session name — the name is only a human label — but the
/// label still has to be one tmux will accept, and this is how it is built.
public enum TmuxSession {
    /// SessionPrefix, copied from `internal/agent/tmux.go`'s constant of the
    /// same name: it namespaces our sessions inside the user's tmux server.
    public static let prefix = "nat-"

    /// PlanSentinel, copied from `internal/agent/tmux.go`: the tag the
    /// planning agent's pane carries in place of a slice page ID. It is not
    /// hex, so it cannot collide with a real slice's session name.
    public static let planSentinel = "plan"

    /// PlanSession, copied from `internal/agent/tmux.go`: the one tmux
    /// session the planning agent always launches in.
    public static let planSession = prefix + planSentinel

    /// sessionIDLen, copied from `internal/agent/tmux.go`: eight hex digits
    /// is what tmux can show without truncating the status line.
    private static let idLength = 8

    /// The tmux session name for a slice's page ID: the prefix plus the last
    /// eight hex digits of the ID, with anything else — UUID dashes
    /// included — skipped rather than trusted.
    ///
    /// The tail rather than the head, because page IDs made in one workspace
    /// share a long leading prefix: taken from the front, every slice of a
    /// project would name the same session.
    public static func name(forSlicePageID slicePageID: String) -> String {
        if slicePageID == planSentinel {
            return planSession
        }

        let hex = slicePageID.lowercased().filter { character in
            ("0"..."9").contains(character) || ("a"..."f").contains(character)
        }
        let tail = hex.count > idLength ? String(hex.suffix(idLength)) : hex
        return prefix + tail
    }
}

/// The tmux invocation and environment that attach an embedded terminal to a
/// running agent's session, mirroring `AttachClientCmd` in
/// `internal/agent/tmux.go`.
///
/// The macOS app has no full-screen attach the way the Go TUI's `T` key
/// does — the terminal view always hosts the client on a pseudo-terminal of
/// its own, exactly like the Go TUI's embedded viewer does — so this is the
/// one shape built here, not `AttachCmd`'s.
public struct AttachSpec: Equatable {
    /// The tmux binary, looked up on PATH — `TmuxBinary` in
    /// `internal/agent/tmux.go`.
    public static let executable = "tmux"

    /// The client-features list every attach declares with tmux's top-level
    /// `-T`, copied verbatim from `internal/agent/tmux.go`'s `ViewerFeatures`
    /// constant: 256 colours and direct RGB, extended keys so shift+enter
    /// still arrives distinguishable, and focus reporting. It deliberately
    /// omits sync and sixel, which the terminal view's emulator speaks
    /// neither of.
    public static let viewerFeatures = "256,RGB,extkeys,focus"

    /// The TERM the embedded terminal's client attaches with, replacing
    /// whatever the host process's own TERM was: the client on the far end
    /// of this attach's pseudo-terminal is the terminal view's own emulator,
    /// not the user's real terminal.
    public static let viewerTerm = "xterm-256color"

    /// SessionEnv in `internal/agent/tmux.go`: tmux refuses to attach a
    /// client from inside one of its own panes when this is set, so it has
    /// to be scrubbed from an attach's environment.
    public static let sessionEnvName = "TMUX"

    /// PaneEnv in `internal/agent/tmux.go`, scrubbed alongside
    /// `sessionEnvName` for the same reason.
    public static let paneEnvName = "TMUX_PANE"

    /// TERM is scrubbed too, ahead of being replaced with `viewerTerm`.
    public static let termEnvName = "TERM"

    /// The tmux session this attach targets.
    public let sessionName: String

    /// The argv tmux is invoked with, mirroring `attachArgs`: `-T` is a
    /// top-level client flag, so it precedes the `attach-session` command
    /// rather than following it.
    public let arguments: [String]

    public init(session: String) {
        sessionName = session
        arguments = ["-T", Self.viewerFeatures, "attach-session", "-t", session]
    }

    /// The environment an attach process should run with, given the host
    /// process's own environment — mirroring `scrubEnv` followed by
    /// `AttachClientCmd`'s own replacement of TERM: TMUX and TMUX_PANE
    /// removed so tmux does not refuse a nested attach, and TERM replaced
    /// with the viewer's own rather than left as the host's, since the far
    /// end of this attach's pseudo-terminal is never the host's real
    /// terminal.
    public static func environment(from base: [String: String]) -> [String: String] {
        var out = base
        out.removeValue(forKey: sessionEnvName)
        out.removeValue(forKey: paneEnvName)
        out.removeValue(forKey: termEnvName)
        out[termEnvName] = viewerTerm
        return out
    }

    /// The absolute path the attach actually execs. SwiftTerm spawns with
    /// execve, which searches nothing, and an app launched from the Finder
    /// carries a PATH with no Homebrew on it — so the bare name is resolved
    /// here: the environment's PATH first, then the places tmux is actually
    /// installed on a Mac. A name found nowhere is returned bare, so the
    /// failure reads as tmux missing rather than as this code's guess.
    public static func resolvedExecutable(
        environment: [String: String] = ProcessInfo.processInfo.environment,
        fileExists: (String) -> Bool = { FileManager.default.isExecutableFile(atPath: $0) }
    ) -> String {
        let fromPath = (environment["PATH"] ?? "")
            .split(separator: ":")
            .map { "\($0)/\(executable)" }
        let fallbacks = ["/opt/homebrew/bin/tmux", "/usr/local/bin/tmux", "/usr/bin/tmux"]
        return (fromPath + fallbacks).first(where: fileExists) ?? executable
    }
}
