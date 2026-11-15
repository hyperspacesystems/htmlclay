// swift-tools-version: 5.10
import PackageDescription

let package = Package(
    name: "HTMLClayPreview",
    platforms: [.macOS(.v14)],
    targets: [
        .target(name: "PreviewCore"),
        .target(name: "PreviewUI", dependencies: ["PreviewCore"]),
        .target(name: "AppShell", dependencies: ["PreviewCore", "PreviewUI"]),
        .target(name: "SparkleBridge", path: "Sources/SparkleBridge", publicHeadersPath: "include", linkerSettings: [.linkedFramework("AppKit")]),
        .executableTarget(name: "HTMLClayPreview", dependencies: ["PreviewCore", "PreviewUI", "AppShell"]),
        .executableTarget(name: "HTMLClay", dependencies: ["PreviewCore", "PreviewUI", "AppShell", "SparkleBridge"]),
        .testTarget(name: "PreviewCoreTests", dependencies: ["PreviewCore"]),
        .testTarget(name: "PreviewUITests", dependencies: ["PreviewCore", "PreviewUI"]),
        .testTarget(name: "SparkleBridgeTests", dependencies: ["SparkleBridge"]),
    ]
)
