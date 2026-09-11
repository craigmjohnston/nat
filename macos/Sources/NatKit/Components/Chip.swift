import SwiftUI

/// What a chip is saying, which is the only thing its call site should have to
/// choose. See `InkRole` for why these are outcomes and not colour names.
public enum Tone: Sendable {
    case accent, success, danger, warning, neutral

    var chipTint: DesignTokens.ChipTint {
        switch self {
        case .accent: .accent
        case .success: .green
        case .danger: .red
        case .warning: .yellow
        case .neutral: .labelSecondary
        }
    }
}

/// A word in a capsule of its own tint: a PR's state, a change's kind, a
/// pending marker.
///
/// One hue drawn twice — the word at full strength over the capsule behind it
/// — which is a pairing, and so a thing that can be wrong. Drawn by hand it
/// was: Latte's yellow chip came out at 1.57:1, its word invisible on its own
/// capsule, because a mid-lightness hue contrasts with nothing. The chip
/// derives both halves from the ground it finds itself on and the word is
/// shaded until it is legible, so the failure cannot be re-introduced by the
/// next call site that assembles one.
public struct Chip: View {
    let label: String
    let tone: Tone
    @Environment(\.ground) private var ground

    public init(_ label: String, tone: Tone) {
        self.label = label
        self.tone = tone
    }

    public var body: some View {
        Text(label)
            .font(.system(size: Typo.subhead, weight: .semibold))
            .foregroundStyle(DesignTokens.chipInk(tone.chipTint, on: ground))
            .padding(.horizontal, 10)
            .frame(height: 22)
            .background(DesignTokens.chipWash(tone.chipTint, on: ground))
            .clipShape(Capsule())
    }
}

/// A hairline between two things, at the weight the separation calls for.
/// It takes the ground from the environment like everything else, so a rule
/// inside a card is a card's rule.
public struct Rule: View {
    var weight: RuleWeight
    var axis: Axis
    @Environment(\.ground) private var ground

    public init(_ weight: RuleWeight = .separator, axis: Axis = .horizontal) {
        self.weight = weight
        self.axis = axis
    }

    public var body: some View {
        Rectangle()
            .fill(DesignTokens.rule(weight, on: ground))
            .frame(
                width: axis == .vertical ? 0.5 : nil,
                height: axis == .horizontal ? 0.5 : nil
            )
    }
}
