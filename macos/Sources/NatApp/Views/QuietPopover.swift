import AppKit
import SwiftUI

extension View {
    /// A popover for hover detail: shown without NSPopover's own animation
    /// — the zoom and spring SwiftUI's `.popover` always plays, which on a
    /// detail that comes and goes with the pointer reads as a bounce on every
    /// pass — and faded in briefly instead.
    func quietPopover<Content: View>(
        isPresented: Binding<Bool>, arrowEdge: NSRectEdge, @ViewBuilder content: @escaping () -> Content
    ) -> some View {
        background(QuietPopoverAnchor(isPresented: isPresented, edge: arrowEdge, content: content))
    }
}

private struct QuietPopoverAnchor<Content: View>: NSViewRepresentable {
    @Binding var isPresented: Bool
    let edge: NSRectEdge
    let content: () -> Content

    /// How long the detail takes to fade in.
    static var fadeIn: TimeInterval { 0.12 }

    final class Coordinator {
        var popover: NSPopover?
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    func makeNSView(context: Context) -> NSView { NSView() }

    func updateNSView(_ anchor: NSView, context: Context) {
        let coordinator = context.coordinator
        guard isPresented else {
            coordinator.popover?.close()
            coordinator.popover = nil
            return
        }
        if let popover = coordinator.popover {
            (popover.contentViewController as? NSHostingController<Content>)?.rootView = content()
            return
        }
        guard anchor.window != nil else { return }
        let controller = NSHostingController(rootView: content())
        controller.sizingOptions = .preferredContentSize
        let popover = NSPopover()
        popover.animates = false
        // Opened and closed by the pointer alone, never by a click elsewhere.
        popover.behavior = .applicationDefined
        popover.contentViewController = controller
        popover.show(relativeTo: anchor.bounds, of: anchor, preferredEdge: edge)
        if let window = controller.view.window {
            window.alphaValue = 0
            NSAnimationContext.runAnimationGroup { context in
                context.duration = Self.fadeIn
                window.animator().alphaValue = 1
            }
        }
        coordinator.popover = popover
    }

    static func dismantleNSView(_ anchor: NSView, coordinator: Coordinator) {
        coordinator.popover?.close()
        coordinator.popover = nil
    }
}
