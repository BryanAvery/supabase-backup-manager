// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "SBMDesktopFoundation",
    platforms: [.macOS(.v13)],
    products: [
        .library(name: "SBMDesktopFoundation", targets: ["SBMDesktopFoundation"])
    ],
    dependencies: [
        .package(path: "../AuthFoundation")
    ],
    targets: [
        .target(name: "SBMDesktopFoundation", dependencies: [
            .product(name: "SBMNativeAuth", package: "AuthFoundation")
        ]),
        .testTarget(name: "SBMDesktopFoundationTests", dependencies: [
            "SBMDesktopFoundation"
        ])
    ]
)
