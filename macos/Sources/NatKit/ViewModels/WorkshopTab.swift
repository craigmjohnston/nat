import Foundation

/// One tab of the titlebar band, whatever kind of pane it is a tab of: the
/// word it shows and what tells it from its neighbours. A slice's or
/// session's `MainPaneTab` and the workshop's `WorkshopTab` both map to it,
/// so the band draws either without knowing which.
public struct TitlebarTab: Hashable, Sendable {
    public let label: String
    public let id: String

    public init(label: String, id: String) {
        self.label = label
        self.id = id
    }
}

extension MainPaneTab {
    public var titlebarTab: TitlebarTab { TitlebarTab(label: label, id: "pane.\(self)") }
}

extension TitlebarTab {
    /// A run command's tab (`MainPaneMode.run`), beside Terminal while a run
    /// the selection can show is live — no navigator section's.
    public static let run = TitlebarTab(label: "Run", id: "pane.run")

    /// `tabs` with the Run tab put just after Terminal — first, where there is
    /// no Terminal.
    public static func withRun(_ tabs: [MainPaneTab]) -> [TitlebarTab] {
        var out = tabs.map(\.titlebarTab)
        out.insert(run, at: tabs.firstIndex(of: .terminal).map { $0 + 1 } ?? 0)
        return out
    }
}

/// The workshop's main-pane tabs: the planning agent's terminal, and the
/// proposal read brief by brief. Like the navigator's sections, a tab is
/// there only while its view is.
public enum WorkshopTab: CaseIterable, Equatable, Sendable {
    case terminal, plan

    public var label: String {
        switch self {
        case .terminal: return "Terminal"
        case .plan: return "Plan"
        }
    }

    public var titlebarTab: TitlebarTab { TitlebarTab(label: label, id: "workshop.\(self)") }

    /// The tabs a workshop has: none before launch, while the brief editor
    /// is up — nor once the session has ended; Terminal from launch on; Plan
    /// beside it once there is a proposal to read.
    public static func available(launched: Bool, hasProposal: Bool) -> [WorkshopTab] {
        guard launched else { return [] }
        return hasProposal ? [.terminal, .plan] : [.terminal]
    }
}

/// An ask for the Plan tab to scroll to a proposed slice's box, bumped with
/// a token so asking for the same slice twice still scrolls.
public struct WorkshopPlanScroll: Equatable, Sendable {
    public let sliceID: String
    public let token: Int

    public init(sliceID: String, token: Int) {
        self.sliceID = sliceID
        self.token = token
    }
}
