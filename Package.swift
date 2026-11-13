// swift-tools-version: 5.10
import PackageDescription

let package = Package(
    name: "HTMLClayPreview",
    platforms: [.macOS(.v14)],
    targets: [
        .target(name: "PreviewCore"),
        .target(name: "PreviewUI", dependencies: ["PreviewCore"]),
        .executableTarget(name: "HTMLClayPreview", dependencies: ["PreviewCore", "PreviewUI"]),
        .testTarget(name: "PreviewCoreTests", dependencies: ["PreviewCore"]),
        .testTarget(name: "PreviewUITests", dependencies: ["PreviewCore", "PreviewUI"]),
    ]
)
