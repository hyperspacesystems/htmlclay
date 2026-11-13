import SwiftUI
import PreviewCore

struct SettingsSheetView: View {
    @Environment(AppModel.self) private var model
    let sheet: PreviewSheet
    let actions: SettingsActions

    var body: some View {
        Group {
            switch sheet {
            case let .serveFolder(folderID): ServeSheet(folderID: folderID, snapshot: model.snapshot)
            case let .trustFolder(name, path): TrustSheet(name: name, path: path)
            case .connect: ConnectSheet()
            case let .accountFolder(username): AccountFolderSheet(username: username)
            case let .teamSetup(folderID): TeamSetupSheet(folderID: folderID)
            case .addProgram: AddProgramSheet()
            case .moveFromLocal, .movedFromLocal: EmptyView()
            }
        }
        .onExitCommand { model.sheet = nil }
    }
}
