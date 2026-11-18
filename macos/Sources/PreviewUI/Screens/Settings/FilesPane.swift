import SwiftUI
import PreviewCore

struct FilesPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var name: YourNameModel
    @State private var owner: FileTypeOwner
    @State private var confirmingStop = false

    init(model: AppModel) {
        _name = State(initialValue: YourNameModel(model: model))
        _owner = State(initialValue: model.fileTypes?.owner() ?? .htmlclay)
    }

    static func mode(_ snapshot: HostSnapshot) -> YourNameModel.Mode { YourNameModel.mode(snapshot) }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 11) {
                Squircle(.teal, symbol: Symbols.doc, size: .large)
                Text(Strings.Files.headline)
                    .font(.system(size: 18, weight: .bold))
                    .foregroundStyle(kiln.ink)
            }
            .padding(.bottom, 8)
            Text(Strings.Files.lead)
                .font(.system(size: 13.5))
                .foregroundStyle(kiln.ink2)
                .lineSpacing(3)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.bottom, 14)
            if isHTMLClays {
                KilnCard {
                    openingRow
                    KilnRowDivider()
                    openWithRow
                }
            } else {
                KilnCard(kind: .warn) {
                    openingRow
                }
                KilnCard {
                    openWithRow
                }
                .padding(.top, 10)
            }
            KilnGroupTitle(Strings.YourName.heading)
            nameCard
            Text(name.mode == .off ? Strings.YourName.offSub : Strings.YourName.note)
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 10)
            KilnCard {
                KilnRow(title: Strings.Files.stopTitle,
                        subtitle: isHTMLClays ? Strings.Files.stopSubOn : Strings.Files.stopSubOff) {
                    Button(Strings.Files.turnOff) {
                        confirmingStop = true
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                    .disabled(!isHTMLClays)
                }
            }
            .padding(.top, 16)
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

    private var openingRow: some View {
        KilnRow(title: isHTMLClays ? Strings.Files.opensTitle : Strings.Files.otherTitle,
                subtitle: isHTMLClays ? Strings.Files.opensSub : otherSub,
                leading: { isHTMLClays ? StatusDot(.green) : StatusDot(.amber) }) {
            if isHTMLClays {
                KilnPill(.trusted, text: Strings.Files.defaultPill)
            } else {
                Button(Strings.Files.makeDefault) {
                    Task { await makeDefault() }
                }
                .buttonStyle(KilnButtonStyle(.primary))
                .disabled(model.fileTypes == nil)
            }
        }
    }

    private var otherSub: String {
        if case let .other(name) = owner, let name, !name.isEmpty {
            return Strings.Files.otherSub(name)
        }
        return Strings.Files.otherUnknownSub
    }

    private var openWithRow: some View {
        KilnRow(title: Strings.Files.openWithTitle, subtitle: Strings.Files.openWithSub) {
            VStack(alignment: .trailing, spacing: 4) {
                Toggle(isOn: Binding(get: { true }, set: { _ in })) {
                    EmptyView()
                }
                .toggleStyle(KilnSwitchStyle())
                .disabled(true)
                Text(Strings.Files.openWithMac)
                    .font(.system(size: 11.5))
                    .foregroundStyle(kiln.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(width: 190, alignment: .trailing)
            }
        }
    }

    private var nameCard: some View {
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

    private func readOwner() {
        guard let fileTypes = model.fileTypes else {
            owner = .htmlclay
            return
        }
        owner = fileTypes.owner()
    }

    private func makeDefault() async {
        guard let fileTypes = model.fileTypes else { return }
        do {
            try await fileTypes.makeDefault()
        } catch {
            model.say(error.localizedDescription)
        }
        readOwner()
    }

    private func stopOpening() async {
        guard let fileTypes = model.fileTypes else { return }
        do {
            try await fileTypes.stopOpening()
        } catch {
            model.say(error.localizedDescription)
        }
        readOwner()
    }
}
