import Foundation

/// C4: `htmlclay update-now` posts `request` with the object "<bundle path>\n<version>\n<reply name>"
/// and waits for a notification named <reply name> whose object is the answer line.
public enum UpdateNowWire {
    public static let request = Notification.Name("com.htmlclay.update-now")
    public static let replyPrefix = "com.htmlclay.update-now.reply."
    public static let wait: TimeInterval = 5

    public struct Request: Equatable, Sendable {
        public var bundlePath: String
        public var version: String
        public var reply: String
        public init(bundlePath: String, version: String, reply: String) {
            self.bundlePath = bundlePath
            self.version = version
            self.reply = reply
        }
    }

    public static func encode(_ r: Request) -> String { [r.bundlePath, r.version, r.reply].joined(separator: "\n") }

    public static func decode(_ object: String) -> Request? {
        let parts = object.components(separatedBy: "\n")
        guard parts.count == 3, parts.allSatisfy({ !$0.isEmpty }), parts[2].hasPrefix(replyPrefix) else { return nil }
        return Request(bundlePath: parts[0], version: parts[1], reply: parts[2])
    }

    /// The app's side: the reply to post, or nil when the request is not for this bundle.
    @MainActor
    public static func answer(object: String, ownBundlePath: String, decide: (String) -> String) -> (reply: String, line: String)? {
        guard let request = decode(object), samePath(request.bundlePath, ownBundlePath) else { return nil }
        return (request.reply, decide(request.version))
    }

    public static func samePath(_ a: String, _ b: String) -> Bool {
        URL(fileURLWithPath: a).standardizedFileURL.resolvingSymlinksInPath().path
            == URL(fileURLWithPath: b).standardizedFileURL.resolvingSymlinksInPath().path
    }

    /// Exit codes of cmd/htmlclay/update_now.go:26-45.
    public static let exitOK: Int32 = 0, exitUsage: Int32 = 1, exitNotRunning: Int32 = 2, exitRefused: Int32 = 3,
                      exitTooOld: Int32 = 4, exitFailed: Int32 = 5, exitStarting: Int32 = 6

    public static func exitCode(for answer: String) -> Int32 {
        if answer == "starting" { return exitStarting }
        let first = answer.split(separator: " ", maxSplits: 1).first.map(String.init) ?? ""
        switch first {
        case "accepted", "already": return exitOK
        case "failed": return exitFailed
        default: return exitRefused
        }
    }
}
