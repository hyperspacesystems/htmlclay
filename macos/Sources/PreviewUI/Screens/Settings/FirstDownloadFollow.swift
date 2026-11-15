import SwiftUI
import PreviewCore

/// After a set-up sheet's folder is set up: the first download's progress bar
/// while it runs, then the sheet closes (08-sync.md 8.42).
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
            var saw = false
            while !Task.isCancelled {
                if download != nil { saw = true }
                if SetUpSheetModel.closes(download: download, sawDownload: saw, waited: Date().timeIntervalSince(start)) {
                    model.sheet = nil
                    return
                }
                try? await Task.sleep(nanoseconds: 250_000_000)
            }
        }
    }
}
