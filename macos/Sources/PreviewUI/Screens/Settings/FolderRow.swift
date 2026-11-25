import AppKit
import SwiftUI
import PreviewCore

struct FolderRow: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @Environment(\.openURL) private var openURL
    @State private var expanded = false
    @State private var showAdvanced = false
    @State private var hoveringAddress = false
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

    private func pillLabel(for level: KilnPillLevel) -> String {
        switch level {
        case .trusted: return Strings.Levels.trusted
        case .served: return Strings.Levels.served
        case .synced: return Strings.Levels.synced
        case .warn: return Strings.Levels.warning
        case .err: return Strings.Levels.error
        }
    }

    var body: some View {
        if folder.onThisMac {
            VStack(alignment: .leading, spacing: 0) {
                HStack(alignment: .center, spacing: 11) {
                    leading
                    VStack(alignment: .leading, spacing: 0) {
                        HStack(spacing: 6) {
                            Text(folder.name)
                                .font(.system(size: 14, weight: .medium))
                                .foregroundStyle(kiln.ink)
                            Button {
                                copyFullPath()
                            } label: {
                                CopyPathIcon()
                                    .fill(kiln.muted, style: FillStyle(eoFill: true))
                                    .frame(width: 14, height: 14)
                                    .padding(2)
                                    .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                            .help(Strings.Folders.copyPathHelp(folder.path))
                            .accessibilityLabel(Text(Strings.Folders.copyPathLabel(folder.name)))
                        }
                        HStack(spacing: 6) {
                            ForEach(Array(Self.pills(for: folder).enumerated()), id: \.offset) { item in
                                KilnPill(item.element, text: pillLabel(for: item.element))
                            }
                        }
                        .padding(.top, 5)
                        subtitleView
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    HStack(alignment: .center, spacing: 8) {
                        badge
                        VStack(alignment: .trailing, spacing: 6) {
                            Button {
                                openFolder()
                            } label: {
                                Text(Strings.Folders.openInDashboard)
                                    .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(KilnButtonStyle(.plain))
                            if model.isSample || model.can(HostActionType.reveal) {
                                Button {
                                    revealFolder()
                                } label: {
                                    Text(Strings.Folders.openInFinder)
                                        .frame(maxWidth: .infinity)
                                }
                                .buttonStyle(KilnButtonStyle(.plain))
                            }
                            Button {
                                expanded.toggle()
                            } label: {
                                HStack(spacing: 4) {
                                    Image(systemName: expanded ? Symbols.chevronDown : Symbols.chevronRight)
                                        .font(.system(size: 10, weight: .semibold))
                                    Text(Strings.Folders.more)
                                }
                                .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(KilnButtonStyle(.plain))
                        }
                        .fixedSize(horizontal: true, vertical: false)
                    }
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                if expanded {
                    inlineDetails
                }
            }
            .onReceive(NotificationCenter.default.publisher(for: NSWindow.willCloseNotification)) { _ in
                clearHoverCursor()
            }
            .onReceive(NotificationCenter.default.publisher(for: NSWindow.didResignKeyNotification)) { _ in
                clearHoverCursor()
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
                            model.send(.setServed(folderID: folder.id, on: true))
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

    private func copyFullPath() {
        let target = folder.fullPath.isEmpty ? folder.path : folder.fullPath
        guard !target.isEmpty else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(target, forType: .string)
        model.say(Strings.Folders.copiedPath(folder.path.isEmpty ? target : folder.path))
    }

    private var addressURL: URL? {
        Strings.Folders.addressURL(url: folder.url, address: folder.address)
    }

    private func clearHoverCursor() {
        if hoveringAddress {
            NSCursor.pop()
            hoveringAddress = false
        }
    }

    @ViewBuilder private var subtitleView: some View {
        if let parent {
            Text(Strings.Folders.insideParent(parent.name))
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .padding(.top, 4)
        }
        if folder.served && !folder.address.isEmpty {
            if let url = addressURL {
                Link(destination: url) {
                    Text(folder.address)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundStyle(kiln.link)
                        .underline(hoveringAddress)
                }
                .buttonStyle(.plain)
                .onHover { inside in
                    guard inside != hoveringAddress else { return }
                    hoveringAddress = inside
                    if inside {
                        NSCursor.pointingHand.push()
                    } else {
                        NSCursor.pop()
                    }
                }
                .onDisappear {
                    clearHoverCursor()
                }
                .contextMenu {
                    Button(Strings.Folders.openLink) {
                        openURL(url)
                    }
                    Button(Strings.Folders.copyAddress) {
                        copyAddress()
                    }
                }
                .padding(.top, 5)
            } else {
                Text(folder.address)
                    .font(.system(size: 13, weight: .medium))
                    .foregroundStyle(kiln.link)
                    .padding(.top, 5)
            }
        } else if folder.origin != .local && !folder.status.isEmpty {
            Text(folder.status)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .padding(.top, 4)
        }
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
}
