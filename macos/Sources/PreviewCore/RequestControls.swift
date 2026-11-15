import Foundation

/// The answer strings the host knows, so no view writes them as text.
public enum RequestAnswer {
    public static let deny = "deny"
    public static let allowOnce = "allow-once"
    public static let trustFolder = "trust-folder"
    public static let allowDocument = "allow-document"
    public static let allowAllTrusted = "allow-all-trusted"
    public static let chooseFile = "choose-file"
    public static let allowAnyway = "allow-anyway"
}

/// One button the request window shows: the answer it sends and what it says.
public struct RequestChoice: Equatable, Sendable {
    public enum Look: Equatable, Sendable { case plain, large }
    public let answer: String
    public let title: String
    public let look: Look

    public init(answer: String, title: String, look: Look = .plain) {
        self.answer = answer
        self.title = title
        self.look = look
    }
}

/// The request window's decisions, away from the views: which answers a request
/// offers, what each button sends, and whether one click can grant every trusted
/// folder. A host snapshot carries the list; sample data has none, so its kind
/// decides.
public enum RequestControls {
    public static func answers(_ request: ProgramRequest, isSample: Bool) -> [String] {
        isSample ? sampleAnswers(request) : request.answers
    }

    public static func offers(_ answer: String, in request: ProgramRequest, isSample: Bool) -> Bool {
        answers(request, isSample: isSample).contains(answer)
    }

    /// The choice for one answer, or nil when the request does not offer it.
    public static func choice(_ answer: String, in request: ProgramRequest, isSample: Bool) -> RequestChoice? {
        guard offers(answer, in: request, isSample: isSample) else { return nil }
        return RequestChoice(answer: answer,
                             title: title(answer, request),
                             look: request.kind == .found ? .large : .plain)
    }

    /// The answers the More options radios offer, in the order they show.
    public static func options(_ request: ProgramRequest, isSample: Bool) -> [String] {
        [RequestAnswer.allowDocument, RequestAnswer.allowAllTrusted]
            .filter { offers($0, in: request, isSample: isSample) }
    }

    /// One click grants every trusted folder only when the request offers it: a
    /// team document asks for one document at a time.
    public static func showsAllTrusted(_ request: ProgramRequest, isSample: Bool) -> Bool {
        offers(RequestAnswer.allowAllTrusted, in: request, isSample: isSample)
    }

    /// What the More options radio for one answer says.
    public static func optionTitle(_ answer: String, in request: ProgramRequest) -> String {
        answer == RequestAnswer.allowAllTrusted
            ? Strings.ProgramPage.allTrustedTitle
            : Strings.Request.only(documentName(request))
    }

    /// The document that asks, shown by its file name: the host names it by its path.
    public static func documentName(_ request: ProgramRequest) -> String {
        ProgramControls.documentLabel(request.document)
    }

    public static func fileName(_ request: ProgramRequest) -> String {
        let name = (request.path as NSString).lastPathComponent
        return name.isEmpty ? request.program : name
    }

    private static func title(_ answer: String, _ request: ProgramRequest) -> String {
        switch answer {
        case RequestAnswer.deny: return Strings.Request.dontAllow
        case RequestAnswer.allowOnce: return Strings.Request.allowOnce
        case RequestAnswer.trustFolder:
            return request.kind == .folderReadAgain
                ? Strings.Request.trustNamed(request.folder)
                : Strings.Request.trustThisFolder
        case RequestAnswer.allowDocument:
            switch request.kind {
            case .found: return Strings.Request.allowFile(fileName(request))
            case .teamDocument: return Strings.Request.allowForThisDocument
            default: return Strings.Request.allow
            }
        case RequestAnswer.allowAllTrusted: return Strings.ProgramPage.allTrustedTitle
        case RequestAnswer.chooseFile:
            return request.kind == .noFile ? Strings.Request.chooseAFile : Strings.Request.chooseAnother
        case RequestAnswer.allowAnyway: return Strings.Request.allowAnyway
        default: return answer
        }
    }

    private static func sampleAnswers(_ request: ProgramRequest) -> [String] {
        switch request.kind {
        case .found:
            return [RequestAnswer.deny, RequestAnswer.allowDocument, RequestAnswer.allowAllTrusted, RequestAnswer.chooseFile]
        case .noFile:
            return [RequestAnswer.deny, RequestAnswer.chooseFile]
        case .teamDocument:
            return [RequestAnswer.deny, RequestAnswer.allowDocument, RequestAnswer.chooseFile]
        case .riskyFile:
            return [RequestAnswer.deny, RequestAnswer.chooseFile, RequestAnswer.allowAnyway]
        case .folderRead:
            return Strings.Request.mainFolders.contains(request.path)
                ? [RequestAnswer.deny, RequestAnswer.allowOnce]
                : [RequestAnswer.deny, RequestAnswer.allowOnce, RequestAnswer.trustFolder]
        case .folderReadAgain:
            return [RequestAnswer.deny, RequestAnswer.trustFolder]
        }
    }
}

/// What the window redraws on: the question and its revision.
public struct RequestIdentity: Hashable, Sendable {
    public let id: String
    public let revision: Int

    public init(_ request: ProgramRequest) {
        self.id = request.id
        self.revision = request.revision
    }
}

/// Where one click leaves the request window.
public enum RequestMove: Equatable, Sendable {
    case stay
    case next(WaitingItem?)
}

/// One click in the request window: what it sends, and whether the window moves on.
@MainActor
public enum RequestFlow {
    /// The action one click sends: the request's id and revision, and a path with
    /// choose-file only.
    public static func action(_ answer: String, path: String = String(), request: ProgramRequest) -> HostAction {
        .answer(requestID: request.id, revision: request.revision, answer: answer, path: path)
    }

    /// Sends the click and answers where the window goes: it stays on this
    /// question when the host refused, and moves on only once the host accepted.
    public static func answer(_ answer: String, path: String = String(), request: ProgramRequest,
                              item: WaitingItem, model: AppModel) async -> RequestMove {
        guard case .done = await model.submit(action(answer, path: path, request: request)) else { return .stay }
        return .next(RequestQueue.next(after: item.id, in: model.snapshot))
    }
}
