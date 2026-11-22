import SwiftUI
import PreviewCore

struct FoldersPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openURL) private var openURL
    @Environment(\.kiln) private var kiln
    private let actions: SettingsActions

    init(actions: SettingsActions = SettingsActions()) {
        self.actions = actions
    }

    static func orderedLocalFolders(_ snapshot: HostSnapshot) -> [String] {
        func walk(_ folder: Folder) -> [String] {
            [folder.id] + snapshot.children(of: folder.id).flatMap(walk)
        }
        return snapshot.localFolders.filter { $0.parentID == nil }.flatMap(walk)
    }

    static func orderedAccountFolders(_ snapshot: HostSnapshot) -> [String] {
        let onMac = snapshot.accountFolders.filter(\.onThisMac)
        let offMac = snapshot.accountFolders.filter { !$0.onThisMac }
        return (personal(onMac) + teams(onMac) + personal(offMac) + teams(offMac)).map(\.id)
    }

    static func waitingCount(_ snapshot: HostSnapshot, folderID: String) -> Int {
        snapshot.waiting.filter { $0.subjectID == folderID }.count
    }

    private static func personal(_ folders: [Folder]) -> [Folder] {
        folders.filter { folder in
            if case .account = folder.origin { return true }
            return false
        }
    }

    private static func teams(_ folders: [Folder]) -> [Folder] {
        folders.filter { folder in
            if case .team = folder.origin { return true }
            return false
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            Text(Strings.Folders.intro)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.bottom, 12)
            if model.snapshot.folders.isEmpty && !model.snapshot.localMoveOffer {
                emptyCard
            }
            if model.snapshot.localMoveOffer {
                moveCard
            }
            localGroup
            accountGroup
            readAccessGroup
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var header: some View {
        HStack(spacing: 10) {
            Text(Strings.Folders.title)
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
            Spacer(minLength: 0)
            Menu {
                Button(Strings.Folders.serveMenu) {
                    model.sheet = .serveFolder(folderID: nil)
                }
                Button(Strings.Folders.trustMenu) {
                    chooseToTrust()
                }
                if !model.snapshot.sync.connected {
                    Divider()
                    Button(Strings.Folders.connectMenu) {
                        model.sheet = .connect
                    }
                }
            } label: {
                Text(Strings.Folders.addFolder)
                    .font(.system(size: 13.5, weight: .medium))
                    .foregroundStyle(kiln.btnFg)
                    .padding(.vertical, 5)
                    .padding(.horizontal, 12)
                    .background(LinearGradient(colors: kiln.btnBg, startPoint: .top, endPoint: .bottom), in: RoundedRectangle(cornerRadius: 7))
                    .overlay(RoundedRectangle(cornerRadius: 7).strokeBorder(kiln.btnBorder, lineWidth: 1))
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .fixedSize()
        }
        .padding(.bottom, 8)
    }

    private var emptyCard: some View {
        KilnCard {
            KilnRow(title: Strings.Folders.emptyTitle, subtitle: Strings.Folders.emptySub) {
                Button(Strings.Folders.serveMenu) {
                    model.sheet = .serveFolder(folderID: nil)
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
        .padding(.bottom, 4)
    }

    private func chooseToTrust() {
        Task {
            if let path = await model.folderChooser?.chooseToTrust(after: nil, in: model.snapshot) {
                let sample = SampleData.trustCandidates.first { $0.path == path }
                model.sheet = .trustFolder(name: sample?.name ?? (path as NSString).lastPathComponent, path: path)
            }
        }
    }

    private var moveCard: some View {
        KilnCard {
            KilnRow(title: Strings.Folders.moveTitle, subtitle: Strings.Folders.moveSub) {
                Squircle(.teal, symbol: Symbols.folder)
            } trailing: {
                Button(Strings.Folders.move) {
                    model.sheet = .moveFromLocal
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
    }

    @ViewBuilder private var localGroup: some View {
        if !model.snapshot.localFolders.isEmpty {
            KilnGroupTitle(Strings.Folders.onThisMac)
            KilnCard {
                let ids = Self.orderedLocalFolders(model.snapshot)
                ForEach(Array(ids.enumerated()), id: \.element) { index, id in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    if let folder = model.snapshot.folder(id) {
                        FolderRow(folder: folder,
                                  parent: parent(of: folder),
                                  waiting: Self.waitingCount(model.snapshot, folderID: id),
                                  actions: actions)
                    }
                }
            }
        }
    }

    @ViewBuilder private var readAccessGroup: some View {
        if !model.snapshot.readAccess.isEmpty {
            KilnGroupTitle(Strings.Folders.readAccessTitle)
            KilnCard {
                KilnNoteRow(Strings.Folders.readAccessHelp)
                ForEach(model.snapshot.readAccess) { access in
                    KilnRowDivider()
                    KilnRow(title: access.place, subtitle: Strings.Folders.readAccessPages(access.pages), leading: {
                        Image(systemName: Symbols.folder)
                            .font(.system(size: 14))
                            .foregroundStyle(kiln.muted)
                    }, trailing: {
                        if model.can(HostActionType.readAccessRemove) {
                            Button(Strings.Folders.readAccessRemove) {
                                model.send(.removeReadAccess(id: access.id))
                            }
                            .buttonStyle(KilnButtonStyle(.plain))
                            .accessibilityLabel(Text(Strings.Folders.readAccessRemoveLabel(access.place)))
                        }
                    })
                }
            }
        }
    }

    @ViewBuilder private var accountGroup: some View {
        if model.snapshot.sync.connected {
            KilnGroupTitle(Strings.Folders.accountGroup(username: model.snapshot.sync.username))
            KilnCard {
                let ids = Self.orderedAccountFolders(model.snapshot)
                ForEach(Array(ids.enumerated()), id: \.element) { index, id in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    if let folder = model.snapshot.folder(id) {
                        FolderRow(folder: folder,
                                  parent: parent(of: folder),
                                  waiting: Self.waitingCount(model.snapshot, folderID: id),
                                  actions: actions)
                    }
                }
                if model.snapshot.sync.viewOnlyTeams > 0 {
                    if !ids.isEmpty {
                        KilnRowDivider()
                    }
                    viewOnlyRow(model.snapshot.sync.viewOnlyTeams)
                }
            }
        }
    }

    private func viewOnlyRow(_ count: Int) -> some View {
        HStack(spacing: 11) {
            Text(Strings.Folders.viewOnlyTeams(count))
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .frame(maxWidth: .infinity, alignment: .leading)
            KilnLink(Strings.Folders.openHyperclay) {
                if model.isSample {
                    model.say(Strings.Folders.openHyperclayNote)
                } else if let url = URL(string: Strings.Connect.keyPageURL) {
                    openURL(url)
                }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }

    private func parent(of folder: Folder) -> Folder? {
        guard let parentID = folder.parentID else { return nil }
        return model.snapshot.folder(parentID)
    }
}
