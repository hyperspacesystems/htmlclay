import AppKit
import SwiftUI
import PreviewCore

/// The Connect sheet. With the live host it sends sync.connect and shows the
/// host's refusal under the key field; the key field is secure and is cleared
/// on a refusal and when the sheet closes. Sample data keeps the preview's walk.
struct ConnectSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @Environment(\.openURL) private var openURL
    @State private var username: String
    @State private var key = String()
    @State private var refused: Bool
    @State private var form: ConnectSheetModel?

    init(refused: Bool = false) {
        _username = State(initialValue: SampleData.snapshot(for: .syncedWithTeams).sync.username)
        _refused = State(initialValue: refused)
        _form = State(initialValue: nil)
    }

    init(model: AppModel, refused: Bool = false) {
        _username = State(initialValue: SampleData.snapshot(for: .syncedWithTeams).sync.username)
        _refused = State(initialValue: refused)
        _form = State(initialValue: model.isSample ? nil : ConnectSheetModel(model: model))
    }

    var body: some View {
        Group {
            if model.isSample {
                sample
            } else if let form {
                live(form)
            }
        }
        .onAppear {
            if !model.isSample && form == nil { form = ConnectSheetModel(model: model) }
        }
        .onDisappear { form?.close() }
    }

    private func live(_ form: ConnectSheetModel) -> some View {
        @Bindable var form = form
        let wasConnected = model.snapshot.sync.connected
        return KilnDialog(Strings.Connect.title) {
            intro
            KilnField(Strings.Connect.username) {
                KilnInput(text: $form.username, placeholder: Strings.Connect.username)
            }
            .padding(.top, 14)
            KilnField(Strings.Connect.syncKey) {
                KilnInput(text: $form.key, placeholder: Strings.Connect.keyPlaceholder, mono: true, secure: true)
                Button(Strings.Connect.paste) {
                    form.key = NSPasteboard.general.string(forType: .string)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? String()
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
            .padding(.top, 10)
            if let refusal = form.refusal {
                KilnFieldNote(Text(refusal), error: true)
            }
            KilnLink(Strings.Connect.getKey) {
                if let url = URL(string: Strings.Connect.keyPageURL) { openURL(url) }
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
                form.close()
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(form.connecting ? Strings.Connect.connecting : Strings.Connect.connect) {
                Task {
                    guard await form.connect() else { return }
                    model.sheet = ConnectSheetModel.nextSheet(after: model.snapshot, wasConnected: wasConnected)
                }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!form.ready)
            .opacity(form.ready ? 1 : 0.42)
        }
    }

    private var ready: Bool {
        !username.isEmpty && !key.isEmpty
    }

    private var intro: some View {
        KilnLead(introText)
            .tint(kiln.link)
            .environment(\.openURL, OpenURLAction { url in
                if model.isSample {
                    model.say(Strings.Connect.wouldOpenSignUp)
                    return .handled
                }
                openURL(url)
                return .handled
            })
            .padding(.top, 6)
    }

    private var introText: Text {
        var link = AttributedString(Strings.Connect.signUpLink)
        link.foregroundColor = kiln.link
        link.font = .system(size: 13.5, weight: .medium)
        if let url = URL(string: Strings.Connect.signUpURL) {
            link.link = url
        }
        return Text(Strings.Connect.introLead) + Text(link)
    }

    private var sample: some View {
        KilnDialog(Strings.Connect.title) {
            intro
            KilnField(Strings.Connect.username) {
                KilnInput(text: $username, placeholder: Strings.Connect.username)
            }
            .padding(.top, 14)
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
    @State private var form: SetUpSheetModel?

    init(username: String, model: AppModel) {
        self.username = username
        if model.isSample {
            _form = State(initialValue: nil)
        } else if let id = model.snapshot.folders.first(where: { $0.origin == .account(username: username) && !$0.onThisMac })?.id {
            _form = State(initialValue: SetUpSheetModel(folderID: id, path: AccountFolderSheet.path(username), model: model))
        } else {
            _form = State(initialValue: nil)
        }
    }

    static func path(_ username: String) -> String {
        TeamSetupRules.samplePaths(for: Folder(id: username, name: username, path: String())).first ?? String()
    }

    /// The person's own account row, not on this Mac yet.
    private var folderID: String? {
        model.snapshot.folders.first { $0.origin == .account(username: username) && !$0.onThisMac }?.id
    }

    var body: some View {
        Group {
            if model.isSample {
                sample
            } else if let form {
                live(form)
            }
        }
        .onAppear {
            if !model.isSample && form == nil {
                if let folderID {
                    form = SetUpSheetModel(folderID: folderID, path: AccountFolderSheet.path(username), model: model)
                } else {
                    model.sheet = nil
                }
            }
        }
    }

    private func live(_ form: SetUpSheetModel) -> some View {
        KilnDialog(Strings.AccountFolder.title(username)) {
            KilnCard {
                Button {
                    form.chooseNew(AccountFolderSheet.path(username))
                } label: {
                    HStack(spacing: 11) {
                        KilnRadio(on: !form.existing)
                        Text(Strings.AccountFolder.newFolder) + Text(AccountFolderSheet.path(username)).monospaced()
                        Spacer(minLength: 0)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .font(.system(size: 14, weight: .medium))
                .foregroundStyle(kiln.ink)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                KilnRowDivider()
                Button {
                    Task { await form.chooseExisting() }
                } label: {
                    HStack(spacing: 11) {
                        KilnRadio(on: form.existing)
                        (form.existing ? Text(form.path).monospaced() : Text(Strings.AccountFolder.existing))
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
            if let counts = form.counts {
                Text(Strings.AccountFolder.willSync(counts))
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.ink2)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 12)
            }
            if let refusal = form.refusal {
                KilnFieldNote(Text(refusal), error: true)
            }
            KilnLead(Text(Strings.AccountFolder.willStart) + Text(ServeRules.address(for: username)).monospaced() + Text(Strings.AccountFolder.willEnd))
                .padding(.top, 6)
            if form.following {
                FirstDownloadFollow(folderID: form.folderID)
            }
        } buttons: {
            Button(Strings.AccountFolder.back) {
                model.sheet = .connect
            }
            .buttonStyle(KilnButtonStyle(.plain))
            .disabled(form.following)
            if !form.following {
                Button(form.working ? Strings.AccountFolder.settingUp : (form.counts == nil ? Strings.AccountFolder.setUpAndSync : Strings.AccountFolder.syncThese)) {
                    Task { _ = await form.setUp() }
                }
                .buttonStyle(KilnButtonStyle(.primary))
                .disabled(form.working)
            }
        }
    }

    private var sample: some View {
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

    @State private var form: SetUpSheetModel?

    init(folderID: String, model: AppModel) {
        self.folderID = folderID
        if !model.isSample, let folder = model.snapshot.folder(folderID) {
            _form = State(initialValue: SetUpSheetModel(folderID: folderID, path: TeamSetupRules.samplePaths(for: folder).first ?? String(), model: model))
        } else {
            _form = State(initialValue: nil)
        }
    }

    var body: some View {
        if let folder = model.snapshot.folder(folderID) {
            if model.isSample {
                sheet(folder)
            } else {
                live(folder)
            }
        } else {
            EmptyView()
        }
    }

    /// The live team sheet: a team's first sync starts in an empty folder, which
    /// the host checks; while it sets the folder up the button says so.
    @ViewBuilder private func live(_ folder: Folder) -> some View {
        if let form {
            KilnDialog(Strings.TeamSetup.title(folder.name)) {
                HStack(spacing: 10) {
                    KilnInputBox(form.path, mono: true)
                    Button(Strings.Sheets.choose) {
                        Task {
                            if let picked = await model.folderChooser?.choose(after: form.path, in: model.snapshot), !picked.isEmpty {
                                form.chooseNew(picked)
                            }
                        }
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                    .disabled(form.working)
                }
                .padding(.top, 12)
                if let refusal = form.refusal {
                    KilnFieldNote(Text(refusal), error: true)
                } else {
                    KilnFieldNote(Text(Strings.TeamSetup.emptyFolder))
                }
                (Text(Strings.TeamSetup.opensAt) + Text(TeamSetupRules.address(for: folder)).monospaced())
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.muted)
                    .padding(.top, 6)
                if form.following {
                    FirstDownloadFollow(folderID: folderID)
                }
            } buttons: {
                Button(form.following ? Strings.TeamSetup.close : Strings.Sheets.cancel) {
                    model.sheet = nil
                }
                .buttonStyle(KilnButtonStyle(.plain))
                if !form.following {
                    Button(form.working ? Strings.AccountFolder.settingUp : Strings.TeamSetup.setUp) {
                        Task { _ = await form.setUp() }
                    }
                    .buttonStyle(KilnButtonStyle(.primary))
                    .disabled(form.working)
                }
            }
        } else {
            Color.clear.onAppear {
                form = SetUpSheetModel(folderID: folderID, path: TeamSetupRules.samplePaths(for: folder).first ?? String(), model: model)
            }
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
