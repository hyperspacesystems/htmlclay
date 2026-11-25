import SwiftUI

public enum KilnCardKind {
    case plain, warn, err
}

struct KilnCardKindKey: EnvironmentKey {
    static let defaultValue = KilnCardKind.plain
}

extension EnvironmentValues {
    var kilnCardKind: KilnCardKind {
        get { self[KilnCardKindKey.self] }
        set { self[KilnCardKindKey.self] = newValue }
    }
}

public struct KilnCard<Content: View>: View {
    @Environment(\.kiln) private var kiln
    private let kind: KilnCardKind
    private let content: Content
    public init(kind: KilnCardKind = .plain, @ViewBuilder content: () -> Content) {
        self.kind = kind
        self.content = content()
    }
    public var body: some View {
        VStack(spacing: 0) { content }
            .foregroundStyle(ink)
            .background(fill, in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(line, lineWidth: 1))
            .environment(\.kilnCardKind, kind)
    }
    private var fill: Color {
        switch kind {
        case .plain: return kiln.card
        case .warn: return kiln.warnBg
        case .err: return kiln.errBg
        }
    }
    private var line: Color {
        switch kind {
        case .plain: return kiln.cardBorder
        case .warn: return kiln.warnBorder
        case .err: return kiln.errBorder
        }
    }
    private var ink: Color {
        switch kind {
        case .plain: return kiln.ink
        case .warn: return kiln.warnFg
        case .err: return kiln.errFg
        }
    }
}
