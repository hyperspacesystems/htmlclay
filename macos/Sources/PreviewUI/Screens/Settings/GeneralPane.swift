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
                KilnRow(title: Strings.General.aiTitle, subtitle: Strings.General.aiSub) {
                    Toggle(isOn: Binding(
                        get: { model.snapshot.preferences.aiEditing },
                        set: { model.send(.setAIEditing($0)) }
                    )) {
                        EmptyView()
                    }
                    .toggleStyle(KilnSwitchStyle())
                }
                if model.snapshot.preferences.aiEditing {
                    KilnRowDivider()
                    toolRows
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

    @ViewBuilder private var toolRows: some View {
        let rows = SettingsControls.aiToolRows(model.snapshot.preferences)
        ForEach(rows, id: \.id) { row in
            KilnRow(title: row.title, subtitle: row.subtitle,
                    leading: {
                        Button {
                            model.send(.setAIEditTool(row.id))
                        } label: {
                            KilnRadio(on: row.selected)
                        }
                        .buttonStyle(.plain)
                        .disabled(row.selected || !row.selectable)
                    }) {
                        if row.custom {
                            Button(Strings.General.aiCustomButton) {
                                model.sheet = .customCommand
                            }
                            .buttonStyle(KilnButtonStyle(.plain))
                        } else {
                            VStack(alignment: .trailing, spacing: 2) {
                                Text(Strings.General.aiModel)
                                    .font(.system(size: 11.5))
                                    .foregroundStyle(kiln.muted)
                                ModelField(row: row) { text in
                                    model.send(.setAIEditModel(tool: row.id, model: text))
                                }
                                if !row.efforts.isEmpty {
                                    Text(Strings.General.aiEffort)
                                        .font(.system(size: 11.5))
                                        .foregroundStyle(kiln.muted)
                                    Menu(row.effortTitle) {
                                        ForEach(row.efforts, id: \.self) { effort in
                                            Button(Strings.General.aiEffortName(effort)) {
                                                model.send(.setAIEditEffort(tool: row.id, effort: effort))
                                            }
                                        }
                                    }
                                }
                            }
                            .frame(width: 190, alignment: .trailing)
                        }
                    }
        }
        if SettingsControls.showsNoToolsNote(model.snapshot.preferences) {
            Text(Strings.General.aiNoTools)
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.muted)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
        }
    }

    private var dashboardRow: some View {
        HStack(spacing: 11) {
            VStack(alignment: .leading, spacing: 0) {
                Text(Strings.General.dashboard)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(kiln.ink)
                Text(SettingsControls.dashboardAddress(model.isSample ? Strings.General.dashboardAddress : model.snapshot.host.dashboardURL))
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

/// The inline Model field commits on Return and on focus loss.
private struct ModelField: View {
    private let row: AIToolRow
    private let commit: (String) -> Void
    @State private var text: String
    @FocusState private var focused: Bool

    init(row: AIToolRow, commit: @escaping (String) -> Void) {
        self.row = row
        self.commit = commit
        _text = State(initialValue: row.model)
    }

    var body: some View {
        KilnInput(text: $text, placeholder: row.placeholder, mono: true)
            .focused($focused)
            .onSubmit { commit(text) }
            .onChange(of: focused) { _, isFocused in
                if !isFocused { commit(text) }
            }
            .onChange(of: row.model) { _, model in
                if !focused { text = model }
            }
    }
}
