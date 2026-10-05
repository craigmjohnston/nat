import SwiftUI
import NatKit

/// A row glyph stacked on itself — the "every check" form of a check row's
/// re-run or cancel, for the Checks heading. A second copy stands behind the
/// glyph, up and to the right, and the front one is knocked out of its lines
/// by a point all round, as the Done folder's check is cut out of its folder.
/// Drawn because SF Symbols (macOS 15) has no stacked `arrow.clockwise` or
/// `xmark` to pair with the rows'.
struct StackedGlyph: View {
    let systemName: String
    var size: CGFloat = 10
    var weight: Font.Weight = .semibold

    /// The knock-out's copies, on a circle a point out (see `DoneFolderGlyph`).
    private static let halo: [CGSize] = (0..<16).map { step in
        let angle = Double(step) * .pi / 8
        return CGSize(width: cos(angle), height: sin(angle))
    }

    var body: some View {
        let side = size * 1.35
        ZStack {
            glyph
                .frame(width: side, height: side, alignment: .topTrailing)
                .opacity(0.6)
                .mask {
                    ZStack {
                        Rectangle()
                        ForEach(Array(Self.halo.enumerated()), id: \.offset) { _, nudge in
                            glyph.offset(nudge)
                                .frame(width: side, height: side, alignment: .bottomLeading)
                                .blendMode(.destinationOut)
                        }
                    }
                    .compositingGroup()
                }
            glyph.frame(width: side, height: side, alignment: .bottomLeading)
        }
        .frame(width: side, height: side)
    }

    private var glyph: some View {
        Image(systemName: systemName).font(.system(size: size, weight: weight))
    }
}

/// The glyphs the check controls draw: a refresh to re-run, a cross to
/// cancel — one check's on its row, every check's stacked on the heading.
enum ChecksGlyph {
    static let rerun = "arrow.clockwise"
    static let cancel = "xmark"
}

/// One check control's slot: a fixed square at the row's trailing edge, so
/// the rows' buttons and the heading's line up in the same columns — the
/// glyph, or a spinner while its own call is under way.
struct CheckControlSlot<Glyph: View>: View {
    let busy: Bool
    @ViewBuilder let glyph: () -> Glyph

    static var side: CGFloat { 18 }

    var body: some View {
        Group {
            if busy {
                ProgressView().controlSize(.mini)
            } else {
                glyph().ink(.secondary)
            }
        }
        .frame(width: Self.side, height: Self.side)
        .contentShape(Rectangle())
    }
}

/// The Checks heading with its two controls at the trailing edge: Re-run, a
/// menu of Re-run all and Re-run failed, and Cancel. Both absent where no
/// check has an Actions run behind it.
struct ChecksHeading: View {
    let controls: ChecksControls
    let store: PRStore?

    var body: some View {
        HStack(spacing: 2) {
            NavHeading(text: "Checks")
            Spacer(minLength: 4)
            if let store, controls.hasControls {
                let busy = store.checksActionSource != nil
                Menu {
                    Button("Re-run all") { run(store) { await $0.rerunChecks(.all, from: .rerunAll) } }
                    Button("Re-run failed") { run(store) { await $0.rerunChecks(.failed, from: .rerunAll) } }
                        .disabled(!controls.rerunFailed)
                } label: {
                    CheckControlSlot(busy: store.checksActionSource == .rerunAll) {
                        StackedGlyph(systemName: ChecksGlyph.rerun)
                    }
                }
                .menuStyle(.button)
                .buttonStyle(GnatIconButtonStyle())
                .menuIndicator(.hidden)
                .fixedSize()
                .disabled(busy || !controls.rerunAll)
                .help(controls.rerunAll ? "Re-run checks" : "No check has run yet")

                Button { run(store) { await $0.cancelChecks([], from: .cancelAll) } } label: {
                    CheckControlSlot(busy: store.checksActionSource == .cancelAll) {
                        StackedGlyph(systemName: ChecksGlyph.cancel)
                    }
                }
                .buttonStyle(GnatIconButtonStyle())
                .disabled(busy || !controls.cancelAll)
                .help(controls.cancelAll ? "Cancel every check still running" : "No check is running")
            }
        }
    }
}

/// A check row's two controls: re-run and cancel its own job.
struct CheckRowControls: View {
    let check: PRCheck
    let controls: ChecksControls
    let store: PRStore

    var body: some View {
        let busy = store.checksActionSource != nil
        let rerun = controls.rerun(check)
        let cancel = controls.cancel(check)
        HStack(spacing: 2) {
            Button { run(store) { await $0.rerunChecks(.checks([check.name]), from: .rerun(check.name)) } } label: {
                CheckControlSlot(busy: store.checksActionSource == .rerun(check.name)) {
                    Image(systemName: ChecksGlyph.rerun).font(.system(size: 10, weight: .medium))
                }
            }
            .buttonStyle(GnatIconButtonStyle())
            .disabled(busy || !rerun.enabled)
            .help(controls.rerunHelp(check))

            Button { run(store) { await $0.cancelChecks([check.name], from: .cancel(check.name)) } } label: {
                CheckControlSlot(busy: store.checksActionSource == .cancel(check.name)) {
                    Image(systemName: ChecksGlyph.cancel).font(.system(size: 10, weight: .medium))
                }
            }
            .buttonStyle(GnatIconButtonStyle())
            .disabled(busy || !cancel.enabled)
            .help(controls.cancelHelp(check))
        }
    }
}

/// Starts one checks call on the store, off the button's own action.
@MainActor
private func run(_ store: PRStore, _ call: @escaping @MainActor (PRStore) async -> Void) {
    Task { await call(store) }
}
