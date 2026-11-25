import AppKit
import PreviewCore

/// A second HTML Clay hands its files to the copy already running and quits (decision 17).
/// If the running copy is a 1.x app, 2.0 asks it to quit first so a manual download
/// over a running 1.x instance upgrades cleanly instead of silently exiting.
@MainActor
enum HandOff {
    static func otherRunning() -> NSRunningApplication? {
        let me = ProcessInfo.processInfo.processIdentifier
        return NSRunningApplication.runningApplications(withBundleIdentifier: AppDelegate.bundleID)
            .first { $0.processIdentifier != me }
    }

    static func isLegacy(_ other: NSRunningApplication) -> Bool {
        let hasHostBinary: Bool
        if let bundle = other.bundleURL {
            let host = bundle.appendingPathComponent("Contents/MacOS/htmlclay-host")
            hasHostBinary = FileManager.default.isExecutableFile(atPath: host.path)
        } else {
            hasHostBinary = false
        }
        return AppIdentity.isLegacyInstance(
            pid: other.processIdentifier,
            hasHostBinary: hasHostBinary,
            lockPID: readLockPID()
        )
    }

    /// Asks a running 1.x instance to quit and waits briefly for it to exit.
    /// Returns true once the legacy process has exited.
    static func stopLegacy(_ other: NSRunningApplication, timeout: TimeInterval = 3.0) -> Bool {
        _ = other.terminate()
        let deadline = Date().addingTimeInterval(timeout)
        while !other.isTerminated && Date() < deadline {
            Thread.sleep(forTimeInterval: 0.05)
        }
        return other.isTerminated
    }

    static func alertLegacyStillRunning() {
        NSApp.activate(ignoringOtherApps: true)
        let alert = NSAlert()
        alert.messageText = "HTML Clay is already running"
        alert.informativeText = "Quit the older HTML Clay in the menu bar, then open this one again."
        alert.addButton(withTitle: "OK")
        _ = alert.runModal()
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

    private static func readLockPID() -> Int32? {
        guard let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first else {
            return nil
        }
        let lockURL = support.appendingPathComponent("htmlclay/lock")
        guard let text = try? String(contentsOf: lockURL, encoding: .utf8),
              let first = text.split(separator: "\n").first,
              let pid = Int32(first.trimmingCharacters(in: .whitespacesAndNewlines)) else {
            return nil
        }
        return pid
    }
}
