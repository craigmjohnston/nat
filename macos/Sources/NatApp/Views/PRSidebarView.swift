import AppKit
import SwiftUI
import NatKit

/// The PR tab's right sidebar: checks, the review decision, and the changes
/// — a hairline-bordered rail beside the main column, GitHub's own layout for
/// a pull request's sidebar.
struct PRSidebarView: View {
    let pr: PRDetail

    /// The sidebar's width, draggable at its divider and remembered across
    /// launches — the default is the width it was fixed at before it was
    /// resizable.
    @AppStorage("prSidebarWidth") private var sidebarWidth = 216.0

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                checksSection
                reviewSection
                changesSection
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 18)
        }
        .frame(width: sidebarWidth)
        .rectBorder(width: 0.5, edges: [.leading], color: DesignTokens.separator)
        .overlay(alignment: .leading) {
            PaneResizeHandle(width: $sidebarWidth, minWidth: 170, maxWidth: 400, edge: .leading)
                .offset(x: -4.5)
        }
    }

    // MARK: - Checks

    private var checksSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(checksHeading)
                .font(.system(size: Typo.subhead, weight: .semibold))
                .monospacedDigit()
                .foregroundStyle(DesignTokens.labelTertiary)

            if pr.checks.isEmpty {
                Text("No checks have run on this pull request.")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            } else {
                ForEach(Array(pr.checks.enumerated()), id: \.offset) { _, check in
                    checkRow(check)
                }
            }
        }
    }

    private var checksHeading: String {
        guard !pr.checks.isEmpty else { return "CHECKS" }
        let done = pr.checks.filter { checkOutcome(state: $0.state) != .pending }.count
        return "CHECKS · \(done) OF \(pr.checks.count)"
    }

    // The mock's check row: 26pt tall, the name in a code face one step
    // under `Typo.code`, and the right-hand column in caption type — where
    // the mock shows each check's duration, gh's reading carries only its
    // state, so the state's word takes that column rather than a number
    // invented to fill it.
    private func checkRow(_ check: PRCheck) -> some View {
        let outcome = checkOutcome(state: check.state)
        return HStack(spacing: 8) {
            Image(systemName: outcome.markSymbolName)
                .font(.system(size: 12, weight: .medium))
                .foregroundStyle(outcome.tint)
                .symbolEffect(.pulse, isActive: outcome == .pending)

            Text(check.name)
                .font(.system(size: Typo.code - 1, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.label)
                .lineLimit(1)
                .truncationMode(.tail)

            Spacer(minLength: 6)

            Text(checkStateWord(check.state))
                .font(.system(size: Typo.caption, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)
        }
        .frame(height: 26)
    }

    // MARK: - Review

    private var reviewSection: some View {
        let verdict = reviewVerdict(reviewDecision: pr.reviewDecision)
        return VStack(alignment: .leading, spacing: 8) {
            Text("REVIEW")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            HStack(spacing: 8) {
                Image(systemName: verdict.outcome.markSymbolName)
                    .font(.system(size: 12, weight: .medium))
                    .foregroundStyle(verdict.outcome.tint)

                Text(reviewLine(verdict))
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.label)
            }
            .frame(height: 26)

            // nat has no reviewer-request flow of its own — GitHub's is the
            // only one, so this opens the pull request there rather than
            // pretending to add one from here.
            HStack(spacing: 8) {
                Image(systemName: "plus.circle")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundStyle(DesignTokens.labelTertiary)

                Text("Add Reviewer…")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
            .frame(height: 26)
            .contentShape(Rectangle())
            .onTapGesture { openPROnGitHub() }
        }
    }

    /// The mock's "Approved by craig" — the verdict word, crediting the
    /// approver where a submitted review names one; every other verdict is
    /// the word alone, since only an approval has a single author to name.
    private func reviewLine(_ verdict: MergeVerdict) -> String {
        if verdict.outcome == .passing, let author = approvedBy(reviews: pr.reviews) {
            return "\(sentenceCase(verdict.word)) by \(author)"
        }
        return sentenceCase(verdict.word)
    }

    private func openPROnGitHub() {
        guard let url = URL(string: pr.url) else { return }
        NSWorkspace.shared.open(url)
    }

    // MARK: - Changes

    private var changesSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("CHANGES")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            // Additions/deletions/changed files/commits are only sent by a
            // `nat` new enough to carry them; an older one simply omits the
            // keys, which decodes as `nil` — shown honestly as the two branch
            // names alone rather than a number invented to fill the space.
            if let additions = pr.additions, let deletions = pr.deletions {
                HStack(spacing: 4) {
                    Text("+\(additions)")
                        .foregroundStyle(DesignTokens.systemGreen)
                    Text("\u{2212}\(deletions)")
                        .foregroundStyle(DesignTokens.systemRed)
                    if let changedFiles = pr.changedFiles {
                        Text("· \(changedFiles) \(plural(changedFiles, "file", "files"))")
                            .foregroundStyle(DesignTokens.labelSecondary)
                    }
                }
                .font(.system(size: Typo.subhead, weight: .regular))
                .monospacedDigit()

                // The mock sets this line in the sidebar's own subheadline
                // with just the branch name in the code face — a sentence
                // about the branch, not a line of code.
                if let commits = pr.commits {
                    (Text("\(commits) \(plural(commits, "commit", "commits")) on ")
                        + Text(pr.headRefName)
                        .font(.system(size: Typo.code - 1, weight: .regular, design: .monospaced)))
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .foregroundStyle(DesignTokens.labelSecondary)
                }
            } else {
                Text("\(pr.headRefName) → \(pr.baseRefName)")
                    .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
        }
    }
}

/// Capitalizes just the first letter of an already-lower-cased phrase, for a
/// sidebar line drawn as a short sentence ("Approved", "Review required")
/// rather than the merge box's own lower-case verdict word.
func sentenceCase(_ word: String) -> String {
    guard let first = word.first else { return word }
    return first.uppercased() + word.dropFirst()
}
