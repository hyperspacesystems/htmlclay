import Foundation

public enum WireValue: Equatable, Sendable, Encodable {
    case string(String), bool(Bool), int(Int), strings([String])

    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case let .string(value): try container.encode(value)
        case let .bool(value): try container.encode(value)
        case let .int(value): try container.encode(value)
        case let .strings(value): try container.encode(value)
        }
    }
}

public struct WireActionRequest: Equatable, Sendable {
    public let type: String
    public let args: [String: WireValue]
}

public enum HostActionWire {
    public static func wire(_ action: HostAction) -> WireActionRequest? {
        switch action {
        case let .setServed(folderID, on):
            return WireActionRequest(type: on ? "folder.serve" : "folder.stopServing", args: ["id": .string(folderID)])
        case let .serveFolder(path, name):
            return WireActionRequest(type: "folder.serve", args: ["path": .string(path), "name": .string(name)])
        case let .setSynced(folderID, on):
            return WireActionRequest(type: "folder.setSynced", args: ["id": .string(folderID), "on": .bool(on)])
        case let .setFraming(folderID, on):
            return WireActionRequest(type: "folder.setFraming", args: ["id": .string(folderID), "allow": .bool(on)])
        case let .stopTrusting(folderID):
            return WireActionRequest(type: "folder.stopTrusting", args: ["id": .string(folderID)])
        case let .mergeIntoParent(folderID):
            return WireActionRequest(type: "folder.merge", args: ["id": .string(folderID)])
        case let .trustFolder(_, path, acknowledged):
            return WireActionRequest(type: "folder.trust", args: ["path": .string(path), "acknowledged": .strings(acknowledged)])
        case let .setUpTeamFolder(folderID):
            return WireActionRequest(type: "folder.setUpTeam", args: ["id": .string(folderID)])
        case let .revealFolder(id):
            return WireActionRequest(type: "folder.reveal", args: ["id": .string(id)])
        case let .copyAddress(folderID):
            return WireActionRequest(type: "folder.copyAddress", args: ["id": .string(folderID)])
        case let .openDocument(id):
            return WireActionRequest(type: "document.open", args: ["id": .string(id)])
        case let .revealDocument(id):
            return WireActionRequest(type: "document.reveal", args: ["id": .string(id)])
        case let .forgetDocument(id):
            return WireActionRequest(type: "document.forget", args: ["id": .string(id)])
        case let .exportDocument(id):
            return WireActionRequest(type: "document.export", args: ["id": .string(id)])
        case let .recoverDocument(id, destination):
            return WireActionRequest(type: "document.recover", args: ["id": .string(id), "destination": .string(destination)])
        case let .openVersions(id):
            return WireActionRequest(type: "document.versions", args: ["id": .string(id)])
        case .openKeptPage:
            return WireActionRequest(type: "app.openKeptPage", args: [:])
        case let .openFile(path):
            return WireActionRequest(type: "file.open", args: ["path": .string(path)])
        case let .openDashboard(route):
            return WireActionRequest(type: "dashboard.open", args: route.isEmpty ? [:] : ["route": .string(route)])
        case let .showRequest(requestID):
            return WireActionRequest(type: "request.show", args: ["id": .string(requestID)])
        case let .answer(requestID, revision, answer, path):
            var args: [String: WireValue] = ["id": .string(requestID), "revision": .int(revision), "answer": .string(answer)]
            if !path.isEmpty { args["path"] = .string(path) }
            return WireActionRequest(type: "request.answer", args: args)
        case let .setSyncPaused(paused):
            return WireActionRequest(type: "sync.setPaused", args: ["paused": .bool(paused)])
        case let .connect(username, key):
            return WireActionRequest(type: "sync.connect", args: ["username": .string(username), "key": .string(key)])
        case .disconnect:
            return WireActionRequest(type: "sync.disconnect", args: [:])
        case let .setOpenAtLogin(on):
            return WireActionRequest(type: "prefs.set", args: ["key": .string("openAtLogin"), "value": .bool(on)])
        case let .setAIEditing(on):
            return WireActionRequest(type: "prefs.set", args: ["key": .string("aiEditing"), "value": .bool(on)])
        case let .setUseName(on):
            return WireActionRequest(type: "prefs.set", args: ["key": .string("useName"), "value": .bool(on)])
        case let .setAutoUpdate(on):
            return WireActionRequest(type: "prefs.set", args: ["key": .string("autoUpdate"), "value": .bool(on)])
        case let .setYourName(name):
            return WireActionRequest(type: "prefs.set", args: ["key": .string("yourName"), "value": .string(name)])
        case let .setAppearance(appearance):
            return WireActionRequest(type: "prefs.set", args: ["key": .string("appearance"), "value": .string(appearance.rawValue)])
        case let .removeProgramAccess(programID, document):
            return WireActionRequest(type: "program.removeAccess", args: ["programID": .string(programID), "documentID": .string(document)])
        case let .setProgramAllTrusted(programID, on):
            return WireActionRequest(type: "program.setAllTrusted", args: ["programID": .string(programID), "on": .bool(on)])
        case let .addProgram(name, path, acknowledged):
            return WireActionRequest(type: "program.add", args: ["path": .string(path), "name": .string(name), "acknowledged": .strings(acknowledged)])
        case let .relinkProgram(programID, path, acknowledged):
            return WireActionRequest(type: "program.relink", args: ["programID": .string(programID), "path": .string(path), "acknowledged": .strings(acknowledged)])
        case let .relinkFolder(folderID, path, acknowledged):
            return WireActionRequest(type: "folder.relink", args: ["id": .string(folderID), "path": .string(path), "acknowledged": .strings(acknowledged)])
        case let .askProgramAgain(programID, documentID):
            return WireActionRequest(type: "program.askAgain", args: ["programID": .string(programID), "documentID": .string(documentID)])
        case let .removeProgram(programID):
            return WireActionRequest(type: "program.remove", args: ["programID": .string(programID)])
        case let .revealProgram(programID):
            return WireActionRequest(type: "program.reveal", args: ["programID": .string(programID)])
        case .startUpdate:
            return WireActionRequest(type: "update.start", args: [:])
        // The app restarts the Go process itself, so this sends nothing.
        case .restartHost:
            return nil
        case .restartService:
            return WireActionRequest(type: "service.restart", args: [:])
        case let .moveFromLocal(folderIDs):
            return WireActionRequest(type: "move.start", args: ["folderIDs": .strings(folderIDs)])
        case let .markSeen(waitingIDs):
            return WireActionRequest(type: "waiting.markSeen", args: ["ids": .strings(waitingIDs)])
        case let .quit(force):
            return WireActionRequest(type: "host.quit", args: force ? ["force": .bool(true)] : [:])
        case .connectSample, .finishUpdate:
            return nil
        }
    }

    private struct Body: Encodable {
        let id: String
        let type: String
        let args: [String: WireValue]?
    }

    public static func body(id: String, _ action: WireActionRequest) throws -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = .sortedKeys
        return try encoder.encode(Body(id: id, type: action.type, args: action.args.isEmpty ? nil : action.args))
    }
}
