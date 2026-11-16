import Foundation

public enum HostSnapshotAdapter {
    /// Maps the host's snapshot onto the view model. now and calendar format times.
    public static func snapshot(from wire: WireSnapshot, now: Date = Date(), calendar: Calendar = .current) -> HostSnapshot {
        HostSnapshot(
            documents: wire.documents.map { document($0, now: now, calendar: calendar) },
            folders: wire.folders.compactMap { folder($0, now: now, calendar: calendar) },
            programs: wire.programs.map { program($0) },
            requests: wire.requests.compactMap { request($0) },
            waiting: wire.waiting.compactMap { waiting($0) },
            sync: sync(wire.sync, now: now, calendar: calendar),
            update: update(wire.update, now: now, calendar: calendar),
            preferences: preferences(wire.preferences),
            localMoveOffer: wire.move?.state == "offered",
            hostStopped: wire.host.stopped,
            rev: wire.rev,
            epoch: wire.epoch,
            host: host(wire.host),
            hostCounts: WaitCounts(
                total: wire.counts.total,
                byPane: wire.counts.byPane,
                dot: wire.counts.dot
            ),
            moveFolders: (wire.move?.folders ?? []).map {
                MoveFolderInfo(id: $0.id, name: $0.name, port: $0.port, address: $0.address)
            }
        )
    }

    private static func host(_ wire: WireHostState) -> HostInfo {
        HostInfo(
            version: wire.version,
            os: wire.os,
            port: wire.port,
            dashboardURL: wire.dashboardURL,
            service: wire.service,
            can: wire.can
        )
    }

    private static func document(_ wire: WireDocument, now: Date, calendar: Calendar) -> RecentDocument {
        RecentDocument(
            id: wire.id,
            name: wire.name,
            place: wire.place,
            fileMissing: wire.fileMissing,
            versionsKept: wire.versionsKept,
            openNow: wire.openNow,
            opened: formatTime(wire.openedAt, now: now, calendar: calendar),
            url: wire.url,
            folderID: wire.folderID,
            openedAt: wire.openedAt
        )
    }

    private static func folder(_ wire: WireFolder, now: Date, calendar: Calendar) -> Folder? {
        let origin: FolderOrigin
        switch wire.origin.kind {
        case "local": origin = .local
        case "account": origin = .account(username: wire.origin.username ?? "")
        case "team": origin = .team(name: wire.origin.team ?? "")
        default: return nil
        }
        var folder = Folder(
            id: wire.id,
            name: wire.name,
            path: wire.displayPath,
            origin: origin,
            parentID: wire.parentID,
            served: wire.served,
            allowsFraming: wire.allowsFraming ?? false,
            synced: wire.synced,
            address: wire.address,
            documentCount: wire.documentCount,
            status: wire.displayPath,
            onThisMac: wire.state != "not-on-this-computer",
            missingOnDisk: wire.state == "missing",
            versionsSize: wire.storageBytes > 0 ? ByteCountFormatter.string(fromByteCount: Int64(wire.storageBytes), countStyle: .file) : "",
            lastSync: formatTime(wire.sync.lastFinishedAt, now: now, calendar: calendar),
            becameViewer: wire.state == "viewer",
            fullPath: wire.path,
            url: wire.url,
            state: wire.state,
            can: FolderCan(
                serve: wire.can.serve,
                stopServing: wire.can.stopServing,
                setFraming: wire.can.setFraming ?? false,
                sync: wire.can.sync,
                stopSyncing: wire.can.stopSyncing,
                stopTrusting: wire.can.stopTrusting,
                merge: wire.can.merge,
                openWorkspace: wire.can.openWorkspace
            ),
            why: FolderWhy(
                serve: reason(wire.why.serve),
                sync: reason(wire.why.sync),
                stopTrusting: reason(wire.why.stopTrusting),
                merge: reason(wire.why.merge)
            ),
            storageBytes: wire.storageBytes
        )
        folder.download = wire.sync.download
        folder.syncState = wire.sync.state
        return folder
    }

    private static func reason(_ wire: WireReason?) -> HostReason? {
        guard let wire else { return nil }
        return HostReason(code: wire.code, text: wire.text)
    }

    private static func program(_ wire: WireProgram) -> Program {
        Program(
            id: wire.id,
            name: wire.name,
            path: wire.path,
            fileMissing: wire.fileMissing,
            allowedDocuments: wire.allowedDocuments,
            allTrustedFolders: wire.allTrustedFolders,
            removedDocuments: wire.removedDocuments
        )
    }

    private static func request(_ wire: WireRequest) -> ProgramRequest? {
        guard let kind = ProgramRequest.Kind(rawValue: wire.kind) else { return nil }
        return ProgramRequest(
            id: wire.id,
            kind: kind,
            document: wire.document,
            program: wire.program,
            folder: wire.folder,
            path: wire.path,
            detail: wire.detail,
            revision: wire.revision,
            answers: wire.answers,
            documentID: wire.documentID,
            attention: wire.attention
        )
    }

    private static func waiting(_ wire: WireWaitingItem) -> WaitingItem? {
        guard let kind = WaitingItem.Kind(rawValue: wire.kind) else { return nil }
        return WaitingItem(
            id: wire.id,
            kind: kind,
            title: wire.title,
            detail: wire.detail,
            subjectID: wire.subjectID,
            seen: wire.seen,
            hostPane: SettingsPane(rawValue: wire.pane),
            hostEarnsDot: wire.earnsDot
        )
    }

    private static func sync(_ wire: WireSyncState, now: Date, calendar: Calendar) -> SyncState {
        SyncState(
            connected: wire.connected,
            username: wire.username,
            keySuffix: wire.keySuffix,
            syncingFiles: wire.syncingFiles,
            paused: wire.paused,
            viewOnlyTeams: wire.viewOnlyTeams,
            lastFinished: formatTime(wire.lastFinishedAt, now: now, calendar: calendar),
            fullName: wire.fullName,
            offlineChanges: wire.offlineChanges,
            keyInFile: wire.keyStore == "file"
        )
    }

    private static func update(_ wire: WireUpdateState, now: Date, calendar: Calendar) -> UpdateState {
        UpdateState(
            current: wire.current,
            ready: wire.ready,
            checked: formatTime(wire.checkedAt, now: now, calendar: calendar),
            automatic: wire.automatic,
            finishing: wire.finishing
        )
    }

    private static func preferences(_ wire: WirePreferences) -> PreferenceState {
        PreferenceState(
            openAtLogin: wire.openAtLogin,
            aiEditing: wire.aiEditing,
            appearance: Appearance(rawValue: wire.appearance) ?? .system,
            yourName: wire.yourName,
            useName: wire.useName,
            openAtLoginChosen: wire.openAtLoginChosen
        )
    }

    private static func formatTime(_ date: Date?, now: Date, calendar: Calendar) -> String {
        guard let date else { return "" }
        if calendar.isDate(date, inSameDayAs: now) {
            return "Today at " + timeFormatter(calendar).string(from: date)
        }
        let yesterday = calendar.date(byAdding: .day, value: -1, to: now)
        if let yesterday, calendar.isDate(date, inSameDayAs: yesterday) {
            return "Yesterday at " + timeFormatter(calendar).string(from: date)
        }
        return dateFormatter(calendar).string(from: date)
    }

    private static func timeFormatter(_ calendar: Calendar) -> DateFormatter {
        let formatter = DateFormatter()
        formatter.calendar = calendar
        formatter.timeZone = calendar.timeZone
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateStyle = .none
        formatter.timeStyle = .short
        return formatter
    }

    private static func dateFormatter(_ calendar: Calendar) -> DateFormatter {
        let formatter = DateFormatter()
        formatter.calendar = calendar
        formatter.timeZone = calendar.timeZone
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateStyle = .medium
        formatter.timeStyle = .none
        return formatter
    }
}
