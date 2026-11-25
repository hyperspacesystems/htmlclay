import Foundation
import Observation

/// The Custom AI command sheet: the command typed, the host's answer, and the
/// refusal that keeps the sheet open.
@MainActor
@Observable
public final class CustomCommandModel {
    public private(set) var command: String
    public private(set) var problem: String?
    public private(set) var saving = false

    private let model: AppModel

    public init(model: AppModel) {
        self.model = model
        command = model.snapshot.preferences.aiEditTools.first { $0.custom }?.command ?? String()
    }

    public var isSample: Bool { model.isSample }

    public var ready: Bool {
        !command.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !saving
    }

    public func setCommand(_ text: String) {
        command = text
        problem = nil
    }

    /// True when the host took the command; a refusal stays under the field.
    public func save() async -> Bool {
        guard ready else { return false }
        saving = true
        let outcome = await model.submit(.setAIEditCustom(command: command))
        saving = false
        switch outcome {
        case .done:
            problem = nil
            return true
        case let .refused(refusal):
            problem = refusal.text.isEmpty ? Strings.General.aiSaveFailed : refusal.text
            return false
        }
    }
}
