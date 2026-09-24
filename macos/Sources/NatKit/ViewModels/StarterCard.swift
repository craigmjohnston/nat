import Foundation

/// What an Untitled tab says for itself: the starter card's words, and the
/// rail's explainer beside it. The one place they are written, as
/// `EmptyProjectNote` is for a project with no slices, so the rail and the
/// pane cannot drift apart from the mock they were drawn from
/// (`docs/design/nat-new-project/ui-newproject.jsx`).
public enum StarterCard {
    public static let title = "Get started"
    public static let subtitle = "Start from a description or a plan, or open an existing project."

    public static let describeHeading = "Describe a plan"
    public static let describePlaceholder =
        "What do you want to do? Sketch the milestones and slices, paste a Notion page or URL, or drop a plan file — the planning agent workshops it into a plan with you."
    public static let openPlanLabel = "Open plan from filesystem…"
    public static let startHint = "⌘↩ to start"
    public static let workshopLabel = "Workshop the plan"

    public static let openDivider = "or open an existing project"
    public static let notionTitle = "From Notion"
    public static let notionSubtitle = "Pick a project page from your workspace"
    public static let filesystemTitle = "From filesystem"
    public static let filesystemSubtitle = "Choose a project folder that already has a plan"

    /// The Untitled tab's TODO section, in place of a plan it has none of.
    public static let railExplainer = "Milestones and slices appear here once the project has a plan."

    /// The TODO explainer once the workshop session is running, in place of
    /// `railExplainer` (`NFRail`'s non-proposal branch in
    /// `docs/design/nat-new-project/ui-npflow.jsx`).
    public static let railWorkshopExplainer =
        "Milestones and slices land here as the workshop settles on a plan."

    /// What the folder picker asks, and what a folder with no plan is refused
    /// with — said as what was looked for, so the message can be acted on.
    public static let openFolderPrompt = "Open"
    public static let openFolderMessage = "Choose a folder that holds a nat plan"
    public static let openPlanFileMessage = "Choose the plan document to workshop"

    /// The refusal for a plan file the picker cannot hand to an agent: size is
    /// the only sanity asked, judging what is in it being the agent's job.
    public static func planFileTooLarge(name: String, bytes: Int) -> String {
        "\(name) is \(bytes / 1024) KB — a plan file over \(PlanFile.maxBytes / 1024) KB is too large to hand to the planning agent"
    }

    public static func planFileUnreadable(name: String) -> String {
        "\(name) could not be read as text"
    }
}

/// A plan document chosen from the filesystem, held until the workshop is
/// launched: its content goes to the planning agent alongside whatever
/// description was typed.
public struct PlanFile: Equatable, Sendable {
    /// The most a plan file may be: the content rides in the agent's opening
    /// prompt, which is no place for a book.
    public static let maxBytes = 256 * 1024

    public let name: String
    public let content: String

    public init(name: String, content: String) {
        self.name = name
        self.content = content
    }

    /// Reads a file for the workshop. No format gate: anything that is text
    /// and of sane size is the agent's to judge.
    public static func read(_ url: URL) throws -> PlanFile {
        let name = url.lastPathComponent
        let size = (try? url.resourceValues(forKeys: [.fileSizeKey]))?.fileSize ?? 0
        if size > maxBytes {
            throw PlanFileError(message: StarterCard.planFileTooLarge(name: name, bytes: size))
        }
        guard let data = try? Data(contentsOf: url),
              let content = String(data: data, encoding: .utf8) else {
            throw PlanFileError(message: StarterCard.planFileUnreadable(name: name))
        }
        return PlanFile(name: name, content: content)
    }

    /// The request the planning agent is launched on: the typed description,
    /// then the document, set off so the agent can tell which is which.
    public func request(description: String) -> String {
        let attached = "The plan document \"\(name)\" follows.\n\n----- BEGIN \(name) -----\n\(content)\n----- END \(name) -----"
        return description.isEmpty ? attached : description + "\n\n" + attached
    }
}

/// Why a plan file was not taken, in words to show as they are.
public struct PlanFileError: Error, Equatable {
    public let message: String
}
