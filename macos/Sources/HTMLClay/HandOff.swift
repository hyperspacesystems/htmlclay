import AppKit

/// A second HTML Clay hands its files to the copy already running and quits (decision 17).
@MainActor
enum HandOff {
    static func otherRunning() -> NSRunningApplication? {
        let me = ProcessInfo.processInfo.processIdentifier
        return NSRunningApplication.runningApplications(withBundleIdentifier: AppDelegate.bundleID)
            .first { $0.processIdentifier != me }
    }

    static func give(_ paths: [String], to other: NSRunningApplication) {
        if let bundle = other.bundleURL {
            let done = DispatchSemaphore(value: 0)
            if !paths.isEmpty {
                NSWorkspace.shared.open(paths.map { URL(fileURLWithPath: $0) }, withApplicationAt: bundle,
                                        configuration: NSWorkspace.OpenConfiguration()) { _, _ in done.signal() }
            } else {
                NSWorkspace.shared.openApplication(at: bundle, configuration: NSWorkspace.OpenConfiguration()) { _, _ in done.signal() }
            }
            _ = done.wait(timeout: .now() + 5)
        }
        _ = other.activate()
    }
}
