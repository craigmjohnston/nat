import AppKit
import SwiftUI
import NatKit

/// The PR tab's right sidebar: checks, the review decision, and the changes
/// — a hairline-bordered rail beside the main column, GitHub's own layout for
/// a pull request's sidebar.
struct PRSidebarView: View {
    let pr: PRDetail

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                checksSection
                reviewSection
                changesSection
            }
            .padding(14)
        }
        .frame(width: 216)
        .rectBorder(width: 0.5, edges: [.leading], color: DesignTokens.separator)
    }

    // MARK: - Checks

    private var checksSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(checksHeading)
                .font(.system(size: 11, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            if pr.checks.isEmpty {
                Text("No checks have run on this pull request.")
                    .font(.system(size: 12, weight: .regular))
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

    private func checkRow(_ check: PRCheck) -> some View {
        let outcome = checkOutcome(state: check.state)
        return HStack(spacing: 8) {
            Image(systemName: outcome.markSymbolName)
                .font(.system(size: 12, weight: .regular))
                .foregroundStyle(outcome.tint)
                .symbolEffect(.pulse, isActive: outcome == .pending)

            Text(check.name)
                .font(.system(size: 11, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.label)
                .lineLimit(1)
                .truncationMode(.tail)

            Spacer(minLength: 6)

            Text(checkStateWord(check.state))
                .font(.system(size: 11, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)
        }
        .frame(height: 22)
    }

    // MARK: - Review

    private var reviewSection: some View {
        let verdict = reviewVerdict(reviewDecision: pr.reviewDecision)
        return VStack(alignment: .leading, spacing: 6) {
            Text("REVIEW")
                .font(.system(size: 11, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            HStack(spacing: 8) {
                Image(systemName: verdict.outcome.markSymbolName)
                    .font(.system(size: 12, weight: .regular))
                    .foregroundStyle(verdict.outcome.tint)

                Text(sentenceCase(verdict.word))
                    .font(.system(size: 12, weight: .regular))
                    .foregroundStyle(DesignTokens.label)
            }

            // nat has no reviewer-request flow of its own — GitHub's is the
            // only one, so this opens the pull request there rather than
            // pretending to add one from here.
            HStack(spacing: 8) {
                Image(systemName: "plus.circle")
                    .font(.system(size: 12, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)

                Text("Add Reviewer…")
                    .font(.system(size: 12, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
            .contentShape(Rectangle())
            .onTapGesture { openPROnGitHub() }
        }
    }

    private func openPROnGitHub() {
        guard let url = URL(string: pr.url) else { return }
        NSWorkspace.shared.open(url)
    }

    // MARK: - Changes

    private var changesSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("CHANGES")
                .font(.system(size: 11, weight: .semibold))
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
                .font(.system(size: 12, weight: .regular, design: .monospaced))

                if let commits = pr.commits {
                    Text("\(commits) \(plural(commits, "commit", "commits")) on \(pr.headRefName)")
                        .font(.system(size: 11, weight: .regular, design: .monospaced))
                        .foregroundStyle(DesignTokens.labelTertiary)
                }
            } else {
                Text("\(pr.headRefName) → \(pr.baseRefName)")
                    .font(.system(size: 12, weight: .regular, design: .monospaced))
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
