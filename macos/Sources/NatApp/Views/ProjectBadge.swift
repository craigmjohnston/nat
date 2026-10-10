import SwiftUI
import NatKit

/// A project's badge: its tag on the capsule a container's badge is drawn
/// on, in the project's colour (`DesignTokens.projectBadge`) — the quiet
/// secondary chip for a project with none (the scratch and source projects,
/// one nat has not coloured yet). A source project takes none: its tasks are
/// named by their card's (`CardMarkView`). Its tooltip is the project's full
/// name. The scratch project (`scratch`) takes no badge: it is drawn as
/// `ScratchMark` instead.
struct ProjectBadgeView: View {
    @Environment(\.ground) private var ground
    let tag: String
    let color: ProjectColor?
    /// The project's full name, for the tooltip; the tag where none is given
    /// or it is empty.
    var name: String?
    var scratch = false

    var body: some View {
        if scratch {
            ScratchMark()
        } else {
            let colors = color.map { DesignTokens.projectBadge($0, on: ground) }
            BadgeCapsule(text: tag, ink: colors?.ink, wash: colors?.wash)
                .help(name.flatMap { $0.isEmpty ? nil : $0 } ?? tag)
        }
    }
}

/// Scratch where another project's badge is drawn: its icon — the scratch
/// tab's, `DesignTokens.scratchSymbol` — then the word Scratch, in the
/// surrounding UI font, with no capsule or wash, in the quiet ink the grey
/// chip used.
struct ScratchMark: View {
    @Environment(\.ground) private var ground

    var body: some View {
        HStack(spacing: 4) {
            Image(systemName: DesignTokens.scratchSymbol)
            Text(scratchTitle)
        }
        .foregroundStyle(DesignTokens.chipInk(.labelSecondary, on: ground))
        .lineLimit(1)
        .fixedSize()
    }
}

/// The one capsule every badge is drawn on — a container's and a
/// project's — so the two are one shape by construction: a word in the
/// badge's mono face on a washed rounded rectangle, one width, `width`, the
/// word centred, so a column of rows lines up; a word longer than three
/// characters shrinks to fit rather than widening it. No ink or wash, the
/// quiet secondary chip. With an `icon` — a source's, on a card's badge
/// drawn outside the source's section — the icon leads the word in its ink,
/// and the capsule widens to hold the two.
struct BadgeCapsule: View {
    @Environment(\.ground) private var ground
    let text: String
    let ink: Color?
    let wash: Color?
    var icon: SourceIcon?

    /// Three characters of the badge's mono face — 10.5 pt Fira Code
    /// Medium with 0.3 tracking measures 20.3 pt — and 4 pt either side.
    static let width: CGFloat = 29
    static let height: CGFloat = 16
    static let cornerRadius: CGFloat = 2.5

    var body: some View {
        HStack(spacing: 3) {
            if let icon {
                SourceIconView(icon: icon, size: 12)
            }
            Text(text)
                .font(Typo.mono(size: Typo.scaled(10.5), weight: .medium))
                .tracking(0.3)
                .lineLimit(1)
                .minimumScaleFactor(icon == nil ? 0.6 : 1)
        }
        .foregroundStyle(ink ?? DesignTokens.chipInk(.labelSecondary, on: ground))
        .padding(.horizontal, 4)
        .frame(width: icon == nil ? Self.width : nil, height: Self.height)
        .background(wash ?? DesignTokens.chipWash(.labelSecondary, on: ground))
        .clipShape(RoundedRectangle(cornerRadius: Self.cornerRadius))
        .fixedSize()
    }
}
