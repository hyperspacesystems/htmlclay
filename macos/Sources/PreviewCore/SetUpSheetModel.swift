import Foundation
import Observation

/// What the account and team set-up sheets hold and ask (08-sync.md 8.42). A
/// new folder sets up in one send. A folder already there (own account only)
/// sends twice: the first answers what would sync, and the second, after the
/// person has seen it, sets the folder up.
@MainActor
@Observable
public final class SetUpSheetModel {
    public let folderID: String
    public private(set) var path: String
    public private(set) var existing = false
    public private(set) var counts: SyncCounts?
    public private(set) var refusal: String?
    public private(set) var working = false
    /// True once the folder is set up: the sheet then shows the first download's
    /// progress until it ends.
    public private(set) var following = false
    private let model: AppModel

    public init(folderID: String, path: String, model: AppModel) {
        self.folderID = folderID
        self.path = path
        self.model = model
    }

    /// A new folder at path, as the sheet suggests it.
    public func chooseNew(_ path: String) {
        self.path = path
        existing = false
        counts = nil
        refusal = nil
    }

    /// "A folder I already have…": the system's folder chooser. What would sync
    /// is asked again for the folder picked.
    public func chooseExisting() async {
        guard let picked = await model.folderChooser?.choose(after: path, in: model.snapshot), !picked.isEmpty else { return }
        path = picked
        existing = true
        counts = nil
        refusal = nil
    }

    /// True once the folder is set up. A folder already there answers its counts
    /// first, which the sheet shows before the person sends again.
    public func setUp() async -> Bool {
        working = true
        defer { working = false }
        let action = HostAction.setUpTeamFolder(folderID: folderID, path: Self.absolute(path), existing: existing, confirmed: existing && counts != nil)
        switch await model.submit(action) {
        case .done:
            refusal = nil
            following = true
            return true
        case let .refused(answer):
            if answer.code == "confirm-needed", let counts = answer.counts {
                self.counts = counts
                refusal = nil
            } else {
                refusal = answer.text.isEmpty ? answer.code : answer.text
            }
            return false
        }
    }

    /// Where following a set-up stands.
    public enum Follow: Equatable, Sendable {
        case running, finished, failed
    }

    /// Where a set-up stands while the sheet follows it. The host reads the
    /// folder as syncing until its first download answers; a download that
    /// cannot go on gives the folder back, so it is no longer on this Mac. A
    /// folder not on this Mac yet gets a moment, since the host's next snapshot
    /// may still be on its way.
    public static func follow(_ folder: Folder?, waited: TimeInterval) -> Follow {
        guard let folder, folder.onThisMac else { return waited >= 2 ? .failed : .running }
        return folder.syncState == FolderSyncState.syncing ? .running : .finished
    }

    /// The host takes an absolute path; the sheet shows "~/…".
    public static func absolute(_ path: String) -> String { (path as NSString).expandingTildeInPath }
}
