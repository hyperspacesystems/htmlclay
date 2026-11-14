import SwiftUI
import PreviewCore

enum PanelSection: Hashable {
    case openDashboard, hostStopped, keyRefused, update, finishing, waiting, syncLine, offline
}

public struct PanelView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let actions: PanelActions
    public init(actions: PanelActions = PanelActions()) { self.actions = actions }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(Self.topSections(model.snapshot), id: \.self) {
                section(for: $0)
            }
            if model.snapshot.documents.isEmpty {
                KilnCard {
                    KilnRow(title: Strings.Panel.noDocumentsTitle, subtitle: Strings.Panel.noDocumentsSub) { EmptyView() }
                }
            } else {
                KilnGroupTitle(Strings.Panel.recentDocuments)
                documentsCard
            }
            if model.snapshot.hostStopped, PanelControls.showsAppLog(isSample: model.isSample) {
                KilnLink(Strings.Panel.showLogs) {
                    model.say(Strings.Advanced.wouldShowAppLog)
                }
            }
            footer
            if let note = model.note {
                PreviewNote(note)
            }
        }
        .padding(12)
        .frame(width: 382)
    }

    static func topSections(_ s: HostSnapshot) -> [PanelSection] {
        var sections: [PanelSection] = [s.hostStopped ? .hostStopped : .openDashboard]
        let refused = s.waiting.contains { $0.kind == .syncKeyRefused }
        if refused { sections.append(.keyRefused) }
        if s.update.ready != nil, PanelControls.showsUpdate(can: s.hostCan(HostActionType.updateStart)) {
            sections.append(s.update.finishing ? .finishing : .update)
        }
        if s.waiting.contains(where: { $0.earnsDot && $0.kind != .syncKeyRefused }) { sections.append(.waiting) }
        if s.sync.connected && !refused { sections.append(s.sync.offlineChanges > 0 ? .offline : .syncLine) }
        return sections
    }

    @ViewBuilder private func section(for section: PanelSection) -> some View {
        switch section {
        case .openDashboard:
            Button(Strings.Panel.openDashboard, action: actions.openDashboard)
                .buttonStyle(KilnButtonStyle(.openDashboard))
        case .hostStopped:
            KilnCard(kind: .err) {
                KilnRow(title: Strings.Panel.hostStopped, subtitle: Strings.Panel.hostStoppedSub) {
                    Button(Strings.Panel.restart) {
                        model.send(.restartHost)
                    }
                    .buttonStyle(KilnButtonStyle(.primary))
                } trailing: {
                    EmptyView()
                }
            }
        case .keyRefused:
            KilnCard(kind: .err) {
                KilnRow(title: Strings.Panel.keyRefused) {
                    Button(Strings.Sync.reconnect) {
                        model.sheet = .connect
                        actions.openSettings(.sync)
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
            }
        case .update:
            let ready = model.snapshot.update.ready ?? model.snapshot.update.current
            KilnCard {
                KilnRow(title: Strings.Panel.updateReady(UpdatesPane.short(ready))) {
                    Squircle(.jade, symbol: Symbols.update)
                } trailing: {
                    Button(Strings.Panel.restart) {
                        model.send(.startUpdate)
                    }
                    .buttonStyle(KilnButtonStyle(.primary))
                }
            }
        case .finishing:
            KilnCard {
                KilnRow(title: Strings.Panel.finishing, subtitle: Strings.Panel.restartsSoon) {
                    Image(systemName: Symbols.versions)
                        .font(.system(size: 15))
                        .foregroundStyle(kiln.muted)
                        .frame(width: 24)
                } trailing: {
                    EmptyView()
                }
            }
            .task {
                try? await Task.sleep(nanoseconds: 3_000_000_000)
                if Task.isCancelled { return }
                if let ready = model.snapshot.update.ready {
                    model.say(Strings.Updates.wouldRestart(ready))
                }
                model.send(.finishUpdate)
            }
        case .waiting:
            waitingCard
        case .syncLine:
            syncLine
        case .offline:
            HStack(spacing: 6) {
                Image(systemName: Symbols.offline)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(kiln.muted)
                Text(Strings.Panel.offline(model.snapshot.sync.offlineChanges))
                    .font(.system(size: 12.5))
                    .foregroundStyle(kiln.muted)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var waitingItems: [WaitingItem] {
        model.snapshot.waiting.filter(\.earnsDot)
    }

    private var conflictGroups: [(folder: String, count: Int)] {
        var order: [String] = []
        var counts: [String: Int] = [:]
        for item in waitingItems where item.kind == .conflict {
            if counts[item.detail] == nil { order.append(item.detail) }
            counts[item.detail, default: 0] += 1
        }
        return order.map { (folder: $0, count: counts[$0] ?? 0) }
    }

    private var settingsKinds: [WaitingItem.Kind] { [.folderMissing, .serviceFailed] }

    private var waitingRows: [AnyView] {
        var rows: [AnyView] = []
        for group in conflictGroups {
            rows.append(AnyView(
                KilnRow(title: Strings.Panel.conflicts(folder: group.folder, count: group.count)) {
                    Button(Strings.Panel.review, action: actions.openDashboard)
                        .buttonStyle(KilnButtonStyle(.plain))
                }
            ))
        }
        for item in waitingItems where item.kind == .programRequest {
            rows.append(AnyView(
                KilnRow(title: Strings.Panel.wantsProgram(document: documentName(for: item))) {
                    if PanelControls.showsAnswer(can: model.can(HostActionType.requestAnswer)) {
                        Button(Strings.Panel.answer) { actions.answer(item) }
                            .buttonStyle(KilnButtonStyle(.plain))
                    }
                }
            ))
        }
        for item in waitingItems where settingsKinds.contains(item.kind) {
            rows.append(AnyView(
                KilnRow(title: item.title) {
                    Button(Strings.Panel.review) { actions.openSettings(item.pane) }
                        .buttonStyle(KilnButtonStyle(.plain))
                }
            ))
        }
        return rows
    }

    private var waitingCard: some View {
        let rows = waitingRows
        return KilnCard(kind: .warn) {
            ForEach(rows.indices, id: \.self) { index in
                if index > 0 { KilnRowDivider() }
                rows[index]
            }
        }
    }

    private func documentName(for item: WaitingItem) -> String {
        if let request = model.snapshot.requests.first(where: { $0.id == item.subjectID }) {
            return request.document
        }
        if let range = item.title.range(of: Strings.Panel.wantsSuffix) {
            return String(item.title[..<range.lowerBound])
        }
        return item.title
    }

    private var syncLine: some View {
        HStack(spacing: 6) {
            StatusDot(.blue)
            Text(Strings.Panel.syncLine(model.snapshot.sync))
                .font(.system(size: 12.5))
                .foregroundStyle(kiln.muted)
        }
    }

    private var documentsCard: some View {
        let documents = Array(model.snapshot.documents.prefix(5))
        return KilnCard {
            ForEach(documents.indices, id: \.self) { index in
                if index > 0 { KilnRowDivider() }
                documentRow(documents[index])
            }
        }
    }

    private func documentRow(_ document: RecentDocument) -> some View {
        let snapshot = model.snapshot
        return KilnRow(title: document.name, subtitle: subtitle(for: document)) {
            Image(systemName: Symbols.doc)
                .font(.system(size: 15))
                .foregroundStyle(document.fileMissing ? kiln.warnFg : kiln.muted)
        } trailing: {
            Menu {
                if PanelControls.showsDocumentAction(HostActionType.documentOpen, in: snapshot) {
                    Button(Strings.Menu.rowOpen) {
                        if snapshot.isSample {
                            model.say(Strings.Preview.wouldOpen(document.name))
                        } else {
                            model.send(.openDocument(id: document.id))
                        }
                    }
                }
                if PanelControls.showsDocumentAction(HostActionType.documentVersions, in: snapshot) {
                    Button(Strings.Menu.versionHistory) {
                        if snapshot.isSample {
                            actions.openDashboard()
                        } else {
                            model.send(.openVersions(id: document.id))
                        }
                    }
                }
                if document.fileMissing, PanelControls.showsDocumentAction(HostActionType.documentRecover, in: snapshot) {
                    Button(Strings.Menu.recoverMissing) {
                        if snapshot.isSample {
                            model.say(Strings.Preview.wouldRecover(document.name))
                        } else {
                            model.recoverDocument(document)
                        }
                    }
                }
                if PanelControls.showsDocumentAction(HostActionType.documentReveal, in: snapshot) {
                    Button(Strings.Menu.showInFinder) {
                        if snapshot.isSample {
                            model.say(Strings.Preview.wouldShowInFinder(document.name))
                        } else {
                            model.send(.revealDocument(id: document.id))
                        }
                    }
                }
                if PanelControls.showsExport(isSample: snapshot.isSample, can: model.can(HostActionType.documentExport)) {
                    Button(Strings.Menu.exportZip) {
                        if snapshot.isSample {
                            model.say(Strings.Preview.wouldExport(document.name))
                        } else {
                            model.send(.exportDocument(id: document.id))
                        }
                    }
                }
                if PanelControls.showsDocumentAction(HostActionType.documentForget, in: snapshot) {
                    Divider()
                    Button(Strings.Menu.removeFromRecent) {
                        if snapshot.isSample {
                            model.say(Strings.Preview.wouldRemove(document.name))
                        } else {
                            model.send(.forgetDocument(id: document.id))
                        }
                    }
                }
            } label: {
                Image(systemName: Symbols.more)
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .fixedSize()
        }
    }

    private func subtitle(for document: RecentDocument) -> String {
        document.fileMissing
            ? Strings.Panel.fileMissing(versions: document.versionsKept)
            : Strings.Panel.documentSubtitle(place: document.place, opened: document.opened)
    }

    private var footer: some View {
        HStack(spacing: 8) {
            Button(Strings.Panel.openAFile) {
                if model.isSample {
                    model.say(Strings.Preview.wouldOpenFile)
                } else {
                    model.chooseAFile()
                }
            }
            .buttonStyle(KilnButtonStyle(.plain))
            if model.snapshot.documents.isEmpty {
                KilnLink(Strings.Panel.startFromTemplate, action: actions.openDashboard)
            }
            Spacer()
            if model.snapshot.sync.connected,
               PanelControls.showsSyncPause(can: model.can(HostActionType.syncSetPaused)) {
                Button(model.snapshot.sync.paused ? Strings.Panel.resumeSyncing : Strings.Panel.pauseSyncing) {
                    model.send(.setSyncPaused(!model.snapshot.sync.paused))
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
            Menu {
                Button(Strings.Menu.settings) { actions.openSettings(.general) }
                Button(Strings.Menu.checkForUpdates) { actions.openSettings(.updates) }
                Divider()
                Button(Strings.Menu.quit, action: actions.quit)
            } label: {
                Image(systemName: Symbols.gear)
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .fixedSize()
        }
    }
}
