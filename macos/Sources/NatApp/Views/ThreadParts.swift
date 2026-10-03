import SwiftUI
import NatKit

// MARK: - Cut-short text

/// A chunk of text cut to its first `maxWords` words until asked for the
/// rest — the brief's own Show more, for every Thread item that carries
/// prose, and the PR description at three times the length. Folds again
/// whenever the text itself changes.
struct Excerpt<Content: View>: View {
    let text: String
    var maxWords = briefExcerptWords
    @ViewBuilder let content: (String) -> Content

    @State private var expanded = false

    var body: some View {
        let excerpt = briefExcerpt(text, maxWords: maxWords)
        VStack(alignment: .leading, spacing: 4) {
            content(expanded ? text : excerpt ?? text)
            if excerpt != nil {
                Button(expanded ? "Show less" : "Show more") { expanded.toggle() }
                    .buttonStyle(GnatLinkButtonStyle())
            }
        }
        .onChange(of: text) { _, _ in expanded = false }
    }
}

// MARK: - Icons

extension ThreadEventKind {
    /// The glyph a Thread card is headed with.
    var symbol: String {
        switch self {
        case .launched: return "play.circle"
        case .agent: return "terminal"
        case .handedBack: return "arrow.uturn.backward.circle"
        case .sentBack: return "arrow.uturn.forward.circle"
        case .released: return "arrow.down.to.line.circle"
        case .relaunched: return "arrow.clockwise.circle"
        case .blocked: return "exclamationmark.octagon"
        case .followUps: return "lightbulb"
        case .note: return "note.text"
        case .approved: return "checkmark.seal"
        case .merged: return "arrow.triangle.merge"
        case .closed: return "checkmark.circle"
        }
    }
}

/// A Thread card's leading glyph, sized to sit on the mono `xs` line beside
/// it.
struct ThreadIcon: View {
    let symbol: String
    var role: InkRole = .tertiary

    var body: some View {
        Image(systemName: symbol)
            .font(.system(size: 11, weight: .medium))
            .ink(role)
            .frame(width: 14)
    }
}

// MARK: - Launch

/// The Thread's last item while the slice can be launched: a ghost card —
/// dashed, not yet anything that happened — drawn as the Launched card it
/// will become: its header (with Launch itself at the trailing end), what
/// Launch will do, then the facts that card will carry on the chrome ground
/// under a line. Model and effort are editable there, each a menu; the base
/// the worktree is cut from is not. Blocked, it is the same card greyed and
/// hatched: the menus and Launch disabled, and what it waits on said.
struct LaunchCard: View {
    enum Mode: Equatable {
        case launch
        /// A slice under way whose agent is gone.
        case relaunch
        /// The dependencies still unfinished, by name.
        case blocked(waitingOn: [String])
    }

    let mode: Mode
    @Binding var model: String
    @Binding var effort: String
    let options: AgentOptions
    /// The branch the worktree is cut from, as nat resolves it; nil before
    /// the slice's detail is read, or with no repo to read it in.
    let base: String?
    let enabled: Bool
    let isBusy: Bool
    let onLaunch: () -> Void

    private var blocked: Bool {
        if case .blocked = mode { return true }
        return false
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .center, spacing: 6) {
                ThreadIcon(symbol: blocked ? "lock" : "play.circle")
                Text(blocked ? "Blocked" : mode == .relaunch ? "Relaunch" : "Launch")
                    .monoXS(weight: .medium)
                    .ink(blocked ? .tertiary : .secondary)
                Spacer(minLength: 0)
                Button(action: onLaunch) {
                    HeaderActionLabel(title: mode == .relaunch ? "Relaunch" : "Launch", systemImage: "arrow.right", isBusy: isBusy)
                }
                .buttonStyle(GnatButtonStyle(primary: !blocked))
                .disabled(!enabled || blocked)
            }
            .padding(.horizontal, 10)
            .padding(.top, 6)

            explanation
                .font(.system(size: 13))
                .lineSpacing(2)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 10)
                .padding(.top, 2)
                .padding(.bottom, 8)

            facts
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .background {
            if blocked { BlockedHatch().clipShape(RoundedRectangle(cornerRadius: 4)) }
        }
        .clipShape(RoundedRectangle(cornerRadius: 4))
        .overlay {
            RoundedRectangle(cornerRadius: 4).strokeBorder(
                DesignTokens.rule(.border, on: .window), style: StrokeStyle(lineWidth: 1, dash: [4, 3]))
        }
    }

    /// The Launched card's own facts, in its order: model and effort as
    /// menus, then the base as a plain value.
    private var facts: some View {
        Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 3) {
            GridRow {
                Text("model").ink(.tertiary)
                NavFactMenu(value: model) {
                    Button("Default") { model = "" }
                    ForEach(options.models, id: \.self) { option in Button(option) { model = option } }
                }
            }
            GridRow {
                Text("effort").ink(.tertiary)
                NavFactMenu(value: effort) {
                    Button("Default") { effort = "" }
                    ForEach(options.efforts, id: \.self) { option in Button(option) { effort = option } }
                }
            }
            if let base {
                GridRow {
                    Text("base").ink(.tertiary)
                    Text(base)
                        .ink(blocked ? .quaternary : .primary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
            }
        }
        .disabled(blocked)
        .monoXS()
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(blocked ? Color.clear : DesignTokens.fill(.chrome))
        .overlay(alignment: .top) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
    }

    @ViewBuilder
    private var explanation: some View {
        switch mode {
        case .launch:
            Text("Start an agent with this brief, in a worktree on a new branch. Its log appears here and its terminal opens on the right.")
                .ink(.secondary)
        case .relaunch:
            Text("Start a new agent on its branch, told it is continuing — no agent is running on it now.")
                .ink(.secondary)
        case .blocked(let names):
            let waiting = names.isEmpty ? Text("its dependencies") : names.enumerated().reduce(Text("")) { text, entry in
                text + Text(entry.offset == 0 ? "" : ", ")
                    + Text(entry.element).foregroundStyle(DesignTokens.ink(.primary, on: .window))
            }
            (Text("Waits on ") + waiting
                + Text(names.count > 1 ? ". Launch unlocks when they are done." : ". Launch unlocks when that task is done."))
                .ink(.secondary)
        }
    }
}

/// The blocked launch card's ground: faint diagonal rules, the way a
/// closed-off area is marked.
private struct BlockedHatch: View {
    var body: some View {
        Canvas { context, size in
            var path = Path()
            for x in stride(from: -size.height, to: size.width, by: 7) {
                path.move(to: CGPoint(x: x, y: size.height))
                path.addLine(to: CGPoint(x: x + size.height, y: 0))
            }
            context.stroke(path, with: .color(DesignTokens.rule(.separator, on: .window)), lineWidth: 1)
        }
    }
}

// MARK: - Dependencies

/// One dependency in the brief's depends list: its dot and name, a click
/// selecting it, and its detail up the moment the pointer is over it.
struct DependencyRow: View {
    let slice: Slice
    let state: SliceDisplayState
    let live: Bool
    let milestone: String
    let onSelect: () -> Void

    @State private var hovering = false

    var body: some View {
        Button(action: {
            hovering = false
            onSelect()
        }) {
            HStack(spacing: 6) {
                StateDot(state: state, live: live, size: 6).frame(width: 10)
                Text(slice.name)
                    .ink(state == .done ? .secondary : .primary)
                    .underline(hovering)
                    .lineLimit(1)
                    .truncationMode(.tail)
                Spacer(minLength: 0)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { hovering = $0 }
        .quietPopover(isPresented: $hovering, arrowEdge: .maxX) {
            DependencyDetailView(slice: slice, state: state, live: live, milestone: milestone)
        }
        .accessibilityHint("Selects the task")
    }
}

/// A dependency's hover detail: its whole name, where it stands, its
/// milestone and pull request, and that a click goes to it.
struct DependencyDetailView: View {
    let slice: Slice
    let state: SliceDisplayState
    let live: Bool
    let milestone: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(slice.name)
                .font(.system(size: 13, weight: .semibold))
                .ink(.primary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 6) {
                StateDot(state: state, live: live, size: 6).frame(width: 10)
                Text(state.word).ink(.secondary)
            }
            Grid(alignment: .leading, horizontalSpacing: 10, verticalSpacing: 3) {
                GridRow {
                    Text("milestone").ink(.tertiary)
                    Text(milestone.isEmpty ? "none" : milestone).ink(.primary).lineLimit(1)
                }
                if let number = pullRequestNumber(slice.pr) {
                    GridRow {
                        Text("pr").ink(.tertiary)
                        Text("#\(number)").ink(.primary)
                    }
                }
                if !slice.assignee.isEmpty {
                    GridRow {
                        Text("assignee").ink(.tertiary)
                        Text(slice.assignee).ink(.primary).lineLimit(1)
                    }
                }
            }
            .monoXS()
            Text("Click to open the task.")
                .font(.system(size: 12))
                .ink(.tertiary)
        }
        .font(.system(size: 13))
        .padding(12)
        .frame(width: 260, alignment: .leading)
    }
}
