import Foundation

@MainActor
public protocol FolderChooser: AnyObject {
    /// The folder a new sheet starts with, or nil to start empty.
    func first(in snapshot: HostSnapshot) -> String?
    /// The next folder after the person presses Choose…, or nil when they cancel.
    func choose(after current: String?, in snapshot: HostSnapshot) async -> String?
    /// The next folder the Trust sheet offers, or nil when they cancel.
    func chooseToTrust(after current: String?, in snapshot: HostSnapshot) async -> String?
    /// The file the person picks to open, or nil when they cancel.
    func chooseFile() async -> String?
    /// The program file the person picks for a request, or nil when they cancel.
    func chooseProgram() async -> String?
    /// Where a missing document should be recovered, or nil when they cancel.
    func chooseDestination(suggestedName: String) async -> String?
}

public extension FolderChooser {
    /// The preview has no program files, so a request's Choose a file… does nothing there.
    func chooseProgram() async -> String? { nil }
    /// The preview has no save panel, so a recovery there does nothing.
    func chooseDestination(suggestedName: String) async -> String? { nil }
}

/// The preview's chooser: cycles SampleData.servePaths, starting at the first one ServeRules accepts.
@MainActor
public final class SampleFolderChooser: FolderChooser {
    public init() {}

    public func first(in snapshot: HostSnapshot) -> String? {
        ServeRules.path(folderID: nil, pick: ServeRules.firstPick(in: snapshot), in: snapshot)
    }

    public func choose(after current: String?, in snapshot: HostSnapshot) async -> String? {
        let paths = SampleData.servePaths
        guard let current, let index = paths.firstIndex(of: current) else { return paths.first }
        return paths[(index + 1) % paths.count]
    }

    public func chooseToTrust(after current: String?, in snapshot: HostSnapshot) async -> String? {
        guard let current else { return TrustRules.firstPick(in: snapshot)?.path }
        return TrustRules.pick(after: current, in: snapshot)?.path
    }

    public func chooseFile() async -> String? { nil }
}
