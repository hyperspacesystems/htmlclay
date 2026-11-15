import SwiftUI
import PreviewCore

struct FolderHistoryPage: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let folderID: String
    @State private var history: FolderHistoryModel?

    init(folderID: String) {
        self.folderID = folderID
    }

    static func symbol(_ kind: HistoryEntry.Kind) -> String {
        switch kind {
        case .uploaded: return Symbols.uploaded
        case .downloaded: return Symbols.downloaded
        case .renamed: return Symbols.renamed
        case .removed: return Symbols.removed
        case .conflict: return Symbols.conflict
        case .settled: return Symbols.settled
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let folder = model.snapshot.folder(folderID) {
                page(folder)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .task(id: folderID) {
            let history = model.historyModel(folderID: folderID)
            self.history = history
            await history.loadFirst()
        }
    }

    private func page(_ folder: Folder) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 10) {
                KilnBackButton {
                    model.route = .folder(folderID)
                }
                Text(Strings.History.title(folder.name))
                    .font(.system(size: 18, weight: .bold))
                    .foregroundStyle(kiln.ink)
                Spacer(minLength: 0)
                if SettingsControls.showsSyncLog(isSample: model.isSample, can: model.can(HostActionType.syncRevealLog)) {
                    Button(Strings.History.showSyncLog) {
                        if model.isSample { model.say(Strings.Advanced.wouldShowSyncLog) } else { model.send(.revealSyncLog) }
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
            }
            .padding(.bottom, 4)
            if let history, history.loaded {
                if history.entries.isEmpty {
                    Text(Strings.History.empty)
                        .font(.system(size: 13))
                        .foregroundStyle(kiln.muted)
                        .padding(.top, 12)
                }
                let runs = FolderHistoryModel.days(history.entries)
                ForEach(runs.indices, id: \.self) { run in
                    KilnGroupTitle(runs[run].day)
                    KilnCard {
                        ForEach(runs[run].entries.indices, id: \.self) { index in
                            if index > 0 { KilnRowDivider() }
                            row(runs[run].entries[index])
                                .onAppear {
                                    if run == runs.count - 1 && index == runs[run].entries.count - 1 {
                                        Task { await history.loadMore() }
                                    }
                                }
                        }
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func row(_ entry: HistoryEntry) -> some View {
        HStack(alignment: .top, spacing: 11) {
            Image(systemName: Self.symbol(entry.kind))
                .font(.system(size: 15))
                .foregroundStyle(kiln.muted)
                .frame(width: 24)
            VStack(alignment: .leading, spacing: 2) {
                Text(entry.title)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(kiln.ink)
                if !entry.detail.isEmpty {
                    Text(entry.detail)
                        .font(.system(size: 13))
                        .foregroundStyle(kiln.muted)
                }
                Text(entry.time)
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.muted)
                    .monospacedDigit()
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
}
