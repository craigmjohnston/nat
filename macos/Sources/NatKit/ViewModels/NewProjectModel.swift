import Foundation

/// The decisions behind the "+" tab's sheet, kept out of the view: which
/// projects its open picker offers, when each path's button can act, and what
/// a refusal reads as. The sheet itself is fields and buttons over these.
public enum NewProjectModel {
    /// The rows the open picker offers: the workspace projects this machine
    /// does not track yet, in the order the listing gave them. The configured
    /// half is already the tab strip — offering it again would be a picker
    /// whose pick does nothing.
    public static func openable(_ listing: ProjectListing) -> [ProjectListingEntry] {
        listing.projects.filter { !$0.configured }
    }

    /// Whether the open path has anything to act on: a project picked out of
    /// what the listing offered. A pick that is no longer in the listing —
    /// the listing having been read again under the picker — is no pick at
    /// all, which is what stops "Open" acting on a stale ID.
    public static func canOpen(selection: String, in listing: ProjectListing) -> Bool {
        !selection.isEmpty && openable(listing).contains { $0.id == selection }
    }

    /// Whether the create path has enough to act on: a name, and a directory
    /// for the project's agents to work in. The directory is required here
    /// though `project-create`'s own `--repo` is optional, because what the
    /// CLI falls back to is the directory it was typed in — which for a
    /// windowed app is wherever the Finder happened to launch it, and never
    /// anybody's checkout.
    public static func canCreate(name: String, directory: String) -> Bool {
        !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && !directory.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    /// What a failure reads as on the sheet: nat's own first stderr line
    /// where there is one, since "no such page" and "no projects database is
    /// configured" are the whole of what there is to say, and the error's own
    /// description otherwise.
    public static func message(from error: Error) -> String {
        if let natError = error as? NatError, case .commandFailed(let message) = natError {
            return message
        }
        return error.localizedDescription
    }
}
