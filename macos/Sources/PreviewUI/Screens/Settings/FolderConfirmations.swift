import SwiftUI
import PreviewCore

enum FolderAlertKind: Equatable {
    case stopTrusting, stopServing, disconnect
}

struct FolderAlert: Equatable {
    let kind: FolderAlertKind
}

struct FolderConfirmations: ViewModifier {
    @Environment(AppModel.self) private var model
    private let folder: Folder
    @Binding private var alert: FolderAlert?

    init(folder: Folder, alert: Binding<FolderAlert?>) {
        self.folder = folder
        self._alert = alert
    }

    func body(content: Content) -> some View {
        content
            .alert(Strings.FolderPage.stopTrustingTitle(folder.name),
                   isPresented: presenting(.stopTrusting)) {
                Button(Strings.FolderPage.cancel, role: .cancel) {}
                Button(Strings.FolderPage.stopTrusting, role: .destructive) {
                    model.send(.stopTrusting(folderID: folder.id))
                }
            } message: {
                Text(Strings.FolderPage.stopTrustingMessage(folder.name))
            }
            .alert(Strings.FolderPage.stopServingTitle(folder.name),
                   isPresented: presenting(.stopServing)) {
                Button(Strings.FolderPage.cancel, role: .cancel) {}
                Button(Strings.FolderPage.stopServing) {
                    model.send(.setServed(folderID: folder.id, on: false))
                }
            } message: {
                Text(Strings.FolderPage.stopServingMessage(folder.name))
            }
            .alert(Strings.FolderPage.disconnectTitle(folder.name),
                   isPresented: presenting(.disconnect)) {
                Button(Strings.FolderPage.cancel, role: .cancel) {}
                Button(Strings.FolderPage.disconnect) {
                    model.send(.setSynced(folderID: folder.id, on: false))
                }
            } message: {
                Text(Strings.FolderPage.disconnectMessage(folder.name, team: isTeam))
            }
    }

    private func presenting(_ kind: FolderAlertKind) -> Binding<Bool> {
        Binding(
            get: { alert?.kind == kind },
            set: { shown in
                if !shown { alert = nil }
            }
        )
    }

    private var isTeam: Bool {
        if case .team = folder.origin { return true }
        return false
    }
}

extension View {
    func folderConfirmations(folder: Folder, alert: Binding<FolderAlert?>) -> some View {
        modifier(FolderConfirmations(folder: folder, alert: alert))
    }
}
