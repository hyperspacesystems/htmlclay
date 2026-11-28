import SwiftUI
import PreviewCore

struct AdvancedPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var confirmingClear = false
    @State private var confirmingOff = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(.advanced))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            let service = SettingsControls.serviceStatus(model.snapshot.host.service)
            KilnCard {
                KilnRow(title: Strings.Advanced.port, subtitle: Strings.Advanced.portSub) {
                    HStack(spacing: 10) {
                        Text(SettingsControls.port(isSample: model.isSample, port: model.snapshot.host.port))
                            .font(.system(size: 13, design: .monospaced))
                            .foregroundStyle(kiln.ink2)
                        Button(Strings.Advanced.changePort) {
                            model.sheet = .changePort
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                        .disabled(!model.isSample && !model.can(HostActionType.setPort))
                    }
                }
                KilnRowDivider()
                KilnRow(title: Strings.Advanced.liveService, subtitle: Strings.Advanced.liveServiceSub) {
                    HStack(spacing: 6) {
                        StatusDot(model.isSample ? .green : service.dot)
                        Text(model.isSample ? Strings.Advanced.running : service.text)
                            .font(.system(size: 13))
                            .foregroundStyle(kiln.ink2)
                        if SettingsControls.offersServiceRestart(model.snapshot.host.service,
                                                                 hostCan: model.can(HostActionType.serviceRestart),
                                                                 isSample: model.isSample) {
                            Button(Strings.Panel.restart) { model.send(.restartService) }
                                .buttonStyle(KilnButtonStyle(.plain))
                        }
                    }
                }
            }
            KilnGroupTitle(Strings.Offline.title)
            KilnCard {
                KilnRow(title: Strings.Offline.switchTitle, subtitle: Strings.Offline.help) {
                    Toggle(isOn: Binding(
                        get: { model.snapshot.preferences.offlineCopies },
                        set: { on in
                            if on { model.send(.setOfflineCopies(true)) } else { confirmingOff = true }
                        }
                    )) {
                        Text(Strings.Offline.switchTitle)
                    }
                    .toggleStyle(KilnSwitchStyle())
                    .disabled(!model.isSample && !model.can(HostActionType.prefsSet))
                }
                if model.snapshot.preferences.offlineCopies {
                    KilnRowDivider()
                    KilnRow(title: Strings.Offline.space, subtitle: Strings.Offline.spaceHelp) {
                        KilnSegmented(selection: Binding(
                            get: { model.snapshot.preferences.offlineSpace },
                            set: { model.send(.setOfflineSpace($0)) }
                        ), options: [
                            (value: OfflineSpace.mb500, label: Strings.Offline.space500),
                            (value: OfflineSpace.gb2, label: Strings.Offline.space2),
                            (value: OfflineSpace.gb10, label: Strings.Offline.space10),
                            (value: OfflineSpace.unlimited, label: Strings.Offline.spaceUnlimited),
                        ])
                    }
                    KilnRowDivider()
                    KilnRow(title: Strings.Offline.usage(bytes: model.snapshot.offline.bytes, files: model.snapshot.offline.files)) {
                        Button(Strings.Offline.clear) { confirmingClear = true }
                            .buttonStyle(KilnButtonStyle(.plain))
                            .disabled(model.snapshot.offline.files == 0 || (!model.isSample && !model.can(HostActionType.offlineClear)))
                    }
                }
            }
            if !model.isSample && !model.snapshot.offline.available {
                Text(Strings.Offline.unavailable)
                    .font(.system(size: 12.5))
                    .foregroundStyle(kiln.muted)
                    .padding(.top, 8)
            }
            if SettingsControls.showsStorageTotals(isSample: model.isSample) {
                KilnGroupTitle(Strings.Advanced.storage)
                KilnCard {
                    KilnRow(title: Strings.Advanced.versions) {
                        folderTrailing(Strings.Advanced.versionsSize, note: Strings.Advanced.wouldShowVersions)
                    }
                    KilnRowDivider()
                    KilnRow(title: Strings.Advanced.uploads) {
                        folderTrailing(Strings.Advanced.uploadsSize, note: Strings.Advanced.wouldShowUploads)
                    }
                }
            }
            KilnGroupTitle(Strings.Advanced.troubleshooting)
            KilnCard {
                KilnRow(title: Strings.Advanced.appLog, subtitle: Strings.Advanced.appLogSub) {
                    Button(Strings.Advanced.showAppLog) {
                        model.send(.revealLog)
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                    .disabled(!model.isSample && !model.can(HostActionType.appRevealLog))
                }
                if SettingsControls.showsSyncLog(model.snapshot.sync) {
                    KilnRowDivider()
                    KilnRow(title: Strings.Advanced.syncLog, subtitle: Strings.Advanced.syncLogSub) {
                        Button(Strings.Advanced.showSyncLog) {
                            model.send(.revealSyncLog)
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .alert(Strings.Offline.clearConfirmTitle, isPresented: $confirmingClear) {
            Button(Strings.Offline.cancel, role: .cancel) {}
            Button(Strings.Offline.clearConfirmButton, role: .destructive) { model.send(.clearOfflineCopies) }
        } message: {
            Text(Strings.Offline.clearConfirmBody)
        }
        .alert(Strings.Offline.offConfirmTitle, isPresented: $confirmingOff) {
            Button(Strings.Offline.cancel, role: .cancel) {}
            Button(Strings.Offline.offConfirmButton, role: .destructive) { model.send(.setOfflineCopies(false)) }
        } message: {
            Text(Strings.Offline.offConfirmBody)
        }
    }

    private func folderTrailing(_ size: String, note: String) -> some View {
        HStack(spacing: 10) {
            Text(size)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
            if SettingsControls.showsStorageTotals(isSample: model.isSample) {
                Button(Strings.Advanced.showInFinder) {
                    model.say(note)
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
    }
}
