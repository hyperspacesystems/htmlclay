import Foundation
import Observation

/// What the Connect sheet holds and asks (08-sync.md 8.41). The key lives only
/// in the field: a refusal or a close clears it, so it is never kept in the
/// sheet's state or the model's after the host has answered.
@MainActor
@Observable
public final class ConnectSheetModel {
    public var username: String
    public var key = String() {
        didSet { if !key.isEmpty { refusal = nil } }
    }
    public private(set) var refusal: String?
    public private(set) var connecting = false
    private let model: AppModel

    public init(model: AppModel) {
        self.model = model
        self.username = model.snapshot.sync.username
    }

    public var ready: Bool { !username.isEmpty && !key.isEmpty && !connecting }

    /// Sends sync.connect. True when the host connected; otherwise the refusal's
    /// line shows under the key field.
    public func connect() async -> Bool {
        guard ready else { return false }
        let sent = key
        key = ""
        connecting = true
        let outcome = await model.submit(.connect(username: username, key: sent))
        connecting = false
        switch outcome {
        case .done:
            refusal = nil
            return true
        case let .refused(answer):
            // Only the username was wrong, so the key the person pasted stays. It
            // goes back first: setting the key clears the refusal.
            if answer.code == Self.wrongAccount { key = sent }
            refusal = answer.text.isEmpty ? answer.code : answer.text
            return false
        }
    }

    static let wrongAccount = "sync-wrong-account"

    /// What opens after a connect: the sheet that places the person's own
    /// account folder, when that folder is not on this Mac yet. The username is
    /// the host's, since the one typed may differ in case or carry an @.
    public static func nextSheet(after snapshot: HostSnapshot, wasConnected: Bool) -> PreviewSheet? {
        guard !wasConnected else { return nil }
        let username = snapshot.sync.username
        let waiting = snapshot.folders.contains { $0.origin == .account(username: username) && !$0.onThisMac }
        return waiting ? .accountFolder(username: username) : nil
    }

    /// The sheet closed: nothing typed is kept.
    public func close() {
        key = ""
        refusal = nil
    }
}
