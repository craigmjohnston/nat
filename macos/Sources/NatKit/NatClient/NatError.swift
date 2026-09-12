import Foundation

/// Error from a nat command.
public enum NatError: LocalizedError {
    case commandFailed(String)
    case invalidJSON(String, details: String)
    case missingOutput
    case bundledBinaryMissing(String)

    public var errorDescription: String? {
        switch self {
        case .commandFailed(let message):
            return "nat: \(message)"
        case .invalidJSON(let output, let details):
            return "Failed to parse nat output as JSON: \(details)\n\nOutput: \(output)"
        case .missingOutput:
            return "nat command produced no output"
        case .bundledBinaryMissing(let expected):
            // The bundling exists so the app and its nat are never out of
            // step, so a bundle with no nat is a damaged install to fix
            // rather than a reason to run whatever else is on PATH.
            return "This copy of gnat is missing the nat it was built with "
                + "(expected at \(expected)). Reinstall gnat: running another "
                + "nat would be a version the app was never built against."
        }
    }
}
