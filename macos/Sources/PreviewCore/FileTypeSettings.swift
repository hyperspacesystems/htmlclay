import Foundation

/// Who opens .htmlclay files on this Mac, as a pane sees it.
public enum FileTypeOwner: Equatable, Sendable {
    case htmlclay
    case other(name: String?)
}

/// The two file-type changes a pane can make. The Mac app sets the default
/// itself; the preview has no file types at all.
@MainActor
public protocol FileTypeSettings: AnyObject {
    func owner() -> FileTypeOwner
    func makeDefault() async throws
    func stopOpening() async throws
}
