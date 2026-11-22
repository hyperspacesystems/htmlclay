import SwiftUI
import PreviewCore

enum FolderAlertKind: Equatable {
    case stopTrusting
    case stopTrustingSync
    case stopTrustingUnsaved([String])
    case stopServing
    case disconnect
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
                   isPresented: presentingStopTrusting) {
                Button(Strings.FolderPage.cancel, role: .cancel) {}
                Button(Strings.FolderPage.stopTrusting, role: .destructive) {
                    Task { await runStopTrusting(confirmed: true, force: false) }
                }
            } message: {
                Text(stopTrustingMessage)
            }
            .alert(Strings.FolderPage.unsavedStopTrustingTitle,
                   isPresented: presentingUnsaved) {
                Button(Strings.FolderPage.cancel, role: .cancel) {}
                Button(Strings.Folders.discardAndStopTrusting, role: .destructive) {
                    Task { await runStopTrusting(confirmed: true, force: true) }
                }
            } message: {
                Text(Strings.FolderPage.unsavedStopTrustingMessage(unsavedDocuments))
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

    private func runStopTrusting(confirmed: Bool, force: Bool) async {
        let outcome = await model.submit(.stopTrusting(folderID: folder.id, confirmed: confirmed, force: force))
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

    private var presentingStopTrusting: Binding<Bool> {
        Binding(
            get: { alert?.kind == .stopTrusting || alert?.kind == .stopTrustingSync },
            set: { shown in
                if !shown { alert = nil }
            }
        )
    }

    private var stopTrustingMessage: String {
        if alert?.kind == .stopTrustingSync {
            return Strings.FolderPage.stopTrustingSyncMessage
        }
        return Strings.FolderPage.stopTrustingMessage(folder.name)
    }

    private var presentingUnsaved: Binding<Bool> {
        Binding(
            get: {
                if case .stopTrustingUnsaved = alert?.kind { return true }
                return false
            },
            set: { shown in
                if !shown { alert = nil }
            }
        )
    }

    private var unsavedDocuments: [String] {
        if case let .stopTrustingUnsaved(docs) = alert?.kind { return docs }
        return []
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
