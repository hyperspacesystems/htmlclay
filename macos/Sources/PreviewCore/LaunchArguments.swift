import Foundation

public enum LaunchRole: Equatable, Sendable {
    case app(open: [String])
    case updateNow([String])
    case unregister
    case host
}

public enum LaunchArguments {
    public static let coreEventClass: FourCharCode = 0x61657674
    public static let openApplicationEventID: FourCharCode = 0x6F617070
    public static let propDataKeyword: FourCharCode = 0x70726474
    public static let launchedAsLoginItemCode: FourCharCode = 0x6C676974

    public static func isLoginItemLaunch(_ event: NSAppleEventDescriptor?) -> Bool {
        guard let event,
              event.eventClass == coreEventClass,
              event.eventID == openApplicationEventID,
              let prop = event.paramDescriptor(forKeyword: propDataKeyword)
        else { return false }
        return prop.typeCodeValue == launchedAsLoginItemCode
    }

    public static func shouldShowWelcome(welcomeDone: Bool, isLoginItem: Bool, hasPendingOpens: Bool, arguments: [String], hasProfile: Bool) -> Bool {
        if hasProfile { return false }
        if arguments.contains("--no-welcome") { return false }
        if hasPendingOpens { return false }
        return !welcomeDone || !isLoginItem
    }

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
