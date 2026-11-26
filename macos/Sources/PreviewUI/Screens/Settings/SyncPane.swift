import SwiftUI
import PreviewCore

struct SyncPane: View {
    let actions: SettingsActions
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @Environment(\.openURL) private var openURL
    @State private var confirmDisconnect = false
    @State private var stopSyncingFolder: Folder?

    static func orderedFolders(_ snapshot: HostSnapshot) -> [Folder] {
        snapshot.accountFolders.filter(\.onThisMac) + snapshot.accountFolders.filter { !$0.onThisMac }
    }

    static func conflicts(_ folder: Folder, snapshot: HostSnapshot) -> Int {
        snapshot.waiting.filter { $0.kind == .conflict && $0.subjectID == folder.id }.count
    }

    static func isSyncing(_ folder: Folder, snapshot: HostSnapshot) -> Bool {
        folder.onThisMac && !snapshot.sync.paused && folder.syncState == FolderSyncState.syncing
    }

    static func dot(for folder: Folder, snapshot: HostSnapshot) -> StatusDotKind {
        if !folder.onThisMac || snapshot.sync.paused { return .muted }
        switch folder.syncState {
        case FolderSyncState.conflicts: return .amber
        case FolderSyncState.syncing: return .blue
        case FolderSyncState.upToDate: return conflicts(folder, snapshot: snapshot) > 0 ? .amber : .green
        default: return .muted
        }
    }

    static func stateWord(_ folder: Folder, snapshot: HostSnapshot) -> String {
        if snapshot.sync.paused { return Strings.Sync.statePaused }
        switch folder.syncState {
        case FolderSyncState.syncing:
            if let download = folder.download, download.total > 0 { return Strings.Sync.downloading(done: download.done, total: download.total) }
            return Strings.Sync.syncingWord
        case FolderSyncState.conflicts: return Strings.Sync.stateConflicts
        case FolderSyncState.paused: return Strings.Sync.statePaused
        case FolderSyncState.upToDate: return Strings.Sync.stateUpToDate
        default: return Strings.Sync.stateOff
        }
    }

    static func folderSubtitle(_ folder: Folder, snapshot: HostSnapshot) -> String {
        if !folder.onThisMac { return folder.status }
        return Strings.Sync.folderLine(path: folder.path, state: stateWord(folder, snapshot: snapshot))
    }

    static func syncingLine(_ sync: SyncState) -> String {
        if sync.paused { return Strings.Panel.syncPaused }
        if sync.syncingFiles > 0 { return Strings.Sync.syncingFiles(sync.syncingFiles, lastFinished: sync.lastFinished) }
        return Strings.Sync.upToDate(lastFinished: sync.lastFinished)
    }

    var body: some View {
        if model.snapshot.sync.connected {
            connected
        } else {
            notConnected
        }
    }

    private var notConnected: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Sync.title)
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            HStack(alignment: .top, spacing: 10) {
                benefitCard(.sky, symbol: Symbols.sync, title: Strings.Sync.benefit1Title, sub: Strings.Sync.benefit1Sub)
                benefitCard(.amber, symbol: Symbols.people, title: Strings.Sync.benefit2Title, sub: Strings.Sync.benefit2Sub)
                benefitCard(.jade, symbol: Symbols.globe, title: Strings.Sync.benefit3Title, sub: Strings.Sync.benefit3Sub)
            }
            KilnCard {
                VStack(alignment: .leading, spacing: 10) {
                    Text(Strings.Sync.stepsTitle)
                        .font(.system(size: 13.5, weight: .semibold))
                        .foregroundStyle(kiln.ink)
                    HStack(alignment: .top, spacing: 16) {
                        step(1, title: Strings.Sync.step1Title, body: Strings.Sync.step1Body)
                        step(2, title: Strings.Sync.step2Title, body: Strings.Sync.step2Body)
                        step(3, title: Strings.Sync.step3Title, body: Strings.Sync.step3Body)
                    }
                }
                .padding(14)
                KilnRowDivider()
                HStack(spacing: 11) {
                    (Text(Strings.Sync.inviteLead).fontWeight(.semibold) + Text(" ") + Text(Strings.Sync.invite))
                        .font(.system(size: 14, weight: .medium))
                        .foregroundStyle(kiln.ink)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    Button(Strings.Sync.connect) {
                        model.sheet = .connect
                    }
                    .buttonStyle(KilnButtonStyle(.primary))
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
            }
            .padding(.top, 12)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func step(_ number: Int, title: String, body: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 7) {
                Text("\(number)")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(kiln.ink)
                    .frame(width: 18, height: 18)
                    .background(Circle().fill(kiln.card2))
                    .overlay(Circle().strokeBorder(kiln.cardBorder))
                Text(title)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(kiln.ink)
            }
            Text(body)
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.ink2)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
    }

    private func benefitCard(_ color: SquircleColor, symbol: String, title: String, sub: String) -> some View {
        KilnCard {
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    Squircle(color, symbol: symbol)
                    Text(title)
                        .font(.system(size: 13.5, weight: .semibold))
                        .foregroundStyle(kiln.ink)
                }
                Text(sub)
                    .font(.system(size: 12.5))
                    .foregroundStyle(kiln.ink2)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
            .padding(14)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
    }

    private var connected: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(.sync))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            if keyRefused {
                refusedCard
            }
            accountCard
            foldersGroup
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .alert(stopSyncingTitle, isPresented: presentingStopSyncing) {
            Button(Strings.FolderPage.cancel, role: .cancel) {}
            Button(Strings.Folders.stopSyncing, role: .destructive) {
                if let folder = stopSyncingFolder {
                    model.send(.setSynced(folderID: folder.id, on: false))
                }
            }
        } message: {
            Text(stopSyncingMessage)
        }
    }

    private var presentingStopSyncing: Binding<Bool> {
        Binding(
            get: { stopSyncingFolder != nil },
            set: { shown in
                if !shown { stopSyncingFolder = nil }
            }
        )
    }

    private var stopSyncingTitle: String {
        Strings.FolderPage.disconnectTitle(stopSyncingFolder?.name ?? String())
    }

    private var stopSyncingMessage: String {
        guard let folder = stopSyncingFolder else { return String() }
        let isTeam: Bool
        if case .team = folder.origin { isTeam = true } else { isTeam = false }
        return Strings.FolderPage.disconnectMessage(folder.name, team: isTeam)
    }

    private var keyRefused: Bool {
        model.snapshot.waiting.contains { $0.kind == .syncKeyRefused }
    }

    private var refusedCard: some View {
        KilnCard(kind: .err) {
            KilnRow(title: Strings.Sync.keyRefusedTitle, subtitle: Strings.Sync.keyRefusedSub) {
                Button(Strings.Sync.reconnect) {
                    model.sheet = .connect
                }
                .buttonStyle(KilnButtonStyle(.primary))
            }
        }
        .padding(.bottom, 12)
    }

    private var accountCard: some View {
        KilnCard {
            KilnRow(title: Strings.Sync.account(model.snapshot.sync.username),
                    subtitle: Strings.Sync.connected(keySuffix: model.snapshot.sync.keySuffix),
                    leading: { avatar }) {
                HStack(spacing: 8) {
                    Button(Strings.Sync.replaceKey) {
                        if model.isSample { model.say(Strings.Sync.wouldReplaceKey) } else { model.sheet = .connect }
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                    Button(Strings.Sync.disconnect) {
                        confirmDisconnect = true
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                    .alert(Strings.Sync.disconnectTitle, isPresented: $confirmDisconnect) {
                        Button(Strings.Sheets.cancel, role: .cancel) {}
                        Button(Strings.Sync.disconnectConfirm, role: .destructive) {
                            model.send(.disconnect)
                        }
                    } message: {
                        Text(Strings.Sync.disconnectMessage)
                    }
                }
            }
            KilnRowDivider()
            KilnRow(title: Strings.Sync.syncing,
                    subtitle: SyncPane.syncingLine(model.snapshot.sync)) {
                Button(model.snapshot.sync.paused ? Strings.Panel.resumeSyncing : Strings.Panel.pauseSyncing) {
                    model.send(.setSyncPaused(!model.snapshot.sync.paused))
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
            if model.snapshot.sync.keyInFile {
                KilnRowDivider()
                KilnNoteRow(Strings.Sync.keyInFile) { EmptyView() }
            }
        }
    }

    private var avatar: some View {
        Text(String(model.snapshot.sync.username.prefix(1)).uppercased())
            .font(.system(size: 13, weight: .semibold))
            .foregroundStyle(KilnFixed.squircleInk)
            .frame(width: 28, height: 28)
            .background(LinearGradient(colors: KilnFixed.squircleCobalt, startPoint: .top, endPoint: .bottom), in: Circle())
    }

    @ViewBuilder private var foldersGroup: some View {
        let folders = SyncPane.orderedFolders(model.snapshot)
        if !folders.isEmpty {
            KilnGroupTitle(Strings.Sync.folders)
            KilnCard {
                ForEach(Array(folders.enumerated()), id: \.element.id) { index, folder in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    KilnRow(title: folder.name,
                            subtitle: SyncPane.folderSubtitle(folder, snapshot: model.snapshot),
                            leading: { StatusDot(SyncPane.dot(for: folder, snapshot: model.snapshot)) },
                            trailing: { folderTrailing(folder) })
                }
                if model.snapshot.sync.viewOnlyTeams > 0 {
                    KilnRowDivider()
                    KilnNoteRow(Strings.Folders.viewOnlyTeams(model.snapshot.sync.viewOnlyTeams)) {
                        KilnLink(Strings.Folders.openHyperclay) {
                            if model.isSample {
                                model.say(Strings.Folders.openHyperclayNote)
                            } else if let url = URL(string: Strings.Connect.keyPageURL) {
                                openURL(url)
                            }
                        }
                    }
                }
            }
        }
    }

    @ViewBuilder private func folderTrailing(_ folder: Folder) -> some View {
        let count = SyncPane.conflicts(folder, snapshot: model.snapshot)
        if folder.cannotSetUp {
            Text(Strings.Sync.viewOnly)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
        } else if !folder.onThisMac {
            Button(Strings.Sync.setUp) {
                model.sheet = .teamSetup(folderID: folder.id)
            }
            .buttonStyle(KilnButtonStyle(.plain))
        } else {
            HStack(spacing: 6) {
                if count > 0 {
                    Button(Strings.Sync.review(count)) {
                        actions.openDashboard()
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
                Button(Strings.Folders.open) {
                    if model.isSample {
                        actions.openDashboard()
                    } else if folder.served {
                        model.send(.openDashboard(route: Strings.Folders.rootRoute(folder.id)))
                    } else {
                        model.send(.revealFolder(id: folder.id))
                    }
                }
                .buttonStyle(KilnButtonStyle(.plain))
                if model.isSample || model.can(HostActionType.reveal) {
                    Button(Strings.Folders.finder) {
                        if model.isSample {
                            model.say(Strings.Preview.wouldShowInFinder(folder.path))
                        } else {
                            model.send(.revealFolder(id: folder.id))
                        }
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
                if folder.synced && (model.isSample || model.can(HostActionType.setSynced)) {
                    Button(Strings.Folders.stopSyncing) {
                        stopSyncingFolder = folder
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
            }
        }
    }
}
