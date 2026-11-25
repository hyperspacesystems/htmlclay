import XCTest
import AppKit
import SwiftUI
import PreviewCore
@testable import PreviewUI

@MainActor
final class RenderSmokeTests: XCTestCase {
    func testEveryScreenLaysOutForEveryPersona() {
        _ = NSApplication.shared
        var count = 0
        for persona in Persona.allCases {
            let host = SampleHost(persona: persona)
            let model = AppModel(host: host, initial: host.current)
            for screen in ScreenCatalog.all(model: model, film: nil) {
                let hosting = NSHostingView(rootView: screen.view)
                hosting.layoutSubtreeIfNeeded()
                XCTAssertGreaterThan(hosting.fittingSize.width, 50, "\(screen.name) \(persona.rawValue)")
                XCTAssertGreaterThan(hosting.fittingSize.height, 50, "\(screen.name) \(persona.rawValue)")
                count += 1
            }
        }
        XCTAssertGreaterThanOrEqual(count, Persona.allCases.count * 3)
    }
}
