import AppKit
import Foundation
import PreviewCore

enum UpdateNowClient {
    static func run(_ arguments: [String]) -> Int32 {
        guard let version = target(arguments) else {
            FileHandle.standardError.write(Data("usage: htmlclay update-now --version X.Y.Z\n".utf8))
            return UpdateNowWire.exitUsage
        }
        let own = Bundle.main.bundlePath
        let running = NSRunningApplication.runningApplications(withBundleIdentifier: Bundle.main.bundleIdentifier ?? "")
        guard running.contains(where: { $0.bundleURL.map { UpdateNowWire.samePath($0.path, own) } ?? false }) else {
            FileHandle.standardError.write(Data("htmlclay update-now: HTML Clay is not running\n".utf8))
            return UpdateNowWire.exitNotRunning
        }
        let reply = UpdateNowWire.replyPrefix + UUID().uuidString
        let center = DistributedNotificationCenter.default()
        let answer = Answer()
        let observer = center.addObserver(forName: Notification.Name(reply), object: nil, queue: nil) { note in
            answer.take(note.object as? String)
        }
        center.postNotificationName(UpdateNowWire.request,
                                    object: UpdateNowWire.encode(.init(bundlePath: own, version: version, reply: reply)),
                                    userInfo: nil, deliverImmediately: true)
        let deadline = Date().addingTimeInterval(UpdateNowWire.wait)
        while answer.line == nil, Date() < deadline {
            RunLoop.current.run(mode: .default, before: Date().addingTimeInterval(0.02))
        }
        center.removeObserver(observer)
        guard let line = answer.line else {
            print("refused the running HTML Clay predates update-now")
            return UpdateNowWire.exitTooOld
        }
        print(line)
        return UpdateNowWire.exitCode(for: line)
    }

    /// Exactly `--version X.Y.Z` or `--version=X.Y.Z`, with a strict version.
    private static func target(_ arguments: [String]) -> String? {
        let version: String
        if arguments.count == 1, arguments[0].hasPrefix("--version=") {
            version = String(arguments[0].dropFirst("--version=".count))
        } else if arguments.count == 2, arguments[0] == "--version" {
            version = arguments[1]
        } else {
            return nil
        }
        return UpdateGate.isStrictVersion(version) ? version : nil
    }
}

private final class Answer: @unchecked Sendable {
    private let lock = NSLock()
    private var stored: String?

    var line: String? {
        lock.lock()
        defer { lock.unlock() }
        return stored
    }

    func take(_ line: String?) {
        lock.lock()
        if stored == nil { stored = line }
        lock.unlock()
    }
}
