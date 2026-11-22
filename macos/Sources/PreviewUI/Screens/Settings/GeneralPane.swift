import SwiftUI
import PreviewCore

struct GeneralPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var name: YourNameModel
    @State private var owner: FileTypeOwner
    @State private var confirmingStop = false
    private let actions: SettingsActions

    init(model: AppModel, actions: SettingsActions) {
        _name = State(initialValue: YourNameModel(model: model))
        _owner = State(initialValue: model.fileTypes?.owner() ?? .htmlclay)
        self.actions = actions
    }

    static func mode(_ snapshot: HostSnapshot) -> YourNameModel.Mode {
        YourNameModel.mode(snapshot)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.General.title)
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            KilnCard {
                dashboardRow
                KilnRowDivider()
                KilnRow(title: Strings.General.welcome) {
                    Button(Strings.General.showAgain, action: actions.showWelcome)
                        .buttonStyle(KilnButtonStyle(.plain))
                }
            }
            .padding(.bottom, 12)
            if SettingsControls.showsOpenAtLogin(isSample: model.isSample) {
                KilnCard {
                    KilnRow(title: Strings.General.openAtLogin) {
                        Toggle(isOn: Binding(
                            get: { model.snapshot.preferences.openAtLogin },
                            set: { model.send(.setOpenAtLogin($0)) }
                        )) {
                            Text(Strings.General.openAtLogin)
                        }
                        .toggleStyle(KilnSwitchStyle())
                    }
                }
                .padding(.bottom, 12)
            }
            KilnCard {
                fileTypeRow
                KilnRowDivider()
                KilnNoteRow(Strings.Files.openWithTip)
            }
            .padding(.bottom, 12)
            KilnGroupTitle(Strings.YourName.heading)
            nameCard
            Text(name.mode == .off ? Strings.YourName.offSub : Strings.YourName.note)
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 8)
                .padding(.bottom, 16)
            KilnCard {
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
            Text(Strings.General.documentsLine)
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.muted)
                .padding(.top, 10)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .alert(Strings.Files.stopConfirmTitle, isPresented: $confirmingStop) {
            Button(Strings.Files.cancel, role: .cancel) {}
            Button(Strings.Files.stopConfirm, role: .destructive) {
                Task { await stopOpening() }
            }
        } message: {
            Text(Strings.Files.stopConfirmBody)
        }
        .onAppear { readOwner() }
    }

    private var isHTMLClays: Bool { owner == .htmlclay }

    @ViewBuilder private var fileTypeRow: some View {
        if isHTMLClays {
            KilnRow(title: Strings.Files.switchTitle, subtitle: Strings.Files.opensSub) {
                fileTypeToggle
            }
        } else {
            KilnRow(title: Strings.Files.switchTitle,
                    subtitle: otherSub,
                    leading: { StatusDot(.amber) }) {
                fileTypeToggle
            }
        }
    }

    private var fileTypeToggle: some View {
        Toggle(isOn: Binding(
            get: { isHTMLClays },
            set: { on in
                if on {
                    Task { await makeDefault() }
                } else {
                    confirmingStop = true
                }
            }
        )) {
            Text(Strings.Files.switchTitle)
        }
        .toggleStyle(KilnSwitchStyle())
        .disabled(!model.isSample && model.fileTypes == nil)
    }

    private var otherSub: String {
        if case let .other(appName) = owner, let appName, !appName.isEmpty {
            return Strings.Files.otherSub(appName)
        }
        return Strings.Files.otherUnknownSub
    }

    private var nameCard: some View {
        KilnCard {
            KilnRow(title: Strings.YourName.useName) {
                Toggle(isOn: Binding(
                    get: { name.isOn },
                    set: { name.setUseName($0) }
                )) {
                    Text(Strings.YourName.useName)
                }
                .toggleStyle(KilnSwitchStyle())
            }
            switch name.mode {
            case .local:
                KilnRowDivider()
                KilnRow(title: Strings.YourName.name) {
                    VStack(alignment: .leading, spacing: 4) {
                        KilnInput(text: Binding(
                            get: { name.draft },
                            set: { name.setDraft($0) }
                        ), placeholder: Strings.YourName.placeholder)
                        if let problem = name.problem {
                            Text(problem)
                                .font(.system(size: 12.5))
                                .foregroundStyle(kiln.danger)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }
                    .frame(width: 220, alignment: .leading)
                }
            case .account:
                KilnRowDivider()
                if SettingsControls.showsAccountChange(isSample: name.isSample) {
                    KilnRow(title: name.accountName, subtitle: Strings.YourName.accountSub) {
                        KilnLink(Strings.YourName.change) {
                            name.sayChange()
                        }
                    }
                } else {
                    KilnRow(title: name.accountName, subtitle: Strings.YourName.accountSub) {
                        EmptyView()
                    }
                }
            case .off:
                EmptyView()
            }
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

    private func readOwner() {
        guard let fileTypes = model.fileTypes else {
            owner = .htmlclay
            return
        }
        owner = fileTypes.owner()
    }

    private func makeDefault() async {
        guard let fileTypes = model.fileTypes else {
            owner = .htmlclay
            return
        }
        do {
            try await fileTypes.makeDefault()
        } catch {
            model.say(error.localizedDescription)
        }
        readOwner()
    }

    private func stopOpening() async {
        guard let fileTypes = model.fileTypes else {
            owner = .other(name: nil)
            return
        }
        do {
            try await fileTypes.stopOpening()
        } catch {
            model.say(error.localizedDescription)
        }
        readOwner()
    }
}
