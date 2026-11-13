import SwiftUI
import PreviewCore

public struct SettingsActions {
    public var openDashboard: () -> Void
    public var showWelcome: () -> Void
    public init(openDashboard: @escaping () -> Void = {}, showWelcome: @escaping () -> Void = {}) {
        self.openDashboard = openDashboard; self.showWelcome = showWelcome
    }
}

public struct SettingsView: View {
    @Environment(AppModel.self) private var model
    private let pinned: SettingsPane?
    private let actions: SettingsActions

    public init(actions: SettingsActions = SettingsActions()) {
        self.pinned = nil
        self.actions = actions
    }

    public init(pane: SettingsPane, actions: SettingsActions = SettingsActions()) {
        self.pinned = pane
        self.actions = actions
    }

    private var activePane: SettingsPane { pinned ?? model.route.pane }

    public var body: some View {
        HStack(spacing: 0) {
            SettingsSidebar(active: activePane)
            pane
        }
        .frame(width: 840)
        .frame(minHeight: 490)
    }

    private var pane: some View {
        ScrollView(.vertical) {
            VStack(alignment: .leading, spacing: 0) {
                switch activePane {
                case .general: GeneralPane(actions: actions)
                default: PaneStub(pane: activePane)
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

struct PaneStub: View {
    @Environment(\.kiln) private var kiln
    private let pane: SettingsPane
    init(pane: SettingsPane) { self.pane = pane }
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(pane))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            PreviewNote(Strings.Preview.notYet)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
