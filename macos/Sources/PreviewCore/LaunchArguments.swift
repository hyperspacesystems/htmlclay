import Foundation

public enum LaunchRole: Equatable, Sendable {
    case app(open: [String])
    case updateNow([String])
    case unregister
    case host
}

public enum LaunchArguments {
    public static func role(_ arguments: [String], fileExists: (String) -> Bool) -> LaunchRole {
        let rest = Array(arguments.dropFirst())
        guard let first = rest.first else { return .app(open: []) }
        if first == "update-now" { return .updateNow(Array(rest.dropFirst())) }
        if first == "--unregister" { return .unregister }
        if first == "--profile" || first == "--no-welcome" { return .app(open: []) }
        if first.hasPrefix("-NS") || first.hasPrefix("-Apple") || first.hasPrefix("-psn_") { return .app(open: []) }
        let documents = ["htmlclay", "html", "htm"]
        if rest.allSatisfy({ !$0.hasPrefix("-") && documents.contains(($0 as NSString).pathExtension.lowercased()) && fileExists($0) }) {
            return .app(open: rest)
        }
        return .host
    }
}
