// swift-tools-version: 5.10
import PackageDescription

let package = Package(
    name: "HTMLClayPreview",
    platforms: [.macOS(.v14)],
    targets: [
        .target(name: "PreviewCore"),
        .target(name: "PreviewUI", dependencies: ["PreviewCore"]),
        .target(name: "AppShell", dependencies: ["PreviewCore", "PreviewUI"]),
        .executableTarget(name: "HTMLClayPreview", dependencies: ["PreviewCore", "PreviewUI", "AppShell"]),
        .executableTarget(name: "HTMLClay", dependencies: ["PreviewCore", "PreviewUI", "AppShell"]),
        .testTarget(name: "PreviewCoreTests", dependencies: ["PreviewCore"]),
        .testTarget(name: "PreviewUITests", dependencies: ["PreviewCore", "PreviewUI"]),
    ]
)
