import XCTest
import AppKit
import SwiftUI
@testable import PreviewUI

private final class Recorder { var scheme: ColorScheme? }

private struct Probe: View {
    @Environment(\.colorScheme) private var scheme
    let recorder: Recorder
    var body: some View {
        recorder.scheme = scheme
        return Color.clear.frame(width: 60, height: 60)
    }
}

@MainActor
final class AppearanceTests: XCTestCase {
    func testWindowsFollowTheAppearanceSetting() {
        _ = NSApplication.shared
        for (name, expected) in [(NSAppearance.Name.darkAqua, ColorScheme.dark), (.aqua, .light)] {
            NSApp.appearance = NSAppearance(named: name)
            let recorder = Recorder()
            let hosting = NSHostingView(rootView: KilnRoot { Probe(recorder: recorder) })
            hosting.layoutSubtreeIfNeeded()
            _ = hosting.fittingSize
            XCTAssertEqual(recorder.scheme, expected)
        }
        NSApp.appearance = nil
    }
}
