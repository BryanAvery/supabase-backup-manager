// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "SBMNativeAuth",
    platforms: [.macOS(.v13)],
    products: [
        .library(name: "SBMNativeAuth", targets: ["SBMNativeAuth"])
    ],
    targets: [
        .target(name: "SBMNativeAuth"),
        .testTarget(name: "SBMNativeAuthTests", dependencies: ["SBMNativeAuth"])
    ]
)
