import SwiftUI
import NatKit

/// The chrome every pane opens with: an identity block — an optional
/// breadcrumb over a title that wraps rather than truncates — beside whatever
/// the pane has to put on the right, on the window's own ground with a
/// hairline under it.
///
/// One header rather than one per pane, because two panes that open
/// differently read as two applications: the slice pane had the identity
/// block and the pipeline stepper, and the workshop pane a bare 46pt title
/// row over a rule, so switching between them moved the title and changed the
/// ground under it.
///
/// That ground is the rail's (`.surface(.window)`) rather than the `.band` the
/// slice pane's header used to paint: the header runs along the top of the
/// window beside the rail, and the two reading as one band is what stops the
/// pane looking pasted onto the window.
///
/// The height is the same whether or not there is a breadcrumb
/// (`PaneHeaderMetrics.minHeight`), so the workshop's title — and a slice
/// whose milestone cannot be named — opens exactly where a slice's does.
struct PaneHeader<Trailing: View>: View {
    /// The line above the title, or nil for a pane that has nothing to say
    /// there. A nil drops the line rather than drawing a blank one.
    var breadcrumb: String?
    let title: String
    @ViewBuilder let trailing: () -> Trailing

    init(
        breadcrumb: String? = nil,
        title: String,
        @ViewBuilder trailing: @escaping () -> Trailing = { EmptyView() }
    ) {
        self.breadcrumb = breadcrumb
        self.title = title
        self.trailing = trailing
    }

    var body: some View {
        HStack(alignment: .center, spacing: PaneHeaderMetrics.spacing) {
            VStack(alignment: .leading, spacing: PaneHeaderMetrics.identitySpacing) {
                if let breadcrumb {
                    Text(breadcrumb)
                        .font(.system(size: Typo.caption))
                        .ink(.tertiary)
                        .lineLimit(1)
                }

                Text(title)
                    .font(.system(size: Typo.headline, weight: .semibold))
                    .ink(.primary)
                    .lineLimit(2)
                    .multilineTextAlignment(.leading)
            }

            Spacer()

            trailing()
        }
        .padding(.vertical, PaneHeaderMetrics.verticalPadding)
        .padding(.horizontal, PaneHeaderMetrics.horizontalPadding)
        .frame(minHeight: PaneHeaderMetrics.minHeight)
        .frame(maxWidth: .infinity)
        .surface(.window)
        .overlay(alignment: .bottom) {
            Rule(.hairline)
        }
    }
}

#Preview {
    VStack(spacing: 0) {
        PaneHeader(breadcrumb: "M38: App fixes", title: "Match the workshop pane's header to the slice pane's") {
            Text("stepper")
                .font(.system(size: Typo.subhead))
                .ink(.secondary)
        }
        PaneHeader(title: "Workshop")
        Spacer()
    }
    .frame(width: 700, height: 240)
}
