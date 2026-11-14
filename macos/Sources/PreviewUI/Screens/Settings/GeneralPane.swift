import SwiftUI
import PreviewCore

struct GeneralPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let actions: SettingsActions

    init(actions: SettingsActions) { self.actions = actions }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.General.title)
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            KilnCard {
                if SettingsControls.showsOpenAtLogin(isSample: model.isSample) {
                    KilnRow(title: Strings.General.openAtLogin) {
                        Toggle(isOn: Binding(
                            get: { model.snapshot.preferences.openAtLogin },
                            set: { model.send(.setOpenAtLogin($0)) }
                        )) {
                            EmptyView()
                        }
                        .toggleStyle(KilnSwitchStyle())
                    }
                    KilnRowDivider()
                }
                KilnRow(title: Strings.General.aiTitle, subtitle: Strings.General.aiSub) {
                    Toggle(isOn: Binding(
                        get: { model.snapshot.preferences.aiEditing },
                        set: { model.send(.setAIEditing($0)) }
                    )) {
                        EmptyView()
                    }
                    .toggleStyle(KilnSwitchStyle())
                }
                KilnRowDivider()
                KilnRow(title: Strings.General.appearanceTitle, subtitle: Strings.General.appearanceSub) {
                    KilnSegmented(selection: Binding(
                        get: { model.snapshot.preferences.appearance },
                        set: { model.send(.setAppearance($0)) }
                    ), options: [
                        (value: Appearance.system, label: Strings.General.appearanceSystem),
                        (value: Appearance.light, label: Strings.General.appearanceLight),
                        (value: Appearance.dark, label: Strings.General.appearanceDark),
                    ])
                }
            }
            .padding(.bottom, 12)
            KilnCard {
                dashboardRow
                KilnRowDivider()
                KilnRow(title: Strings.General.welcome) {
                    Button(Strings.General.showAgain, action: actions.showWelcome)
                        .buttonStyle(KilnButtonStyle(.plain))
                }
            }
            Text(Strings.General.documentsLine)
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.muted)
                .padding(.top, 10)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var dashboardRow: some View {
        HStack(spacing: 11) {
            VStack(alignment: .leading, spacing: 0) {
                Text(Strings.General.dashboard)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(kiln.ink)
                Text(Strings.General.dashboardAddress)
                    .font(.system(size: 12.5, design: .monospaced))
                    .foregroundStyle(kiln.muted)
                    .padding(.top, 2)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Button(Strings.General.open, action: actions.openDashboard)
                .buttonStyle(KilnButtonStyle(.plain))
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
}
