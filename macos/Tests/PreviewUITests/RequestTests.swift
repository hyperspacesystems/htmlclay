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

    private func aiEditRequest() -> (request: ProgramRequest, item: WaitingItem) {
        var sample = SampleData.request(.teamDocument)
        sample.request.program = ProgramControls.aiEditID
        sample.request.path = ""
        sample.request.folder = "Design"
        return sample
    }

    private func aiEditProgram(_ documents: [String]) -> Program {
        Program(id: ProgramControls.aiEditID, name: ProgramControls.aiEditID, path: "", allowedDocuments: documents)
    }

    func testAIEditTeamRequestSaysAIEditing() {
        let ai = aiEditRequest().request
        XCTAssertTrue(RequestBody.isAIEditing(ai))
        XCTAssertEqual(RequestBody.teamTitle(ai), "Let \(ai.document) use AI editing?")
        XCTAssertFalse(RequestBody.teamTitle(ai).contains(ProgramControls.aiEditID))
        XCTAssertEqual(RequestBody.teamLead(ai), Strings.Request.aiEditTeamLead("Design"))

        var ordinary = SampleData.request(.teamDocument).request
        ordinary.program = "todo-search"
        XCTAssertFalse(RequestBody.isAIEditing(ordinary))
        XCTAssertEqual(RequestBody.teamTitle(ordinary),
                       Strings.Request.runNamed(document: ordinary.document, program: ordinary.program))
        XCTAssertFalse(RequestBody.teamTitle(ordinary).contains("AI editing"))
        XCTAssertEqual(RequestBody.teamLead(ordinary), Strings.Request.fromTeam(ordinary.folder))
    }

    func testAIEditProgramRowAndPage() {
        _ = NSApplication.shared
        let ai = aiEditProgram(["roadmap.htmlclay"])
        let aiTeam = aiEditProgram(["roadmap.htmlclay", "budget.htmlclay", "notes.htmlclay"])
        XCTAssertEqual(ProgramRow.subtitle(ai), "Allowed for roadmap.htmlclay")
        XCTAssertEqual(ProgramRow.subtitle(aiTeam), "Allowed for 3 documents")
        XCTAssertEqual(ProgramControls.fileName(ai), "AI editing")
        XCTAssertFalse(ProgramPage.showsFileControls(ai))
        XCTAssertTrue(ProgramPage.showsFileControls(Program(id: "todo-search", name: "todo-search", path: "~/bin/todo-search")))

        let sample = aiEditRequest()
        let snapshot = HostSnapshot(requests: [sample.request], waiting: [sample.item])
        XCTAssertEqual(ProgramsPane.waitingTitle(sample.item, snapshot: snapshot),
                       Strings.Programs.aiEditWaiting(document: sample.request.document))

        let host = SampleHost(persona: .trustedFolder)
        let model = AppModel(host: host, initial: host.current)
        let hosting = NSHostingView(rootView: KilnRoot {
            RequestBody(request: sample.request, item: sample.item, position: 1, total: 1, actions: RequestActions())
        }.environment(model))
        hosting.layoutSubtreeIfNeeded()
        XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 460)
        XCTAssertGreaterThan(hosting.fittingSize.height, 120)
    }
}
