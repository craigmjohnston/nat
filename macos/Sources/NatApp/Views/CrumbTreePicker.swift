import SwiftUI
import NatKit

/// The titlebar breadcrumb's tree picker: projects, then the open project's
/// milestones (its loose slices above them), then the open milestone's
/// slices — a column apiece, a column view's way. Picking a slice selects it
/// and closes the picker.
struct CrumbTreePicker: View {
    @State private var tree: CrumbTree
    let onPick: (SidebarSliceRow) -> Void

    init(tree: CrumbTree, onPick: @escaping (SidebarSliceRow) -> Void) {
        _tree = State(initialValue: tree)
        self.onPick = onPick
    }

    private static let columnWidth: CGFloat = 230
    private static let height: CGFloat = 320

    var body: some View {
        HStack(spacing: 0) {
            column {
                ForEach(tree.projects) { project in
                    row(selected: project.id == tree.projectID, opens: true) {
                        StackedFolderGlyph(
                            open: project.id == tree.projectID,
                            color: DesignTokens.ink(.tertiary, on: .header),
                            backColor: DesignTokens.ink(.tertiary, on: .header))
                            .frame(width: 16)
                        Text(project.name)
                    } action: {
                        tree.open(project: project.id)
                    }
                }
            }
            divider
            column {
                ForEach(tree.entries) { entry in
                    switch entry {
                    case .slice(let slice):
                        sliceRow(slice)
                    case .milestone(let milestone):
                        row(selected: milestone.name == tree.milestone, opens: true) {
                            FolderGlyph(
                                open: milestone.name == tree.milestone,
                                color: DesignTokens.ink(.tertiary, on: .header))
                                .frame(width: 16)
                            Text(milestone.name)
                        } action: {
                            tree.milestone = milestone.name
                        }
                    }
                }
            }
            if let slices = tree.slices {
                divider
                column {
                    ForEach(slices) { sliceRow($0) }
                }
            }
        }
        .frame(height: Self.height)
        .surface(.header)
    }

    private func column<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        ScrollView {
            VStack(spacing: 0) { content() }
                .padding(.vertical, 6)
                .inelastic()
        }
        .frame(width: Self.columnWidth)
    }

    private var divider: some View {
        DesignTokens.rule(.separator, on: .header).frame(width: 1)
    }

    private func sliceRow(_ slice: SidebarSliceRow) -> some View {
        row(selected: false, opens: false) {
            StateDot(state: slice.state, live: slice.live).frame(width: 12)
            Text(slice.title)
                .strikethrough(slice.state == .done)
                .ink(slice.state == .done || slice.state == .blocked ? .quaternary : .secondary)
        } action: {
            onPick(slice)
        }
    }

    private func row<Label: View>(
        selected: Bool, opens: Bool, @ViewBuilder label: () -> Label, action: @escaping () -> Void
    ) -> some View {
        HStack(spacing: 7) {
            label()
                .font(.system(size: GnatMetrics.body))
                .ink(.secondary)
                .lineLimit(1)
                .truncationMode(.tail)
            Spacer(minLength: 0)
            if opens {
                Image(systemName: "chevron.right")
                    .font(.system(size: 9, weight: .semibold))
                    .ink(.tertiary)
            }
        }
        .padding(.horizontal, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(selected: selected)
        .contentShape(Rectangle())
        .onTapGesture(perform: action)
    }
}
