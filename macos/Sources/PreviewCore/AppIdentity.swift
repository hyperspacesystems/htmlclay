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

    private static func canonical(_ path: String) -> String {
        (URL(fileURLWithPath: path).resolvingSymlinksInPath().standardizedFileURL.path as NSString).standardizingPath
    }
}
