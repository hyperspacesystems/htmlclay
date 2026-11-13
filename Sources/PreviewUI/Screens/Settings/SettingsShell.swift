import SwiftUI
import PreviewCore

public struct SettingsActions {
    public var openDashboard: () -> Void
    public var showWelcome: () -> Void
    public var answer: (WaitingItem) -> Void
    public init(openDashboard: @escaping () -> Void = {}, showWelcome: @escaping () -> Void = {}, answer: @escaping (WaitingItem) -> Void = { _ in }) {
        self.openDashboard = openDashboard; self.showWelcome = showWelcome; self.answer = answer
    }
}

public struct SettingsView: View {
    @Environment(AppModel.self) private var model
    private let pinned: SettingsRoute?
    private let actions: SettingsActions

    public init(actions: SettingsActions = SettingsActions()) {
        self.pinned = nil
        self.actions = actions
    }

    public init(pane: SettingsPane, actions: SettingsActions = SettingsActions()) {
        self.pinned = .pane(pane)
        self.actions = actions
    }

    public init(route: SettingsRoute, actions: SettingsActions = SettingsActions()) {
        self.pinned = route
        self.actions = actions
    }

    private var activeRoute: SettingsRoute { pinned ?? model.route }

    private var activePane: SettingsPane { activeRoute.pane }

    public var body: some View {
        HStack(spacing: 0) {
            SettingsSidebar(active: activePane)
            pane
        }
        .frame(width: 840)
        .frame(minHeight: 490)
        .sheet(item: Binding(get: { pinned == nil ? model.sheet : nil }, set: { model.sheet = $0 })) { sheet in
            KilnRoot { SettingsSheetView(sheet: sheet, actions: actions) }.environment(model)
        }
        .overlay(alignment: .bottom) {
            if pinned == nil, let note = model.note {
                PreviewNote(note)
                    .padding(.leading, 246)
                    .padding(.trailing, 24)
                    .padding(.bottom, 6)
            }
        }
    }

    private var pane: some View {
        ScrollView(.vertical) {
            VStack(alignment: .leading, spacing: 0) {
                switch activePane {
                case .general: GeneralPane(actions: actions)
                case .folders:
                    switch activeRoute {
                    case let .folder(id):
                        if model.snapshot.folder(id) != nil {
                            FolderPage(folderID: id, actions: actions)
                        } else {
                            FoldersPane()
                        }
                    default: FoldersPane()
                    }
                case .programs:
                    switch activeRoute {
                    case let .program(id):
                        if model.snapshot.program(id) != nil {
                            ProgramPage(programID: id, actions: actions)
                        } else {
                            ProgramsPane(actions: actions)
                        }
                    default: ProgramsPane(actions: actions)
                    }
                case .sync: SyncPane(actions: actions)
                case .yourName: YourNamePane()
                case .updates: UpdatesPane()
                case .advanced: AdvancedPane()
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.top, 18)
        .padding(.horizontal, 24)
        .padding(.bottom, 24)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}
