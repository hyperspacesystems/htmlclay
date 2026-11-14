import XCTest
import AppKit
import SwiftUI
import PreviewCore
@testable import PreviewUI

@MainActor
final class RequestTests: XCTestCase {
    func testCounterShowsOnlyWhenQueued() {
        XCTAssertNil(RequestQueue.counter(position: 1, total: 1))
        XCTAssertEqual(RequestQueue.counter(position: 1, total: 2), "1 of 2")
    }

    func testQueueAnswersNextRequest() {
        let host = SampleHost(persona: .trustedFolder)
        host.addSampleRequest(.riskyFile)
        XCTAssertEqual(RequestQueue.items(host.current).count, 2)
        let position = RequestQueue.position(of: "w-req-sample-riskyFile", in: host.current)
        XCTAssertEqual(position.position, 2)
        XCTAssertEqual(position.total, 2)
        XCTAssertEqual(RequestQueue.next(after: "w-req-todo", in: host.current)?.id, "w-req-sample-riskyFile")
        host.addSampleRequest(.riskyFile)
        XCTAssertEqual(RequestQueue.items(host.current).count, 2)
    }

    func testTrustIsHiddenForMainFolders() {
        var download = SampleData.request(.folderRead).request
        download.path = "~/Downloads"
        XCTAssertFalse(RequestControls.offers(RequestAnswer.trustFolder, in: download, isSample: true))
        XCTAssertTrue(RequestControls.offers(RequestAnswer.trustFolder, in: SampleData.request(.folderRead).request, isSample: true))
    }

    func testFileNameComesFromPath() {
        XCTAssertEqual(RequestBody.fileName(SampleData.request(.found).request), "search.sh")
        XCTAssertEqual(RequestBody.fileName(SampleData.request(.noFile).request), "todo-search")
    }

    func testEveryRequestKindLaysOut() {
        _ = NSApplication.shared
        let host = SampleHost(persona: .trustedFolder)
        let model = AppModel(host: host, initial: host.current)
        for kind in ProgramRequest.Kind.allCases {
            let sample = SampleData.request(kind)
            let hosting = NSHostingView(rootView: KilnRoot {
                RequestBody(request: sample.request, item: sample.item, position: 1, total: 1, actions: RequestActions())
            }.environment(model))
            hosting.layoutSubtreeIfNeeded()
            XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 460, kind.rawValue)
            XCTAssertGreaterThan(hosting.fittingSize.height, 120, kind.rawValue)
        }
    }
}
