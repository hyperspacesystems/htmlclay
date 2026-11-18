import SwiftUI
import PreviewCore

struct SettingsSheetView: View {
    @Environment(AppModel.self) private var model
    let sheet: PreviewSheet
    let actions: SettingsActions

    var body: some View {
        Group {
            switch sheet {
            case let .serveFolder(folderID): ServeSheet(folderID: folderID, model: model)
            case let .trustFolder(name, path): TrustSheet(name: name, path: path, model: model)
            case .connect: ConnectSheet()
            case let .accountFolder(username): AccountFolderSheet(username: username)
            case let .teamSetup(folderID): TeamSetupSheet(folderID: folderID)
            case .addProgram: AddProgramSheet(kind: .add, model: model)
            case .customCommand: CustomCommandSheet(model: model)
            case .changePort: ChangePortSheet(model: model)
            case let .relinkProgram(programID, path): AddProgramSheet(kind: .relink(programID: programID), path: path, model: model)
            case let .relinkFolder(folderID, path): FolderRelinkSheet(folderID: folderID, path: path, model: model)
            case let .merge(childID): MergeSheet(childID: childID, model: model)
            case .moveFromLocal: MoveSheet()
            case .movedFromLocal: MovedSheet(actions: actions)
            }
        }
        .onExitCommand { model.sheet = nil }
    }
}
