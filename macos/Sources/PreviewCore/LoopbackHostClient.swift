import Foundation

enum LoopbackText {
    // TODO-COPY
    static let unavailable = "This isn't available yet."
    // TODO-COPY
    static let interrupted = "HTML Clay didn't get that. Try again."
    // TODO-COPY
    static let unreachable = "HTML Clay isn't responding."
    // TODO-COPY
    static let hostError = "HTML Clay couldn't do that."
}

private struct LoopbackRefusalBody: Decodable {
    var code: String
    var retryable: Bool?
    var text: String?
    var field: String?
    var subjectID: String?
    var documents: [String]?
}

private struct LoopbackWarningBody: Decodable {
    var code: String
    var text: String
    var names: [String]?
}

private struct LoopbackPreviewBody: Decodable {
    var address: String?
    var name: String?
    var path: String?
    var warnings: [LoopbackWarningBody]?
}

private struct LoopbackReply: Decodable {
    var ok: Bool?
    var rev: Int?
    var snapshot: WireSnapshot?
    var preview: LoopbackPreviewBody?
    var error: LoopbackRefusalBody?
}

private enum LoopbackAnswer {
    case response(status: Int, body: Data)
    case transport

    var status: Int? {
        if case let .response(status, _) = self { return status }
        return nil
    }

    var data: Data? {
        if case let .response(_, body) = self { return body }
        return nil
    }
}

private final class LoopbackHostDelegate: NSObject, URLSessionDataDelegate, @unchecked Sendable {
    weak var client: LoopbackHostClient?

    func urlSession(
        _ session: URLSession,
        task: URLSessionTask,
        willPerformHTTPRedirection response: HTTPURLResponse,
        newRequest request: URLRequest,
        completionHandler: @escaping (URLRequest?) -> Void
    ) {
        completionHandler(nil)
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        let identifier = dataTask.taskIdentifier
        Task { @MainActor [weak self] in
            self?.client?.streamData(data, identifier: identifier)
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        let identifier = task.taskIdentifier
        Task { @MainActor [weak self] in
            self?.client?.streamFinished(identifier: identifier)
        }
    }
}

@MainActor
public final class LoopbackHostClient: HostClient, CustomStringConvertible, CustomDebugStringConvertible {
    private let port: Int
    private let secret: String
    private let makeID: () -> String
    private let lookupDelays: [Double]
    private let reconnectDelays: [Double]
    private let delegate: LoopbackHostDelegate
    let session: URLSession
    private var state = HostStreamState()
    private var listeners: [Int: @MainActor (HostSnapshot) -> Void] = [:]
    private var listenerCount = 0
    private var streamTask: URLSessionDataTask?
    private var streamIdentifier = 0
    private var streamGeneration = 0
    private var streamBuffer = HostLineBuffer()
    private var streamDelivered = false
    private var streamAttempt = 0
    private var streamStopped = false

    public private(set) var current: HostSnapshot?

    public var onNewEpoch: (@MainActor (_ old: String, _ new: String) -> Void)?
    public var onEnded: (@MainActor (_ reason: String) -> Void)?
    public var onUnsupported: (@MainActor (_ version: Int) -> Void)?

    public init(port: Int, secret: String,
                configuration: URLSessionConfiguration = .ephemeral,
                makeID: @escaping () -> String = { UUID().uuidString },
                lookupDelays: [Double] = [0.2, 0.5, 1, 2],
                reconnectDelays: [Double] = [0.25, 0.5, 1, 2, 4]) {
        self.port = port
        self.secret = secret
        self.makeID = makeID
        self.lookupDelays = lookupDelays
        self.reconnectDelays = reconnectDelays
        let settings = (configuration.copy() as? URLSessionConfiguration) ?? URLSessionConfiguration.ephemeral
        settings.httpCookieStorage = nil
        settings.httpShouldSetCookies = false
        settings.httpCookieAcceptPolicy = .never
        settings.urlCache = nil
        settings.requestCachePolicy = .reloadIgnoringLocalCacheData
        settings.connectionProxyDictionary = [:]
        settings.timeoutIntervalForRequest = 30
        let delegate = LoopbackHostDelegate()
        self.delegate = delegate
        self.session = URLSession(configuration: settings, delegate: delegate, delegateQueue: nil)
        delegate.client = self
    }

    public var description: String { "LoopbackHostClient(port: \(port))" }
    public var debugDescription: String { description }

    public func load() async throws -> HostSnapshot {
        let answer = await send(request("GET", "snapshot"))
        if answer.status == 200, let data = answer.data,
           let wire = try? HostWireCoding.decoder().decode(WireSnapshot.self, from: data) {
            accept(wire)
        }
        guard let current else {
            throw refusal(code: "unreachable", text: LoopbackText.unreachable, retryable: true)
        }
        return current
    }

    public func perform(_ action: HostAction) async -> HostOutcome {
        guard let wire = HostActionWire.wire(action) else {
            return .refused(refusal(code: "not-yet", text: LoopbackText.unavailable))
        }
        let id = makeID()
        guard let body = try? HostActionWire.body(id: id, wire) else {
            return .refused(hostError())
        }
        switch await send(request("POST", "actions", body: body)) {
        case let answer:
            switch answer.status {
            case 200:
                if let snapshot = try? snapshot(from: answer) {
                    accept(snapshot)
                }
                guard let current else { return .refused(hostError()) }
                return .done(current)
            case 409:
                guard let error = self.error(from: answer) else { return .refused(hostError()) }
                return .refused(refusal(error))
            case .some:
                return .refused(hostError())
            case .none:
                return await lookup(id)
            }
        }
    }

    public func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> {
        guard let wire = HostActionWire.wire(action) else {
            return .failure(refusal(code: "not-yet", text: LoopbackText.unavailable))
        }
        let id = makeID()
        guard let body = try? HostActionWire.body(id: id, wire) else {
            return .failure(hostError())
        }
        let answer = await send(request("POST", "check", body: body))
        switch answer.status {
        case 200:
            guard let reply = try? HostWireCoding.decoder().decode(LoopbackReply.self, from: answer.data ?? Data()),
                  let preview = reply.preview else {
                return .failure(hostError())
            }
            return .success(HostCheckAnswer(
                address: preview.address ?? "",
                name: preview.name ?? "",
                path: preview.path ?? "",
                warnings: (preview.warnings ?? []).map {
                    HostWarning(code: $0.code, text: $0.text, names: $0.names ?? [])
                }
            ))
        case 409:
            guard let error = self.error(from: answer) else { return .failure(hostError()) }
            return .failure(refusal(error))
        case .some:
            return .failure(hostError())
        case .none:
            return .failure(refusal(code: "unreachable", text: LoopbackText.unreachable, retryable: true))
        }
    }

    public func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        listenerCount += 1
        let key = listenerCount
        listeners[key] = onChange
        if listeners.count == 1 { openStream() }
        return HostSubscription { [weak self] in
            guard let self else { return }
            self.listeners[key] = nil
            if self.listeners.isEmpty { self.stopStream() }
        }
    }

    private func lookup(_ id: String) async -> HostOutcome {
        var stillRunning: HostRefusal?
        for delay in lookupDelays {
            try? await Task.sleep(nanoseconds: UInt64(max(delay, 0) * 1_000_000_000))
            let answer = await send(request("GET", "actions/\(id)"))
            switch answer.status {
            case 200:
                if let snapshot = try? snapshot(from: answer) {
                    accept(snapshot)
                }
                guard let current else { return .refused(hostError()) }
                return .done(current)
            case 409:
                guard let error = self.error(from: answer) else { return .refused(hostError()) }
                let answer = refusal(error)
                if answer.code == "still-running" {
                    stillRunning = answer
                    continue
                }
                return .refused(answer)
            case 404:
                _ = try? await load()
                return .refused(refusal(code: "interrupted", text: LoopbackText.interrupted, retryable: true))
            case .some:
                return .refused(hostError())
            case .none:
                continue
            }
        }
        if let stillRunning { return .refused(stillRunning) }
        return .refused(refusal(code: "unreachable", text: LoopbackText.unreachable, retryable: true))
    }

    private func request(_ method: String, _ path: String, body: Data? = nil) -> URLRequest {
        var request = URLRequest(url: URL(string: "http://127.0.0.1:\(port)/shell/v1/\(path)")!)
        request.httpMethod = method
        request.setValue("Bearer \(secret)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body {
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        return request
    }

    private func send(_ request: URLRequest) async -> LoopbackAnswer {
        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse else { return .transport }
            return .response(status: http.statusCode, body: data)
        } catch {
            return .transport
        }
    }

    private func snapshot(from answer: LoopbackAnswer) throws -> WireSnapshot {
        guard let data = answer.data else { throw hostError() }
        let reply = try HostWireCoding.decoder().decode(LoopbackReply.self, from: data)
        guard let snapshot = reply.snapshot else { throw hostError() }
        return snapshot
    }

    private func error(from answer: LoopbackAnswer) -> LoopbackRefusalBody? {
        guard let data = answer.data,
              let reply = try? HostWireCoding.decoder().decode(LoopbackReply.self, from: data) else {
            return nil
        }
        return reply.error
    }

    private func refusal(_ error: LoopbackRefusalBody) -> HostRefusal {
        HostRefusal(
            code: error.code,
            text: error.text ?? "",
            retryable: error.retryable ?? false,
            field: error.field,
            subjectID: error.subjectID,
            documents: error.documents ?? []
        )
    }

    private func refusal(code: String, text: String, retryable: Bool = false) -> HostRefusal {
        HostRefusal(code: code, text: text, retryable: retryable)
    }

    private func hostError() -> HostRefusal {
        refusal(code: "host-error", text: LoopbackText.hostError)
    }

    private func accept(_ wire: WireSnapshot) {
        apply(state.accept(wire))
    }

    private func apply(_ changes: [HostStreamChange]) {
        for change in changes {
            switch change {
            case let .show(wire):
                let snapshot = HostSnapshotAdapter.snapshot(from: wire)
                current = snapshot
                listeners.values.forEach { $0(snapshot) }
            case let .newEpoch(old, new):
                onNewEpoch?(old, new)
            case let .ended(reason):
                streamStopped = true
                stopStream()
                onEnded?(reason)
            case let .unsupported(version):
                streamStopped = true
                stopStream()
                onUnsupported?(version)
            }
        }
    }

    private func openStream() {
        guard !streamStopped, !listeners.isEmpty, streamTask == nil else { return }
        streamGeneration += 1
        streamBuffer = HostLineBuffer()
        streamDelivered = false
        let task = session.dataTask(with: request("GET", "events?since=\(state.since)"))
        streamIdentifier = task.taskIdentifier
        streamTask = task
        task.resume()
    }

    private func stopStream() {
        streamGeneration += 1
        streamTask?.cancel()
        streamTask = nil
        streamIdentifier = 0
        streamBuffer = HostLineBuffer()
    }

    fileprivate func streamData(_ data: Data, identifier: Int) {
        guard identifier == streamIdentifier, !streamStopped else { return }
        for line in streamBuffer.append(data) {
            receive(line)
        }
    }

    fileprivate func streamFinished(identifier: Int) {
        guard identifier == streamIdentifier else { return }
        streamTask = nil
        streamIdentifier = 0
        if streamDelivered { streamAttempt = 0 }
        guard !streamStopped, !listeners.isEmpty else { return }
        let delay = reconnectDelays.isEmpty ? 2 : reconnectDelays[min(streamAttempt, reconnectDelays.count - 1)]
        streamAttempt += 1
        let generation = streamGeneration
        Task { @MainActor [weak self] in
            try? await Task.sleep(nanoseconds: UInt64(max(delay, 0) * 1_000_000_000))
            guard let self, generation == self.streamGeneration, !self.streamStopped, !self.listeners.isEmpty else { return }
            self.openStream()
        }
    }

    private func receive(_ line: Data) {
        guard !streamStopped, let changes = try? state.receive(line: line) else { return }
        streamDelivered = true
        apply(changes)
    }
}
