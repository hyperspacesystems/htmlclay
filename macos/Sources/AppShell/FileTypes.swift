import AppKit
import UniformTypeIdentifiers
import PreviewCore

/// HTML Clay's file type on this Mac: who opens .htmlclay files, and the switch
/// to make HTML Clay that app again or hand the files to the default browser.
@MainActor
public enum FileTypes {
    /// A shell check bundle names its own throwaway type in HTMLClayDocumentType,
    /// so Make default can be tried without touching who opens .htmlclay files.
    public static let htmlclay = UTType(
        exportedAs: Bundle.main.object(forInfoDictionaryKey: "HTMLClayDocumentType") as? String ?? "com.htmlclay.document",
        conformingTo: .html)

    public enum Owner: Equatable {
        case htmlclay
        case other(name: String?)
    }

    public static func owner() -> Owner {
        guard let url = NSWorkspace.shared.urlForApplication(toOpen: htmlclay) else { return .other(name: nil) }
        if url.standardizedFileURL == Bundle.main.bundleURL.standardizedFileURL { return .htmlclay }
        if let bundleID = Bundle(url: url)?.bundleIdentifier,
           bundleID == AppIdentity.productBundleID || bundleID == AppIdentity.candidateBundleID {
            return .htmlclay
        }
        return .other(name: FileManager.default.displayName(atPath: url.path).replacingOccurrences(of: ".app", with: ""))
    }

    public static func makeDefault() async throws {
        try await NSWorkspace.shared.setDefaultApplication(at: Bundle.main.bundleURL, toOpen: htmlclay)
    }

    /// Hands .htmlclay files to the default web browser. macOS asks the person
    /// to confirm, and this returns once they answer.
    public static func stopOpening() async throws {
        guard let probe = URL(string: "https://example.com"),
              let browser = NSWorkspace.shared.urlForApplication(toOpen: probe) else { return }
        try await NSWorkspace.shared.setDefaultApplication(at: browser, toOpen: htmlclay)
    }
}

@MainActor
public final class MacFileTypes: FileTypeSettings {
    public init() {}

    public func owner() -> FileTypeOwner {
        switch FileTypes.owner() {
        case .htmlclay: return .htmlclay
        case let .other(name): return .other(name: name)
        }
    }

    public func makeDefault() async throws { try await FileTypes.makeDefault() }

    public func stopOpening() async throws { try await FileTypes.stopOpening() }
}
