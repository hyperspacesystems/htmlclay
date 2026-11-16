import Foundation
import ServiceManagement
import PreviewCore

/// The product's login item. Constructed only when the app is the product bundle.
@MainActor
final class SMAppServiceLoginItem: LoginItemService {
    var status: LoginItemStatus {
        switch SMAppService.mainApp.status {
        case .enabled: return .enabled
        case .requiresApproval: return .requiresApproval
        case .notRegistered: return .notRegistered
        default: return .notFound
        }
    }
    func register() throws { try SMAppService.mainApp.register() }
    func unregister() throws { try SMAppService.mainApp.unregister() }
}

/// A candidate's login item: it records what the product would do, one line per call, in
/// <candidate home>/login-item.log, and answers the status it last set.
@MainActor
final class RecordingLoginItem: LoginItemService {
    private let log: URL
    private(set) var status: LoginItemStatus = .notRegistered

    init(home: URL) { log = home.appendingPathComponent("login-item.log") }

    func register() throws { status = .enabled; append("register") }
    func unregister() throws { status = .notRegistered; append("unregister") }

    private func append(_ line: String) {
        let data = Data("\(line)\n".utf8)
        if let handle = try? FileHandle(forWritingTo: log) {
            handle.seekToEndOfFile()
            handle.write(data)
            try? handle.close()
        } else {
            try? data.write(to: log)
        }
    }
}

enum LoginItems {
    /// The service for this app, and the 1.x LaunchAgent file it replaces. A candidate works inside its own home.
    @MainActor
    static func make(identity: AppIdentity, candidateHome: URL?) -> (service: LoginItemService, legacyPlist: URL)? {
        switch identity {
        case .product:
            let home = FileManager.default.homeDirectoryForCurrentUser
            return (SMAppServiceLoginItem(), home.appendingPathComponent("Library/LaunchAgents/com.htmlclay.plist"))
        case .candidate:
            guard let candidateHome else { return nil }
            return (RecordingLoginItem(home: candidateHome),
                    candidateHome.appendingPathComponent("Library/LaunchAgents/com.htmlclay.plist"))
        }
    }
}
