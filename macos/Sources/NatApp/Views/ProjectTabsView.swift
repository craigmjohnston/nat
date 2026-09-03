import SwiftUI
import NatKit

struct ProjectTabsView: View {
    @Bindable var appModel: AppModel

    var body: some View {
        HStack(alignment: .bottom, spacing: 0) {
            ForEach(Array(appModel.projectTabs.enumerated()), id: \.element.id) { index, tab in
                let liveCount = appModel.liveCount(projectID: tab.id)
                let isActive = appModel.activeProjectID == tab.id
                let color = colorForProject(tab.id)

                projectTabView(tab: tab, liveCount: liveCount, isActive: isActive, color: color, index: index)

                // Hairline separator between inactive tabs
                if !isActive && index + 1 < appModel.projectTabs.count {
                    let nextTab = appModel.projectTabs[index + 1]
                    let nextIsActive = appModel.activeProjectID == nextTab.id
                    if !nextIsActive {
                        Divider()
                            .frame(width: 1, height: 16)
                            .foregroundStyle(DesignTokens.labelQuaternary)
                    }
                }
            }

            // "+" button (disabled)
            Button(action: {}) {
                Image(systemName: "plus")
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(DesignTokens.labelSecondary)
                    .frame(width: 32, height: 32)
            }
            .buttonStyle(.plain)
            .disabled(true)
            .help("Projects are created from the TUI or nat project-create")
            .padding(.bottom, 6)
            .padding(.leading, 6)

            Spacer()
        }
        // The row is 40pt tall outright — the mock's own 6pt top padding is
        // what the `.bottom`-aligned 34pt tabs already leave above themselves
        // inside that frame, not an addition on top of it. Padding this again
        // on the outside was pushing the whole row 6pt below the header band
        // it is meant to fill, which is why nothing here lined up with the
        // toolbar cluster beside it.
        .frame(height: 40)
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

            // Project name
            Text(tab.name)
                .font(.system(size: Typo.subhead, weight: isActive ? .semibold : .regular))
                .foregroundStyle(isActive ? DesignTokens.label : DesignTokens.labelSecondary)
                .lineLimit(1)

            // Count badge — tight, caption-scale, tabular digits rather than
            // a switch to monospaced design (there's no code here to align).
            if liveCount > 0 {
                Text("\(liveCount)")
                    .font(.system(size: Typo.caption, weight: .regular))
                    .monospacedDigit()
                    .foregroundStyle(isActive ? DesignTokens.labelSecondary : DesignTokens.labelTertiary)
                    .padding(.horizontal, 6)
                    .background(DesignTokens.labelQuaternary)
                    .cornerRadius(8)
            }
        }
        .frame(height: 34)
        .padding(.horizontal, 22)
        .background(
            // The mock's browser tab is one silhouette: a top-rounded body
            // with two concave flares where it meets the window below — one
            // path, so the joins can never drift from the corners they curve
            // out of.
            BrowserTabShape(cornerRadius: 10, flare: 10)
                .fill(isActive ? DesignTokens.windowBg : Color.clear)
        )
        .contentShape(Rectangle())
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
    ProjectTabsView(appModel: appModel)
        .frame(height: 40)
}
