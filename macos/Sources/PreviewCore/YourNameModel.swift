import Foundation
import Observation

/// The Your name pane's decisions: the host refuses sharing with no name, so turning
/// the switch on first asks for the name, then turns sharing on.
@MainActor
@Observable
public final class YourNameModel {
    public enum Mode: Equatable { case off, local, account }

    public private(set) var settingUp = false
    public private(set) var draft: String
    public private(set) var problem: String?

    private let model: AppModel
    private var starting = false

    public init(model: AppModel) {
        self.model = model
        draft = model.snapshot.preferences.yourName
    }

    public static func mode(_ snapshot: HostSnapshot) -> Mode {
        guard snapshot.preferences.useName else { return .off }
        return snapshot.sync.connected && !snapshot.sync.fullName.isEmpty ? .account : .local
    }

    /// Setting up counts as local: the pane shows the field where the name is typed.
    public var mode: Mode { settingUp ? .local : Self.mode(model.snapshot) }

    public var isOn: Bool { settingUp || model.snapshot.preferences.useName }

    public var isSample: Bool { model.isSample }

    public var accountName: String { model.snapshot.sync.fullName }

    public func sayChange() { model.say(Strings.YourName.wouldChange) }

    public func setUseName(_ on: Bool) {
        guard on else {
            guard settingUp else {
                model.send(.setUseName(false))
                return
            }
            settingUp = false
            starting = false
            problem = nil
            return
        }
        guard model.snapshot.preferences.yourName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            model.send(.setUseName(true))
            return
        }
        settingUp = true
        problem = nil
    }

    public func setDraft(_ text: String) {
        draft = text
        guard settingUp else {
            model.send(.setYourName(text))
            return
        }
        guard !starting, !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        starting = true
        problem = nil
        Task { await self.startSharing(text) }
    }

    private func startSharing(_ name: String) async {
        switch await model.submit(.setYourName(name)) {
        case .done:
            break
        case let .refused(refusal):
            starting = false
            if settingUp { problem = refusal.text }
            return
        }
        guard settingUp else {
            starting = false
            return
        }
        switch await model.submit(.setUseName(true)) {
        case .done:
            starting = false
            settingUp = false
            problem = nil
        case let .refused(refusal):
            starting = false
            problem = refusal.text
        }
    }
}
