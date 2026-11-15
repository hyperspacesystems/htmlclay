import AppKit
import PreviewCore

@main
@MainActor
enum Entry {
    private static var delegate: AppDelegate?

    static func main() {
        let arguments = CommandLine.arguments
        switch LaunchArguments.role(arguments, fileExists: { FileManager.default.fileExists(atPath: $0) }) {
        case .host, .updateNow, .unregister:
            HostExec.replace(arguments)
        case let .app(open):
            run(opening: open)
        }
    }

    private static func run(opening paths: [String]) {
        let app = NSApplication.shared
        let current = ProcessInfo.processInfo.processIdentifier
        if let other = NSRunningApplication.runningApplications(withBundleIdentifier: AppDelegate.bundleID)
            .first(where: { $0.processIdentifier != current }) {
            _ = other.activate()
            exit(0)
        }
        let delegate = AppDelegate()
        Entry.delegate = delegate
        app.delegate = delegate
        if !paths.isEmpty {
            delegate.application(app, open: paths.map { URL(fileURLWithPath: $0) })
        }
        app.setActivationPolicy(.accessory)
        app.run()
    }
}
