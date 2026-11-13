import SwiftUI

public enum KilnPillLevel: CaseIterable {
    case trusted, served, synced, warn, err
}

public struct KilnPill: View {
    @Environment(\.kiln) private var kiln
    private let level: KilnPillLevel
    private let text: String
    public init(_ level: KilnPillLevel, text: String) {
        self.level = level
        self.text = text
    }
    public var body: some View {
        Text(text)
            .font(.system(size: 13, weight: .medium))
            .foregroundStyle(ink)
            .padding(.horizontal, 9)
            .padding(.vertical, 2)
            .background(fill, in: Capsule())
            .overlay { border }
            .fixedSize()
    }
    private var fill: Color {
        switch level {
        case .trusted: return kiln.lvlTrustBg
        case .served: return kiln.lvlServeBg
        case .synced: return kiln.lvlSyncBg
        case .warn: return kiln.warnBg
        case .err: return kiln.errBg
        }
    }
    private var ink: Color {
        switch level {
        case .trusted: return kiln.lvlTrustFg
        case .served: return kiln.lvlServeFg
        case .synced: return kiln.lvlSyncFg
        case .warn: return kiln.warnFg
        case .err: return kiln.errFg
        }
    }
    @ViewBuilder private var border: some View {
        switch level {
        case .warn: Capsule().strokeBorder(kiln.warnBorder, lineWidth: 1)
        case .err: Capsule().strokeBorder(kiln.errBorder, lineWidth: 1)
        default: EmptyView()
        }
    }
}
