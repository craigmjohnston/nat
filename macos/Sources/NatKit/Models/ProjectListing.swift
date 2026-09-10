import Foundation

/// The structured form of `nat project-list --json`: the two halves of
/// "which project" the TUI's own switch picker offers — the projects this
/// machine's config already tracks, and the rows of the workspace's projects
/// database it does not — as one list, each row saying which half it is.
///
/// The listing is deliberately forgiving about the workspace half: a projects
/// database that cannot be found or read prints the configured half alone
/// with a `note` saying why, since a slow or absent database must not take
/// the whole listing down. That is what leaves the "+" tab's create path
/// available when the open path has nothing to offer.
public struct ProjectListing: Codable, Equatable, Sendable {
    public let projects: [ProjectListingEntry]

    /// Why the workspace's own projects are missing from the listing, in
    /// nat's words — absent when nothing went wrong.
    public let note: String?

    enum CodingKeys: String, CodingKey {
        case projects
        case note
    }

    public init(projects: [ProjectListingEntry], note: String? = nil) {
        self.projects = projects
        self.note = note
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        // An empty listing is a listing, so both fields tolerate absence: a
        // workspace with nothing to offer prints a note and no projects, and
        // reading that as a decode failure would turn "nothing to open" into
        // "the app is broken".
        projects = try container.decodeIfPresent([ProjectListingEntry].self, forKey: .projects) ?? []
        note = try container.decodeIfPresent(String.self, forKey: .note)
    }
}

/// One project of the listing: what it is called, the page ID `--project`
/// takes, and which half of the reading it came from.
public struct ProjectListingEntry: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let name: String

    /// Whether local config already tracks this project — a tab on the board
    /// rather than something to open.
    public let configured: Bool

    /// Where its agents work, for a configured project; empty for one this
    /// machine does not track yet, which has no working directory until the
    /// user gives it one.
    public let workingDir: String

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case configured
        case workingDir = "working_dir"
    }

    public init(id: String, name: String, configured: Bool, workingDir: String = "") {
        self.id = id
        self.name = name
        self.configured = configured
        self.workingDir = workingDir
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decode(String.self, forKey: .name)
        configured = try container.decodeIfPresent(Bool.self, forKey: .configured) ?? false
        // Absent on the workspace half, which has no config entry to read one
        // from — the same "unset" the empty string already means.
        workingDir = try container.decodeIfPresent(String.self, forKey: .workingDir) ?? ""
    }
}

/// The config entry `nat project-open --json` reports having written: the
/// page ID `--project` takes, the name the tab is labelled with, and the
/// working directory — empty, since opening a project records where its plan
/// lives and nothing about where its code does.
public struct ProjectEntry: Codable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let slicesDSID: String
    public let workingDir: String

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case slicesDSID = "slices_ds_id"
        case workingDir = "working_dir"
    }

    public init(id: String, name: String, slicesDSID: String = "", workingDir: String = "") {
        self.id = id
        self.name = name
        self.slicesDSID = slicesDSID
        self.workingDir = workingDir
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decode(String.self, forKey: .name)
        slicesDSID = try container.decodeIfPresent(String.self, forKey: .slicesDSID) ?? ""
        workingDir = try container.decodeIfPresent(String.self, forKey: .workingDir) ?? ""
    }
}

/// The project `nat project-create --json` reports having made: the page it
/// is, the database its plan lives in, and the directory local config now
/// points at — `createdProjectJSON` in `internal/cli/projectcreate.go`.
public struct CreatedProject: Codable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let url: String
    public let slicesDBID: String
    public let slicesDSID: String
    public let workingDir: String

    /// Whether the Slices table it made tracks an assignee, which follows
    /// this machine's configured user rather than anything the sheet asked.
    public let assignee: Bool

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case url
        case slicesDBID = "slices_db_id"
        case slicesDSID = "slices_ds_id"
        case workingDir = "working_dir"
        case assignee
    }

    public init(
        id: String,
        name: String,
        url: String = "",
        slicesDBID: String = "",
        slicesDSID: String = "",
        workingDir: String = "",
        assignee: Bool = false
    ) {
        self.id = id
        self.name = name
        self.url = url
        self.slicesDBID = slicesDBID
        self.slicesDSID = slicesDSID
        self.workingDir = workingDir
        self.assignee = assignee
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decode(String.self, forKey: .name)
        url = try container.decodeIfPresent(String.self, forKey: .url) ?? ""
        slicesDBID = try container.decodeIfPresent(String.self, forKey: .slicesDBID) ?? ""
        slicesDSID = try container.decodeIfPresent(String.self, forKey: .slicesDSID) ?? ""
        workingDir = try container.decodeIfPresent(String.self, forKey: .workingDir) ?? ""
        assignee = try container.decodeIfPresent(Bool.self, forKey: .assignee) ?? false
    }
}
