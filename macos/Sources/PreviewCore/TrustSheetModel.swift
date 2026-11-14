import Foundation
import Observation

/// What the Trust sheet holds and asks: the host rules on the folder, and a
/// refusal keeps the sheet open. A folder inside a trusted folder is trusted only
/// by a click on the button that names the new address, under the warning.
@MainActor
@Observable
public final class TrustSheetModel {
    public private(set) var name: String
    public private(set) var path: String
    public private(set) var isMainFolder = false
    public private(set) var insideNames: [String] = []
    /// The trusted folder this one sits inside, once the host has said so for this path.
    public private(set) var insideTrusted: String?
    public private(set) var problem: String?
    public private(set) var problemIsRetryable = false
    public private(set) var checking = false
    public private(set) var trusting = false
    public var ready: Bool { (problem == nil || problemIsRetryable) && !checking && !trusting && !path.isEmpty }

    /// What the sheet says while the host's inside-trusted warning stands, nil otherwise.
    public var newAddressWarning: String? {
        insideTrusted.map { Strings.TrustSheet.insideTrusted(name, around: $0) }
    }

    /// The trust button: under the warning it names what the click accepts.
    public var trustTitle: String {
        insideTrusted == nil ? Strings.Request.trustNamed(name) : Strings.TrustSheet.trustNewAddress
    }

    /// The codes a click on the trust button accepts: only a warning this sheet is showing.
    public var acknowledged: [String] {
        insideTrusted == nil ? [] : [TrustRules.insideTrusted]
    }

    private let model: AppModel
    private var checkToken = 0

    public init(name: String, path: String, model: AppModel) {
        self.name = name
        self.path = path
        self.model = model
    }

    /// Asks the host about the folder: how it will be stored, and what the person should know first.
    public func check() async {
        checkToken += 1
        let token = checkToken
        checking = true
        let answer = await model.check(.trustFolder(name: name, path: path))
        guard token == checkToken else { return }
        checking = false
        switch answer {
        case let .success(check):
            if !check.path.isEmpty { path = check.path }
            isMainFolder = check.warnings.contains { $0.code == "personal-folder" }
            insideNames = check.warnings.first { $0.code == "contains-trusted" }?.names ?? []
            insideTrusted = check.warnings.first { $0.code == TrustRules.insideTrusted }.map { $0.names.first ?? String() }
            problem = nil
            problemIsRetryable = false
        case let .failure(refusal):
            problem = refusal.text
            problemIsRetryable = SheetRules.isRetryable(refusal)
            isMainFolder = false
            insideNames = []
            insideTrusted = nil
        }
    }

    /// Picks another folder (the main-folder alert's "Choose another"); false when the person cancels or there is none.
    public func chooseAnother() async -> Bool {
        guard let next = await model.folderChooser?.chooseToTrust(after: path, in: model.snapshot) else { return false }
        path = next
        name = (next as NSString).lastPathComponent
        isMainFolder = false
        insideNames = []
        insideTrusted = nil
        problem = nil
        problemIsRetryable = false
        await check()
        return true
    }

    /// True when the host trusted it (the sheet closes); false keeps the sheet and shows the refusal.
    /// A host that wants a warning accepted which this sheet had not shown gets no acceptance:
    /// the sheet asks again, shows the warning, and waits for the next click.
    public func trust() async -> Bool {
        trusting = true
        let accepted = acknowledged
        switch await model.submit(.trustFolder(name: name, path: path, acknowledged: accepted)) {
        case .done:
            return true
        case let .refused(refusal):
            trusting = false
            guard refusal.code != "duplicate", refusal.code != "queued" else { return false }
            if refusal.code == "needs-confirmation", accepted.isEmpty {
                await check()
                if insideTrusted != nil { return false }
            }
            problem = refusal.text
            problemIsRetryable = SheetRules.isRetryable(refusal)
            return false
        }
    }
}
