import SwiftUI
import NatKit

/// A full-bleed header split button — the run button, the action bar's Fix
/// with the action it replaced behind it: the main part in
/// `GnatHeaderButtonStyle`, then a chevron opening `menu` in a popover. The
/// divider between them runs the header's full height and is drawn as the
/// titlebar tabs draw their lines: over the chevron's own leading edge, on
/// top of its hover wash, so a washed part reaches the line and the line
/// never fades or breaks under the pointer.
struct HeaderSplitButton<Label: View, Menu: View>: View {
    var primary = false
    /// Whether the main part is greyed — the chevron stays live, its menu
    /// may still hold something to press.
    var mainDisabled = false
    var mainHelp = ""
    var chevronLabel: String
    /// Whether the menu is open — the popover's, or a story's.
    @Binding var menuOpen: Bool
    let action: () -> Void
    @ViewBuilder var label: () -> Label
    @ViewBuilder var menu: () -> Menu

    var body: some View {
        HStack(spacing: 0) {
            Button(action: action, label: label)
                .buttonStyle(GnatHeaderButtonStyle(primary: primary))
                .disabled(mainDisabled)
                .help(mainHelp)
            Button { menuOpen.toggle() } label: {
                Image(systemName: "chevron.down")
                    .font(.system(size: 9, weight: .semibold))
                    .accessibilityLabel(chevronLabel)
            }
            .buttonStyle(HeaderSplitChevronStyle(primary: primary))
            .help(chevronLabel)
        }
        .fixedSize(horizontal: true, vertical: false)
        .popover(isPresented: $menuOpen, arrowEdge: .bottom, content: menu)
    }
}

/// The split button's chevron: `GnatHeaderButtonStyle`'s full-bleed part,
/// narrower — a glyph needs less room than words — on the same ground as
/// the main part, and the divider over its leading edge at full height.
private struct HeaderSplitChevronStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled
    var primary = false

    func makeBody(configuration: Configuration) -> some View {
        HoverReader { hovering in
            configuration.label
                .padding(.horizontal, 9)
                .frame(maxHeight: .infinity)
                .foregroundStyle(primary ? DesignTokens.accentInk(on: .window) : DesignTokens.ink(.primary, on: .chrome))
                .background(background(hovering: hovering))
                .brightness(primary && hovering ? hoverBrightness : 0)
                .opacity(isEnabled ? (configuration.isPressed ? 0.85 : 1) : 0.4)
                // After the fade, so the line reads as the tabs' do whatever
                // the part's state.
                .overlay(alignment: .leading) {
                    DesignTokens.rule(.separator, on: .chrome).frame(width: 1)
                }
                .contentShape(Rectangle())
        }
    }

    private func background(hovering: Bool) -> Color {
        if primary {
            return isEnabled ? DesignTokens.accentDim(on: .window) : DesignTokens.fill(.window)
        }
        return hovering ? DesignTokens.rowWash(selected: false, on: .chrome) : .clear
    }
}

/// A split button's menu of plain actions, drawn as a view of gnat's own
/// rather than an `NSMenu` so a story can render it — the action bar's Fix's,
/// holding the action it replaced. A disabled row is greyed, its help its
/// tooltip.
struct HeaderSplitMenuList: View {
    struct Item: Identifiable {
        let title: String
        var systemImage: String?
        var glyph: HeaderGlyph?
        var enabled = true
        var help: String?
        let action: () -> Void

        var id: String { title }
    }

    let items: [Item]

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(items) { item in
                Button(action: item.action) {
                    HStack(spacing: 8) {
                        Group {
                            if item.glyph == .merge {
                                MergeIcon(size: 12)
                            } else if let systemImage = item.systemImage {
                                Image(systemName: systemImage).font(.system(size: 11, weight: .semibold))
                            }
                        }
                        .ink(item.enabled ? .secondary : .quaternary)
                        .frame(width: 14)
                        Text(item.title)
                            .font(.system(size: GnatMetrics.body))
                            .ink(item.enabled ? .primary : .tertiary)
                        Spacer(minLength: 0)
                    }
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .hoverWash(cornerRadius: 5, enabled: item.enabled)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .disabled(!item.enabled)
                .help(item.help ?? "")
            }
        }
        .padding(5)
        .frame(minWidth: 180, alignment: .leading)
    }
}
