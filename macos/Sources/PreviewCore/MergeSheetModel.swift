import Foundation
import Observation

/// What the Merge sheet holds and asks: the child folder's documents answer at
/// the parent's address afterwards, and nothing on disk moves. The sheet closes
/// only when the host has merged it; a refusal (a page that could not save, a
/// config write that failed) keeps it open with the host's words.
@MainActor
@Observable
public final class MergeSheetModel {
    public let childID: String
    public private(set) var problem: String?
    public private(set) var saving = false

    public var ready: Bool { !saving }

    private let model: AppModel

    public init(childID: String, model: AppModel) {
        self.childID = childID
        self.model = model
    }

    /// True when the host merged the folder. Only then does the sheet close.
    @discardableResult
    public func confirm() async -> Bool {
        guard ready else { return false }
        saving = true
        switch await model.submit(.mergeIntoParent(folderID: childID)) {
        case .done:
            model.sheet = nil
            return true
        case let .refused(refusal):
            saving = false
            guard refusal.code != "duplicate", refusal.code != "queued" else { return false }
            problem = refusal.text
            return false
        }
    }
}

/// The folder page's Merge into…: opening the sheet asks nothing of the host, and
/// the merge goes only when the person confirms in the sheet.
@MainActor
public enum MergeFlow {
    public static func open(childID: String, model: AppModel) {
        model.sheet = .merge(childID: childID)
    }
}
