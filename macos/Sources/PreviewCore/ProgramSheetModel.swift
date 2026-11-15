import Foundation
import Observation

/// What the add and relink sheets hold and ask: the host checks the program
/// file, the sheet shows what it found, and the person's click acknowledges
/// those codes. A refusal keeps the sheet open with its input.
@MainActor
@Observable
public final class ProgramSheetModel {
    public enum Kind: Equatable, Sendable {
        case add
        case relink(programID: String)
    }

    public let kind: Kind
    public private(set) var path: String
    public private(set) var name: String
    public private(set) var warnings: [HostWarning] = []
    public private(set) var problem: String?
    public private(set) var problemIsRetryable = false
    public private(set) var checking = false
    public private(set) var saving = false

    public var isAdding: Bool {
        if case .add = kind { return true }
        return false
    }

    public var warningLines: [String] { ProgramControls.warningLines(warnings) }

    /// The codes the host showed, which are the codes the click acknowledges.
    public var acknowledged: [String] { ProgramControls.acknowledged(warnings) }

    public var nameProblem: ProgramNameProblem? {
        isAdding ? ProgramRules.nameProblem(name, in: model.snapshot) : nil
    }

    public var ready: Bool {
        !path.isEmpty && (problem == nil || problemIsRetryable) && nameProblem == nil && !checking && !saving
    }

    private let model: AppModel
    private var checkToken = 0

    public init(kind: Kind, model: AppModel, path picked: String = String()) {
        self.kind = kind
        self.model = model
        let start: String
        let label: String
        switch kind {
        case .add:
            start = picked.isEmpty && model.isSample ? SampleData.programFiles[0] : picked
            label = ProgramRules.defaultName(for: start)
        case let .relink(programID):
            let program = model.snapshot.program(programID)
            let current = program?.path ?? String()
            start = picked.isEmpty ? current : picked
            label = program?.name ?? ProgramRules.defaultName(for: start)
        }
        path = start
        name = label
    }

    /// Asks the host about the file: the codes a person has to acknowledge, or why it cannot be used.
    public func check() async {
        checkToken += 1
        let token = checkToken
        guard !path.isEmpty else {
            warnings = []
            problem = nil
            problemIsRetryable = false
            checking = false
            return
        }
        checking = true
        let answer = await model.check(action)
        guard token == checkToken else { return }
        checking = false
        switch answer {
        case let .success(check):
            warnings = check.warnings
            problem = nil
            problemIsRetryable = false
        case let .failure(refusal):
            warnings = []
            problem = refusal.text
            problemIsRetryable = SheetRules.isRetryable(refusal)
        }
    }

    /// Picks another file: the native panel in the real app, the sample list in the preview.
    public func chooseAnother() async -> Bool {
        let picked = model.isSample
            ? ProgramSheetModel.nextSampleFile(after: path)
            : await model.folderChooser?.chooseProgram()
        guard let picked, !picked.isEmpty else { return false }
        path = picked
        if isAdding { name = ProgramRules.defaultName(for: picked) }
        warnings = []
        problem = nil
        problemIsRetryable = false
        await check()
        return true
    }

    /// What the person types in the name field.
    public func setName(_ next: String) {
        name = next
    }

    /// True when the host took it (the sheet closes); false keeps the file and shows the refusal.
    public func confirm() async -> Bool {
        guard ready else { return false }
        saving = true
        switch await model.submit(action) {
        case .done:
            return true
        case let .refused(refusal):
            saving = false
            guard refusal.code != "duplicate", refusal.code != "queued" else { return false }
            problem = refusal.text
            problemIsRetryable = SheetRules.isRetryable(refusal)
            return false
        }
    }

    private var action: HostAction {
        switch kind {
        case .add:
            return .addProgram(name: name, path: path, acknowledged: acknowledged)
        case let .relink(programID):
            return .relinkProgram(programID: programID, path: path, acknowledged: acknowledged)
        }
    }

    static func nextSampleFile(after path: String) -> String {
        let files = SampleData.programFiles
        let index = files.firstIndex(of: path) ?? -1
        return files[(index + 1) % files.count]
    }
}

/// The programs pane and page clicks a person makes without a sheet.
@MainActor
public enum ProgramFlow {
    /// Find it… and Choose another file…: the native panel picks the file in the
    /// real app, the sample list in the preview, then the relink sheet checks it
    /// and shows what the person has to acknowledge.
    public static func chooseFile(_ program: Program, model: AppModel) {
        Task {
            let picked = model.isSample
                ? ProgramSheetModel.nextSampleFile(after: program.path)
                : await model.folderChooser?.chooseProgram()
            guard let picked, !picked.isEmpty else { return }
            model.sheet = .relinkProgram(programID: program.id, path: picked)
        }
    }

    /// Allow again…: the document asks for the program again.
    public static func allowAgain(_ document: String, program: Program, model: AppModel) async -> Bool {
        guard case .done = await model.submit(.askProgramAgain(programID: program.id, documentID: document)) else { return false }
        return true
    }

    /// Remove all access: every document loses it, and the broad grant goes with them.
    public static func removeAllAccess(_ program: Program, model: AppModel) async -> Bool {
        if program.allTrustedFolders {
            guard case .done = await model.submit(.setProgramAllTrusted(programID: program.id, on: false)) else { return false }
        }
        guard case .done = await model.submit(.removeProgramAccess(programID: program.id, document: String())) else { return false }
        return true
    }
}
