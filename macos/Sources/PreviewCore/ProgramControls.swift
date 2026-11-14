import Foundation

/// The programs pane and program page decisions, away from the views: which
/// control the host can run, what a program file row says, and how a file
/// warning reads. Sample data answers every `can` with true.
public enum ProgramControls {
    public static func canAdd(_ snapshot: HostSnapshot) -> Bool {
        snapshot.hostCan(HostActionType.programAdd)
    }

    public static func canRelink(_ snapshot: HostSnapshot) -> Bool {
        snapshot.hostCan(HostActionType.programRelink)
    }

    public static func canAskAgain(_ snapshot: HostSnapshot) -> Bool {
        snapshot.hostCan(HostActionType.programAskAgain)
    }

    public static func canRemoveAccess(_ snapshot: HostSnapshot) -> Bool {
        snapshot.hostCan(HostActionType.programRemoveAccess)
    }

    public static func canRemove(_ snapshot: HostSnapshot) -> Bool {
        snapshot.hostCan(HostActionType.programRemove)
    }

    public static func canReveal(_ snapshot: HostSnapshot) -> Bool {
        snapshot.hostCan(HostActionType.programReveal)
    }

    public static func canSetAllTrusted(_ snapshot: HostSnapshot) -> Bool {
        snapshot.hostCan(HostActionType.programSetAllTrusted)
    }

    /// The switch shows the newest value the person chose while it is pending.
    public static func allTrusted(_ program: Program, pending: HostAction?) -> Bool {
        if case let .setProgramAllTrusted(_, on)? = pending { return on }
        return program.allTrustedFolders
    }

    /// The codes a sheet shows, which are the codes the action acknowledges.
    public static func acknowledged(_ warnings: [HostWarning]) -> [String] {
        var codes: [String] = []
        for warning in warnings where !codes.contains(warning.code) {
            codes.append(warning.code)
        }
        return codes
    }

    /// What one warning says: the host's sentence, or the sentence for its code.
    public static func warningLine(_ warning: HostWarning) -> String? {
        if !warning.text.isEmpty { return warning.text }
        return Strings.Request.detailText(warning.code)
    }

    public static func warningLines(_ warnings: [HostWarning]) -> [String] {
        warnings.compactMap(warningLine)
    }

    /// The id the built-in AI editing goes by wherever a program would appear.
    public static let aiEditID = "ai-edit"

    public static func isAIEditing(_ program: Program) -> Bool { program.id == aiEditID }

    /// The file a program runs, shown by its name.
    public static func fileName(_ program: Program) -> String {
        if isAIEditing(program) { return Strings.General.aiTitle }
        let last = (program.path as NSString).lastPathComponent
        return last.isEmpty ? program.name : last
    }

    /// A document the host lists by path, shown by its file name.
    public static func documentLabel(_ document: String) -> String {
        document.contains("/") ? (document as NSString).lastPathComponent : document
    }
}
