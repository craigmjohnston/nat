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
    dependencies: [
        .package(url: "https://github.com/migueldeicaza/SwiftTerm", from: "1.20.0")
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
            dependencies: [
                "NatKit",
                .product(name: "SwiftTerm", package: "SwiftTerm")
            ],
            path: "Sources/NatApp"
        )
    ]
)
