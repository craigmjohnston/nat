import Foundation

/// The Brief tab's dependency line: what the slice waits on, by name.
///
/// `dependsOn` is the slice's own relation — page IDs — and the names come
/// from the plan already loaded, so no extra read is made for a line of text.
/// IDs are matched with their dashes stripped, since a relation ID and the
/// plan's own can disagree about them without naming different pages.
///
/// While the slice is blocked, only the dependencies still unfinished are
/// listed — they are what is actually being waited on, the same reading the
/// board's status bar gives. Once nothing blocks it the whole list is named
/// with "all done", so the line still says why the slice was gated. A
/// dependency the plan cannot name (trashed, or filed elsewhere) is left
/// unlisted — the blocker rule passes over pages it cannot read, so naming
/// one as a wait would say more than anything knows — and a list with
/// nothing nameable at all falls back to the count.
public func dependencyLine(dependsOn: [String]?, blocked: Bool, plan: [Slice]) -> String {
    guard let deps = dependsOn, !deps.isEmpty else {
        return blocked ? "Blocked" : "Nothing blocks this slice"
    }

    let named = dependencyEntries(dependsOn, plan: plan)
    let listed = blocked ? named.filter { !$0.done } : named
    guard !listed.isEmpty else {
        return "Waits on \(deps.count) slice\(deps.count == 1 ? "" : "s")"
    }

    let line = "Waits on " + listed.map(\.name).joined(separator: ", ")
    return blocked ? line : line + " — all done"
}

/// The resolvable dependencies of a slice, in plan order, each with whether
/// it is finished — the shared resolution both `dependencyLine`'s prose and
/// the Brief tab's properties panel read off, so the two never disagree about
/// which dependencies exist to show. An ID the plan cannot name (trashed, or
/// filed elsewhere) is left out rather than guessed at, the same rule
/// `dependencyLine` applies to its own count fallback.
public func dependencyEntries(_ dependsOn: [String]?, plan: [Slice]) -> [(name: String, done: Bool)] {
    guard let deps = dependsOn, !deps.isEmpty else { return [] }

    let byID: [String: Slice] = plan.reduce(into: [:]) { map, slice in
        map[normalisedPageID(slice.id)] = slice
    }
    return deps.compactMap { byID[normalisedPageID($0)] }.map { ($0.name, $0.status == "Done") }
}

/// A page ID with its dashes stripped and case folded, the one spelling two
/// copies of the same ID always share.
private func normalisedPageID(_ id: String) -> String {
    id.replacingOccurrences(of: "-", with: "").lowercased()
}
