import SwiftUI
import NatKit

@main
struct NatApp: App {
    var body: some Scene {
        WindowGroup("nat") {
            ContentView()
        }
    }
}

struct ContentView: View {
    var body: some View {
        ZStack {
            DesignTokens.windowBg
                .ignoresSafeArea()

            VStack(spacing: 16) {
                Text("nat")
                    .font(.system(size: 32, weight: .semibold))
                    .foregroundStyle(DesignTokens.label)

                Text("board loading will land here")
                    .font(.system(size: 14, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
        }
    }
}
