import Foundation

/// The rail's three sections — what is running, what is queued, what is
/// finished — as one value, so the view, the stories and the rules that
/// measure them all name them the same way.
///
/// Each carries the icon its heading wears. They are said here rather than
/// typed into the heading builder because the headings are one kind of
/// control now: three sections that fold the same way, drawn by one row, and
/// a section that forgot its icon would be the odd one out.
public enum RailSection: String, CaseIterable, Sendable, Hashable {
    case active
    case todo
    case done

    /// The all-caps label the heading draws.
    public var title: String {
        switch self {
        case .active: return "ACTIVE"
        case .todo: return "TODO"
        case .done: return "DONE"
        }
    }

    /// The SF Symbol in the heading's shared slot, on the icon axis every
    /// other rail row sits on.
    public var icon: String {
        switch self {
        case .active: return "bolt"
        case .todo: return "list.bullet"
        case .done: return "checkmark.circle"
        }
    }
}

/// How the rail's height is shared out between the sections that are open.
///
/// Every section is a pinned heading over a scroll of its own, so what is
/// left of the rail once the headings and the rules between them have taken
/// their lines is what the open sections divide between them. The rule is
/// water-filling: a section wanting less than an even share takes what it
/// wants and gives the rest back, and what is left over is divided again
/// between the sections still asking for more — so a quiet TODO does not sit
/// under a half-empty column while ACTIVE scrolls a dozen agents inside a
/// third of the rail, and an over-tall section scrolls within its share
/// rather than squeezing the others off the rail altogether.
///
/// A collapsed section is simply not passed in: it is its heading alone, and
/// the space it was taking goes back to the rest by the same arithmetic that
/// shares it out in the first place.
public enum RailSectionLayout {
    /// The air left under the last section, so the foot of the plan is not
    /// flush against the bottom of the window.
    public static let footRoom: Double = 16

    /// What the open sections have between them: the rail as its container
    /// offers it, less every drawn section's own chrome — the rules and the
    /// pinned headings, which never scroll and never yield — and less the air
    /// under the last of them.
    ///
    /// The rail here is the height the shell offers, never the height the
    /// column it produces comes to. Measuring the column was a circle: the
    /// column is as tall as the sections it draws, the sections are as tall
    /// as this number lets them be, so a first pass with nothing measured
    /// gave every section its whole content and the column handed that back
    /// as the rail — which is the number that produced it, confirming itself
    /// forever. A container's height is nothing the share can inflate.
    public static func available(rail: Double, chrome: Double) -> Double {
        rail - chrome - footRoom
    }

    /// The height to draw each open section's scroll at, in the order they
    /// are given. A rail nobody has measured yet — the first layout pass,
    /// before the geometry lands — gives every section its whole content
    /// rather than nothing, the way an unmeasured band always drew what it
    /// held: a section that is never measured is still a section that draws.
    public static func heights(open contents: [Double], available: Double) -> [Double] {
        guard !contents.isEmpty else { return [] }
        let wants = contents.map { max(0, $0) }
        guard available > 0 else { return wants }
        guard wants.reduce(0, +) > available else { return wants }

        var heights = [Double](repeating: 0, count: wants.count)
        var asking = Set(wants.indices)
        var remaining = available
        while !asking.isEmpty {
            let share = remaining / Double(asking.count)
            let modest = asking.filter { wants[$0] <= share }
            guard !modest.isEmpty else {
                // Everyone left wants more than an even share, so an even
                // share is what they each get.
                for index in asking { heights[index] = share }
                break
            }
            for index in modest {
                heights[index] = wants[index]
                remaining -= wants[index]
                asking.remove(index)
            }
        }
        return heights
    }

    /// Whether a section has to scroll within the height it was given. Read
    /// off the height `heights` handed back rather than computed a second
    /// way, so a section and its scrolling cannot disagree. The hair of
    /// tolerance is for the fractional heights layout hands back — a section
    /// a twentieth of a point over its share is not one with anything to
    /// scroll to.
    public static func scrolls(content: Double, height: Double) -> Bool {
        content - height > 0.5
    }
}
