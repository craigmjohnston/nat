// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "nat",
    platforms: [
        .macOS(.v15)
    ],
    products: [
        .library(name: "NatKit", targets: ["NatKit"]),
        .executable(name: "NatApp", targets: ["NatApp"])
    ],
    targets: [
        .target(
            name: "NatKit",
            dependencies: [],
            path: "Sources/NatKit"
        ),
        .testTarget(
            name: "NatKitTests",
            dependencies: ["NatKit"],
            path: "Tests/NatKitTests"
        ),
        .executableTarget(
            name: "NatApp",
            dependencies: ["NatKit"],
            path: "Sources/NatApp"
        )
    ]
)
