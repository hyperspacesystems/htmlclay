import SwiftUI
import PreviewCore

struct UpdatesPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln

    static func short(_ version: String) -> String {
        let dot = Character(Strings.Updates.versionDot)
        return version.split(separator: dot).prefix(2).joined(separator: Strings.Updates.versionDot)
    }

    private var update: UpdateState { model.snapshot.update }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(.updates))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            KilnCard {
                versionRow
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
                if update.ready != nil {
                    Text(Strings.Updates.restartNote)
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

    @ViewBuilder private var versionRow: some View {
        if let ready = update.ready {
            KilnRow(title: Strings.Updates.ready(UpdatesPane.short(ready)),
                    subtitle: Strings.Updates.youHave(UpdatesPane.short(update.current),
                                                     checked: update.checked.lowercased()),
                    leading: { Squircle(.jade, symbol: Symbols.update) }) {
                Button(Strings.Updates.restart) {
                    model.say(Strings.Updates.wouldRestart(ready))
                }
                .buttonStyle(KilnButtonStyle(.primary))
            }
        } else {
            KilnRow(title: Strings.Updates.current(UpdatesPane.short(update.current)),
                    subtitle: Strings.Updates.upToDate(checked: update.checked.lowercased()),
                    leading: { Squircle(.jade, symbol: Symbols.update) }) {
                Button(Strings.Updates.checkNow) {
                    model.say(Strings.Updates.wouldCheck)
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
    }
}
