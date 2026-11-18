import Foundation

/// The dot kinds a status row offers: green for good, amber for in between,
/// red for stopped, blue for working, muted for off.
public enum StatusDotKind: CaseIterable {
    case green, amber, blue, red, muted
}

/// One row of the AI editing tool picker: a tool found on this Mac, or the
/// custom command the person set up.
public struct AIToolRow: Equatable, Sendable {
    public var id: String
    public var title: String
    public var subtitle: String
    public var selected: Bool
    public var model: String
    public var placeholder: String
    /// True for the custom command row, which has no model field.
    public var custom: Bool
    /// False for the custom command row before a command is set: there is nothing to pick yet.
    public var selectable: Bool
    /// The reasoning effort chosen ("" for the default) and the ones to pick
    /// from; empty for a tool with no effort setting.
    public var effort: String
    public var efforts: [String]
    public var effortTitle: String

    public init(id: String, title: String, subtitle: String, selected: Bool = false,
                model: String = "", placeholder: String = "", custom: Bool = false, selectable: Bool = true,
                effort: String = "", efforts: [String] = [], effortTitle: String = "") {
        self.id = id; self.title = title; self.subtitle = subtitle; self.selected = selected
        self.model = model; self.placeholder = placeholder; self.custom = custom; self.selectable = selectable
        self.effort = effort; self.efforts = efforts; self.effortTitle = effortTitle
    }
}

/// The settings window's decisions, away from the views: sample data shows every
/// pane and control, and the real host shows the ones it says it can run.
public enum SettingsControls {
    /// The AI editing tool picker: every built-in tool found on this Mac, then
    /// the custom command, selected from the host's aiEditTool preference.
    public static func aiToolRows(_ prefs: PreferenceState) -> [AIToolRow] {
        var rows: [AIToolRow] = []
        for tool in prefs.aiEditTools where !tool.custom && tool.found {
            rows.append(AIToolRow(id: tool.id,
                                  title: aiToolTitle(tool.id),
                                  subtitle: Strings.General.aiFoundHere,
                                  selected: prefs.aiEditTool == tool.id,
                                  model: tool.model,
                                  placeholder: aiToolPlaceholder(tool),
                                  effort: tool.effort,
                                  efforts: tool.efforts,
                                  effortTitle: Strings.General.aiEffortName(tool.effort.isEmpty ? tool.defaultEffort : tool.effort)))
        }
        for tool in prefs.aiEditTools where tool.custom {
            rows.append(AIToolRow(id: tool.id,
                                  title: Strings.General.aiCustomTitle,
                                  subtitle: tool.command.isEmpty ? Strings.General.aiCustomSub : tool.command,
                                  selected: prefs.aiEditTool == tool.id,
                                  custom: true))
        }
        if !prefs.aiEditTools.contains(where: { $0.custom }) {
            rows.append(AIToolRow(id: "custom", title: Strings.General.aiCustomTitle,
                                  subtitle: Strings.General.aiCustomSub, custom: true, selectable: false))
        }
        return rows
    }

    /// The note under the picker, shown only when neither Claude nor Codex is installed.
    public static func showsNoToolsNote(_ prefs: PreferenceState) -> Bool {
        !prefs.aiEditTools.contains { !$0.custom && $0.found }
    }

    private static func aiToolTitle(_ id: String) -> String {
        switch id {
        case "claude": return Strings.General.aiToolClaude
        case "codex": return Strings.General.aiToolCodex
        default: return id
        }
    }

    /// The model field is empty with the tool's own default as its placeholder.
    private static func aiToolPlaceholder(_ tool: AIEditTool) -> String {
        if !tool.defaultModel.isEmpty { return tool.defaultModel }
        return tool.id == "codex" ? Strings.General.aiCodexDefault : Strings.General.aiModel
    }

    private static let programActions = [
        HostActionType.programAdd,
        HostActionType.programSetAllTrusted,
        HostActionType.programRemoveAccess,
    ]

    private static let syncActions = [
        HostActionType.syncConnect,
        HostActionType.syncDisconnect,
    ]

    /// Open at Login shows on a live host too: the host saves the preference and the app's login item follows it (11.16).
    public static func showsOpenAtLogin(isSample: Bool) -> Bool { true }

    /// The account row's Change link opens hyperclay.com, which has no host action.
    public static func showsAccountChange(isSample: Bool) -> Bool { isSample }

    /// The automatic-update switch, What's New, and the restart row: the host the
    /// Mac app starts never runs its updater.
    public static func showsUpdateControls(can: Bool) -> Bool {
        PanelControls.showsUpdate(can: can)
    }

    /// The sync log row is offered once this Mac has had sync on.
    public static func showsSyncLog(_ sync: SyncState) -> Bool { sync.everOn }

    /// The port typed into Change port…: nil when it is one HTML Clay can use.
    public static func portProblem(_ text: String) -> String? {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, trimmed.allSatisfy(\.isNumber), let port = Int(trimmed),
              port >= 1024, port <= 65535 else { return Strings.Advanced.portRange }
        return nil
    }

    /// The sync log row: sample data shows it, and a live host shows it once it
    /// offers sync.revealLog (08-sync.md 8.43).
    public static func showsSyncLog(isSample: Bool, can: Bool) -> Bool { isSample || can }

    /// The port every address on this Mac uses: sample data shows the designed one.
    public static func port(isSample: Bool, port: Int) -> String {
        isSample ? Strings.Advanced.portNumber : String(port)
    }

    /// The live service row's dot and text, from the host's `host.service`.
    public static func serviceStatus(_ service: String) -> (dot: StatusDotKind, text: String) {
        switch service {
        case "running": return (.green, Strings.Advanced.running)
        case "starting", "restarting": return (.amber, Strings.Advanced.starting)
        case "failed": return (.red, Strings.Advanced.stopped)
        default: return (.muted, Strings.Advanced.off)
        }
    }

    /// Whether the live service row offers Restart: only once the service has
    /// stopped by itself, and only when the host can restart it.
    public static func offersServiceRestart(_ service: String, hostCan: Bool, isSample: Bool) -> Bool {
        !isSample && hostCan && service == "failed"
    }

    /// The dashboard address without its scheme and trailing slash.
    public static func dashboardAddress(_ url: String) -> String {
        var address = url
        for prefix in ["http://", "https://"] where address.hasPrefix(prefix) {
            address.removeFirst(prefix.count)
            break
        }
        while address.hasSuffix("/") { address.removeLast() }
        return address
    }

    /// The storage totals: the host does not send them yet, so only sample data shows them.
    public static func showsStorageTotals(isSample: Bool) -> Bool { isSample }

    /// Restarting the live service.
    public static func showsServiceRestart(can: Bool) -> Bool { can }

    /// Whether the sidebar lists a pane: the ones whose actions the host cannot run
    /// at all are hidden, and the ones that only display settings always show.
    public static func showsPane(_ pane: SettingsPane, in snapshot: HostSnapshot) -> Bool {
        switch pane {
        case .programs: return snapshot.isSample || any(programActions, snapshot)
        case .sync: return snapshot.isSample || any(syncActions, snapshot)
        default: return true
        }
    }

    /// The pane the settings window opens on: a route into a hidden pane shows General.
    public static func visibleRoute(_ route: SettingsRoute, in snapshot: HostSnapshot) -> SettingsRoute {
        showsPane(route.pane, in: snapshot) ? route : .pane(.general)
    }

    private static func any(_ types: [String], _ snapshot: HostSnapshot) -> Bool {
        types.contains { snapshot.hostCan($0) }
    }
}
