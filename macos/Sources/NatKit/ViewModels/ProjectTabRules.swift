import Foundation

/// What a project tab's close button does, and what it lets a click reach.
///
/// Both answers used to be expressions inside `ProjectTabsView`, which is
/// where the second of them hid a bug: a ✕ drawn at `opacity(0)` is still a
/// live hit target, so a tab could be closed by a button nobody could see.
/// Said here they are one answer each, and the view asks the same question
/// for what it draws as for what it lets through.
public enum ProjectTabRules {
    /// Whether a tab carries a close button at all.
    ///
    /// The last tab standing does not. `AppModel.closeProject` refuses it
    /// outright — a board with no project is the onboarding screen's shape,
    /// and this is not onboarding — so a ✕ there would offer the one thing
    /// it cannot do. Suppressed rather than drawn disabled, because a strip
    /// with one tab is what a machine tracking one project looks like all the
    /// time, and a permanently greyed ✕ would be a standing apology for a
    /// state that is not an error.
    public static func showsClose(tabCount: Int) -> Bool {
        tabCount > 1
    }

    /// Whether that button is drawn — and, because the two must agree,
    /// whether a click can reach it.
    ///
    /// Browser-fashion: always on the active tab, and on any other one while
    /// the mouse is over it. Fading rather than removing is what keeps the
    /// tab's contents still as the mouse crosses it; gating the hit test on
    /// the very same answer is what stops a faded-out ✕ from closing a tab
    /// the user meant to select.
    public static func closeIsVisible(isActive: Bool, isHovered: Bool) -> Bool {
        isActive || isHovered
    }
}
