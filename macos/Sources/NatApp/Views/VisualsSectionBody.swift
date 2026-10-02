import SwiftUI
import NatKit

/// The Visual changes section's body: one row per handed-in image — its
/// thumbnail, its name, and a mark while comments are pending on it. Picking
/// a row scrolls the main pane's image list to it and puts the list up.
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
                    ForEach(visuals) { row($0) }
                }
                .padding(.vertical, 4)
            }
            .thinScrollers()

            if let error = review.sendError { NavNotice(text: error) }
        }
    }

    private func row(_ visual: VisualChange) -> some View {
        let commented = store.comments(for: slice.id).contains { $0.index == visual.index }
        return HStack(spacing: 8) {
            VisualThumbnail(image: store.image(for: visual.uri))
            Text(visual.name)
                .font(.system(size: 13))
                .ink(.primary)
                .lineLimit(1)
                .frame(maxWidth: .infinity, alignment: .leading)
            if commented {
                Image(systemName: "text.bubble.fill")
                    .font(.system(size: 10))
                    .ink(.secondary)
                    .help("Pending comments")
            }
        }
        .padding(.horizontal, 10)
        .frame(height: GnatMetrics.rowHeight)
        .gnatRow()
        .contentShape(Rectangle())
        .onTapGesture {
            review.requestScroll(to: visual.index)
            onPick()
        }
    }
}

/// An image fitted into a 36×24 box — or the box alone, outlined, while it
/// loads or where it could not be opened.
struct VisualThumbnail: View {
    @Environment(\.ground) private var ground
    let image: VisualImage?

    var body: some View {
        Group {
            if case .image(let nsImage, _) = image {
                Image(nsImage: nsImage)
                    .resizable()
                    .interpolation(.medium)
                    .aspectRatio(contentMode: .fit)
                    .overlay { Rectangle().strokeBorder(DesignTokens.rule(.border, on: ground), lineWidth: 1) }
            } else {
                RoundedRectangle(cornerRadius: 2)
                    .strokeBorder(DesignTokens.rule(.border, on: ground), style: StrokeStyle(lineWidth: 1, dash: [2, 2]))
            }
        }
        .frame(width: 36, height: 24)
    }
}
