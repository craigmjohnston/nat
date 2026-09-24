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

    /// What each control drawn ahead of its wiring says when hovered: the
    /// slice that wires it. Staged deliberately — the card is drawn whole so
    /// the mock can be checked against it, and each of these lands disabled
    /// until its own slice does.
    public static let workshopStaging = "Launch the planning agent from the starter card"
    public static let openPlanStaging = "Open existing plans from the starter"
    public static let filesystemStaging = "Open existing plans from the starter"
}
