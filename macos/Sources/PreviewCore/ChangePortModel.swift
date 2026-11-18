import Foundation
import Observation

/// The Change port sheet: the number typed, the local range check, and the
/// host's refusal when it cannot move.
@MainActor
@Observable
public final class ChangePortModel {
    public private(set) var text: String
    public private(set) var problem: String?
    public private(set) var documents: [String] = []
    public private(set) var saving = false
    /// What the restart would stop, after the host answered restart-stops-work.
    /// The button then restarts anyway.
    public private(set) var stopsWork: [String] = []

    private let model: AppModel

    public init(model: AppModel) {
        self.model = model
        text = String(model.snapshot.host.port)
    }

    public var currentPort: Int { model.snapshot.host.port }

    public var isSample: Bool { model.isSample }

    public var ready: Bool { !saving && SettingsControls.portProblem(text) == nil }

    public var actionTitle: String { stopsWork.isEmpty ? Strings.Advanced.changePortAction : Strings.Updates.restartAnyway }

    public func setText(_ typed: String) {
        let next = typed.filter(\.isNumber)
        // Leaving the field or a key that adds no digit keeps the answer, and
        // with it the Restart anyway step.
        guard next != text else { return }
        text = next
        problem = nil
        documents = []
        stopsWork = []
    }

    /// True when the host took the new port; a refusal stays under the field.
    public func save() async -> Bool {
        if let problem = SettingsControls.portProblem(text) {
            self.problem = problem
            return false
        }
        guard let port = Int(text) else { return false }
        saving = true
        let outcome = await model.submit(.setPort(port, confirmed: !stopsWork.isEmpty))
        saving = false
        switch outcome {
        case .done:
            problem = nil
            documents = []
            stopsWork = []
            return true
        case let .refused(refusal):
            problem = refusal.text
            documents = refusal.documents
            stopsWork = refusal.code == HostRefusal.restartStopsWork ? refusal.running : []
            return false
        }
    }
}
