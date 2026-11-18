import SwiftUI
import PreviewCore

struct UpdatesPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln

    static func short(_ version: String) -> String {
        UpdateState.short(version)
    }

    /// When Sparkle last answered, in words: just now, 5 minutes ago, or the date.
    static func when(_ date: Date, now: Date = Date()) -> String {
        if now.timeIntervalSince(date) < 60 { return Strings.Updates.justNow }
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .full
        return formatter.localizedString(for: date, relativeTo: now)
    }

    private var updater: UpdateCoordinator? { model.updater }
    private var checking: Bool { updater?.checking ?? false }
    private var ready: String? { updater?.prepared }
    private var version: String { UpdatesPane.short(model.snapshot.update.current) }

    var body: some View {
        let _ = model.updaterRev
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(.updates))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            KilnCard {
                statusRow
                KilnRowDivider()
                KilnRow(title: Strings.Updates.automatic) {
                    Toggle(isOn: Binding(
                        get: { model.snapshot.update.automatic },
                        set: { model.send(.setAutoUpdate($0)) }
                    )) {
                        EmptyView()
                    }
                    .toggleStyle(KilnSwitchStyle())
                }
            }
            HStack(spacing: 4) {
                if ready != nil {
                    Text(Strings.Updates.restartNote)
                        .font(.system(size: 12.5))
                        .foregroundStyle(kiln.muted)
                    Text(Strings.Updates.versionDot)
                        .font(.system(size: 12.5))
                        .foregroundStyle(kiln.muted)
                }
                KilnLink(Strings.Updates.whatsNew) {
                    model.say(Strings.Updates.wouldShowWhatsNew)
                }
            }
            .padding(.top, 10)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private var statusRow: some View {
        if let ready {
            KilnRow(title: Strings.Updates.ready(UpdatesPane.short(ready)),
                    subtitle: Strings.Updates.readySub(version),
                    leading: { Squircle(.jade, symbol: Symbols.update) }) {
                Button(Strings.Updates.restart) {
                    updater?.restartToUpdate()
                }
                .buttonStyle(KilnButtonStyle(.primary))
                .disabled(updater == nil)
            }
        } else if checking {
            KilnRow(title: Strings.Updates.checking,
                    subtitle: Strings.Updates.checkingYouHave(version),
                    leading: { Squircle(.jade, symbol: Symbols.update) }) {
                ProgressView()
                    .controlSize(.small)
            }
        } else {
            KilnRow(title: Strings.Updates.youAreOnLatest,
                    subtitle: subtitle,
                    leading: { Squircle(.jade, symbol: Symbols.check) }) {
                Button(Strings.Updates.checkNow) {
                    updater?.checkNow()
                }
                .buttonStyle(KilnButtonStyle(.plain))
                .disabled(updater == nil)
            }
        }
    }

    private var subtitle: String {
        guard let checkedAt = updater?.checkedAt else { return Strings.Updates.latest(version) }
        return Strings.Updates.latest(version, checked: UpdatesPane.when(checkedAt))
    }
}
