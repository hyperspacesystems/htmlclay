import XCTest
import AppKit
import SwiftUI
import PreviewCore
@testable import PreviewUI

@MainActor
final class FolderPageTests: XCTestCase {
    func testEveryPersonaRendersEveryFolderPage() {
        _ = NSApplication.shared
        for persona in Persona.allCases {
            let host = SampleHost(persona: persona)
            let model = AppModel(host: host, initial: host.current)
            XCTAssertTrue(model.isSample, "\(persona.rawValue) is not sample data")
            for folder in model.snapshot.folders {
                let hosting = NSHostingView(rootView: KilnRoot {
                    SettingsView(route: .folder(folder.id))
                }.environment(model))
                hosting.layoutSubtreeIfNeeded()
                XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 840,
                                            "\(persona.rawValue) \(folder.id)")
            }
        }
    }

    func testAServedSampleFolderKeepsItsControls() {
        _ = NSApplication.shared
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        guard let prototypes = model.snapshot.folder("prototypes") else { return XCTFail("missing prototypes") }
        XCTAssertTrue(model.isSample)
        XCTAssertTrue(prototypes.served)
        XCTAssertFalse(prototypes.address.isEmpty)

        XCTAssertEqual(Strings.Menu.showInFinder, "Show in Finder")
        XCTAssertEqual(Strings.FolderPage.copy, "Copy")
        XCTAssertTrue(model.can(FolderActionType.reveal))
        XCTAssertEqual(FolderControls.trusted(folder: prototypes), FolderToggle(value: true, enabled: true))
        XCTAssertEqual(FolderControls.served(folder: prototypes, pending: nil),
                       FolderToggle(value: true, enabled: true))

        let hosting = NSHostingView(rootView: KilnRoot {
            SettingsView(route: .folder("prototypes"))
        }.environment(model))
        hosting.layoutSubtreeIfNeeded()
        XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 840)
        XCTAssertGreaterThan(hosting.fittingSize.height, 200)
    }

    func testThePageShowsTheHostsReasonWhenItRefusesASwitch() {
        _ = NSApplication.shared
        var snapshot = SampleData.snapshot(for: .servedFolder)
        guard let index = snapshot.folders.firstIndex(where: { $0.id == "prototypes" }) else {
            return XCTFail("missing prototypes")
        }
        snapshot.folders[index].served = false
        snapshot.folders[index].address = ""
        snapshot.folders[index].can = FolderCan(serve: false)
        snapshot.folders[index].why = FolderWhy(serve: HostReason(code: "inside-served", text: "Inside a served folder"))
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: snapshot)
        model.route = .folder("prototypes")

        let hosting = NSHostingView(rootView: KilnRoot { SettingsView() }.environment(model))
        hosting.layoutSubtreeIfNeeded()
        XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 840)
        XCTAssertEqual(FolderControls.served(folder: model.snapshot.folders[index], pending: nil),
                       FolderToggle(value: false, enabled: false, note: "Inside a served folder"))
    }
}
