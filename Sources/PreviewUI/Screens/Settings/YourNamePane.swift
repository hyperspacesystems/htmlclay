import SwiftUI
import PreviewCore

struct YourNamePane: View {
    enum Mode: Equatable { case off, local, account }

    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var draft = String()

    static func mode(_ snapshot: HostSnapshot) -> Mode {
        guard snapshot.preferences.useName else { return .off }
        return snapshot.sync.connected && !snapshot.sync.fullName.isEmpty ? .account : .local
    }

    private var mode: Mode { YourNamePane.mode(model.snapshot) }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(.yourName))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            KilnCard {
                KilnRow(title: Strings.YourName.useName) {
                    Toggle(isOn: Binding(
                        get: { model.snapshot.preferences.useName },
                        set: { model.send(.setUseName($0)) }
                    )) {
                        EmptyView()
                    }
                    .toggleStyle(KilnSwitchStyle())
                }
                switch mode {
                case .local:
                    KilnRowDivider()
                    KilnRow(title: Strings.YourName.name) {
                        KilnInput(text: $draft, placeholder: Strings.Settings.title(.yourName))
                            .frame(width: 220)
                    }
                case .account:
                    KilnRowDivider()
                    KilnRow(title: model.snapshot.sync.fullName, subtitle: Strings.YourName.accountSub) {
                        KilnLink(Strings.YourName.change) {
                            model.say(Strings.YourName.wouldChange)
                        }
                    }
                case .off:
                    EmptyView()
                }
            }
            Text(mode == .off ? Strings.YourName.offSub : Strings.YourName.note)
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.muted)
                .padding(.top, 10)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onAppear { draft = model.snapshot.preferences.yourName }
        .onChange(of: draft) { _, new in
            if new != model.snapshot.preferences.yourName {
                model.send(.setYourName(new))
            }
        }
    }
}
