import Foundation

/// The plan a workshop session proposed — for an Untitled tab or a tracked
/// project's own workshop — what `nat plan-proposal --json` reads back from
/// the file `nat plan-propose` wrote. Held as the tree the workshop's Plan
/// section draws and the Plan tab reads: milestones in plan order, each with
/// its proposed slices — title, brief and what each waits on.
/// A project's proposal may file slices under milestones the project already
/// has; those show as groups too, after the new ones, since a slice headed
/// for an existing milestone is still proposed work. Nothing here is a slice
/// yet; accepting is what files them (`nat plan-accept`).
///
/// A project's proposal may also change Todo slices already on its board —
/// the work it supersedes: `removals`, `moves` and `edits`, each naming the
/// slice by title, as the plan document does. Accepting applies those too.
public struct PlanProposal: Equatable, Sendable, Decodable {
    /// One proposed slice: its title, its brief (markdown, empty where the
    /// plan gives none) and the titles of the slices it waits on. A string
    /// literal is a slice with only a title.
    public struct ProposedSlice: Equatable, Sendable, ExpressibleByStringLiteral {
        public let name: String
        public let brief: String
        public let dependsOn: [String]

        public init(name: String, brief: String = "", dependsOn: [String] = []) {
            self.name = name
            self.brief = brief
            self.dependsOn = dependsOn
        }

        public init(stringLiteral name: String) {
            self.init(name: name)
        }
    }

    /// One milestone of the tree and the slices filed under it. `isNew`
    /// says the proposal creates the milestone itself; false is one the
    /// project already has, there only to hold its proposed slices.
    public struct Milestone: Equatable, Sendable {
        public let name: String
        public let slices: [ProposedSlice]
        public let isNew: Bool

        public init(name: String, slices: [ProposedSlice], isNew: Bool = true) {
            self.name = name
            self.slices = slices
            self.isNew = isNew
        }
    }

    /// A slice already on the board the proposal refiles: its title, and the
    /// milestone it goes under.
    public struct ProposedMove: Equatable, Sendable {
        public let name: String
        public let milestone: String

        public init(name: String, milestone: String) {
            self.name = name
            self.milestone = milestone
        }
    }

    /// A slice already on the board the proposal retitles, rewrites or
    /// both: its title, the new title (nil where the edit keeps it), and the
    /// new brief whole (empty where the edit keeps it).
    public struct ProposedEdit: Equatable, Sendable {
        public let name: String
        public let title: String?
        public let brief: String

        public init(name: String, title: String? = nil, brief: String) {
            self.name = name
            self.title = title
            self.brief = brief
        }
    }

    /// The project name the planning agent suggested.
    public let name: String
    public let milestones: [Milestone]
    /// The titles of the slices already on the board accepting removes.
    public let removals: [String]
    public let moves: [ProposedMove]
    public let edits: [ProposedEdit]

    public init(
        name: String, milestones: [Milestone],
        removals: [String] = [], moves: [ProposedMove] = [], edits: [ProposedEdit] = []
    ) {
        self.name = name
        self.milestones = milestones
        self.removals = removals
        self.moves = moves
        self.edits = edits
    }

    /// Whether accepting changes any slice already on the board.
    public var changesBoard: Bool { !removals.isEmpty || !moves.isEmpty || !edits.isEmpty }

    /// How many milestones accepting creates — an existing one a slice is
    /// filed under is not among them.
    public var milestoneCount: Int { milestones.filter(\.isNew).count }
    public var sliceCount: Int { milestones.reduce(0) { $0 + $1.slices.count } }

    /// The tree as milestone folders, as the workshop's Plan section draws
    /// it: every slice Todo, no milestone current, nothing done, and each
    /// marked new where the proposal creates it.
    public var folders: [MilestoneFolder] {
        milestones.enumerated().map { index, milestone in
            MilestoneFolder(
                milestoneID: milestone.name,
                title: milestone.name,
                done: 0,
                total: milestone.slices.count,
                isCurrent: false,
                slices: milestone.slices.enumerated().map { sliceIndex, slice in
                    MilestoneSliceRow(
                        sliceID: Self.sliceID(milestone: index, slice: sliceIndex), name: slice.name, glyph: .todo,
                        isBlocked: false)
                },
                isNew: milestone.isNew
            )
        }
    }

    /// A proposed slice's id, by its place in the plan: what the Plan
    /// section's row and the Plan tab's box for it share.
    public static func sliceID(milestone: Int, slice: Int) -> String {
        "proposed-\(milestone)-\(slice)"
    }

    // The wire shape is `plan-propose`'s proposal file: the plan document as
    // validated, slices naming their milestone. Grouping them under it is all
    // this does — validation already ran when the file was written. A
    // milestone a slice names that the document does not create is one the
    // project already has (plan-propose --project validated it against the
    // project's shape), and gets a group of its own after the new ones, in
    // the order the slices first name it.
    private enum CodingKeys: String, CodingKey { case name, plan }
    private struct Plan: Decodable {
        struct Named: Decodable { let name: String }
        struct Move: Decodable { let slice: String; let milestone: String }
        // nat always writes description (empty for a rename alone); title is
        // there only where the edit renames.
        struct Edit: Decodable { let slice: String; let title: String?; let description: String }
        struct Slice: Decodable {
            let title: String
            let milestone: String
            let description: String?
            let dependsOn: [String]?

            enum CodingKeys: String, CodingKey {
                case title, milestone, description
                case dependsOn = "depends_on"
            }
        }
        let milestones: [Named]?
        let slices: [Slice]?
        let remove: [String]?
        let move: [Move]?
        let edit: [Edit]?
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        // A project's own workshop proposes into a project that already has
        // a name, and may suggest none.
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        let plan = try container.decode(Plan.self, forKey: .plan)
        let slices = plan.slices ?? []
        func proposed(under key: String) -> [ProposedSlice] {
            slices.filter { Self.key($0.milestone) == key }.map {
                ProposedSlice(
                    name: $0.title.trimmingCharacters(in: .whitespacesAndNewlines),
                    brief: ($0.description ?? "").trimmingCharacters(in: .whitespacesAndNewlines),
                    dependsOn: ($0.dependsOn ?? []).map { $0.trimmingCharacters(in: .whitespacesAndNewlines) })
            }
        }
        let new = (plan.milestones ?? []).map { milestone in
            Milestone(
                name: milestone.name.trimmingCharacters(in: .whitespacesAndNewlines),
                slices: proposed(under: Self.key(milestone.name)))
        }
        var seen = Set(new.map { Self.key($0.name) })
        var existing: [Milestone] = []
        for slice in slices where seen.insert(Self.key(slice.milestone)).inserted {
            existing.append(Milestone(
                name: slice.milestone.trimmingCharacters(in: .whitespacesAndNewlines),
                slices: proposed(under: Self.key(slice.milestone)),
                isNew: false))
        }
        milestones = new + existing
        func trimmed(_ text: String) -> String { text.trimmingCharacters(in: .whitespacesAndNewlines) }
        removals = (plan.remove ?? []).map(trimmed)
        moves = (plan.move ?? []).map { ProposedMove(name: trimmed($0.slice), milestone: trimmed($0.milestone)) }
        edits = (plan.edit ?? []).map {
            let title = trimmed($0.title ?? "")
            return ProposedEdit(name: trimmed($0.slice), title: title.isEmpty ? nil : title, brief: trimmed($0.description))
        }
    }

    /// How the CLI matches a slice to its milestone: trimmed, case-folded.
    private static func key(_ name: String) -> String {
        name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }
}

/// What `nat plan-accept --json` reports: the project it made and how much of
/// the plan went into it.
public struct PlanAccepted: Equatable, Sendable, Decodable {
    public let project: ProjectEntry
    public let milestones: Int
    public let slices: Int

    public init(project: ProjectEntry, milestones: Int, slices: Int) {
        self.project = project
        self.milestones = milestones
        self.slices = slices
    }
}

/// The words of the proposal's rail section and the accepted pane, written once
/// — the mock is `NFRail`/`NFShell` in `docs/design/nat-new-project/ui-npflow.jsx`.
public enum ProposalText {
    public static let nameCaption = "Project name, suggested by the planning agent"
    public static let acceptLabel = "Accept plan"
    public static let keepLabel = "Keep workshopping"
    public static let emptyNameError = "Give the project a name to accept the plan."
    public static let acceptedTitle = "Plan accepted"

    /// "4 milestones · 14 slices", each pluralised on its own.
    public static func counts(milestones: Int, slices: Int) -> String {
        "\(milestones) \(milestones == 1 ? "milestone" : "milestones") · \(slices) \(slices == 1 ? "task" : "tasks")"
    }

    /// The caption under the buttons, tracking the name field.
    public static func acceptCaption(name: String) -> String {
        "Accepting writes the plan to local storage as “\(name)”."
    }

    /// The section the proposal's changes to slices already on the board are
    /// drawn under, below the work it creates.
    public static let boardChangesHeading = "Changes to tasks already planned"
    public static let removeLabel = "Remove"
    public static let moveLabel = "Move"
    public static let editLabel = "Edit"

    /// Said before Accept wherever the proposal removes anything: Accept is
    /// the one approval, so what it takes off the board is said plainly.
    public static func removalWarning(count: Int) -> String {
        "Accepting also removes \(count) \(count == 1 ? "task" : "tasks") already planned."
    }

    /// The destination a moved task is drawn with.
    public static func moveDestination(_ milestone: String) -> String {
        "→ \(milestone)"
    }

    /// The title a retitled task had, drawn under the name it will have once
    /// the proposal is accepted.
    public static func renamedFrom(_ title: String) -> String {
        "Renamed from \(title)"
    }

    /// The same caption on a project's own workshop, which has its name.
    public static func projectAcceptCaption(project: String) -> String {
        "Accepting files these milestones and tasks into “\(project)”."
    }

    public static func acceptedSubtitle(milestones: Int, slices: Int) -> String {
        "\(counts(milestones: milestones, slices: slices)) written locally. Select a task to begin."
    }
}
