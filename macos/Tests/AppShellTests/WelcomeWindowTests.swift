import XCTest
import AppKit
import PreviewCore
@testable import AppShell

@MainActor
final class WelcomeWindowTests: XCTestCase {
    private func manager(_ persona: Persona = .servedFolder) -> (WindowManager, SampleHost) {
        _ = NSApplication.shared
        let host = SampleHost(persona: persona)
        return (WindowManager(model: AppModel(host: host, initial: host.current)), host)
    }

    func testOpeningSettingsClosesTheWelcomeScreen() {
        let (windows, _) = manager()
        windows.showWelcome()
        let welcome = windows.welcome
        XCTAssertEqual(welcome?.isVisible, true)
        windows.showSettings(.general)
        XCTAssertEqual(welcome?.isVisible, false)
    }

    func testOpeningAQuestionClosesTheWelcomeScreen() throws {
        let persona = Persona.allCases.first { SampleHost(persona: $0).current.waiting.contains { $0.kind == .programRequest } }
        let (windows, host) = manager(try XCTUnwrap(persona))
        let item = try XCTUnwrap(host.current.waiting.first { $0.kind == .programRequest })
        windows.showWelcome()
        let welcome = windows.welcome
        XCTAssertEqual(welcome?.isVisible, true)
        windows.showRequest(item)
        XCTAssertEqual(welcome?.isVisible, false)
    }
}
