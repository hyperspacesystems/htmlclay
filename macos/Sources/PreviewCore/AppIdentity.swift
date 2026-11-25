import Foundation

public enum AppIdentity: Equatable, Sendable {
    case product
    case candidate

    public static let productBundleID = "com.htmlclay.HTMLClay"
    public static let candidateBundleID = "com.htmlclay.HTMLClayCandidate"

    public static func of(bundleID: String?) -> AppIdentity {
        bundleID == productBundleID ? .product : .candidate
    }

    public var usesCandidateHome: Bool { self == .candidate }

    public func hostEnvironment(candidateHome: [String: String]) -> [String: String] {
        usesCandidateHome ? candidateHome : [:]
    }

    public struct Running: Equatable, Sendable {
        public let pid: Int32
        public let bundlePath: String?

        public init(pid: Int32, bundlePath: String?) {
            self.pid = pid
            self.bundlePath = bundlePath
        }
    }

    public static func otherInstance(pid: Int32, bundlePath: String, among running: [Running]) -> Int32? {
        let own = canonical(bundlePath)
        return running.first { $0.pid != pid && $0.bundlePath.map(canonical) == own }?.pid
    }

    /// Reports whether another running HTML Clay process is a 1.x instance rather
    /// than a 2.0 Swift shell: either its bundle has no `Contents/MacOS/htmlclay-host`
    /// binary, or it holds the single-instance `lock` file directly (in 2.0 the
    /// child `htmlclay-host` process owns `lock`, never the Swift app process).
    public static func isLegacyInstance(pid: Int32, hasHostBinary: Bool, lockPID: Int32?) -> Bool {
        if !hasHostBinary { return true }
        return lockPID == pid
    }

    private static func canonical(_ path: String) -> String {
        (URL(fileURLWithPath: path).resolvingSymlinksInPath().standardizedFileURL.path as NSString).standardizingPath
    }
}
