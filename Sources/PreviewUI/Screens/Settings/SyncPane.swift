import SwiftUI
import PreviewCore

struct SyncPane: View {
    let actions: SettingsActions
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln

    static func orderedFolders(_ snapshot: HostSnapshot) -> [Folder] {
        snapshot.accountFolders.filter(\.onThisMac) + snapshot.accountFolders.filter { !$0.onThisMac }
    }

    static func conflicts(_ folder: Folder, snapshot: HostSnapshot) -> Int {
        snapshot.waiting.filter { $0.kind == .conflict && $0.subjectID == folder.id }.count
    }

    static func isSyncing(_ folder: Folder, snapshot: HostSnapshot) -> Bool {
        if case .account = folder.origin { return folder.synced && !snapshot.sync.paused && snapshot.sync.syncingFiles > 0 }
        return false
    }

    static func dot(for folder: Folder, snapshot: HostSnapshot) -> StatusDotKind {
        if !folder.onThisMac || !folder.synced || snapshot.sync.paused { return .muted }
        if conflicts(folder, snapshot: snapshot) > 0 { return .amber }
        if isSyncing(folder, snapshot: snapshot) { return .blue }
        return .green
    }

    static func folderSubtitle(_ folder: Folder, snapshot: HostSnapshot) -> String {
        if !folder.onThisMac { return folder.status }
        return Strings.Sync.folderLine(path: folder.path, state: isSyncing(folder, snapshot: snapshot) ? Strings.Sync.syncingWord : folder.status)
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
        VStack(spacing: 10) {
            Squircle(.cobalt, symbol: Symbols.sync, size: .extraLarge)
            Text(Strings.Sync.title)
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.top, 4)
            Text(Strings.Sync.invite)
                .font(.system(size: 13.5))
                .foregroundStyle(kiln.ink2)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 400)
                .fixedSize(horizontal: false, vertical: true)
            Button(Strings.Sync.connect) {
                model.sheet = .connect
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .padding(.top, 8)
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 56)
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
                        model.say(Strings.Sync.wouldReplaceKey)
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                    Button(Strings.Sync.disconnect) {
                        model.send(.disconnect)
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
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
                            model.say(Strings.Folders.openHyperclayNote)
                        }
                    }
                }
            }
        }
    }

    @ViewBuilder private func folderTrailing(_ folder: Folder) -> some View {
        let count = SyncPane.conflicts(folder, snapshot: model.snapshot)
        if !folder.onThisMac {
            Button(Strings.Sync.setUp) {
                model.sheet = .teamSetup(folderID: folder.id)
            }
            .buttonStyle(KilnButtonStyle(.plain))
        } else if count > 0 {
            Button(Strings.Sync.review(count)) {
                actions.openDashboard()
            }
            .buttonStyle(KilnButtonStyle(.plain))
        }
    }
}
