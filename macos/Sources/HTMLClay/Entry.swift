import AppKit

@main
@MainActor
enum Entry {
    private static var delegate: AppDelegate?

    static func main() {
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
        app.setActivationPolicy(.accessory)
        app.run()
    }
}
