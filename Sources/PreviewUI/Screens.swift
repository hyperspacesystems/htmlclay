import SwiftUI
import PreviewCore

public struct RequestView: View {
    private let item: WaitingItem
    private let answer: (Bool) -> Void
    public init(item: WaitingItem, answer: @escaping (Bool) -> Void = { _ in }) { self.item = item; self.answer = answer }
    public var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(item.title).font(.system(size: 16, weight: .bold))
            HStack {
                Spacer()
                Button(Strings.Request.dontAllow) { answer(false) }
                Button(Strings.Request.allow) { answer(true) }
            }
        }
        .padding(22)
        .frame(width: 460)
    }
}

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
        for program in model.snapshot.programs {
            screens.append((settingsPrefix + "program." + program.id, AnyView(KilnRoot { SettingsView(route: .program(program.id)) }.environment(model))))
        }
        screens.append(contentsOf: [
            ("welcome", AnyView(KilnRoot { WelcomeView(film: film, menuBarIcon: nil) }.environment(model))),
            ("components", AnyView(KilnRoot { ComponentGallery() }.environment(model))),
        ])
        if let item = model.snapshot.waiting.first {
            screens.append(("request", AnyView(KilnRoot { RequestView(item: item) }.environment(model))))
        }
        return screens
    }
}
