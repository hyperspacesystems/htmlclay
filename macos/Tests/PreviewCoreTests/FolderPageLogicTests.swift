import XCTest
@testable import PreviewCore

final class FolderPageLogicTests: XCTestCase {
    private func folder(can: FolderCan? = nil, why: FolderWhy = FolderWhy(),
                        served: Bool = false, synced: Bool = false,
                        address: String = "", onThisMac: Bool = true) -> Folder {
        Folder(id: "f", name: "f", path: "/tmp/f", served: served, synced: synced,
               address: address, onThisMac: onThisMac, can: can, why: why)
    }

    func testSampleFoldersLeaveEverySwitchEnabled() {
        let sample = folder()
        XCTAssertEqual(FolderControls.trusted(folder: sample),
                       FolderToggle(value: true, enabled: true))
        XCTAssertEqual(FolderControls.served(folder: sample, pending: nil),
                       FolderToggle(value: false, enabled: true))
        XCTAssertEqual(FolderControls.synced(folder: sample, pending: nil, hostCan: true),
                       FolderToggle(value: false, enabled: true))
        XCTAssertTrue(FolderControls.showsMerge(child: sample, hostCan: true))
    }

    func testARefusedServeDisablesTheSwitchAndSaysWhy() {
        let refused = folder(can: FolderCan(serve: false),
                             why: FolderWhy(serve: HostReason(code: "inside-served", text: "X")))
        XCTAssertEqual(FolderControls.served(folder: refused, pending: nil),
                       FolderToggle(value: false, enabled: false, note: "X"))
    }

    func testAPendingServeShowsTheValueBeingSent() {
        let stopped = folder(can: FolderCan(serve: true), served: false)
        XCTAssertEqual(FolderControls.served(folder: stopped,
                                             pending: .setServed(folderID: "f", on: true)),
                       FolderToggle(value: true, enabled: true))
    }

    func testAStoppedServedFolderAsksStopServingToMove() {
        let served = folder(can: FolderCan(serve: true, stopServing: false),
                            why: FolderWhy(serve: HostReason(code: "busy", text: "Still writing")),
                            served: true, address: "f.htmlclay.localhost:1212")
        XCTAssertEqual(FolderControls.served(folder: served, pending: nil),
                       FolderToggle(value: true, enabled: false, note: "Still writing"))
    }

    func testATrustedFolderSaysWhyStopTrustingIsRefused() {
        let trusted = folder(can: FolderCan(stopTrusting: false),
                             why: FolderWhy(stopTrusting: HostReason(code: "busy", text: "Reading it")))
        XCTAssertEqual(FolderControls.trusted(folder: trusted),
                       FolderToggle(value: true, enabled: false, note: "Reading it"))
    }

    func testAnEmptyReasonLeavesTheNoteEmpty() {
        let refused = folder(can: FolderCan(serve: false),
                             why: FolderWhy(serve: HostReason(code: "inside-served")))
        XCTAssertEqual(FolderControls.served(folder: refused, pending: nil),
                       FolderToggle(value: false, enabled: false, note: nil))
    }

    func testASyncSwitchNeedsBothTheFolderAndTheHost() {
        let synced = folder(can: FolderCan(sync: true, stopSyncing: true), synced: true)
        XCTAssertEqual(FolderControls.synced(folder: synced,
                                             pending: .setSynced(folderID: "f", on: false),
                                             hostCan: false),
                       FolderToggle(value: false, enabled: false))

        let refused = folder(can: FolderCan(sync: false),
                             why: FolderWhy(sync: HostReason(code: "not-connected", text: "Connect first")))
        XCTAssertEqual(FolderControls.synced(folder: refused, pending: nil, hostCan: true),
                       FolderToggle(value: false, enabled: false, note: "Connect first"))
    }

    func testMergeNeedsBothTheChildAndTheHost() {
        XCTAssertFalse(FolderControls.showsMerge(child: folder(can: FolderCan(merge: false)), hostCan: true))
        XCTAssertFalse(FolderControls.showsMerge(child: folder(can: FolderCan(merge: true)), hostCan: false))
        XCTAssertTrue(FolderControls.showsMerge(child: folder(can: FolderCan(merge: true)), hostCan: true))
    }
}
