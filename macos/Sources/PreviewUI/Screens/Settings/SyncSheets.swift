import SwiftUI
import PreviewCore

struct ConnectSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var username: String
    @State private var key = String()
    @State private var refused: Bool

    init(refused: Bool = false) {
        _username = State(initialValue: SampleData.snapshot(for: .syncedWithTeams).sync.username)
        _refused = State(initialValue: refused)
    }

    private var ready: Bool {
        !username.isEmpty && !key.isEmpty
    }

    var body: some View {
        KilnDialog(Strings.Connect.title) {
            KilnField(Strings.Connect.username) {
                KilnInput(text: $username, placeholder: Strings.Connect.username)
            }
            .padding(.top, 12)
            KilnField(Strings.Connect.syncKey) {
                KilnInput(text: $key, placeholder: Strings.Connect.keyPlaceholder, mono: true)
                Button(Strings.Connect.paste) {
                    key = SampleData.syncKey
                    refused = false
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
            .padding(.top, 10)
            if refused {
                KilnFieldNote(Text(Strings.Connect.refused), error: true)
            }
            KilnLink(Strings.Connect.getKey) {
                model.say(Strings.Connect.wouldOpenKeys)
            }
            .padding(.leading, 88)
            .padding(.top, 6)
            Text(Strings.Connect.nothingSent)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 14)
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Connect.connect) {
                guard ConnectRules.accepts(key) else {
                    refused = true
                    return
                }
                if model.snapshot.sync.connected {
                    model.send(.connectSample)
                    model.sheet = nil
                } else {
                    model.sheet = .accountFolder(username: username)
                }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!ready)
            .opacity(ready ? 1 : 0.42)
        }
        .onChange(of: key) {
            refused = false
        }
    }
}

struct AccountFolderSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    let username: String

    static func path(_ username: String) -> String {
        TeamSetupRules.samplePaths(for: Folder(id: username, name: username, path: String())).first ?? String()
    }

    var body: some View {
        KilnDialog(Strings.AccountFolder.title(username)) {
            KilnCard {
                HStack(spacing: 11) {
                    KilnRadio(on: true)
                    Text(Strings.AccountFolder.newFolder) + Text(AccountFolderSheet.path(username)).monospaced()
                }
                .font(.system(size: 14, weight: .medium))
                .foregroundStyle(kiln.ink)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                KilnRowDivider()
                Button {
                    model.say(Strings.AccountFolder.wouldChooseExisting)
                } label: {
                    HStack(spacing: 11) {
                        KilnRadio(on: false)
                        Text(Strings.AccountFolder.existing)
                            .font(.system(size: 14, weight: .medium))
                            .foregroundStyle(kiln.ink)
                        Spacer(minLength: 0)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
            }
            .padding(.top, 12)
            Text(Strings.AccountFolder.willDownload(documents: SampleData.accountDownload.documents, size: SampleData.accountDownload.size))
                .font(.system(size: 13))
                .foregroundStyle(kiln.ink2)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 12)
            KilnLead(Text(Strings.AccountFolder.willStart) + Text(ServeRules.address(for: username)).monospaced() + Text(Strings.AccountFolder.willEnd))
                .padding(.top, 6)
        } buttons: {
            Button(Strings.AccountFolder.back) {
                model.sheet = .connect
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.AccountFolder.setUpAndSync) {
                model.send(.connectSample)
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.primary))
        }
    }
}

struct TeamSetupSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    let folderID: String
    @State private var pick = 0
    @State private var trust = true
    @State private var stage = 0

    var body: some View {
        if let folder = model.snapshot.folder(folderID) {
            sheet(folder)
        } else {
            EmptyView()
        }
    }

    private func sheet(_ folder: Folder) -> some View {
        let path = TeamSetupRules.samplePaths(for: folder)[pick % 3]
        let problem = TeamSetupRules.problem(path, in: model.snapshot)
        let steps = SampleData.teamDownloadSteps
        let total = SampleData.teamDownloadTotal
        return KilnDialog(Strings.TeamSetup.title(folder.name)) {
            HStack(spacing: 10) {
                KilnInputBox(path, mono: true)
                Button(Strings.Sheets.choose) {
                    pick += 1
                }
                .buttonStyle(KilnButtonStyle(.plain))
                .disabled(stage > 0)
            }
            .padding(.top, 12)
            if let problem {
                Text(problem)
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.danger)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 4)
            }
            (Text(Strings.TeamSetup.opensAt) + Text(TeamSetupRules.address(for: folder)).monospaced())
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .padding(.top, 6)
            Button {
                trust.toggle()
            } label: {
                HStack(alignment: .top, spacing: 9) {
                    KilnCheck(on: trust)
                    Text(Strings.TeamSetup.trustCheck)
                        .font(.system(size: 13.5))
                        .foregroundStyle(kiln.ink2)
                        .multilineTextAlignment(.leading)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .disabled(stage > 0)
            .padding(.top, 14)
            if stage > 0 {
                let step = steps[min(stage, steps.count) - 1]
                Text(Strings.TeamSetup.downloading(files: step.files, total: total.files, size: step.size, totalSize: total.size))
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.muted)
                    .monospacedDigit()
                    .padding(.top, 14)
                KilnProgress(fraction: Double(step.files) / Double(total.files))
                    .padding(.top, 6)
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            if stage == 0 {
                Button(Strings.TeamSetup.setUp) {
                    stage = 1
                }
                .buttonStyle(KilnButtonStyle(.primary))
                .disabled(problem != nil)
                .opacity(problem == nil ? 1 : 0.42)
            }
        }
        .task(id: stage) {
            guard stage > 0 else { return }
            try? await Task.sleep(nanoseconds: 700_000_000)
            if Task.isCancelled { return }
            if stage < SampleData.teamDownloadSteps.count {
                stage += 1
            } else {
                model.send(.setUpTeamFolder(folderID: folderID))
                model.sheet = nil
            }
        }
    }
}
