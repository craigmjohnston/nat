import Foundation

/// What the gallery was asked to do, read off the process's own arguments.
///
/// The three shapes are the three questions there are about a catalog of
/// stories: what is in it, draw me that one, draw me all of them. Parsing is
/// separate from rendering — and lives here rather than beside the AppKit
/// capture — because it is the half that can be tested: a window needs a
/// WindowServer and an argument list needs nothing.
public enum GalleryCommand: Equatable {
    /// `--list`: every story's name, one per line, and nothing drawn.
    case list
    /// `--story <name> --out <file.png>`: that one story, to that file.
    case one(story: String, out: String)
    /// `--all --out <dir>`: the whole catalog, one PNG per story, into that
    /// directory.
    case all(directory: String)
}

/// Why an argument list naming a gallery flag is not a gallery command.
///
/// Each case carries what was wrong rather than a code, because the only
/// thing done with one is print it: a run that cannot be understood should
/// say what it did not understand and exit, not guess.
public enum GalleryCommandError: Error, Equatable, CustomStringConvertible {
    /// A flag that takes a value was given none — it was last on the line,
    /// or the next argument was another flag.
    case missingValue(flag: String)
    /// `--story` and `--all` both named. They are different runs.
    case storyAndAll
    /// `--list` alongside anything else. It draws nothing, so an `--out` or
    /// a `--story` beside it is a line whose author meant one of two things.
    case listWithOthers
    /// `--story` or `--all` with no `--out` to write to.
    case missingOut
    /// `--out` with neither `--story` nor `--all` — a destination for
    /// nothing.
    case outWithoutTarget
    /// An argument the gallery does not know, on a line that named a gallery
    /// flag. Anything at all is allowed on a line that named none: that is
    /// the app being launched, and AppKit passes arguments of its own.
    case unknown(argument: String)

    public var description: String {
        switch self {
        case .missingValue(let flag):
            return "\(flag) needs a value"
        case .storyAndAll:
            return "--story and --all are different runs; name one"
        case .listWithOthers:
            return "--list draws nothing, so it takes no other flag"
        case .missingOut:
            return "--out is where the PNG goes; name it"
        case .outWithoutTarget:
            return "--out needs --story <name> or --all"
        case .unknown(let argument):
            return "unknown argument \(argument)"
        }
    }
}

extension GalleryCommand {
    /// The flags that make a line a gallery line. Recognising them is what
    /// tells "the app, launched" from "the gallery, mis-typed": a line with
    /// none of these is the app's, however odd it looks, and a line with one
    /// of them is the gallery's and is held to the gallery's rules.
    static let flags = ["--list", "--story", "--all", "--out"]

    /// Reads the command out of an argument list, argv's own — the first
    /// element is the executable and is skipped.
    ///
    /// `nil` is the answer for a line that names no gallery flag at all,
    /// which is the app being launched normally and the overwhelmingly
    /// common case.
    public static func parse(_ arguments: [String]) throws -> GalleryCommand? {
        let args = Array(arguments.dropFirst())
        guard args.contains(where: flags.contains) else { return nil }

        var list = false
        var all = false
        var story: String?
        var out: String?
        var index = 0
        while index < args.count {
            let arg = args[index]
            switch arg {
            case "--list":
                list = true
            case "--all":
                all = true
            case "--story":
                story = try value(after: arg, in: args, at: &index)
            case "--out":
                out = try value(after: arg, in: args, at: &index)
            default:
                throw GalleryCommandError.unknown(argument: arg)
            }
            index += 1
        }

        if list {
            guard !all, story == nil, out == nil else { throw GalleryCommandError.listWithOthers }
            return .list
        }
        if all, story != nil { throw GalleryCommandError.storyAndAll }
        guard let out else {
            throw all || story != nil
                ? GalleryCommandError.missingOut
                : GalleryCommandError.outWithoutTarget
        }
        if all { return .all(directory: out) }
        guard let story else { throw GalleryCommandError.outWithoutTarget }
        return .one(story: story, out: out)
    }

    /// The argument after a flag, advancing past it. A flag at the end of the
    /// line, or one followed by another flag, has no value — taking `--out`
    /// as `--story`'s filename would write a PNG called `--out`.
    private static func value(
        after flag: String, in args: [String], at index: inout Int
    ) throws -> String {
        let next = index + 1
        guard next < args.count, !args[next].hasPrefix("--") else {
            throw GalleryCommandError.missingValue(flag: flag)
        }
        index = next
        return args[next]
    }
}
