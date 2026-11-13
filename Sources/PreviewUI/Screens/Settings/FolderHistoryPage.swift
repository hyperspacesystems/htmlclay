import SwiftUI
import PreviewCore

struct FolderHistoryPage: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let folderID: String

    init(folderID: String) {
        self.folderID = folderID
    }

    static func symbol(_ kind: HistoryEntry.Kind) -> String {
        switch kind {
        case .uploaded: return Symbols.uploaded
        case .downloaded: return Symbols.downloaded
        case .renamed: return Symbols.renamed
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
    }

    private func page(_ folder: Folder) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 10) {
                KilnBackButton {
                    model.route = .folder(folderID)
                }
                Text(History.title(folder.name))
                    .font(.system(size: 18, weight: .bold))
                    .foregroundStyle(kiln.ink)
                Spacer(minLength: 0)
                Button(History.showSyncLog) {
                    model.say(Strings.Advanced.wouldShowSyncLog)
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
            .padding(.bottom, 4)
            KilnGroupTitle(History.today)
            KilnCard {
                ForEach(SampleData.history.indices, id: \.self) { index in
                    if index > 0 { KilnRowDivider() }
                    row(SampleData.history[index])
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
