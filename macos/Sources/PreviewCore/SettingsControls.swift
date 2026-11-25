import Foundation

/// The dot kinds a status row offers: green for good, amber for in between,
/// red for stopped, blue for working, muted for off.
public enum StatusDotKind: CaseIterable {
    case green, amber, blue, red, muted
}

public struct AIToolRow: Equatable, Sendable {
    public var id: String
    public var title: String
    public var subtitle: String
    public var selected: Bool
    public var found: Bool
    public var model: String
    public var selectedModel: String
    public var models: [String]
    public var placeholder: String
    public var custom: Bool
    public var selectable: Bool
    public var effort: String
    public var selectedEffort: String
    public var efforts: [String]
    public var effortTitle: String

    public init(id: String, title: String, subtitle: String, selected: Bool = false, found: Bool = true,
                model: String = "", selectedModel: String = "", models: [String] = [], placeholder: String = "",
                custom: Bool = false, selectable: Bool = true,
                effort: String = "", selectedEffort: String = "", efforts: [String] = [], effortTitle: String = "") {
        self.id = id; self.title = title; self.subtitle = subtitle; self.selected = selected; self.found = found
        self.model = model; self.selectedModel = selectedModel.isEmpty ? (model.isEmpty ? placeholder : model) : selectedModel
        self.models = models; self.placeholder = placeholder; self.custom = custom; self.selectable = selectable
        self.effort = effort; self.selectedEffort = selectedEffort.isEmpty ? effort : selectedEffort
        self.efforts = efforts; self.effortTitle = effortTitle
    }
}

public enum SettingsControls {
    private static let builtinOrder = ["claude", "codex", "agy"]

    private static let builtinDefaults: [String: AIEditTool] = [
        "claude": AIEditTool(
            id: "claude",
            defaultModel: "claude-opus-5-5",
            models: ["claude-opus-5-5", "claude-sonnet-5-5", "claude-fable-5-2"],
            defaultEffort: "high",
            efforts: ["low", "medium", "high", "xhigh", "max"]
        ),
        "codex": AIEditTool(
            id: "codex",
            defaultModel: "gpt-5.6-sol",
            models: ["gpt-5.6-sol", "gpt-6-astra"],
            defaultEffort: "high",
            efforts: ["low", "medium", "high", "xhigh"]
        ),
        "agy": AIEditTool(
            id: "agy",
            defaultModel: "gemini-4-carbon",
            models: ["gemini-4-carbon", "gemini-4-neon", "gemini-4-argon", "gemini-3.8-flash"],
            defaultEffort: "high",
            efforts: ["low", "medium", "high", "xhigh"],
            modelEfforts: [
                "gemini-4-neon": ["low", "medium", "high", "xhigh"],
                "gemini-4-carbon": ["low", "medium", "high"],
                "gemini-4-argon": ["low", "medium", "high"],
                "gemini-3.8-flash": ["low", "medium", "high"],
            ]
        ),
    ]

    public static func aiToolRows(_ prefs: PreferenceState) -> [AIToolRow] {
        var rows: [AIToolRow] = []
        var seen: Set<String> = []
        for id in builtinOrder {
            seen.insert(id)
            let fallback = builtinDefaults[id]!
            let incoming = prefs.aiEditTools.first(where: { !$0.custom && $0.id == id })
            rows.append(makeBuiltinRow(incoming: incoming, fallback: fallback, selectedID: prefs.aiEditTool))
        }
        for tool in prefs.aiEditTools where !tool.custom && !seen.contains(tool.id) {
            rows.append(makeBuiltinRow(incoming: tool, fallback: tool, selectedID: prefs.aiEditTool))
        }
        if let custom = prefs.aiEditTools.first(where: { $0.custom }) {
            rows.append(AIToolRow(id: custom.id,
                                  title: Strings.General.aiCustomTitle,
                                  subtitle: custom.command.isEmpty ? Strings.General.aiCustomSub : custom.command,
                                  selected: prefs.aiEditTool == custom.id,
                                  found: !custom.command.isEmpty,
                                  custom: true,
                                  selectable: !custom.command.isEmpty))
        } else {
            rows.append(AIToolRow(id: "custom", title: Strings.General.aiCustomTitle,
                                  subtitle: Strings.General.aiCustomSub, found: false, custom: true, selectable: false))
        }
        return rows
    }

    private static func makeBuiltinRow(incoming: AIEditTool?, fallback: AIEditTool, selectedID: String) -> AIToolRow {
        let id = incoming?.id ?? fallback.id
        let found = incoming?.found ?? false
        let defaultModel = (incoming?.defaultModel.isEmpty == false) ? incoming!.defaultModel : fallback.defaultModel
        let model = incoming?.model ?? ""
        let selectedModel = model.isEmpty ? defaultModel : model
        var models = (incoming?.models.isEmpty == false) ? incoming!.models : fallback.models
        if !selectedModel.isEmpty && !models.contains(selectedModel) {
            models.append(selectedModel)
        }
        let defaultEffort = (incoming?.defaultEffort.isEmpty == false) ? incoming!.defaultEffort : fallback.defaultEffort
        let mergedTool = AIEditTool(
            id: id,
            found: found,
            model: model,
            defaultModel: defaultModel,
            models: models,
            effort: incoming?.effort ?? "",
            defaultEffort: defaultEffort,
            efforts: (incoming?.efforts.isEmpty == false) ? incoming!.efforts : fallback.efforts,
            modelEfforts: incoming?.modelEfforts ?? fallback.modelEfforts
        )
        let allowedEfforts = mergedTool.efforts(for: selectedModel)
        let rawEffort = mergedTool.effort.isEmpty ? defaultEffort : mergedTool.effort
        let selectedEffort: String
        if allowedEfforts.isEmpty {
            selectedEffort = rawEffort
        } else if allowedEfforts.contains(rawEffort) {
            selectedEffort = rawEffort
        } else if allowedEfforts.contains(defaultEffort) {
            selectedEffort = defaultEffort
        } else {
            selectedEffort = allowedEfforts.last ?? rawEffort
        }
        let subtitle = aiToolSubtitle(id: id, found: found)
        return AIToolRow(
            id: id,
            title: aiToolTitle(id),
            subtitle: subtitle,
            selected: selectedID == id,
            found: found,
            model: model,
            selectedModel: selectedModel,
            models: models,
            placeholder: aiToolPlaceholder(mergedTool),
            custom: false,
            selectable: found,
            effort: mergedTool.effort,
            selectedEffort: selectedEffort,
            efforts: allowedEfforts,
            effortTitle: Strings.General.aiEffortName(selectedEffort)
        )
    }

    public static func showsNoToolsNote(_ prefs: PreferenceState) -> Bool {
        !prefs.aiEditTools.contains { !$0.custom && $0.found }
    }

    public static func aiToolTitle(_ id: String) -> String {
        switch id {
        case "claude": return Strings.General.aiToolClaude
        case "codex": return Strings.General.aiToolCodex
        case "agy": return Strings.General.aiToolGemini
        default: return id
        }
    }

    private static func aiToolSubtitle(id: String, found: Bool) -> String {
        if id == "agy" {
            return found ? Strings.General.aiAgyFound : Strings.General.aiAgyNotFound
        }
        return found ? Strings.General.aiFoundHere : Strings.General.aiNotFound
    }

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
