import XCTest
@testable import PreviewCore

final class HostSnapshotAdapterTests: XCTestCase {
    private static let fixtures = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1")

    private static let snapshotFixtures = [
        "fiveDocuments",
        "localUser",
        "newPerson",
        "servedFolder",
        "syncedWithTeams",
        "trustedFolder",
    ]

    private func wire(_ name: String) throws -> WireSnapshot {
        let data = try Data(contentsOf: Self.fixtures.appendingPathComponent("\(name).snapshot.json"))
        return try HostWireCoding.decoder().decode(WireSnapshot.self, from: data)
    }

    private func decode(_ json: String) throws -> WireSnapshot {
        try HostWireCoding.decoder().decode(WireSnapshot.self, from: Data(json.utf8))
    }

    func testEveryFixtureMaps() throws {
        for name in Self.snapshotFixtures {
            let wire = try wire(name)
            let snapshot = HostSnapshotAdapter.snapshot(from: wire, now: Date(), calendar: .current)
            XCTAssertEqual(snapshot.folders.count, wire.folders.count, name)
            XCTAssertEqual(snapshot.programs.count, wire.programs.count, name)
            XCTAssertEqual(snapshot.documents.count, wire.documents.count, name)
            XCTAssertEqual(snapshot.totalCount, wire.counts.total, name)
            for pane in SettingsPane.allCases {
                XCTAssertEqual(snapshot.count(in: pane), wire.counts.byPane[pane.rawValue] ?? 0, "\(name) \(pane.rawValue)")
            }
            XCTAssertEqual(snapshot.rev, wire.rev, name)
            XCTAssertEqual(snapshot.epoch, wire.epoch, name)
            XCTAssertEqual(snapshot.host.can, wire.host.can, name)
        }
    }

    func testReadAccessReachesTheFoldersPane() throws {
        var wire = try wire("trustedFolder")
        XCTAssertEqual(HostSnapshotAdapter.snapshot(from: wire).readAccess, [])
        wire.readAccess = [WireReadAccess(id: "a1", folder: "/Users/x/Receipts", place: "~/Receipts", pages: ["/Users/x/a.htmlclay", "/Users/x/b.htmlclay"])]
        let access = try XCTUnwrap(HostSnapshotAdapter.snapshot(from: wire).readAccess.first)
        XCTAssertEqual(access, ReadAccess(id: "a1", place: "~/Receipts", pages: ["/Users/x/a.htmlclay", "/Users/x/b.htmlclay"]))
        XCTAssertEqual(Strings.Folders.readAccessPages(access.pages), "Readable by a.htmlclay and 1 more")
        XCTAssertEqual(Strings.Folders.readAccessPages(["/Users/x/a.htmlclay"]), "Readable by a.htmlclay")
        XCTAssertEqual(Strings.Folders.readAccessPages([]), "No HTML files open right now")
    }

    func testServedFolderFixture() throws {
        let snapshot = HostSnapshotAdapter.snapshot(from: try wire("servedFolder"))
        let served = try XCTUnwrap(snapshot.folders.first { $0.served })
        XCTAssertEqual(served.address, "prototypes.htmlclay.localhost:1212")
        XCTAssertEqual(served.can?.stopServing, true)
        XCTAssertTrue(served.fullPath.hasPrefix("/"))
        XCTAssertTrue(served.path.hasPrefix("~"))
    }

    func testWhyTextReachesTheFolder() throws {
        let wire = try wire("trustedFolder")
        let snapshot = HostSnapshotAdapter.snapshot(from: wire)
        let folder = try XCTUnwrap(snapshot.folder("notes"))
        let text = try XCTUnwrap(folder.why.sync?.text)
        XCTAssertFalse(text.isEmpty)
        XCTAssertEqual(text, wire.folders.first { $0.id == "notes" }?.why.sync?.text)
    }

    func testRequestKeepsRevisionAndAnswers() throws {
        let wire = try wire("trustedFolder")
        let snapshot = HostSnapshotAdapter.snapshot(from: wire)
        let request = try XCTUnwrap(snapshot.requests.first)
        let source = try XCTUnwrap(wire.requests.first)
        XCTAssertEqual(request.revision, source.revision)
        XCTAssertEqual(request.answers, source.answers)
        XCTAssertFalse(request.answers.isEmpty)
        XCTAssertEqual(request.documentID, source.documentID)
    }

    @MainActor func testHostCountsAndDotWin() throws {
        let json = """
        {
          "v": 1,
          "rev": 7,
          "epoch": "e1",
          "host": {"version":"2.0.3","os":"macOS","port":1212,"dashboardURL":"http://localhost:1212","service":"stopped","stopped":false,"can":{}},
          "documents": [],
          "folders": [
            {"id":"martian","name":"Martian","path":"/tmp/martian","displayPath":"~/martian","origin":{"kind":"martian"},"parentID":null,"trusted":true,"served":false,"synced":false,"address":"","url":"","state":"ok","documentCount":0,"storageBytes":0,"sync":{"state":"off","files":0,"lastFinishedAt":null,"uploadedToday":0,"downloadedToday":0},"can":{"serve":true,"stopServing":false,"sync":false,"stopSyncing":false,"stopTrusting":true,"merge":false,"openWorkspace":false},"why":{}},
            {"id":"notes","name":"Notes","path":"/tmp/notes","displayPath":"~/notes","origin":{"kind":"local"},"parentID":null,"trusted":true,"served":false,"synced":false,"address":"","url":"","state":"ok","documentCount":0,"storageBytes":0,"sync":{"state":"off","files":0,"lastFinishedAt":null,"uploadedToday":0,"downloadedToday":0},"can":{"serve":true,"stopServing":false,"sync":false,"stopSyncing":false,"stopTrusting":true,"merge":false,"openWorkspace":false},"why":{}}
          ],
          "programs": [],
          "requests": [
            {"id":"r1","kind":"martian","revision":1,"documentID":"d","document":"d.htmlclay","program":"p","folder":"","path":"","detail":"","answers":[]}
          ],
          "waiting": [
            {"id":"w1","kind":"martian","pane":"folders","earnsDot":true,"seen":false,"title":"t","detail":"","subjectID":null}
          ],
          "counts": {"total":2,"byPane":{"folders":2},"dot":true},
          "sync": {"connected":false,"username":"","keySuffix":"","paused":false,"syncingFiles":0,"offlineChanges":0,"viewOnlyTeams":0,"lastFinishedAt":null,"fullName":""},
          "update": {"current":"2.0.3","ready":null,"checkedAt":null,"automatic":true,"finishing":false},
          "preferences": {"openAtLogin":true,"openAtLoginChosen":true,"aiEditing":true,"appearance":"system","yourName":"","useName":false},
          "deniedDocuments": ["/Users/sam/Downloads/stranger.html"]
        }
        """
        let snapshot = HostSnapshotAdapter.snapshot(from: try decode(json))
        XCTAssertEqual(snapshot.deniedDocuments, ["/Users/sam/Downloads/stranger.html"])
        XCTAssertTrue(snapshot.waiting.isEmpty)
        XCTAssertEqual(snapshot.totalCount, 2)
        XCTAssertEqual(snapshot.count(in: .folders), 2)
        XCTAssertEqual(snapshot.count(in: .programs), 0)
        let model = AppModel(host: SampleHost(persona: .newPerson), initial: snapshot)
        XCTAssertTrue(model.showsDot)
    }

    func testUnknownKindsAreSkipped() throws {
        let json = """
        {
          "v": 1,
          "rev": 7,
          "epoch": "e1",
          "host": {"version":"2.0.3","os":"macOS","port":1212,"dashboardURL":"http://localhost:1212","service":"stopped","stopped":false,"can":{}},
          "documents": [],
          "folders": [
            {"id":"martian","name":"Martian","path":"/tmp/martian","displayPath":"~/martian","origin":{"kind":"martian"},"parentID":null,"trusted":true,"served":false,"synced":false,"address":"","url":"","state":"ok","documentCount":0,"storageBytes":0,"sync":{"state":"off","files":0,"lastFinishedAt":null,"uploadedToday":0,"downloadedToday":0},"can":{"serve":true,"stopServing":false,"sync":false,"stopSyncing":false,"stopTrusting":true,"merge":false,"openWorkspace":false},"why":{}},
            {"id":"notes","name":"Notes","path":"/tmp/notes","displayPath":"~/notes","origin":{"kind":"local"},"parentID":null,"trusted":true,"served":false,"synced":false,"address":"","url":"","state":"ok","documentCount":0,"storageBytes":0,"sync":{"state":"off","files":0,"lastFinishedAt":null,"uploadedToday":0,"downloadedToday":0},"can":{"serve":true,"stopServing":false,"sync":false,"stopSyncing":false,"stopTrusting":true,"merge":false,"openWorkspace":false},"why":{}}
          ],
          "programs": [],
          "requests": [
            {"id":"r1","kind":"martian","revision":1,"documentID":"d","document":"d.htmlclay","program":"p","folder":"","path":"","detail":"","answers":[]}
          ],
          "waiting": [
            {"id":"w1","kind":"martian","pane":"folders","earnsDot":true,"seen":false,"title":"t","detail":"","subjectID":null}
          ],
          "counts": {"total":0,"byPane":{},"dot":false},
          "sync": {"connected":false,"username":"","keySuffix":"","paused":false,"syncingFiles":0,"offlineChanges":0,"viewOnlyTeams":0,"lastFinishedAt":null,"fullName":""},
          "update": {"current":"2.0.3","ready":null,"checkedAt":null,"automatic":true,"finishing":false},
          "preferences": {"openAtLogin":true,"openAtLoginChosen":true,"aiEditing":true,"appearance":"system","yourName":"","useName":false}
        }
        """
        let snapshot = HostSnapshotAdapter.snapshot(from: try decode(json))
        XCTAssertEqual(snapshot.folders.map(\.id), ["notes"])
        XCTAssertTrue(snapshot.requests.isEmpty)
        XCTAssertTrue(snapshot.waiting.isEmpty)
    }

    func testTimesFormat() throws {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = try XCTUnwrap(TimeZone(identifier: "UTC"))
        let now = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 11, day: 13, hour: 15, minute: 30)))
        let today = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 11, day: 13, hour: 9, minute: 12)))
        let yesterday = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 11, day: 12, hour: 18, minute: 5)))
        let older = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 11, day: 4, hour: 12, minute: 0)))

        func checked(_ date: Date) -> String {
            HostSnapshotAdapter.snapshot(from: WireSnapshot(
                v: 1,
                rev: 0,
                epoch: "",
                host: WireHostState(version: "", os: "", port: 0, dashboardURL: "", service: "", stopped: false, can: [:]),
                documents: [],
                folders: [],
                programs: [],
                requests: [],
                waiting: [],
                counts: WireCounts(total: 0, byPane: [:], dot: false),
                sync: WireSyncState(connected: false, username: "", keySuffix: "", paused: false, syncingFiles: 0, offlineChanges: 0, viewOnlyTeams: 0, lastFinishedAt: nil, fullName: ""),
                update: WireUpdateState(current: "", ready: nil, checkedAt: date, automatic: false, finishing: false),
                preferences: WirePreferences(openAtLogin: true, openAtLoginChosen: true, aiEditing: true, appearance: "system", yourName: "", useName: false),
                move: nil
            ), now: now, calendar: calendar).update.checked
        }

        XCTAssertTrue(checked(today).hasPrefix("Today at "), checked(today))
        XCTAssertTrue(checked(yesterday).hasPrefix("Yesterday at "), checked(yesterday))
        XCTAssertFalse(checked(older).contains(" at "), checked(older))
    }

    func testModelEffortsMapFromWireAndResolvePerModel() throws {
        var wire = try wire("syncedWithTeams")
        wire.preferences.aiEditTools = [
            WireAIEditTool(
                id: "agy",
                custom: false,
                found: true,
                model: "",
                defaultModel: "gemini-4-carbon",
                models: ["gemini-4-carbon", "gemini-4-neon"],
                effort: "high",
                defaultEffort: "high",
                efforts: ["low", "medium", "high", "xhigh"],
                modelEfforts: [
                    "gemini-4-neon": ["low", "medium", "high", "xhigh"],
                    "gemini-4-carbon": ["low", "medium", "high"],
                ],
                command: ""
            ),
        ]
        let snapshot = HostSnapshotAdapter.snapshot(from: wire)
        let agy = try XCTUnwrap(snapshot.preferences.aiEditTools.first { $0.id == "agy" })
        XCTAssertEqual(agy.efforts(for: ""), ["low", "medium", "high"])
        XCTAssertEqual(agy.efforts(for: "gemini-4-carbon"), ["low", "medium", "high"])
        XCTAssertEqual(agy.efforts(for: "gemini-4-neon"), ["low", "medium", "high", "xhigh"])
    }

    func testSampleSnapshotsUnchanged() {
        let expected: [Persona: (total: Int, folders: Int, programs: Int)] = [
            .newPerson: (0, 0, 0),
            .fiveDocuments: (0, 0, 0),
            .trustedFolder: (1, 0, 1),
            .servedFolder: (1, 0, 1),
            .syncedWithTeams: (3, 2, 1),
            .localUser: (0, 0, 0),
        ]
        for persona in Persona.allCases {
            let snapshot = SampleData.snapshot(for: persona)
            XCTAssertNil(snapshot.hostCounts, persona.rawValue)
            XCTAssertEqual(snapshot.totalCount, snapshot.waiting.count, persona.rawValue)
            for pane in SettingsPane.allCases {
                XCTAssertEqual(snapshot.count(in: pane), snapshot.waiting.filter { $0.pane == pane }.count, "\(persona.rawValue) \(pane.rawValue)")
            }
            let want = expected[persona]!
            XCTAssertEqual(snapshot.totalCount, want.total, persona.rawValue)
            XCTAssertEqual(snapshot.count(in: .folders), want.folders, persona.rawValue)
            XCTAssertEqual(snapshot.count(in: .programs), want.programs, persona.rawValue)
            XCTAssertTrue(snapshot.hostCan("anything"), persona.rawValue)
        }
    }
}
