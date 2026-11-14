import Foundation

/// When Restart may start another host: only when the current one is not already
/// starting or running, so a second click cannot put two hosts in the app.
public enum HostRestartPolicy {
    public static func shouldStart(current: HostState?) -> Bool {
        guard let current else { return true }
        switch current {
        case .starting, .running:
            return false
        case .failed, .stopped:
            return true
        }
    }
}
