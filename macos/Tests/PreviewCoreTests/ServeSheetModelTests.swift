import XCTest
@testable import PreviewCore

/// A host that was interrupted: every answer is the retryable refusal a restart gives.
@MainActor
private final class InterruptedHost: HostClient {
    private(set) var performed: [HostAction] = []
    static let refusal = HostRefusal(code: "interrupted", text: "HTML Clay restarted before that finished. Try again.", retryable: true)

    func load() async throws -> HostSnapshot { HostSnapshot() }

    func perform(_ action: HostAction) async -> HostOutcome {
        performed.append(action)
        return .refused(Self.refusal)
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .failure(Self.refusal) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        HostSubscription {}
    }
}

@MainActor
private final class FixedFolderChooser: FolderChooser {
    private let picked: String?

    init(_ picked: String?) { self.picked = picked }

    func first(in snapshot: HostSnapshot) -> String? { picked }

    func choose(after current: String?, in snapshot: HostSnapshot) async -> String? { picked }

    func chooseToTrust(after current: String?, in snapshot: HostSnapshot) async -> String? { picked }

    func chooseFile() async -> String? { nil }
}

@MainActor
final class ServeSheetModelTests: XCTestCase {
    private func waitUntil(_ condition: @MainActor () -> Bool, _ message: String) async {
        let deadline = Date().addingTimeInterval(2)
        while !condition() && Date() < deadline {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(condition(), message)
    }

    func testServeFolderPicksTheSampleFolderAndAutoSlugsItsAddress() async throws {
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        model.folderChooser = SampleFolderChooser()

        FolderFlow.serveFolder(model: model)
        await waitUntil({ host.current.folders.contains { $0.path == SampleData.servePaths[1] && $0.served } },
                        "sample folder was not served")

        let folder = try XCTUnwrap(host.current.folders.first { $0.path == SampleData.servePaths[1] })
        XCTAssertTrue(folder.served)
        XCTAssertTrue(folder.address.contains("apps.htmlclay.localhost"))
    }

    func testServeFolderAutoSlugsAHomeFolderWithSpaces() async throws {
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: HostSnapshot())
        let target = "/Users/me/Documents/Default Project"
        model.folderChooser = FixedFolderChooser(target)

        FolderFlow.serveFolder(model: model)
        await waitUntil({ host.current.folders.contains { $0.path == target && $0.served } },
                        "chosen folder was not served")

        let folder = try XCTUnwrap(host.current.folders.first { $0.path == target })
        XCTAssertTrue(folder.served)
        XCTAssertTrue(folder.address.contains("default-project.htmlclay.localhost"))
    }

    func testCancellingTheFolderChooserSendsNothing() async {
        let host = InterruptedHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        model.folderChooser = FixedFolderChooser(nil)

        FolderFlow.serveFolder(model: model)
        try? await Task.sleep(nanoseconds: 50_000_000)

        XCTAssertEqual(host.performed, [])
        XCTAssertNil(model.note)
    }

    func testARefusalFromServeFolderAnnouncesTheRefusalText() async {
        let host = InterruptedHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        model.folderChooser = FixedFolderChooser("/Users/me/Documents/Client Notes (2024)")

        FolderFlow.serveFolder(model: model)
        await waitUntil({ model.note == InterruptedHost.refusal.text }, "refusal note was not shown")

        XCTAssertEqual(host.performed, [.serveFolder(path: "/Users/me/Documents/Client Notes (2024)", name: "")])
        XCTAssertEqual(model.lastRefusal?.code, "interrupted")
    }
}
