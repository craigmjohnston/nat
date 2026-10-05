import SwiftUI
import NatKit

/// The Visual changes section's body: one row per handed-in item — its
/// viewed box, its thumbnail in a socket every row shares (a pair's after
/// alone), its name over its file's name, New while it is, and a mark while
/// comments are pending on it — the rows
/// separated by a hairline as the follow-up cards' are. Picking a row scrolls
/// the main pane's image list to it and puts the list up.
struct VisualsSectionBody: View {
    @Bindable var appModel: AppModel
    let review: VisualReview
    let slice: Slice
    let visuals: [VisualChange]
    let onPick: () -> Void

    private var store: VisualStore { review.store(appModel) }

    var body: some View {
        VStack(spacing: 0) {
            ScrollView {
                VStack(spacing: 0) {
                    ForEach(Array(visuals.enumerated()), id: \.element.id) { offset, visual in
                        row(visual)
                            .overlay(alignment: .top) {
                                if offset > 0 { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
                            }
                    }
                }
                .padding(.vertical, 4)
            }
            .thinScrollers()

            if let error = review.sendError { NavNotice(text: error) }
        }
    }

    private func row(_ visual: VisualChange) -> some View {
        let viewed = store.isViewed(sliceID: slice.id, visual)
        let commented = store.comments(for: slice.id).contains { $0.index == visual.index }
        return HStack(spacing: 8) {
            Button(action: { store.toggleViewed(sliceID: slice.id, visual) }) {
                ViewedCheckbox(checked: viewed)
            }
            .buttonStyle(.plain)
            .help(viewed ? "Mark not viewed" : "Mark viewed")
            VisualThumbnail(image: store.image(for: visual))
            VStack(alignment: .leading, spacing: 1) {
                Text(visual.name)
                    .font(.system(size: Typo.scaled(13)))
                    .ink(.primary)
                    .lineLimit(1)
                    .truncationMode(.tail)
                Text(fileName(visual.uri))
                    .monoXS()
                    .ink(.secondary)
                    .lineLimit(1)
                    .truncationMode(.head)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if let badge = store.badge(sliceID: slice.id, visual) {
                SeenBadgeChip(badge: badge)
            }
            if commented {
                Image(systemName: "text.bubble.fill")
                    .font(.system(size: 10))
                    .ink(.secondary)
                    .help("Pending comments")
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .gnatRow()
        .contentShape(Rectangle())
        .onTapGesture {
            review.requestScroll(to: visual.index)
            onPick()
        }
    }

    /// A URI's last path component — the file a render was saved as.
    private func fileName(_ uri: String) -> String {
        uri.split(separator: "/").last.map(String.init) ?? uri
    }
}

/// The socket every thumbnail sits in: one fixed slot on the chrome ground,
/// ruled round, the image fitted inside it whatever its aspect — or, where it
/// could not be opened, a photo glyph; empty while it loads.
struct VisualThumbnail: View {
    @Environment(\.ground) private var ground
    let image: VisualImage?

    static let size = CGSize(width: 48, height: 32)

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 3).fill(DesignTokens.fill(.chrome))
            switch image {
            case .image(let nsImage, _):
                Image(nsImage: nsImage)
                    .resizable()
                    .interpolation(.medium)
                    .aspectRatio(contentMode: .fit)
                    .padding(2)
            case .unavailable:
                Image(systemName: "photo")
                    .font(.system(size: 11))
                    .ink(.secondary)
            case nil:
                EmptyView()
            }
            RoundedRectangle(cornerRadius: 3).strokeBorder(DesignTokens.rule(.border, on: ground), lineWidth: 1)
        }
        .frame(width: Self.size.width, height: Self.size.height)
        .clipShape(RoundedRectangle(cornerRadius: 3))
    }
}
