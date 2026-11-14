import Foundation

/// One folder page switch: the value it shows, whether the host would move it,
/// and the host's reason when it would not.
public struct FolderToggle: Equatable, Sendable {
    public let value: Bool
    public let enabled: Bool
    public let note: String?

    public init(value: Bool, enabled: Bool, note: String? = nil) {
        self.value = value
        self.enabled = enabled
        self.note = note
    }
}

/// The host action types a page or panel asks about, so no view writes them as text.
public enum HostActionType {
    public static let relinkFolder = "folder.relink"
    public static let setSynced = "folder.setSynced"
    public static let setFraming = "folder.setFraming"
    public static let merge = "folder.merge"
    public static let reveal = "folder.reveal"
    public static let documentOpen = "document.open"
    public static let documentReveal = "document.reveal"
    public static let documentForget = "document.forget"
    public static let documentExport = "document.export"
    public static let documentRecover = "document.recover"
    public static let documentVersions = "document.versions"
    public static let updateStart = "update.start"
    public static let syncSetPaused = "sync.setPaused"
    public static let requestAnswer = "request.answer"
    public static let programAdd = "program.add"
    public static let programSetAllTrusted = "program.setAllTrusted"
    public static let programRemoveAccess = "program.removeAccess"
    public static let programRelink = "program.relink"
    public static let programAskAgain = "program.askAgain"
    public static let programRemove = "program.remove"
    public static let programReveal = "program.reveal"
    public static let syncConnect = "sync.connect"
    public static let syncDisconnect = "sync.disconnect"
    public static let serviceRestart = "service.restart"
    public static let prefsSet = "prefs.set"
}

/// The folder page's decisions, away from the views: what each switch shows and
/// whether the host allows the move. A folder with no `can` is sample data, and
/// every move is allowed there.
public enum FolderControls {
    /// Find it… on a missing folder, only where the host can relink one.
    public static func canRelink(_ snapshot: HostSnapshot) -> Bool {
        snapshot.hostCan(HostActionType.relinkFolder)
    }

    public static func trusted(folder: Folder) -> FolderToggle {
        let enabled = folder.can?.stopTrusting ?? true
        return FolderToggle(value: folder.onThisMac,
                            enabled: enabled,
                            note: enabled ? nil : reason(folder.why.stopTrusting))
    }

    public static func served(folder: Folder, pending: HostAction?) -> FolderToggle {
        let value = servedValue(folder, pending: pending)
        let enabled = folder.served ? (folder.can?.stopServing ?? true) : (folder.can?.serve ?? true)
        return FolderToggle(value: value,
                            enabled: enabled,
                            note: enabled ? nil : reason(folder.why.serve))
    }

    public static func synced(folder: Folder, pending: HostAction?, hostCan: Bool) -> FolderToggle {
        let value = syncedValue(folder, pending: pending)
        let enabled = folder.synced ? (folder.can?.stopSyncing ?? true) : (folder.can?.sync ?? true)
        return FolderToggle(value: value,
                            enabled: enabled && hostCan,
                            note: enabled ? nil : reason(folder.why.sync))
    }

    /// Frames allowed for a served folder: the Mac app's choice, never the dashboard's.
    public static func framing(folder: Folder, pending: HostAction?, hostCan: Bool) -> FolderToggle {
        let value = framingValue(folder, pending: pending)
        let enabled = (folder.can?.setFraming ?? true) && hostCan
        return FolderToggle(value: value, enabled: enabled)
    }

    public static func showsMerge(child: Folder, hostCan: Bool) -> Bool {
        (child.can?.merge ?? true) && hostCan
    }

    private static func servedValue(_ folder: Folder, pending: HostAction?) -> Bool {
        if case let .setServed(_, on)? = pending { return on }
        return folder.served
    }

    private static func syncedValue(_ folder: Folder, pending: HostAction?) -> Bool {
        if case let .setSynced(_, on)? = pending { return on }
        return folder.synced
    }

    private static func framingValue(_ folder: Folder, pending: HostAction?) -> Bool {
        if case let .setFraming(_, on)? = pending { return on }
        return folder.allowsFraming
    }

    private static func reason(_ why: HostReason?) -> String? {
        guard let text = why?.text, !text.isEmpty else { return nil }
        return text
    }
}
