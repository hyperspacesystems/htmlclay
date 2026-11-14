import XCTest
import AppKit
import SwiftUI
import PreviewCore
@testable import PreviewUI

@MainActor
final class TrustInsideTrustedSheetTests: XCTestCase {
    private static let path = "~/Documents/Notes/Drafts"

    /// The Trust sheet for the same folder, checked against a host where the folder around it is trusted or is not.
    private func checkedSheet(_ persona: Persona) async -> (sheet: TrustSheetModel, size: CGSize) {
        let host = SampleHost(persona: persona)
        let model = AppModel(host: host, initial: host.current)
        let sheet = TrustSheetModel(name: "Drafts", path: Self.path, model: model)
        await sheet.check()
        let hosting = NSHostingView(rootView: KilnRoot { TrustSheet(sheet: sheet) }.environment(model))
        hosting.layoutSubtreeIfNeeded()
        return (sheet, hosting.fittingSize)
    }

    func testTheWarningTakesRoomInTheSheetAndTheButtonChanges() async {
        _ = NSApplication.shared
        let plain = await checkedSheet(.newPerson)
        let inside = await checkedSheet(.trustedFolder)

        XCTAssertNil(plain.sheet.newAddressWarning)
        XCTAssertEqual(plain.sheet.trustTitle, Strings.Request.trustNamed("Drafts"))
        XCTAssertEqual(inside.sheet.newAddressWarning, Strings.TrustSheet.insideTrusted("Drafts", around: "Notes"))
        XCTAssertEqual(inside.sheet.trustTitle, Strings.TrustSheet.trustNewAddress)
        XCTAssertEqual(inside.size.width, plain.size.width)
        XCTAssertGreaterThan(inside.size.height, plain.size.height + 30)
    }

    func testEveryNewSentenceFitsItsLimit() {
        XCTAssertLessThanOrEqual(Strings.TrustSheet.trustNewAddress.count, 22)
        XCTAssertLessThanOrEqual(Strings.TrustSheet.insideTrusted("Drafts", around: "Notes").count, 150)
    }
}
