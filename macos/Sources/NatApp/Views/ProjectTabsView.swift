import SwiftUI
import NatKit

struct ProjectTabsView: View {
    @Bindable var appModel: AppModel

    /// What the "+" beside the strip does: open the sheet offering the two
    /// ways a project comes to be on the board. The sheet is the shell's, not
    /// this row's — a sheet presented from inside the header band would be
    /// anchored to a 40pt strip.
    let onNewProject: () -> Void
    /// The tab the mouse is over, for the hover wash — parent state rather
    /// than per-tab, because the tabs are built by a function and a function
    /// has no `@State` of its own to keep.
    @State private var hoveredTabID: String?

    var body: some View {
        HStack(alignment: .bottom, spacing: 0) {
            ForEach(Array(appModel.projectTabs.enumerated()), id: \.element.id) { index, tab in
                let liveCount = appModel.liveCount(projectID: tab.id)
                let isActive = appModel.activeProjectID == tab.id
                let color = colorForProject(tab.id)

                projectTabView(tab: tab, liveCount: liveCount, isActive: isActive, color: color, index: index)

                // Hairline separator after an inactive tab, unless the next
                // one is active (the raised tab is its own edge). The last
                // tab has no next, so an inactive one carries the divider —
                // the mock's rule, and what puts a divider between the strip
                // and the "+" beside it.
                let nextIsActive = index + 1 < appModel.projectTabs.count
                    && appModel.activeProjectID == appModel.projectTabs[index + 1].id
                if !isActive && !nextIsActive {
                    Rectangle()
                        .fill(DesignTokens.labelQuaternary)
                        .frame(width: 1, height: 16)
                        // Centered on the band's content line, like the
                        // tab labels beside it — bottom-aligned it hung
                        // to the very foot of the band.
                        .padding(.bottom, 12)
                }
            }

            // "+" — the two ways a project comes to be on the board, both
            // behind one sheet.
            Button(action: onNewProject) {
                Image(systemName: "plus")
                    .font(.system(size: 14, weight: .medium))
                    .ink(.secondary)
                    .frame(width: 32, height: 32)
            }
            .buttonStyle(.plain)
            .hoverWash(cornerRadius: 7)
            .help("Open or Create a Project…")
            // 4pt rather than the tabs' 6: the box is 32 in a 40pt band, so
            // 4 is what puts its icon on the same content line the tab
            // labels and the toolbar cluster sit on.
            .padding(.bottom, 4)
            .padding(.leading, 6)

            Spacer()
        }
        // The row is 40pt tall outright — the mock's own 6pt top padding is
        // what the `.bottom`-aligned 34pt tabs already leave above themselves
        // inside that frame, not an addition on top of it. The alignment
        // matters as much as the height: a frame centers by default, which
        // floated the whole strip 2pt high of the band and took every label
        // off the toolbar cluster's line — `.bottom` is what actually seats
        // the tabs on the band's foot the way the HStack's own `.bottom`
        // already promised.
        .frame(height: 40, alignment: .bottom)
    }

    @ViewBuilder
    private func projectTabView(
        tab: (id: String, name: String),
        liveCount: Int,
        isActive: Bool,
        color: Color?,
        index: Int
    ) -> some View {
        HStack(spacing: 7) {
            // Colored dot (pulsing when live sessions exist)
            if let color = color {
                if liveCount > 0 && !isActive {
                    Circle()
                        .fill(color)
                        .frame(width: 8, height: 8)
                        .modifier(PulseModifier())
                } else {
                    Circle()
                        .fill(color)
                        .frame(width: 8, height: 8)
                }
            }

            // Project name. The width is reserved at semibold whichever
            // weight is drawn — activating a tab bolds its label, and a
            // label measured at its own weight resized the whole tab with
            // every switch.
            Text(tab.name)
                .font(.system(size: Typo.subhead, weight: .semibold))
                .lineLimit(1)
                .opacity(0)
                .overlay(alignment: .leading) {
                    Text(tab.name)
                        .font(.system(size: Typo.subhead, weight: isActive ? .semibold : .regular))
                        .ink(isActive ? .primary : .secondary)
                        .lineLimit(1)
                }

            // Count badge — tight, caption-scale, tabular digits rather than
            // a switch to monospaced design (there's no code here to align).
            if liveCount > 0 {
                Text("\(liveCount)")
                    .font(.system(size: Typo.caption, weight: .regular))
                    .monospacedDigit()
                    .ink(isActive ? .secondary : .tertiary)
                    .padding(.horizontal, 6)
                    .background(DesignTokens.labelQuaternary)
                    .cornerRadius(8)
            }

            // Close button, browser-fashion: always there on the active tab,
            // shown on hover elsewhere, and not there at all on the last tab
            // standing — the strip never closes to nothing. Opacity rather
            // than removal for the hover case, so a tab's width never jumps
            // as the mouse crosses it.
            if appModel.projectTabs.count > 1 {
                Button(action: {
                    Task { await appModel.closeProject(tab.id) }
                }) {
                    Image(systemName: "xmark")
                        .font(.system(size: 9, weight: .semibold))
                        .ink(.tertiary)
                        .frame(width: 16, height: 16)
                }
                .buttonStyle(.plain)
                .hoverWash(cornerRadius: 4)
                .opacity(isActive || hoveredTabID == tab.id ? 1 : 0)
                .help("Close Tab")
            }
        }
        // The mock's tab carries 6px of its own bottom padding, which is
        // what sets its content on the band's center line — the line the
        // toolbar cluster and the slice count already sit on.
        .padding(.bottom, 6)
        .frame(height: 34)
        .padding(.horizontal, 22)
        // The mock's tab bounds: no narrower than 130 whatever its name,
        // no wider than 220 however long — padding included, as the mock's
        // border-box is.
        .frame(minWidth: 130, maxWidth: 220, alignment: .leading)
        .background(
            // The mock's browser tab is one silhouette: a top-rounded body
            // with two concave flares where it meets the window below — one
            // path, so the joins can never drift from the corners they curve
            // out of. An inactive tab under the mouse takes the hover wash
            // in this same silhouette, so what lights up is what a click
            // would raise.
            BrowserTabShape(cornerRadius: 10, flare: 10)
                .fill(
                    isActive
                        ? DesignTokens.fill(.window)
                        : (hoveredTabID == tab.id ? DesignTokens.fill(.hover) : Color.clear)
                )
        )
        .contentShape(Rectangle())
        .onHover { inside in
            if inside {
                hoveredTabID = tab.id
            } else if hoveredTabID == tab.id {
                hoveredTabID = nil
            }
        }
        .onTapGesture {
            Task {
                await appModel.activateProject(tab.id)
            }
        }
    }

    /// Return a color for each project (cycling through a palette).
    private func colorForProject(_ projectID: String) -> Color? {
        let colors: [Color] = [
            DesignTokens.systemOrange,
            DesignTokens.systemGreen,
            DesignTokens.systemYellow,
            DesignTokens.systemRed
        ]

        // Use hash to consistently map projects to colors
        let hash = projectID.hashValue
        let index = abs(hash) % colors.count
        return colors[index]
    }
}

// MARK: - Helpers

/// The mock's browser tab as one path (`ui-v2-shell.jsx`'s `VProjectTabs`,
/// which builds it from a top-rounded rectangle plus two radial-gradient
/// corner pieces): the body's top corners round inward by `cornerRadius`,
/// and its bottom edge flares outward by `flare` on each side through a
/// concave quarter-arc, which is the curve that merges the tab into the
/// window band below it. The flares live inside this shape's own rect —
/// the tab's content padding is what leaves them room.
struct BrowserTabShape: Shape {
    let cornerRadius: CGFloat
    let flare: CGFloat

    func path(in rect: CGRect) -> Path {
        var p = Path()
        let r = cornerRadius
        let f = flare
        let left = rect.minX + f
        let right = rect.maxX - f

        p.move(to: CGPoint(x: rect.minX, y: rect.maxY))
        // Concave flare up into the body's left edge.
        p.addArc(center: CGPoint(x: rect.minX, y: rect.maxY - f), radius: f,
                 startAngle: .degrees(90), endAngle: .degrees(0), clockwise: true)
        p.addLine(to: CGPoint(x: left, y: rect.minY + r))
        // Rounded top-left corner.
        p.addArc(center: CGPoint(x: left + r, y: rect.minY + r), radius: r,
                 startAngle: .degrees(180), endAngle: .degrees(270), clockwise: false)
        p.addLine(to: CGPoint(x: right - r, y: rect.minY))
        // Rounded top-right corner.
        p.addArc(center: CGPoint(x: right - r, y: rect.minY + r), radius: r,
                 startAngle: .degrees(270), endAngle: .degrees(0), clockwise: false)
        p.addLine(to: CGPoint(x: right, y: rect.maxY - f))
        // Concave flare back out to the band's bottom on the right.
        p.addArc(center: CGPoint(x: rect.maxX, y: rect.maxY - f), radius: f,
                 startAngle: .degrees(180), endAngle: .degrees(90), clockwise: true)
        p.closeSubpath()
        return p
    }
}

struct PulseModifier: ViewModifier {
    @State private var isAnimating = false

    func body(content: Content) -> some View {
        content
            .opacity(isAnimating ? 0.6 : 1)
            .animation(
                Animation.easeInOut(duration: 1.5)
                    .repeatForever(autoreverses: true),
                value: isAnimating
            )
            .onAppear {
                isAnimating = true
            }
    }
}


#Preview {
    let appModel = AppModel()
    ProjectTabsView(appModel: appModel, onNewProject: {})
        .frame(height: 40)
}
