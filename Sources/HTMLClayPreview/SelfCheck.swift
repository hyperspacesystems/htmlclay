import AppKit
import SwiftUI
import PreviewCore
import PreviewUI

@MainActor
enum SelfCheck {
    static func run() -> Int32 {
        var failures: [String] = []
        var views = 0
        for name in ["icon-film.html", "tray-icon@2x.png", "tray-icon-alert@2x.png", "AppIcon.icns"] {
            let parts = name.split(separator: ".").map(String.init)
            if Bundle.main.url(forResource: parts[0], withExtension: parts[1]) == nil { failures.append("missing resource \(name)") }
        }
        for appearance in [NSAppearance.Name.aqua, .darkAqua] {
            NSApp.appearance = NSAppearance(named: appearance)
            for persona in Persona.allCases {
                let host = SampleHost(persona: persona)
                let model = AppModel(host: host, initial: host.current)
                for screen in ScreenCatalog.all(model: model, film: nil) {
                    let hosting = NSHostingView(rootView: screen.view)
                    hosting.layoutSubtreeIfNeeded()
                    let size = hosting.fittingSize
                    views += 1
                    if size.width < 50 || size.height < 50 { failures.append("\(screen.name) for \(persona.rawValue) laid out at \(size)") }
                }
            }
        }
        failures.forEach { print("SELF-CHECK FAIL: \($0)") }
        print(failures.isEmpty ? "SELF-CHECK OK views=\(views) personas=\(Persona.allCases.count)" : "SELF-CHECK FAILED failures=\(failures.count)")
        return failures.isEmpty ? 0 : 1
    }
}
