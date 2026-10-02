import SwiftUI
import NatKit

/// The main pane's list of the images an agent handed in: each under a header
/// pinned while it scrolls past — its name, its URI, its own zoom and a
/// comment on the whole image — and each zoomable and commentable on its own.
///
/// This is the one scrolling pane SwiftUI lays out (the diff is AppKit, laid
/// out exactly — `macos/CLAUDE.md`), and that is safe only because every
/// height is known before anything draws: nothing is shown until every
/// image's pixel size is loaded, and every image is given an explicit frame
/// from it. Keep it that way — an estimated height here is the diff's old
/// jumping-scroller bug back.
struct VisualsPane: View {
    @Bindable var appModel: AppModel
    let review: VisualReview
    let slice: Slice
    let visuals: [VisualChange]
    let authorName: String
    /// Where each image's sideways scroll starts — the gallery's seam for a
    /// zoomed image shown scrolled along.
    var horizontalAnchor: UnitPoint = .leading

    private var store: VisualStore { review.store(appModel) }

    var body: some View {
        Group {
            if visuals.isEmpty {
                MainPaneNote(text: "No images have been handed in")
            } else if !store.isLoaded(visuals) {
                QuietLoadingView(label: "Opening the images")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                list
            }
        }
        .task(id: "\(slice.id)|\(visuals.map(\.uri).joined(separator: "|"))") {
            await store.load(sliceID: slice.id, visuals: visuals)
        }
    }

    private var list: some View {
        GeometryReader { geometry in
            let paneWidth = geometry.size.width
            ScrollViewReader { proxy in
                ScrollView(.vertical) {
                    LazyVStack(spacing: 0, pinnedViews: .sectionHeaders) {
                        ForEach(visuals) { visual in
                            Section {
                                VisualImageSection(
                                    appModel: appModel, review: review, slice: slice, visual: visual,
                                    paneWidth: paneWidth, authorName: authorName,
                                    horizontalAnchor: horizontalAnchor)
                            } header: {
                                VisualHeader(appModel: appModel, review: review, slice: slice, visual: visual)
                            }
                            .id(visual.index)
                        }
                    }
                }
                .thinScrollers()
                .onChange(of: review.scrollRequest?.token, initial: true) { _, _ in
                    if let request = review.scrollRequest, request.token > review.handledScrollToken {
                        review.handledScrollToken = request.token
                        proxy.scrollTo(request.index, anchor: .top)
                    }
                }
            }
        }
    }
}

/// The geometry the image list is drawn to.
enum VisualMetrics {
    /// The air either side of an image at its fitted width, and above and
    /// below it.
    static let padding: CGFloat = 16
    /// A placeholder card's height, where an image could not be opened.
    static let placeholderHeight: CGFloat = 120
    /// A comment pin's diameter.
    static let pinSize: CGFloat = 20
    /// The comment box's width.
    static let editorWidth: CGFloat = 320
    /// What the comment box is allowed below a point before it goes above.
    static let editorRoom: CGFloat = 150

    /// An image's fitted width in a pane `paneWidth` wide.
    static func fitWidth(paneWidth: CGFloat) -> CGFloat {
        max(paneWidth - 2 * padding, 1)
    }
}

/// One image's header: what it shows and where it is, a mark while comments
/// are pending on it, its own zoom, and a comment on the whole of it. The
/// diff's file-header band, on the chrome ground.
struct VisualHeader: View {
    @Bindable var appModel: AppModel
    let review: VisualReview
    let slice: Slice
    let visual: VisualChange

    private var store: VisualStore { review.store(appModel) }

    var body: some View {
        let zoom = store.zoom(sliceID: slice.id, index: visual.index)
        let pending = store.comments(for: slice.id).contains { $0.index == visual.index }
        let size = store.image(for: visual.uri)?.pixelSize
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Text(visual.name)
                    .font(.system(size: 13, weight: .medium))
                    .ink(.primary)
                    .lineLimit(1)
                    .layoutPriority(1)
                Text(visual.uri)
                    .monoXS()
                    .ink(.secondary)
                    .lineLimit(1)
                    .truncationMode(.head)
                    .textSelection(.enabled)
                if pending {
                    Image(systemName: "text.bubble.fill")
                        .font(.system(size: 10))
                        .ink(.secondary)
                        .help("Pending comments")
                }
                Spacer(minLength: 8)
                if size != nil {
                    zoomControls(zoom)
                }
                Button {
                    review.openDraft(visual, point: nil, imageSize: size ?? .zero)
                } label: {
                    Image(systemName: "plus.bubble")
                        .font(.system(size: 12))
                        .ink(.secondary)
                        .frame(width: 22, height: 22)
                }
                .buttonStyle(GnatIconButtonStyle())
                .help("Comment on the whole image")
            }
            .padding(.horizontal, 12)
            .frame(height: DiffMetrics().headerHeight - 1)
            .background(DesignTokens.fill(.chrome))
            .environment(\.ground, .chrome)
            DesignTokens.rule(.separator, on: .chrome).frame(height: 1)
        }
    }

    private func zoomControls(_ zoom: CGFloat) -> some View {
        HStack(spacing: 2) {
            iconButton("minus", help: "Zoom out") { store.zoomOut(sliceID: slice.id, index: visual.index) }
                .disabled(zoom <= VisualStore.minZoom)
            Text("\(Int((zoom * 100).rounded()))%")
                .monoXS()
                .monospacedDigit()
                .ink(.secondary)
                .frame(minWidth: 40)
            iconButton("plus", help: "Zoom in") { store.zoomIn(sliceID: slice.id, index: visual.index) }
                .disabled(zoom >= VisualStore.maxZoom)
            iconButton("arrow.up.left.and.down.right.magnifyingglass", help: "Fit to the pane's width") {
                store.resetZoom(sliceID: slice.id, index: visual.index)
            }
            .disabled(zoom == VisualStore.fitZoom)
        }
    }

    private func iconButton(_ symbol: String, help: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: 11, weight: .medium))
                .ink(.secondary)
                .frame(width: 22, height: 22)
        }
        .buttonStyle(GnatIconButtonStyle())
        .help(help)
    }
}

/// One image under its header: drawn at its fitted width times its own zoom
/// inside a sideways scroll of its own, so zooming it past the pane moves it
/// alone; its pending comments pinned where they were left; the comment box
/// open on it; and its comments listed under it as cards.
struct VisualImageSection: View {
    @Bindable var appModel: AppModel
    let review: VisualReview
    let slice: Slice
    let visual: VisualChange
    let paneWidth: CGFloat
    let authorName: String
    let horizontalAnchor: UnitPoint

    /// The zoom a pinch started from, while one is under way.
    @State private var pinchStart: CGFloat?

    private var store: VisualStore { review.store(appModel) }
    private var comments: [PendingVisualComment] {
        store.comments(for: slice.id).filter { $0.index == visual.index }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ZStack(alignment: .topTrailing) {
                switch store.image(for: visual.uri) {
                case .image(let image, let pixelSize):
                    zoomable(image, pixelSize: pixelSize)
                default:
                    placeholder
                }
                if let draft = review.draft, draft.visual.index == visual.index, draft.point == nil {
                    editor(draft)
                        .padding(.trailing, VisualMetrics.padding)
                }
            }
            if !comments.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    ForEach(Array(comments.enumerated()), id: \.element.id) { offset, comment in
                        card(comment, ordinal: offset + 1)
                    }
                }
                .padding(.horizontal, VisualMetrics.padding)
            }
        }
        .padding(.vertical, VisualMetrics.padding)
    }

    // MARK: - The image

    private func zoomable(_ image: NSImage, pixelSize: CGSize) -> some View {
        let zoom = store.zoom(sliceID: slice.id, index: visual.index)
        let width = VisualMetrics.fitWidth(paneWidth: paneWidth) * zoom
        let height = width * pixelSize.height / pixelSize.width
        let pinned = comments.enumerated().compactMap { offset, comment in
            comment.point.map { (ordinal: offset + 1, comment: comment, point: $0) }
        }
        return ScrollView(.horizontal) {
            ZStack(alignment: .topLeading) {
                Image(nsImage: image)
                    .resizable()
                    .interpolation(.high)
                    .frame(width: width, height: height)
                    .overlay { Rectangle().strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1) }
                    .contentShape(Rectangle())
                    .onTapGesture(coordinateSpace: .local) { location in
                        // Normalised first, so where it lands is the same
                        // whatever the zoom, then in the image's own pixels.
                        let point = CGPoint(
                            x: (location.x / width * pixelSize.width).rounded(),
                            y: (location.y / height * pixelSize.height).rounded())
                        review.openDraft(visual, point: point, imageSize: pixelSize)
                    }
                ForEach(pinned, id: \.comment.id) { pin in
                    VisualPin(ordinal: pin.ordinal)
                        .position(
                            x: pin.point.x / pixelSize.width * width,
                            y: pin.point.y / pixelSize.height * height)
                        .onTapGesture { review.editComment(pin.comment, visual: visual) }
                        .help(pin.comment.text)
                }
                if let draft = review.draft, draft.visual.index == visual.index, let point = draft.point {
                    let at = CGPoint(x: point.x / pixelSize.width * width, y: point.y / pixelSize.height * height)
                    VisualPin(ordinal: nil).position(at)
                    editor(draft)
                        .offset(
                            x: min(max(at.x - VisualMetrics.editorWidth / 2, 0), max(width - VisualMetrics.editorWidth, 0)),
                            y: at.y + VisualMetrics.editorRoom > height
                                ? max(at.y - VisualMetrics.editorRoom - VisualMetrics.pinSize, 0)
                                : at.y + VisualMetrics.pinSize)
                }
            }
            .frame(width: width, height: height, alignment: .topLeading)
            .padding(.horizontal, VisualMetrics.padding)
        }
        .defaultScrollAnchor(horizontalAnchor)
        .thinScrollers(.horizontal)
        .frame(width: paneWidth, height: height)
        .gesture(
            MagnifyGesture()
                .onChanged { value in
                    let start = pinchStart ?? zoom
                    pinchStart = start
                    store.setZoom(start * value.magnification, sliceID: slice.id, index: visual.index)
                }
                .onEnded { _ in pinchStart = nil })
    }

    /// Where an image could not be opened: a dashed card naming its URI, the
    /// same fixed height every time so the list never reflows over it.
    private var placeholder: some View {
        VStack(spacing: 6) {
            Image(systemName: "photo")
                .font(.system(size: 18))
                .ink(.tertiary)
            Text(visual.uri)
                .monoXS()
                .ink(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
                .textSelection(.enabled)
            Text("couldn't be opened")
                .font(.system(size: 12))
                .ink(.tertiary)
        }
        .padding(.horizontal, 16)
        .frame(maxWidth: .infinity)
        .frame(height: VisualMetrics.placeholderHeight)
        .overlay {
            RoundedRectangle(cornerRadius: 6)
                .strokeBorder(DesignTokens.rule(.border, on: .window), style: StrokeStyle(lineWidth: 1, dash: [4, 3]))
        }
        .padding(.horizontal, VisualMetrics.padding)
    }

    // MARK: - Comments

    private func editor(_ draft: VisualDraft) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(draft.point.map { "Comment at (\(Int($0.x)), \(Int($0.y)))" } ?? "Comment on the whole image")
                .monoXS()
                .ink(.secondary)
            CommentEditorView(
                initialText: draft.text,
                onSave: { review.saveDraft($0, sliceID: slice.id, store: store) },
                onCancel: { review.draft = nil })
        }
        .padding(10)
        .frame(width: VisualMetrics.editorWidth)
        .surface(.chrome, radius: 6)
        .overlay {
            RoundedRectangle(cornerRadius: 6).strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1)
        }
        .id(draft.id?.uuidString ?? "\(draft.visual.index)|\(draft.point.map { "\($0.x),\($0.y)" } ?? "whole")")
    }

    private func card(_ comment: PendingVisualComment, ordinal: Int) -> some View {
        HStack(alignment: .top, spacing: 8) {
            if comment.point != nil {
                VisualPin(ordinal: ordinal).padding(.top, 6)
            } else {
                Image(systemName: "photo")
                    .font(.system(size: 11))
                    .ink(.tertiary)
                    .frame(width: VisualMetrics.pinSize, height: VisualMetrics.pinSize)
                    .padding(.top, 6)
            }
            PendingCommentCardView(
                authorName: authorName, initials: initialsFor(authorName), text: comment.text,
                meta: comment.placement,
                onEdit: { review.editComment(comment, visual: visual) },
                onDelete: { review.deleteComment(comment, sliceID: slice.id, store: store) })
        }
    }
}

/// A comment's pin on an image: a filled circle in the accent with its
/// 1-based number among the image's comments — or, for the comment being
/// written, none.
struct VisualPin: View {
    let ordinal: Int?

    var body: some View {
        ZStack {
            Circle().fill(DesignTokens.ink(.accent, on: .window))
            Circle().strokeBorder(DesignTokens.fill(.window), lineWidth: 1.5)
            if let ordinal {
                Text("\(ordinal)")
                    .font(.system(size: 10, weight: .bold))
                    .ink(.onAccent)
            }
        }
        .frame(width: VisualMetrics.pinSize, height: VisualMetrics.pinSize)
    }
}
