import AppKit
import SwiftUI
import PreviewCore

struct FolderRow: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var expanded = false
    @State private var showAdvanced = false
    @State private var alert: FolderAlert?
    private let folder: Folder
    private let parent: Folder?
    private let waiting: Int
    private let actions: SettingsActions

    init(folder: Folder, parent: Folder? = nil, waiting: Int = 0, actions: SettingsActions = SettingsActions()) {
        self.folder = folder
        self.parent = parent
        self.waiting = waiting
        self.actions = actions
    }

    static func pills(for folder: Folder) -> [KilnPillLevel] {
        var levels: [KilnPillLevel] = [.trusted]
        if folder.served { levels.append(.served) }
        if folder.synced { levels.append(.synced) }
        return levels
    }

    var body: some View {
        if folder.onThisMac {
            VStack(alignment: .leading, spacing: 0) {
                KilnRow(title: folder.name, subtitle: subtitle, pills: Self.pills(for: folder)) {
                    leading
                } trailing: {
                    HStack(spacing: 6) {
                        badge
                        Button(Strings.Folders.open) {
                            openFolder()
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                        if model.isSample || model.can(HostActionType.reveal) {
                            Button(Strings.Folders.finder) {
                                revealFolder()
                            }
                            .buttonStyle(KilnButtonStyle(.plain))
                        }
                        Button {
                            expanded.toggle()
                        } label: {
                            HStack(spacing: 4) {
                                Text(Strings.Folders.more)
                                Image(systemName: expanded ? Symbols.chevronDown : Symbols.chevronRight)
                                    .font(.system(size: 10, weight: .semibold))
                            }
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                }
                if expanded {
                    inlineDetails
                }
            }
            .folderConfirmations(folder: folder, alert: $alert)
        } else {
            KilnRow(title: folder.name, subtitle: folder.status) {
                Squircle(.slate, symbol: Symbols.folder)
            } trailing: {
                if folder.cannotSetUp {
                    Text(Strings.Sync.viewOnly)
                        .font(.system(size: 13))
                        .foregroundStyle(kiln.muted)
                } else {
                    Button(Strings.Folders.setUp) {
                        model.sheet = .teamSetup(folderID: folder.id)
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
            }
        }
    }

    private var inlineDetails: some View {
        VStack(alignment: .leading, spacing: 0) {
            KilnRowDivider()
            if folder.missingOnDisk {
                missingRow
                KilnRowDivider()
            } else if folder.becameViewer {
                viewerRow
                KilnRowDivider()
            }
            trustedRow
            KilnRowDivider()
            servedRow
            if folder.served {
                KilnRowDivider()
                syncRow
            }
            let children = model.snapshot.children(of: folder.id)
            if !children.isEmpty {
                KilnRowDivider()
                childrenSection(children)
            }
            if folder.served {
                KilnRowDivider()
                advancedDisclosure
            }
        }
        .background(kiln.card2)
    }

    private var missingRow: some View {
        KilnRow(title: Strings.FolderPage.missingTitle(folder.path), subtitle: Strings.FolderPage.missingSub) {
            StatusDot(.red)
        } trailing: {
            if model.isSample {
                Button(Strings.Programs.findIt) {
                    model.say(Strings.Programs.wouldFind(folder.name))
                }
                .buttonStyle(KilnButtonStyle(.plain))
            } else if FolderControls.canRelink(model.snapshot) {
                Button(Strings.Programs.findIt) {
                    FolderFlow.findIt(folder, model: model)
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
    }

    private var viewerRow: some View {
        KilnRow(title: Strings.FolderPage.askOwner, subtitle: Strings.FolderPage.viewerSub(folder.name)) {
            StatusDot(.amber)
        } trailing: {
            EmptyView()
        }
    }

    private var trustedRow: some View {
        let trusted = FolderControls.trusted(folder: folder)
        return KilnRow(title: Strings.FolderPage.trusted, subtitle: trusted.note ?? Strings.FolderPage.trustedSub) {
            Toggle(isOn: Binding(
                get: { folder.onThisMac },
                set: { on in
                    if !on {
                        handleStopTrusting()
                    }
                }
            )) {
                Text(Strings.FolderPage.trusted)
            }
            .toggleStyle(KilnSwitchStyle())
            .disabled(!trusted.enabled)
        }
    }

    private var servedRow: some View {
        let served = FolderControls.served(folder: folder, pending: model.pendingServed(folder.id))
        let sub = served.note ?? (folder.served && !folder.address.isEmpty ? folder.address : Strings.FolderPage.serveSubOff)
        return KilnRow(title: Strings.Folders.serveLocally, subtitle: sub) {
            HStack(spacing: 8) {
                if folder.served && !folder.address.isEmpty {
                    Button(Strings.Folders.copy) {
                        copyAddress()
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
                Toggle(isOn: Binding(
                    get: { served.value },
                    set: { on in
                        if on {
                            if folder.address.isEmpty {
                                model.sheet = .serveFolder(folderID: folder.id)
                            } else {
                                model.send(.setServed(folderID: folder.id, on: true))
                            }
                        } else {
                            alert = FolderAlert(kind: .stopServing)
                        }
                    }
                )) {
                    Text(Strings.Folders.serveLocally)
                }
                .toggleStyle(KilnSwitchStyle())
                .disabled(!served.enabled)
            }
        }
    }

    @ViewBuilder private var syncRow: some View {
        switch folder.origin {
        case .local:
            KilnRow(title: Strings.FolderPage.sync, subtitle: Strings.FolderPage.onlyOnThisMacSub) {
                Button(Strings.Folders.connectHyperclay) {
                    model.route = .pane(.sync)
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        case .account, .team:
            let synced = FolderControls.synced(folder: folder,
                                               pending: model.pendingSynced(folder.id),
                                               hostCan: model.can(HostActionType.setSynced))
            let sub = synced.note ?? (folder.synced ? FolderPage.syncStatus(folder, snapshot: model.snapshot).line : Strings.FolderPage.syncSub)
            KilnRow(title: Strings.FolderPage.sync, subtitle: sub) {
                HStack(spacing: 8) {
                    if folder.synced {
                        Button(Strings.FolderPage.folderHistory) {
                            model.route = .folderHistory(folder.id)
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                    Toggle(isOn: Binding(
                        get: { synced.value },
                        set: { on in
                            if on {
                                model.send(.setSynced(folderID: folder.id, on: true))
                            } else {
                                alert = FolderAlert(kind: .disconnect)
                            }
                        }
                    )) {
                        Text(Strings.FolderPage.sync)
                    }
                    .toggleStyle(KilnSwitchStyle())
                    .disabled(!synced.enabled)
                }
            }
        }
    }

    private func childrenSection(_ children: [Folder]) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(children.enumerated()), id: \.element.id) { index, child in
                if index > 0 {
                    KilnRowDivider()
                }
                KilnRow(title: child.name, subtitle: Strings.FolderPage.pathLine(path: child.path, documents: child.documentCount)) {
                    HStack(spacing: 4) {
                        Text(Strings.Folders.elbow)
                            .font(.system(size: 13))
                            .foregroundStyle(kiln.faint)
                        Squircle(.sky, symbol: Symbols.folder)
                    }
                } trailing: {
                    if FolderControls.showsMerge(child: child, hostCan: model.can(HostActionType.merge)) {
                        Button(Strings.FolderPage.mergeButton(parent: folder.name)) {
                            MergeFlow.open(childID: child.id, model: model)
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                }
            }
        }
    }

    private var advancedDisclosure: some View {
        let framing = FolderControls.framing(folder: folder,
                                             pending: model.pendingFraming(folder.id),
                                             hostCan: model.can(HostActionType.setFraming))
        return VStack(alignment: .leading, spacing: 0) {
            Button {
                showAdvanced.toggle()
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: showAdvanced ? Symbols.chevronDown : Symbols.chevronRight)
                        .font(.system(size: 10, weight: .semibold))
                        .foregroundStyle(kiln.muted)
                    Text(Strings.Folders.advanced)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundStyle(kiln.ink2)
                    if framing.value {
                        KilnPill(.warn, text: Strings.Folders.framingOnPill)
                    }
                    Spacer(minLength: 0)
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 9)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            if showAdvanced {
                KilnRowDivider()
                KilnRow(title: Strings.FolderPage.framing, subtitle: Strings.FolderPage.framingSub) {
                    Toggle(isOn: Binding(
                        get: { framing.value },
                        set: { model.send(.setFraming(folderID: folder.id, on: $0)) }
                    )) {
                        Text(Strings.FolderPage.framing)
                    }
                    .toggleStyle(KilnSwitchStyle())
                    .disabled(!framing.enabled)
                }
                KilnNoteRow(Strings.FolderPage.framingNote)
            }
        }
    }

    private func handleStopTrusting() {
        switch FolderControls.stopTrustingDecision(for: folder) {
        case .confirmSync:
            alert = FolderAlert(kind: .stopTrustingSync)
        case .immediate:
            Task {
                let outcome = await model.submit(.stopTrusting(folderID: folder.id))
                if case let .refused(refusal) = outcome {
                    if refusal.code == HostRefusal.confirmSyncStop {
                        alert = FolderAlert(kind: .stopTrustingSync)
                    } else if refusal.code == HostRefusal.unsavedEdits {
                        alert = FolderAlert(kind: .stopTrustingUnsaved(refusal.documents))
                    } else if !refusal.text.isEmpty {
                        model.say(refusal.text)
                    }
                }
            }
        }
    }

    private func openFolder() {
        if model.isSample {
            actions.openDashboard()
        } else if folder.served {
            model.send(.openDashboard(route: Strings.Folders.rootRoute(folder.id)))
        } else {
            model.send(.openDashboard(route: Strings.Folders.trustedRoute(folder.id)))
        }
    }

    private func revealFolder() {
        if model.isSample {
            model.say(Strings.Preview.wouldShowInFinder(folder.path))
        } else {
            model.send(.revealFolder(id: folder.id))
        }
    }

    private func copyAddress() {
        if model.isSample {
            NSPasteboard.general.clearContents()
            NSPasteboard.general.setString(folder.address, forType: .string)
        } else {
            model.send(.copyAddress(folderID: folder.id))
        }
        model.say(Strings.Folders.copied(folder.address))
    }

    private var leading: some View {
        HStack(spacing: 4) {
            if parent != nil {
                Text(Strings.Folders.elbow)
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.faint)
            }
            Squircle(squircleColor, symbol: Symbols.folder)
        }
    }

    @ViewBuilder private var badge: some View {
        if waiting > 0 {
            KilnBadge(waiting)
        }
    }

    private var squircleColor: SquircleColor {
        switch folder.origin {
        case .local: return folder.served ? .teal : .sky
        case .account: return .cobalt
        case .team: return .plum
        }
    }

    private var subtitle: String? {
        if let parent {
            return Strings.Folders.inside(parent: parent.name, path: folder.path)
        }
        if folder.served && !folder.address.isEmpty && folder.origin == .local {
            return Strings.Folders.pathAndAddress(path: folder.path, address: folder.address)
        }
        switch folder.origin {
        case .local: return folder.path
        case .account, .team: return folder.status
        }
    }
}
