import SwiftUI
import PreviewCore

public struct PanelView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let actions: PanelActions
    public init(actions: PanelActions = PanelActions()) { self.actions = actions }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Button(Strings.Panel.openDashboard, action: actions.openDashboard)
                .buttonStyle(KilnButtonStyle(.openDashboard))
            if !waitingItems.isEmpty {
                waitingCard
            }
            if model.snapshot.sync.connected {
                syncLine
            }
            if model.snapshot.documents.isEmpty {
                KilnCard {
                    KilnRow(title: Strings.Panel.noDocumentsTitle, subtitle: Strings.Panel.noDocumentsSub) { EmptyView() }
                }
            } else {
                KilnGroupTitle(Strings.Panel.recentDocuments)
                documentsCard
            }
            footer
            if let note = model.note {
                PreviewNote(note)
            }
        }
        .padding(12)
        .frame(width: 382)
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

    private var settingsKinds: [WaitingItem.Kind] { [.syncKeyRefused, .folderMissing, .serviceFailed] }

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
                    Button(Strings.Panel.answer) { actions.answer(item) }
                        .buttonStyle(KilnButtonStyle(.plain))
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
        KilnRow(title: document.name, subtitle: subtitle(for: document)) {
            Image(systemName: Symbols.doc)
                .font(.system(size: 15))
                .foregroundStyle(document.fileMissing ? kiln.warnFg : kiln.muted)
        } trailing: {
            Menu {
                Button(Strings.Menu.rowOpen) { model.say(Strings.Preview.wouldOpen(document.name)) }
                Button(Strings.Menu.versionHistory, action: actions.openDashboard)
                Button(Strings.Menu.showInFinder) { model.say(Strings.Preview.wouldShowInFinder(document.name)) }
                Button(Strings.Menu.exportZip) { model.say(Strings.Preview.wouldExport(document.name)) }
                Divider()
                Button(Strings.Menu.removeFromRecent) { model.say(Strings.Preview.wouldRemove(document.name)) }
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
            Button(Strings.Panel.openAFile) { model.say(Strings.Preview.wouldOpenFile) }
                .buttonStyle(KilnButtonStyle(.plain))
            if model.snapshot.documents.isEmpty {
                KilnLink(Strings.Panel.startFromTemplate, action: actions.openDashboard)
            }
            Spacer()
            if model.snapshot.sync.connected {
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
