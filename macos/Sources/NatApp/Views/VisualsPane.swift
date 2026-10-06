import SwiftUI
import NatKit

/// The main pane's list of the images an agent handed in: each under a header
/// pinned while it scrolls past — its name, New while it is, its URI, its own
/// zoom (and a pair's Before / After and Highlight differences) and a comment
/// on the whole image — and each zoomable and commentable on its own. An
/// image's section coming on screen here is what sees it, taking its New.
///
/// This is the one scrolling pane SwiftUI lays out (the diff is AppKit, laid
/// out exactly — `macos/CLAUDE.md`), and that is safe only because every
/// height is known before anything draws: nothing is shown until every
/// image's pixel size is loaded, and every image is given an explicit frame
/// from it. Keep it that way — an estimated height here is the diff's old
/// jumping-scroller bug back. The comment box open at a point is laid over
/// the scroll, not in it, so its measured height is no part of the list's.
struct VisualsPane: View {
    @Bindable var appModel: AppModel
    let review: VisualReview
    let slice: Slice
    /// The slice's hand-in as its detail reads it, nil while that detail has
    /// not loaded — when nothing is loaded into the store, so the pending
    /// comments on a slice coming back to the screen are not dropped.
    let handIn: [VisualChange]?
    let authorName: String
    /// Where each image's sideways scroll starts — the gallery's seam for a
    /// zoomed image shown scrolled along.
    var horizontalAnchor: UnitPoint = .leading

    private var store: VisualStore { review.store(appModel) }
    private var visuals: [VisualChange] { handIn ?? [] }

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
        .task(id: "\(slice.id)|\(VisualChange.loadIdentity(visuals))") {
            await store.load(sliceID: slice.id, handIn: handIn)
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
                                // A folded image is its header alone, as a
                                // folded diff file is.
                                if !store.isCollapsed(sliceID: slice.id, visual) {
                                    VisualImageSection(
                                        appModel: appModel, review: review, slice: slice, visual: visual,
                                        paneWidth: paneWidth, authorName: authorName,
                                        horizontalAnchor: horizontalAnchor)
                                    // On screen at all — its frame meeting
                                    // the viewport — is seen.
                                    .onScrollVisibilityChange(threshold: 0.01) { visible in
                                        if visible { store.markSeen(sliceID: slice.id, visual) }
                                    }
                                }
                            } header: {
                                VisualHeader(appModel: appModel, review: review, slice: slice, visual: visual)
                            }
                            .id(visual.index)
                        }
                    }
                }
                .thinScrollers()
                // The comment box open at a point floats over the whole pane,
                // beside its pin wherever the scrolls have taken it — drawn
                // over the list, so it adds nothing to the scroll's content.
                .overlayPreferenceValue(VisualDraftPinKey.self) { anchor in
                    if let draft = review.draft, draft.point != nil {
                        GeometryReader { pane in
                            VisualFloatingBox(pin: anchor.map { pane[$0] }, pane: pane.size) {
                                VisualCommentBox(review: review, store: store, sliceID: slice.id, draft: draft)
                            }
                        }
                        .id(VisualCommentBox.identity(draft))
                    }
                }
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

    /// An image's fitted width in a pane `paneWidth` wide.
    static func fitWidth(paneWidth: CGFloat) -> CGFloat {
        max(paneWidth - 2 * padding, 1)
    }
}

/// One image's header: what it shows, New while it is, and where it is, a
/// mark while comments are pending on it, a pair's Before / After and
/// Highlight differences, its own zoom, and a comment on the whole of it. The
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
        let size = store.image(for: visual)?.pixelSize
        let viewed = store.isViewed(sliceID: slice.id, visual)
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                DisclosureChevron(open: !store.isCollapsed(sliceID: slice.id, visual))
                    .transaction { $0.animation = nil }
                Text(visual.name)
                    .font(.system(size: Typo.scaled(13), weight: .medium))
                    .ink(.primary)
                    .lineLimit(1)
                    .layoutPriority(1)
                if let badge = store.badge(sliceID: slice.id, visual) {
                    SeenBadgeChip(badge: badge)
                }
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
                if visual.before != nil {
                    pairControls
                }
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
                // The diff header's viewed toggle, box and word, at its
                // trailing edge.
                Button(action: { store.toggleViewed(sliceID: slice.id, visual) }) {
                    HStack(spacing: 6) {
                        ViewedCheckbox(checked: viewed)
                        Text("viewed").monoXS().ink(viewed ? .primary : .secondary)
                    }
                    .fixedSize()
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .help(viewed ? "Mark not viewed" : "Mark viewed")
            }
            .padding(.horizontal, 12)
            .frame(height: DiffMetrics().headerHeight - 1)
            .background(DesignTokens.fill(.chrome))
            .environment(\.ground, .chrome)
            // The header's bare parts fold the image, as a diff file
            // header's do.
            .contentShape(Rectangle())
            .onTapGesture { store.toggleCollapsed(sliceID: slice.id, visual) }
            DesignTokens.rule(.separator, on: .chrome).frame(height: 1)
        }
    }

    /// A pair's Before / After toggle — a side selected only while the
    /// divider is at its end — and its Highlight differences toggle, disabled
    /// with the reason as its tooltip where the two cannot be compared.
    private var pairControls: some View {
        let comparable = store.isComparable(visual)
        let refusal = store.highlightRefusal(for: visual)
        let highlighting = refusal == nil && store.isHighlighting(sliceID: slice.id, index: visual.index)
        let shown = store.shownSide(sliceID: slice.id, index: visual.index)
        return HStack(spacing: 6) {
            HStack(spacing: 0) {
                segment("Before", selected: comparable && shown == .before) {
                    store.show(.before, sliceID: slice.id, index: visual.index)
                }
                segment("After", selected: comparable && shown == .after) {
                    store.show(.after, sliceID: slice.id, index: visual.index)
                }
            }
            .padding(1)
            .background(RoundedRectangle(cornerRadius: 5).fill(DesignTokens.fill(.window)))
            .overlay { RoundedRectangle(cornerRadius: 5).strokeBorder(DesignTokens.rule(.border, on: .chrome), lineWidth: 1) }
            .disabled(!comparable)
            .help(comparable ? "Show the before or the after whole" : "The before couldn't be opened")
            Button {
                Task { await store.toggleHighlight(sliceID: slice.id, visual: visual) }
            } label: {
                HStack(spacing: 4) {
                    Image(systemName: "square.on.square.dashed")
                        .font(.system(size: 10, weight: .medium))
                    Text("Highlight").font(.system(size: Typo.scaled(11)))
                }
                .ink(highlighting ? .primary : .secondary)
                .padding(.horizontal, 6)
                .frame(height: 20)
                .background {
                    if highlighting {
                        RoundedRectangle(cornerRadius: 4).fill(DesignTokens.rowWash(selected: true, on: .chrome))
                    }
                }
            }
            .buttonStyle(GnatIconButtonStyle())
            .disabled(refusal != nil)
            .help(refusal ?? (highlighting ? "Stop highlighting differences" : "Highlight differences"))
        }
        // The URI gives way first: the controls keep their labels.
        .fixedSize()
    }

    private func segment(_ label: String, selected: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(label)
                .font(.system(size: Typo.scaled(11)))
                .ink(selected ? .primary : .secondary)
                .padding(.horizontal, 8)
                .frame(height: 18)
                .background {
                    if selected {
                        RoundedRectangle(cornerRadius: 4).fill(DesignTokens.rowWash(selected: true, on: .window))
                    }
                }
                .contentShape(Rectangle())
        }
        .buttonStyle(GnatIconButtonStyle())
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
/// alone; its pending comments pinned where they were left, and the pin of
/// one being written (its box floated by the pane); the comment box open on
/// the whole of it; and its comments listed under it as cards. A pair draws its
/// two images in one frame, the before left of a divider dragged across them
/// and the after right of it — or, where its before could not be opened, the
/// after alone.
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
                switch (store.image(for: visual), store.beforeImage(for: visual)) {
                case let (.image(after, afterSize)?, .image(before, beforeSize)?):
                    compared(before: before, beforeSize: beforeSize, after: after, afterSize: afterSize)
                case let (.image(image, pixelSize)?, _):
                    zoomable(image, pixelSize: pixelSize)
                default:
                    placeholder
                }
                if let draft = review.draft, draft.visual.index == visual.index, draft.point == nil {
                    VisualCommentBox(review: review, store: store, sliceID: slice.id, draft: draft)
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
        let width = VisualMetrics.fitWidth(paneWidth: paneWidth) * store.zoom(sliceID: slice.id, index: visual.index)
        let scale = width / pixelSize.width
        let height = pixelSize.height * scale
        return framed(width: width, height: height) {
            ZStack(alignment: .topLeading) {
                Image(nsImage: image)
                    .resizable()
                    .interpolation(.high)
                    .frame(width: width, height: height)
                    .overlay { Rectangle().strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1) }
                    .contentShape(Rectangle())
                    .onTapGesture(coordinateSpace: .local) { openDraft(at: $0, scale: scale, imageSize: pixelSize) }
                commentOverlays(scale: scale, imageSize: pixelSize)
            }
        }
    }

    /// A pair: both images at one scale from the frame's top leading corner —
    /// the frame the larger of each dimension — the before showing left of
    /// the divider and the after right of it, the differences tinted over
    /// both where they are highlighted, and the comments on the after's
    /// pixels over whichever side is showing.
    private func compared(before: NSImage, beforeSize: CGSize, after: NSImage, afterSize: CGSize) -> some View {
        let frame = VisualCompare.frameSize(afterSize, beforeSize)
        let width = VisualMetrics.fitWidth(paneWidth: paneWidth) * store.zoom(sliceID: slice.id, index: visual.index)
        let scale = width / frame.width
        let height = frame.height * scale
        let split = store.divider(sliceID: slice.id, index: visual.index) * width
        let highlighting = store.highlightRefusal(for: visual) == nil
            && store.isHighlighting(sliceID: slice.id, index: visual.index)
        let mask = highlighting ? store.mask(for: visual) : nil
        return framed(width: width, height: height) {
            ZStack(alignment: .topLeading) {
                Image(nsImage: before)
                    .resizable()
                    .interpolation(.high)
                    .frame(width: beforeSize.width * scale, height: beforeSize.height * scale)
                    .frame(width: width, height: height, alignment: .topLeading)
                    .mask(alignment: .leading) { Rectangle().frame(width: split) }
                Image(nsImage: after)
                    .resizable()
                    .interpolation(.high)
                    .frame(width: afterSize.width * scale, height: afterSize.height * scale)
                    .contentShape(Rectangle())
                    .onTapGesture(coordinateSpace: .local) { openDraft(at: $0, scale: scale, imageSize: afterSize) }
                    .frame(width: width, height: height, alignment: .topLeading)
                    .mask(alignment: .trailing) { Rectangle().frame(width: width - split) }
                if let mask {
                    DesignTokens.ink(.danger, on: .window)
                        .frame(width: afterSize.width * scale, height: afterSize.height * scale)
                        .mask {
                            Image(decorative: mask.image, scale: 1)
                                .resizable()
                                .interpolation(.none)
                        }
                        .opacity(0.75)
                        .allowsHitTesting(false)
                }
                Rectangle()
                    .strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1)
                    .frame(width: width, height: height)
                    .allowsHitTesting(false)
                commentOverlays(scale: scale, imageSize: afterSize)
                VisualDivider(height: height)
                    .position(x: split, y: height / 2)
                    .gesture(
                        DragGesture(minimumDistance: 0, coordinateSpace: .named(coordinateSpace))
                            .onChanged { value in
                                store.setDivider(
                                    VisualCompare.divider(at: value.location.x, width: width),
                                    sliceID: slice.id, index: visual.index)
                            })
            }
            .coordinateSpace(.named(coordinateSpace))
        }
    }

    private var coordinateSpace: String { "visual-pair-\(visual.index)" }

    /// An image's frame inside its own sideways scroll, pinch-zoomable.
    private func framed(width: CGFloat, height: CGFloat, @ViewBuilder content: () -> some View) -> some View {
        let zoom = store.zoom(sliceID: slice.id, index: visual.index)
        return ScrollView(.horizontal) {
            content()
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

    /// A click on the image opens a comment there — normalised by the scale
    /// first, so where it lands is the same whatever the zoom, then in the
    /// image's own pixels.
    private func openDraft(at location: CGPoint, scale: CGFloat, imageSize: CGSize) {
        let point = CGPoint(x: (location.x / scale).rounded(), y: (location.y / scale).rounded())
        review.openDraft(visual, point: point, imageSize: imageSize)
    }

    /// The pending comments' pins, and the pin of the comment being written
    /// at a point, over an image drawn at `scale` — that pin's frame
    /// published to the pane, which floats the comment box beside it.
    @ViewBuilder
    private func commentOverlays(scale: CGFloat, imageSize: CGSize) -> some View {
        let pinned = comments.enumerated().compactMap { offset, comment in
            comment.point.map { (ordinal: offset + 1, comment: comment, point: $0) }
        }
        ForEach(pinned, id: \.comment.id) { pin in
            VisualPin(ordinal: pin.ordinal)
                .position(x: pin.point.x * scale, y: pin.point.y * scale)
                .onTapGesture { review.editComment(pin.comment, visual: visual) }
                .help(pin.comment.text)
        }
        if let draft = review.draft, draft.visual.index == visual.index, let point = draft.point {
            VisualPin(ordinal: nil)
                .anchorPreference(key: VisualDraftPinKey.self, value: .bounds) { $0 }
                .position(x: point.x * scale, y: point.y * scale)
        }
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
                .font(.system(size: Typo.subhead))
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

/// The comment box: what it comments on, then the editor — at a point,
/// floated over the pane by `VisualFloatingBox`; on the whole image, at its
/// section's top trailing corner.
struct VisualCommentBox: View {
    let review: VisualReview
    let store: VisualStore
    let sliceID: String
    let draft: VisualDraft

    /// Which comment the box is open on, so its typed text survives the box
    /// being placed afresh and goes with a box opened on another.
    static func identity(_ draft: VisualDraft) -> String {
        draft.id?.uuidString ?? "\(draft.visual.index)|\(draft.point.map { "\($0.x),\($0.y)" } ?? "whole")"
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(draft.point.map { "Comment at (\(Int($0.x)), \(Int($0.y)))" } ?? "Comment on the whole image")
                .monoXS()
                .ink(.secondary)
            CommentEditorView(
                initialText: draft.text,
                onSave: { review.saveDraft($0, sliceID: sliceID, store: store) },
                onCancel: { review.draft = nil })
        }
        .padding(10)
        .frame(width: VisualMetrics.editorWidth)
        .surface(.chrome, radius: 6)
        .overlay {
            RoundedRectangle(cornerRadius: 6).strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1)
        }
        .id(Self.identity(draft))
    }
}

/// The frame of the pin of the comment being written at a point, published
/// by its image section for the pane to float the comment box beside.
struct VisualDraftPinKey: PreferenceKey {
    static let defaultValue: Anchor<CGRect>? = nil

    static func reduce(value: inout Anchor<CGRect>?, nextValue: () -> Anchor<CGRect>?) {
        value = value ?? nextValue()
    }
}

/// The comment box laid over a pane `pane` in size where
/// `VisualEditorPlacement` puts it beside `pin` (in the pane's coordinates) —
/// at its measured height, hidden until it has one. A pin the list has stopped
/// drawing (its section scrolled far off) is followed from where it was last
/// seen, so the box holds at the pane's edge and what is typed is kept. Only
/// the box takes the pointer: the rest of the pane reaches the images.
struct VisualFloatingBox<Box: View>: View {
    let pin: CGRect?
    let pane: CGSize
    @ViewBuilder let box: () -> Box

    @State private var lastPin: CGRect?
    @State private var height: CGFloat?

    var body: some View {
        ZStack(alignment: .topLeading) {
            if let at = pin ?? lastPin {
                let origin = VisualEditorPlacement.origin(
                    pin: at, boxSize: CGSize(width: VisualMetrics.editorWidth, height: height ?? 0),
                    pane: pane, inset: VisualMetrics.padding)
                box()
                    .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { height = $0 }
                    .opacity(height == nil ? 0 : 1)
                    .offset(x: origin.x, y: origin.y)
            }
        }
        .frame(width: pane.width, height: pane.height, alignment: .topLeading)
        .onChange(of: pin, initial: true) { _, pin in
            if let pin { lastPin = pin }
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

/// A pair's divider: a rule the frame's height with a round handle at its
/// middle, wearing the column-resize pointer — dragged left and right to
/// show more of the before or of the after.
struct VisualDivider: View {
    let height: CGFloat

    /// The width the divider answers the pointer across.
    static let grabWidth: CGFloat = 16
    static let handleSize: CGFloat = 22

    var body: some View {
        ZStack {
            DesignTokens.ink(.accent, on: .window).frame(width: 2, height: height)
            Circle()
                .fill(DesignTokens.ink(.accent, on: .window))
                .overlay { Circle().strokeBorder(DesignTokens.fill(.window), lineWidth: 1.5) }
                .frame(width: Self.handleSize, height: Self.handleSize)
            Image(systemName: "arrow.left.and.right")
                .font(.system(size: 9, weight: .bold))
                .ink(.onAccent)
        }
        .frame(width: Self.grabWidth, height: height)
        .contentShape(Rectangle())
        .pointerStyle(.columnResize)
        .help("Drag to compare the before and the after")
    }
}
