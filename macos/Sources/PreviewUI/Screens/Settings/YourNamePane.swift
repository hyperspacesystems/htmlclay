import SwiftUI
import PreviewCore

struct YourNamePane: View {
    typealias Mode = YourNameModel.Mode

    @Environment(\.kiln) private var kiln
    @State private var name: YourNameModel

    init(model: AppModel) {
        _name = State(initialValue: YourNameModel(model: model))
    }

    static func mode(_ snapshot: HostSnapshot) -> Mode { YourNameModel.mode(snapshot) }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(.yourName))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            KilnCard {
                KilnRow(title: Strings.YourName.useName) {
                    Toggle(isOn: Binding(
                        get: { name.isOn },
                        set: { name.setUseName($0) }
                    )) {
                        EmptyView()
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
                            ), placeholder: Strings.Settings.title(.yourName))
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
            Text(name.mode == .off ? Strings.YourName.offSub : Strings.YourName.note)
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.muted)
                .padding(.top, 10)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
