import AppKit
import SwiftUI
import PreviewCore

enum FolderInnerKind: Equatable {
    case sync, onlyOnThisMac, none
}

enum FolderProblem: Equatable {
    case none, viewer, missing
}

struct FolderPage: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var alert: FolderAlert?
    @State private var showAdvanced = false
    private let folderID: String
    private let actions: SettingsActions

    init(folderID: String, actions: SettingsActions) {
        self.folderID = folderID
        self.actions = actions
    }

    static func problem(for folder: Folder) -> FolderProblem {
        if folder.missingOnDisk { return .missing }
        if folder.becameViewer { return .viewer }
        return .none
    }

    static func syncStatus(_ folder: Folder, snapshot: HostSnapshot) -> (dot: StatusDotKind, line: String) {
        if snapshot.sync.paused { return (.muted, Strings.FolderPage.pausedAll) }
        if snapshot.sync.offlineChanges > 0 { return (.muted, Strings.FolderPage.offline(snapshot.sync.offlineChanges)) }
        return (.green, folder.lastSync)
    }

    static func levels(for folder: Folder) -> (served: Bool, innerKind: FolderInnerKind) {
        guard folder.served else { return (served: false, innerKind: .none) }
        switch folder.origin {
        case .local: return (served: true, innerKind: .onlyOnThisMac)
        case .account, .team: return (served: true, innerKind: .sync)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let folder = model.snapshot.folder(folderID) {
                page(folder)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func page(_ folder: Folder) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            header(folder)
            KilnPath(Strings.FolderPage.pathLine(path: folder.path, documents: folder.documentCount))
            if Self.problem(for: folder) != .none {
                problemCard(folder)
                    .padding(.bottom, 12)
            }
            settingsCard(folder)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .folderConfirmations(folder: folder, alert: $alert)
    }

    @ViewBuilder private func problemCard(_ folder: Folder) -> some View {
        switch Self.problem(for: folder) {
        case .viewer:
            KilnCard(kind: .warn) {
                KilnRow(title: folder.name, subtitle: Strings.FolderPage.viewerSub(folder.name)) {
                    StatusDot(.amber)
                } trailing: {
                    EmptyView()
                }
                KilnRowDivider()
                KilnNoteRow(Strings.FolderPage.askOwner) {
                    if model.isSample {
                        KilnLink(Strings.FolderPage.open) {
                            model.say(Strings.Sync.wouldOpenSite)
                        }
                    }
                }
            }
        case .missing:
            KilnCard(kind: .err) {
                KilnRow(title: Strings.FolderPage.missingTitle(folder.path),
                        subtitle: Strings.FolderPage.missingSub) {
                    EmptyView()
                }
                if model.isSample {
                    KilnRowDivider()
                    HStack(spacing: 8) {
                        Spacer(minLength: 0)
                        Button(Strings.Programs.findIt) {
                            model.say(Strings.Programs.wouldFind(folder.name))
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                        Button(Strings.FolderPage.setUpAgain) {
                            model.say(Strings.FolderPage.wouldSetUpAgain(folder.name))
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                } else if FolderControls.canRelink(model.snapshot) {
                    KilnRowDivider()
                    HStack(spacing: 8) {
                        Spacer(minLength: 0)
                        Button(Strings.Programs.findIt) {
                            FolderFlow.findIt(folder, model: model)
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                }
            }
        case .none:
            EmptyView()
        }
    }

    private func header(_ folder: Folder) -> some View {
        HStack(spacing: 10) {
            KilnBackButton {
                model.route = .pane(.folders)
            }
            Text(folder.name)
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
            Spacer(minLength: 0)
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
        }
        .padding(.bottom, 16)
    }

    private func settingsCard(_ folder: Folder) -> some View {
        let trusted = FolderControls.trusted(folder: folder)
        let served = FolderControls.served(folder: folder, pending: model.pendingServed(folder.id))
        let children = model.snapshot.children(of: folder.id)
        return KilnCard {
            KilnRow(title: Strings.FolderPage.trusted, subtitle: trusted.note ?? Strings.FolderPage.trustedSub) {
                Toggle(isOn: trustedBinding(folder)) {
                    Text(Strings.FolderPage.trusted)
                }
                .toggleStyle(KilnSwitchStyle())
                .disabled(!trusted.enabled)
            }
            if !children.isEmpty {
                KilnRowDivider()
                childRows(folder, children: children)
            }
            KilnRowDivider()
            KilnRow(title: Strings.FolderPage.serve, subtitle: served.note ?? serveSub(folder)) {
                Toggle(isOn: servedBinding) {
                    Text(Strings.FolderPage.serve)
                }
                .toggleStyle(KilnSwitchStyle())
                .disabled(!served.enabled)
            }
            if folder.served {
                KilnRowDivider()
                addressRow(folder)
                KilnRowDivider()
                syncSection(folder)
                KilnRowDivider()
                advancedSection(folder)
            }
        }
    }

    private func childRows(_ folder: Folder, children: [Folder]) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            FolderPageRow(title: Strings.FolderPage.childrenTitle(count: children.count, parent: folder.name),
                          subtitle: Strings.FolderPage.childrenSub(parent: folder.name))
            ForEach(children) { child in
                KilnRowDivider()
                KilnRow(title: child.name,
                        subtitle: Strings.FolderPage.pathLine(path: child.path, documents: child.documentCount)) {
                    HStack(spacing: 4) {
                        Text(Strings.Folders.elbow)
                            .font(.system(size: 13))
                            .foregroundStyle(kiln.faint)
                        Squircle(.sky, symbol: Symbols.folder)
                    }
                } trailing: {
                    if FolderControls.showsMerge(child: child,
                                                 hostCan: model.can(HostActionType.merge)) {
                        Button(Strings.FolderPage.mergeButton(parent: folder.name)) {
                            MergeFlow.open(childID: child.id, model: model)
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                }
            }
        }
    }

    private func addressRow(_ folder: Folder) -> some View {
        HStack(spacing: 11) {
            VStack(alignment: .leading, spacing: 0) {
                Text(Strings.FolderPage.address)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(kiln.ink)
                Text(folder.address)
                    .font(.system(size: 12.5, design: .monospaced))
                    .foregroundStyle(kiln.ink2)
                    .padding(.top, 2)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Button(Strings.FolderPage.copy) {
                if model.isSample {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(folder.address, forType: .string)
                } else {
                    model.send(.copyAddress(folderID: folder.id))
                }
                model.say(Strings.Folders.copied(folder.address))
            }
            .buttonStyle(KilnButtonStyle(.plain))
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }

    @ViewBuilder private func syncSection(_ folder: Folder) -> some View {
        switch Self.levels(for: folder).innerKind {
        case .sync:
            let synced = FolderControls.synced(folder: folder,
                                               pending: model.pendingSynced(folder.id),
                                               hostCan: model.can(HostActionType.setSynced))
            KilnRow(title: Strings.FolderPage.sync, subtitle: synced.note ?? Strings.FolderPage.syncSub) {
                Toggle(isOn: syncedBinding) {
                    Text(Strings.FolderPage.sync)
                }
                .toggleStyle(KilnSwitchStyle())
                .disabled(!synced.enabled)
            }
            if folder.synced {
                let status = Self.syncStatus(folder, snapshot: model.snapshot)
                KilnRowDivider()
                KilnRow(title: syncTitle(folder), subtitle: status.line) {
                    StatusDot(status.dot)
                } trailing: {
                    EmptyView()
                }
                KilnRowDivider()
                KilnRow(title: Strings.FolderPage.folderHistory, subtitle: folder.historySummary) {
                    Button {
                        model.route = .folderHistory(folder.id)
                    } label: {
                        Image(systemName: Symbols.chevronRight)
                            .font(.system(size: 11))
                            .foregroundStyle(kiln.faint)
                    }
                    .buttonStyle(.plain)
                }
            } else if case let .team(name) = folder.origin,
                      Self.problem(for: folder) == .none,
                      model.can(HostActionType.setSynced) {
                KilnRowDivider()
                KilnRow(title: Strings.FolderPage.wasTeam(name), subtitle: Strings.FolderPage.wasTeamSub) {
                    StatusDot(.muted)
                } trailing: {
                    Button(Strings.FolderPage.reconnect) {
                        if model.snapshot.sync.connected {
                            model.send(.setSynced(folderID: folder.id, on: true))
                        } else {
                            model.sheet = .connect
                        }
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
            }
        case .onlyOnThisMac:
            KilnRow(title: Strings.FolderPage.onlyOnThisMac, subtitle: Strings.FolderPage.onlyOnThisMacSub) {
                Button(Strings.FolderPage.connect) {
                    model.route = .pane(.sync)
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        case .none:
            EmptyView()
        }
    }

    private func advancedSection(_ folder: Folder) -> some View {
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
                    Toggle(isOn: framingBinding) {
                        Text(Strings.FolderPage.framing)
                    }
                    .toggleStyle(KilnSwitchStyle())
                    .disabled(!framing.enabled)
                }
                KilnNoteRow(Strings.FolderPage.framingNote)
            }
        }
    }

    private func syncTitle(_ folder: Folder) -> String {
        switch folder.origin {
        case .local: return folder.name
        case let .account(username): return Strings.FolderPage.accountTitle(username: username)
        case let .team(name): return name
        }
    }

    private func serveSub(_ folder: Folder) -> String {
        if folder.served { return Strings.FolderPage.serveSubOn }
        let base = Strings.FolderPage.serveSubOff
        if case .local = folder.origin { return base + Strings.FolderPage.serveSubOnlyOnThisMac }
        return base
    }

    private func trustedBinding(_ folder: Folder) -> Binding<Bool> {
        Binding(
            get: { model.snapshot.folder(folderID)?.onThisMac ?? false },
            set: { on in
                guard !on else { return }
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
        )
    }

    private var servedBinding: Binding<Bool> {
        Binding(
            get: {
                guard let folder = model.snapshot.folder(folderID) else { return false }
                return FolderControls.served(folder: folder, pending: model.pendingServed(folderID)).value
            },
            set: { on in
                guard let folder = model.snapshot.folder(folderID) else { return }
                if on {
                    if folder.address.isEmpty {
                        model.sheet = .serveFolder(folderID: folderID)
                    } else {
                        model.send(.setServed(folderID: folderID, on: true))
                    }
                } else {
                    alert = FolderAlert(kind: .stopServing)
                }
            }
        )
    }

    private var framingBinding: Binding<Bool> {
        Binding(
            get: {
                guard let folder = model.snapshot.folder(folderID) else { return false }
                return FolderControls.framing(folder: folder,
                                              pending: model.pendingFraming(folderID),
                                              hostCan: model.can(HostActionType.setFraming)).value
            },
            set: { on in
                model.send(.setFraming(folderID: folderID, on: on))
            }
        )
    }

    private var syncedBinding: Binding<Bool> {
        Binding(
            get: {
                guard let folder = model.snapshot.folder(folderID) else { return false }
                return FolderControls.synced(folder: folder, pending: model.pendingSynced(folderID),
                                             hostCan: model.can(HostActionType.setSynced)).value
            },
            set: { on in
                if on {
                    model.send(.setSynced(folderID: folderID, on: true))
                } else {
                    alert = FolderAlert(kind: .disconnect)
                }
            }
        )
    }
}

struct FolderPageRow: View {
    @Environment(\.kiln) private var kiln
    private let title: String
    private let subtitle: String

    init(title: String, subtitle: String) {
        self.title = title
        self.subtitle = subtitle
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(title)
                .font(.system(size: 14, weight: .medium))
                .foregroundStyle(kiln.ink)
            Text(subtitle)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .padding(.top, 2)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
}
