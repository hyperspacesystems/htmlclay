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

    /// The names a built-in program asks under.
    public static let fileSearchID = "file-search"
    public static let fileConvertID = "file-convert"
    public static let folderWatchID = "folder-watch"

    /// The three programs HTML Clay ships, in the order the Programs page lists them.
    public static let builtinIDs = [fileSearchID, fileConvertID, folderWatchID]

    public static func isAIEditing(_ program: Program) -> Bool { program.id == aiEditID }

    /// One of the three programs HTML Clay runs itself, not the AI editing row.
    public static func isBuiltinProgram(_ program: Program) -> Bool {
        program.isBuiltin && !isAIEditing(program)
    }

    /// The title a built-in program's id stands for, and the id itself for any
    /// other program: a request never shows `file-search` to a person.
    public static func programTitle(_ id: String) -> String {
        switch id {
        case fileSearchID: return Strings.Programs.fileSearch
        case fileConvertID: return Strings.Programs.fileConverter
        case folderWatchID: return Strings.Programs.folderWatcher
        default: return id
        }
    }

    /// The built-in programs in a snapshot, in the page's order.
    public static func builtinPrograms(_ snapshot: HostSnapshot) -> [Program] {
        builtinIDs.compactMap { id in snapshot.programs.first { $0.id == id && isBuiltinProgram($0) } }
    }

    /// Every program a person added, as the pane lists them.
    public static func userPrograms(_ snapshot: HostSnapshot) -> [Program] {
        snapshot.programs.filter { !$0.isBuiltin }
    }

    /// A built-in row's subtitle: what it does, then who may run it once it is on.
    public static func builtinSubtitle(_ program: Program) -> String {
        let does = program.isOn ? builtinDoes(program.id) : builtinOffSub(program.id)
        guard program.isOn else { return does }
        return does + Strings.Programs.builtinDot + builtinWho(program)
    }

    private static func builtinWho(_ program: Program) -> String {
        if program.allTrustedFolders { return Strings.Programs.allTrusted }
        switch program.allowedDocuments.count {
        case 0: return Strings.Programs.noneYetOn
        case 1: return Strings.Programs.mayRun(documentLabel(program.allowedDocuments[0]))
        default: return Strings.Programs.mayRunCount(program.allowedDocuments.count)
        }
    }

    private static func builtinDoes(_ id: String) -> String {
        switch id {
        case fileSearchID: return Strings.Programs.searchDoes
        case fileConvertID: return Strings.Programs.convertDoes
        default: return Strings.Programs.watchDoes
        }
    }

    private static func builtinOffSub(_ id: String) -> String {
        switch id {
        case fileSearchID: return Strings.Programs.searchOffSub
        case fileConvertID: return Strings.Programs.convertOffSub
        default: return Strings.Programs.watchOffSub
        }
    }

    /// The sentence on the built-in page's Enable row.
    public static func builtinEnableSub(_ id: String) -> String {
        switch id {
        case fileSearchID: return Strings.Programs.searchEnableSub
        case fileConvertID: return Strings.Programs.convertEnableSub
        default: return Strings.Programs.watchEnableSub
        }
    }

    /// The example document a built-in saves and opens.
    public static func exampleName(_ id: String) -> String {
        switch id {
        case fileSearchID: return Strings.Programs.searchExample
        case fileConvertID: return Strings.Programs.convertExample
        default: return Strings.Programs.watchExample
        }
    }

    public static func exampleSub(_ id: String) -> String {
        switch id {
        case fileSearchID: return Strings.Programs.searchExampleSub
        case fileConvertID: return Strings.Programs.convertExampleSub
        default: return Strings.Programs.watchExampleSub
        }
    }

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
