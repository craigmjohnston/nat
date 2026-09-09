import Foundation

/// Error from a nat command.
public enum NatError: LocalizedError {
    case commandFailed(String)
    case invalidJSON(String, details: String)
    case missingOutput

    public var errorDescription: String? {
        switch self {
        case .commandFailed(let message):
            return "nat: \(message)"
        case .invalidJSON(let output, let details):
            return "Failed to parse nat output as JSON: \(details)\n\nOutput: \(output)"
        case .missingOutput:
            return "nat command produced no output"
        }
    }
}
