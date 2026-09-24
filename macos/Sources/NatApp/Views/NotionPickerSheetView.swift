import SwiftUI
import NatKit

/// The sheet "Choose page…" opens — `NFNotionPicker` in
/// `docs/design/nat-new-project/ui-npflow.jsx`: where in the Notion workspace
/// the project page goes. A search field over the workspace's pages and
/// databases, the rows it finds, and Cancel beside "Create page", which makes
/// the page there and turns the mirror on (`AppModel.mirrorActiveProject`).
/// A refusal is drawn above the footer and the sheet stays up, the row still
/// chosen; the card behind it is left where it was.
struct NotionPickerSheetView: View {
    @State private var model: NotionPickerModel
    let onCancel: () -> Void
    /// Makes the page under the chosen row: nil once it has, the refusal
    /// otherwise.
    let onCreate: (NotionPlace) async -> String?
    /// Called once the page is made, to close the sheet.
    let onCreated: () -> Void

    init(
        model: NotionPickerModel,
        onCancel: @escaping () -> Void,
        onCreate: @escaping (NotionPlace) async -> String?,
        onCreated: @escaping () -> Void
    ) {
        _model = State(initialValue: model)
        self.onCancel = onCancel
        self.onCreate = onCreate
        self.onCreated = onCreated
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: 0) {
                HStack(spacing: 10) {
                    NotionMark(size: 18)
                    Text(MirrorText.pickerTitle)
                        .font(.system(size: Typo.headline, weight: .semibold))
                        .ink(.primary)
                }
                .padding(.bottom, 4)

                Text(MirrorText.pickerSubtitle)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.tertiary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.bottom, 14)

                searchField
                    .padding(.bottom, 10)
            }
            .padding(.horizontal, 20)
            .padding(.top, 18)

            results
                .padding(.horizontal, 12)
                .frame(minHeight: 132, alignment: .top)

            if let error = model.createError {
                Text(error)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.danger)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, 20)
                    .padding(.top, 8)
            }

            Rule()
                .padding(.top, 12)

            HStack(spacing: 10) {
                Spacer()
                Button("Cancel", action: onCancel)
                    .buttonStyle(SecondaryButtonStyle())
                    .keyboardShortcut(.cancelAction)
                Button(action: create) {
                    AsyncActionLabel(isBusy: model.isCreating) {
                        Text(MirrorText.create)
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .keyboardShortcut(.defaultAction)
                .disabled(!model.canCreate)
            }
            .padding(.horizontal, 20)
            .padding(.top, 14)
            .padding(.bottom, 16)
        }
        .frame(width: 440)
        .surface(.card)
        .task(id: model.query) {
            // Typing narrows the list a beat after the last key rather than on
            // every one; the first read, with nothing typed, waits for nothing.
            if !model.query.isEmpty {
                try? await Task.sleep(nanoseconds: 250_000_000)
                if Task.isCancelled { return }
            }
            await model.search()
        }
    }

    private var searchField: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass")
                .font(.system(size: 12))
                .ink(.tertiary)
            TextField(MirrorText.searchPrompt, text: $model.query)
                .textFieldStyle(.plain)
                .font(Typo.mono(size: Typo.code))
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 5)
        .field(radius: 6)
    }

    @ViewBuilder
    private var results: some View {
        if let error = model.searchError {
            Text(error)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.warning)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 8)
        } else if model.places.isEmpty {
            if model.isSearching {
                QuietLoadingView(label: "Reading the workspace\u{2026}")
                    .frame(height: 60)
            } else {
                Text(MirrorText.nothingFound)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.tertiary)
                    .padding(.horizontal, 8)
            }
        } else {
            ScrollView {
                VStack(spacing: 0) {
                    ForEach(model.places) { place in
                        NotionPlaceRow(place: place, selected: model.selectedID == place.id)
                            .onTapGesture { model.selectedID = place.id }
                    }
                }
                .inelastic()
            }
            .frame(maxHeight: 260)
        }
    }

    private func create() {
        Task {
            if await model.create(onCreate) { onCreated() }
        }
    }
}

/// One row of the picker: the page or database glyph, its title, and a
/// `database` chip on a database.
private struct NotionPlaceRow: View {
    let place: NotionPlace
    let selected: Bool
    @Environment(\.ground) private var ground

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: place.kind == .database ? "tablecells" : "doc.text")
                .font(.system(size: 13))
                .ink(selected ? .primary : .tertiary)
                .frame(width: 16)
            Text(place.title)
                .font(.system(size: Typo.body))
                .ink(.primary)
                .lineLimit(1)
                .frame(maxWidth: .infinity, alignment: .leading)
            if place.kind == .database {
                Text(MirrorText.databaseChip)
                    .font(.system(size: Typo.caption))
                    .ink(.tertiary)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 1)
                    .overlay(Capsule().stroke(DesignTokens.rule(.border, on: ground), lineWidth: 0.5))
            }
        }
        .padding(.horizontal, 10)
        .frame(height: 32)
        .background(
            RoundedRectangle(cornerRadius: 5)
                .fill(selected ? DesignTokens.wash(.selection, tone: .accent, on: ground) : Color.clear)
        )
        .contentShape(Rectangle())
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
    }
}
