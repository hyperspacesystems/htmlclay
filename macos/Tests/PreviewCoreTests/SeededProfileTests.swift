import XCTest
@testable import PreviewCore

@MainActor
final class SeededProfileTests: XCTestCase {
    override func tearDown() async throws {
        await LiveHost.stopAll()
    }

    func testNewPersonLeavesTheHomeEmpty() async throws {
        let host = try await LiveHost.start()
        try await ProfileSeeder.seed(.newPerson, home: host.home, host: host.client)

        let snapshot = try await host.client.load()
        XCTAssertEqual(snapshot.folders, [], "a new person has no folders")
        XCTAssertEqual(snapshot.documents, [], "a new person has no documents")
        XCTAssertEqual(snapshot.preferences.yourName, "", "a new person has no name yet")
    }

    func testTrustedFolderSeedsOneTrustedFolder() async throws {
        let host = try await LiveHost.start()
        try await ProfileSeeder.seed(.trustedFolder, home: host.home, host: host.client)
        let created = try documentsOnDisk(for: .trustedFolder, home: host.home)

        let snapshot = try await host.client.load()
        XCTAssertEqual(snapshot.folders.count, 1, "trusting one folder left the host with \(snapshot.folders.map(\.name))")
        let folder = try XCTUnwrap(snapshot.folders.first)
        XCTAssertEqual(folder.name, "Notes", "the folder kept the sample's name")
        XCTAssertEqual(folder.fullPath, host.home.appendingPathComponent("Documents/Notes").path)
        XCTAssertEqual(folder.origin, .local, "a trusted folder made here belongs to nobody else")
        XCTAssertFalse(folder.served, "trusting a folder did not serve it")
        XCTAssertEqual(folder.can?.stopTrusting, true, "the folder is trusted")
        XCTAssertEqual(folder.documentCount, created, "the folder holds the \(created) documents the seed wrote")
    }

    func testServedFolderSeedsOneServedFolder() async throws {
        let host = try await LiveHost.start()
        try await ProfileSeeder.seed(.servedFolder, home: host.home, host: host.client)
        let created = try documentsOnDisk(for: .servedFolder, home: host.home)

        let snapshot = try await host.client.load()
        XCTAssertEqual(snapshot.folders.count, 1, "serving one folder left the host with \(snapshot.folders.map(\.name))")
        let folder = try XCTUnwrap(snapshot.folders.first)
        XCTAssertTrue(folder.served, "the seed's second action served the folder")
        XCTAssertTrue(folder.address.contains("prototypes.htmlclay.localhost"),
                      "the folder is served at \(folder.address)")
        XCTAssertEqual(folder.documentCount, created, "the folder holds the \(created) documents the seed wrote")
    }

    func testASampleOnlyPersonaNamesItsMilestone() async throws {
        let host = try await LiveHost.start()
        for persona in Persona.sampleOnly {
            let milestone = try XCTUnwrap(persona.milestone, "\(persona) is sample only but names no milestone")
            do {
                try await ProfileSeeder.seed(persona, home: host.home, host: host.client)
                XCTFail("seeding \(persona) was allowed")
            } catch {
                XCTAssertTrue("\(error)".contains("milestone \(milestone)"), "seeding \(persona) failed with: \(error)")
                XCTAssertTrue("\(error)".contains(persona.rawValue), "seeding \(persona) failed with: \(error)")
            }
        }
        let snapshot = try await host.client.load()
        XCTAssertEqual(snapshot.folders, [], "a sample-only persona touched the home")
    }

    /// The documents the seed wrote for the persona's folder, counted on disk the way the host counts them.
    private func documentsOnDisk(for persona: Persona, home: URL) throws -> Int {
        let sample = SampleData.snapshot(for: persona)
        let folder = try XCTUnwrap(sample.folders.first { persona == .servedFolder ? $0.served : ($0.parentID == nil && !$0.served) },
                                   "\(persona) has no folder in its sample snapshot")
        let place = try XCTUnwrap(folder.path.range(of: "~/"), "\(folder.path) is not under ~")
        let directory = home.appendingPathComponent(String(folder.path[place.upperBound...]), isDirectory: true)
        let names = try FileManager.default.contentsOfDirectory(atPath: directory.path)
        return names.filter { ["html", "htm", "htmlclay"].contains(URL(fileURLWithPath: $0).pathExtension.lowercased()) }.count
    }
}
