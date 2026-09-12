import Foundation
import NatKit

// The process's one fork in the road, before any window or any scene: a
// command line naming a gallery flag is a headless render that writes PNGs
// and exits, and anything else is the app.
//
// It is top-level code rather than `@main` on the `App` because `App.main()`
// takes over the process — it starts AppKit, builds the scene and runs the
// loop — and the gallery needs the arguments read before any of that. A
// SwiftPM executable target with a `main.swift` has its entry point here, so
// `NatApp` carries no `@main` attribute and is started by hand below.
do {
    if let command = try GalleryCommand.parse(CommandLine.arguments) {
        GalleryRunner.run(command)
    }
} catch {
    FileHandle.standardError.write(Data("gnat: \(error)\n".utf8))
    exit(2)
}

NatApp.main()
