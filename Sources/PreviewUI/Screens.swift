import SwiftUI
import PreviewCore

@MainActor
public enum ScreenCatalog {
    private static let settingsPrefix = "settings."
    public static func all(model: AppModel, film: String?) -> [(name: String, view: AnyView)] {
        var screens: [(name: String, view: AnyView)] = [
            ("panel", AnyView(KilnRoot { PanelView() }.environment(model))),
        ]
        for pane in SettingsPane.allCases {
            screens.append((settingsPrefix + pane.rawValue, AnyView(KilnRoot { SettingsView(pane: pane) }.environment(model))))
        }
        for id in ["notes", "prototypes", "panphora"] where model.snapshot.folder(id) != nil {
            screens.append((settingsPrefix + "folder." + id, AnyView(KilnRoot { SettingsView(route: .folder(id)) }.environment(model))))
        }
        for id in ["panphora", "design-team"] where model.snapshot.folder(id) != nil {
            screens.append((settingsPrefix + "history." + id, AnyView(KilnRoot { SettingsView(route: .folderHistory(id)) }.environment(model))))
        }
        for program in model.snapshot.programs {
            screens.append((settingsPrefix + "program." + program.id, AnyView(KilnRoot { SettingsView(route: .program(program.id)) }.environment(model))))
        }
        screens.append(contentsOf: [
            ("welcome", AnyView(KilnRoot { WelcomeView(film: film, menuBarIcon: nil) }.environment(model))),
            ("components", AnyView(KilnRoot { ComponentGallery() }.environment(model))),
        ])
        for kind in ProgramRequest.Kind.allCases {
            let s = SampleData.request(kind)
            screens.append(("request." + kind.rawValue, AnyView(KilnRoot {
                RequestBody(request: s.request, item: s.item, position: 1, total: 1, actions: RequestActions())
            }.environment(model))))
        }
        for sheet in [PreviewSheet.serveFolder(folderID: nil)] + SampleData.trustCandidates.map { PreviewSheet.trustFolder(name: $0.name, path: $0.path) } {
            screens.append(("sheet." + sheet.id, AnyView(KilnRoot { SettingsSheetView(sheet: sheet, actions: SettingsActions()) }.environment(model))))
        }
        if let first = model.snapshot.folders.first {
            let sheet = PreviewSheet.serveFolder(folderID: first.id)
            screens.append(("sheet." + sheet.id, AnyView(KilnRoot { SettingsSheetView(sheet: sheet, actions: SettingsActions()) }.environment(model))))
        }
        for sheet in [PreviewSheet.connect, PreviewSheet.accountFolder(username: SampleData.snapshot(for: .syncedWithTeams).sync.username), PreviewSheet.addProgram] {
            screens.append(("sheet." + sheet.id, AnyView(KilnRoot { SettingsSheetView(sheet: sheet, actions: SettingsActions()) }.environment(model))))
        }
        screens.append(("sheet.connect.refused", AnyView(KilnRoot { ConnectSheet(refused: true) }.environment(model))))
        if let team = model.snapshot.folders.first(where: { !$0.onThisMac }) {
            let sheet = PreviewSheet.teamSetup(folderID: team.id)
            screens.append(("sheet." + sheet.id, AnyView(KilnRoot { SettingsSheetView(sheet: sheet, actions: SettingsActions()) }.environment(model))))
        }
        for sheet in [PreviewSheet.moveFromLocal, PreviewSheet.movedFromLocal] {
            screens.append(("sheet." + sheet.id, AnyView(KilnRoot { SettingsSheetView(sheet: sheet, actions: SettingsActions()) }.environment(model))))
        }
        for note in SampleNotification.allCases {
            screens.append(("notification." + note.rawValue, AnyView(KilnRoot { NotificationCard(note: note) }.environment(model))))
        }
        let queueSample = SampleData.request(.found)
        screens.append(("request.queue", AnyView(KilnRoot {
            RequestBody(request: queueSample.request, item: queueSample.item, position: 1, total: 2, actions: RequestActions())
        }.environment(model))))
        return screens
    }
}
