import Foundation

/// Why the host said no. text is ready to show; code is stable for logic.
public struct HostRefusal: Error, Equatable, Sendable {
    public var code: String
    public var text: String
    public var retryable: Bool
    public var field: String?
    public var subjectID: String?
    public var documents: [String]
    public init(code: String, text: String, retryable: Bool = false, field: String? = nil, subjectID: String? = nil, documents: [String] = []) {
        self.code = code; self.text = text; self.retryable = retryable
        self.field = field; self.subjectID = subjectID; self.documents = documents
    }
}

/// Something a sheet shows before the person confirms; it never blocks.
public struct HostWarning: Equatable, Sendable {
    public var code: String
    public var text: String
    public var names: [String]
    public init(code: String, text: String, names: [String] = []) { self.code = code; self.text = text; self.names = names }
}

/// The host's answer to "would this work?", asked before a sheet's button is pressed.
public struct HostCheckAnswer: Equatable, Sendable {
    public var address: String
    public var name: String
    public var path: String
    public var warnings: [HostWarning]
    public init(address: String = "", name: String = "", path: String = "", warnings: [HostWarning] = []) {
        self.address = address; self.name = name; self.path = path; self.warnings = warnings
    }
}

public enum HostOutcome: Equatable, Sendable {
    case done(HostSnapshot)
    case refused(HostRefusal)
}

/// Returned by subscribe; cancel stops further changes. Cancelling twice is harmless.
@MainActor
public final class HostSubscription {
    private var onCancel: (@MainActor () -> Void)?
    public init(onCancel: @escaping @MainActor () -> Void) { self.onCancel = onCancel }
    public func cancel() {
        let run = onCancel
        onCancel = nil
        run?()
    }
}

@MainActor
public protocol HostClient: AnyObject {
    func load() async throws -> HostSnapshot
    /// Never throws: a host that cannot be reached answers refused with code "unreachable", retryable.
    func perform(_ action: HostAction) async -> HostOutcome
    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal>
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription
}
