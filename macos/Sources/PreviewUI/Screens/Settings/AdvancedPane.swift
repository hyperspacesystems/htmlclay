import SwiftUI
import PreviewCore

struct AdvancedPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln

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
                    }
                }
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
