import XCTest
import AppKit
import PreviewCore
@testable import AppShell

@MainActor
final class StatusItemMenuTests: XCTestCase {
    func testEveryEnabledRowCanBeClicked() {
        _ = NSApplication.shared
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        let controller = StatusItemController(model: model, windows: WindowManager(model: model))
        let input = MenuInput(syncConnected: true, canPause: true, waitingTotal: 0, waitingIDs: [],
                              documentIDs: [], recoveredChanges: 1)
        let rows = MenuModel.rows(input, phase: MenuUpdatePhase(ready: "9.9.9"))
            .filter { $0.id != MenuModel.separator }
        let menu = controller.buildOneMenu(rows: rows)
        menu.update()
        XCTAssertEqual(menu.items.count, rows.count)
        for (row, item) in zip(rows, menu.items) where row.enabled {
            XCTAssertTrue(item.isEnabled, "\(row.id) is greyed out")
        }
        XCTAssertTrue(rows.contains { $0.id == "quit" })
    }
}
