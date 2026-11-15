import AppKit
import PreviewCore

@main
@MainActor
enum Entry {
    private static var delegate: AppDelegate?

    static func main() {
        let arguments = CommandLine.arguments
        switch LaunchArguments.role(arguments, fileExists: { FileManager.default.fileExists(atPath: $0) }) {
        case let .updateNow(rest):
            exit(UpdateNowClient.run(rest))
        case .host, .unregister:
            HostExec.replace(arguments)
        case let .app(open):
            run(opening: open)
        }
    }

    private static func run(opening paths: [String]) {
        let app = NSApplication.shared
        let current = ProcessInfo.processInfo.processIdentifier
        let others = NSRunningApplication.runningApplications(withBundleIdentifier: AppDelegate.bundleID)
        let running = others.map { AppIdentity.Running(pid: $0.processIdentifier, bundlePath: $0.bundleURL?.path) }
        if let pid = AppIdentity.otherInstance(pid: current, bundlePath: Bundle.main.bundleURL.path, among: running),
           let other = others.first(where: { $0.processIdentifier == pid }) {
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
