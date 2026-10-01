import SwiftUI
import NatKit

/// The Follow-ups sidebar: the follow-ups a slice's agent proposed before
/// handing back, one row each with a native Queue / Fold in / Drop picker,
/// under Apply and Discard All. `PaneView` draws it beside whichever tab is
/// up, and only while the slice's detail reads with proposals pending — data
/// presence shows and hides it, as with every other sidebar in the app.
///
/// The same stack as the Diff, PR and Brief sidebars: `InspectorActionsBar`,
/// the rows (which scroll, the bar and foot staying pinned),
/// `InspectorStatusFoot`, and a `PaneResizeHandle` on the leading edge at its
/// own remembered width.
struct FollowUpsSidebarView: View {
    @Environment(\.ground) private var ground
    @Bindable var appModel: AppModel
    let slice: Slice
    let followUps: [FollowUp]
    let milestone: String
    let hasLiveAgent: Bool

    @AppStorage("followUpsSidebarWidth") private var sidebarWidth = 232.0
    @State private var liveSidebarWidth: Double?

    private var store: FollowUpStore { appModel.followUpStore }
    private var choices: [Int: FollowUpChoice] { store.choices(sliceID: slice.id) }
    private var isApplying: Bool { store.isApplying(sliceID: slice.id) }
    private var canApply: Bool {
        FollowUpStore.canApply(followUps: followUps, choices: choices, hasLiveAgent: hasLiveAgent)
    }

    var body: some View {
        VStack(spacing: 0) {
            InspectorActionsBar {
                Button(action: apply) {
                    AsyncActionLabel(isBusy: isApplying) {
                        Text(isApplying ? "Applying…" : "Apply")
                    }
                }
                .buttonStyle(InspectorPrimaryButtonStyle())
                .disabled(!canApply || isApplying)

                Button(action: discardAll) {
                    Text("Discard All")
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .ink(.danger)
                }
                .buttonStyle(InspectorSecondaryButtonStyle())
                .disabled(isApplying)
            }

            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    heading
                        .padding(.bottom, 14)
                    ForEach(Array(followUps.enumerated()), id: \.element.index) { offset, followUp in
                        if offset > 0 {
                            Rule(.hairline)
                                .padding(.horizontal, -14)
                                .padding(.vertical, 14)
                        }
                        row(followUp)
                    }
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 18)
                .inelastic()
            }

            InspectorStatusFoot { foot }
        }
        .frame(width: liveSidebarWidth ?? sidebarWidth)
        .rule(.separator, edges: [.leading], width: 0.5)
        .overlay(alignment: .leading) {
            PaneResizeHandle(width: sidebarWidth, liveWidth: $liveSidebarWidth, onCommit: { sidebarWidth = $0 }, minWidth: 190, maxWidth: 420, edge: .leading)
                .offset(x: -4.5)
        }
    }

    private var heading: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 8) {
                Text("FOLLOW-UPS · \(followUps.count)")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .monospacedDigit()
                    .ink(.tertiary)
                Spacer()
                Circle()
                    .fill(DesignTokens.ink(.success, on: ground))
                    .frame(width: 7, height: 7)
            }
            Text("Proposed by the agent before hand-back")
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private func row(_ followUp: FollowUp) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(followUp.title)
                .font(.system(size: Typo.body, weight: .semibold))
                .ink(.primary)
                .fixedSize(horizontal: false, vertical: true)
            Text(followUp.brief)
                .font(.system(size: Typo.subhead + 1, weight: .regular))
                .ink(.secondary)
                .fixedSize(horizontal: false, vertical: true)
                .textSelection(.enabled)
            picker(followUp)
                .padding(.top, 4)
        }
    }

    private func picker(_ followUp: FollowUp) -> some View {
        let selection = Binding<FollowUpChoice?>(
            get: { choices[followUp.index] },
            set: { store.setChoice($0, sliceID: slice.id, index: followUp.index) }
        )
        return Picker("", selection: selection) {
            ForEach(FollowUpChoice.allCases, id: \.self) { choice in
                Text(choice.rawValue)
                    .tag(Optional(choice))
                    .selectionDisabled(choice == .fold && !hasLiveAgent)
            }
        }
        .labelsHidden()
        .pickerStyle(.segmented)
        .tint(DesignTokens.ink(tint(for: choices[followUp.index]), on: ground))
        .disabled(isApplying)
        .accessibilityLabel(followUp.title)
    }

    /// The selected segment's ink: green for Queue, orange for Fold in, the
    /// label colour for Drop.
    private func tint(for choice: FollowUpChoice?) -> InkRole {
        switch choice {
        case .queue: return .success
        case .fold: return .warning
        case .drop, nil: return .primary
        }
    }

    @ViewBuilder
    private var foot: some View {
        if let error = store.error(sliceID: slice.id) {
            InspectorNotice(text: error, systemImage: "exclamationmark.circle.fill", role: .danger)
        } else if isApplying {
            Text(FollowUpStore.applyingSummary(choices: choices))
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
                .frame(maxWidth: .infinity, alignment: .leading)
        } else if !hasLiveAgent {
            InspectorNotice(
                text: "No live agent, so nothing can be folded in. Relaunch the slice first, or queue it instead.",
                systemImage: "exclamationmark.triangle", role: .warning, lines: 4
            )
        } else {
            Text(FollowUpStore.summary(followUps: followUps, choices: choices, milestone: milestone))
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func apply() {
        let sliceID = slice.id
        let followUps = followUps
        Task { await appModel.applyFollowUps(sliceID: sliceID, followUps: followUps) }
    }

    private func discardAll() {
        let sliceID = slice.id
        Task { await appModel.discardFollowUps(sliceID: sliceID) }
    }
}
