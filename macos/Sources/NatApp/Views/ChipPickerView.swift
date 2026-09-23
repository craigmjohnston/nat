import SwiftUI
import NatKit

/// A compact row of chips to choose one of several things — a session's pull
/// requests, or its branches — sitting above the content the choice selects.
/// Drawn only where there is something to choose between: fewer than two
/// chips draws nothing at all, so a caller never has to test for it.
///
/// Knows nothing of what its chips stand for. A chip carries a lead-in, a
/// title, and optionally a pull-request state or a default mark; the caller's
/// `onSelect` hears the chip's id, and its own model says which is selected.
struct ChipPickerView: View {
    let model: ChipPickerModel
    let onSelect: (String) -> Void

    @Environment(\.ground) private var ground

    var body: some View {
        if model.isVisible {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    ForEach(model.chips) { chip in
                        chipButton(chip, isSelected: chip.id == model.selectedID)
                    }
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
            }
            .rule(.separator, edges: [.bottom], width: 0.5)
        }
    }

    private func chipButton(_ chip: PickerChip, isSelected: Bool) -> some View {
        Button(action: { onSelect(chip.id) }) {
            HStack(spacing: 6) {
                if let lead = chip.lead {
                    Text(lead)
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .monospacedDigit()
                        .ink(.tertiary)
                }

                Text(chip.title)
                    .font(chip.lead == nil
                        ? Typo.mono(size: Typo.subhead, weight: isSelected ? .semibold : .regular)
                        : .system(size: Typo.subhead, weight: isSelected ? .semibold : .regular))
                    .ink(isSelected ? .primary : .secondary)
                    .lineLimit(1)

                if let state = chip.state {
                    Text(state.word)
                        .font(.system(size: Typo.subhead, weight: .semibold))
                        .ink(Self.role(for: state))
                }

                if let mark = chip.mark {
                    Text(mark)
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .ink(.tertiary)
                }
            }
            .padding(.horizontal, 10)
            .frame(height: 26)
            .background(
                RoundedRectangle(cornerRadius: 6)
                    .fill(isSelected ? DesignTokens.wash(.selection, tone: .accent, on: ground) : Color.clear)
            )
            .overlay(
                RoundedRectangle(cornerRadius: 6)
                    .strokeBorder(DesignTokens.rule(.border, on: ground), lineWidth: 0.5)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .hoverWash(cornerRadius: 6, enabled: !isSelected)
        .accessibilityAddTraits(isSelected ? .isSelected : [])
    }

    /// The attention vocabulary's own ink for a pull request's state — the
    /// same three the PR tab's badge draws.
    static func role(for state: PickerChipState) -> InkRole {
        switch state {
        case .open: return .success
        case .merged: return .accent
        case .closed: return .danger
        }
    }
}
