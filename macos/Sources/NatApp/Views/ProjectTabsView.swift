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
                    .font(.system(size: 14, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
            .frame(width: 32, height: 32)
            .cornerRadius(7)
            .disabled(true)
            .help("Projects are created from the TUI or nat project-create")
            .padding(.bottom, 6)
            .padding(.leading, 6)

            Spacer()
        }
        .frame(height: 40)
        .padding(.top, 6)
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
                .font(.system(size: 12, weight: isActive ? .semibold : .regular))
                .foregroundStyle(isActive ? DesignTokens.label : DesignTokens.labelSecondary)
                .lineLimit(1)

            // Count badge
            if liveCount > 0 {
                Text("\(liveCount)")
                    .font(.system(size: 10, weight: .regular, design: .monospaced))
                    .foregroundStyle(isActive ? DesignTokens.labelSecondary : DesignTokens.labelTertiary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(DesignTokens.labelQuaternary)
                    .cornerRadius(8)
            }
        }
        .frame(height: 34)
        .padding(.horizontal, 22)
        .padding(.bottom, 6)
        .background(
            // Only the top corners round — the mock's own "10px 10px 0 0" —
            // so the bottom edge is flat where the two corner-notch pieces
            // below meet it; a corner rounded on all four sides would leave a
            // gap between the tab's own curve and theirs.
            UnevenRoundedRectangle(topLeadingRadius: isActive ? 10 : 0, topTrailingRadius: isActive ? 10 : 0)
                .fill(isActive ? DesignTokens.windowBg : Color.clear)
        )
        .overlay(alignment: .bottomLeading) {
            if isActive {
                TabCornerNotch(circleAt: .topLeading)
                    .eoFilled(DesignTokens.windowBg)
                    .frame(width: 10, height: 10)
                    .offset(x: -10)
            }
        }
        .overlay(alignment: .bottomTrailing) {
            if isActive {
                TabCornerNotch(circleAt: .topTrailing)
                    .eoFilled(DesignTokens.windowBg)
                    .frame(width: 10, height: 10)
                    .offset(x: 10)
            }
        }
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

/// The active tab's own curved joins where it meets the window body below —
/// two of these flank it, one on each side, per the mock's radial-gradient
/// corner pieces (`ui-v2-shell.jsx`'s `VProjectTabs`): a small square filled
/// solid except for a quarter-circle cut out of one corner, which is what
/// makes the cut read as a concave curve rather than a straight diagonal.
///
/// `circleAt` is which corner of this piece's own square the cutout is
/// centered on — `.topLeading` for the piece sitting to the tab's left,
/// `.topTrailing` for the one to its right, so the curve opens toward the
/// tab immediately above it in both cases and the piece's own outer corner
/// (away from the tab) stays square, flush with the header's own bottom edge.
struct TabCornerNotch: Shape {
    enum CircleCorner {
        case topLeading
        case topTrailing
    }

    let circleAt: CircleCorner

    func path(in rect: CGRect) -> Path {
        var path = Path()
        path.addRect(rect)

        let center: CGPoint
        switch circleAt {
        case .topLeading: center = CGPoint(x: rect.minX, y: rect.minY)
        case .topTrailing: center = CGPoint(x: rect.maxX, y: rect.minY)
        }
        let radius = rect.width
        path.addEllipse(in: CGRect(
            x: center.x - radius, y: center.y - radius,
            width: radius * 2, height: radius * 2
        ))

        return path
    }
}

extension TabCornerNotch {
    /// Filled with the even-odd rule, so the rect and the circle cancel out
    /// where they overlap — this is the shape's whole point (`Shape`'s own
    /// plain `fill()` would just paint the union of the two, no cutout at
    /// all, which is why this is a differently-named method rather than an
    /// overload of it).
    func eoFilled<S: ShapeStyle>(_ style: S) -> some View {
        self.fill(style, style: FillStyle(eoFill: true))
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
