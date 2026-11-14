import Foundation
import Observation

/// What the Find it sheet holds and asks: the host checks the folder at the new
/// path, the sheet shows what it found, and the person's click acknowledges those
/// codes. A refusal keeps the sheet open with its path.
@MainActor
@Observable
public final class FolderRelinkModel {
    public let folderID: String
    public private(set) var path: String
    public private(set) var warnings: [HostWarning] = []
    public private(set) var problem: String?
    public private(set) var problemIsRetryable = false
    public private(set) var checking = false
    public private(set) var saving = false

    public var warningLines: [String] { ProgramControls.warningLines(warnings) }

    /// The codes the host showed, which are the codes the click acknowledges.
    public var acknowledged: [String] { ProgramControls.acknowledged(warnings) }

    public var ready: Bool {
        !path.isEmpty && (problem == nil || problemIsRetryable) && !checking && !saving
    }

    private let model: AppModel
    private var checkToken = 0

    public init(folderID: String, path: String, model: AppModel) {
        self.folderID = folderID
        self.path = path
        self.model = model
    }

    /// Asks the host about the folder: the codes a person has to acknowledge, or why it cannot be used.
    public func check() async {
        checkToken += 1
        let token = checkToken
        guard !path.isEmpty else {
            warnings = []
            problem = nil
            problemIsRetryable = false
            checking = false
            return
        }
        checking = true
        let answer = await model.check(action)
        guard token == checkToken else { return }
        checking = false
        switch answer {
        case let .success(check):
            if !check.path.isEmpty { path = check.path }
            warnings = check.warnings
            problem = nil
            problemIsRetryable = false
        case let .failure(refusal):
            warnings = []
            problem = refusal.text
            problemIsRetryable = SheetRules.isRetryable(refusal)
        }
    }

    /// True when the host relinked it (the sheet closes); false keeps the path and shows the refusal.
    public func confirm() async -> Bool {
        guard ready else { return false }
        saving = true
        switch await model.submit(action) {
        case .done:
            return true
        case let .refused(refusal):
            saving = false
            guard refusal.code != "duplicate", refusal.code != "queued" else { return false }
            problem = refusal.text
            problemIsRetryable = SheetRules.isRetryable(refusal)
            return false
        }
    }

    private var action: HostAction {
        .relinkFolder(folderID: folderID, path: path, acknowledged: acknowledged)
    }
}

/// The folder page's Find it…: the native folder panel picks where the folder went,
/// then the Find it sheet checks that folder and shows what the person has to
/// acknowledge.
@MainActor
public enum FolderFlow {
    public static func findIt(_ folder: Folder, model: AppModel) {
        Task {
            guard let picked = await model.folderChooser?.choose(after: folder.path, in: model.snapshot),
                  !picked.isEmpty else { return }
            model.sheet = .relinkFolder(folderID: folder.id, path: picked)
        }
    }
}
