import SwiftUI
import PreviewCore

/// After a set-up sheet's folder is set up: the first download's progress bar
/// while it runs, then the sheet closes (08-sync.md 8.42), saying so when the
/// download could not finish.
struct FirstDownloadFollow: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    let folderID: String

    private var download: FirstDownload? { model.snapshot.folder(folderID)?.download }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let download {
                Text(Strings.TeamSetup.downloading(download))
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.muted)
                    .monospacedDigit()
                    .padding(.top, 14)
                KilnProgress(fraction: download.fraction)
                    .padding(.top, 6)
            }
        }
        .task {
            let start = Date()
            while !Task.isCancelled {
                switch SetUpSheetModel.follow(model.snapshot.folder(folderID), waited: Date().timeIntervalSince(start)) {
                case .running:
                    break
                case .finished:
                    model.sheet = nil
                    return
                case .failed:
                    model.sheet = nil
                    model.say(Strings.TeamSetup.firstDownloadFailed)
                    return
                }
                try? await Task.sleep(nanoseconds: 250_000_000)
            }
        }
    }
}
