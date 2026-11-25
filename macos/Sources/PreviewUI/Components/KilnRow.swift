import SwiftUI
import PreviewCore

public struct KilnRow<Leading: View, Trailing: View>: View {
    @Environment(\.kiln) private var kiln
    @Environment(\.kilnCardKind) private var cardKind
    private let title: String
    private let subtitle: String?
    private let pills: [KilnPillLevel]
    private let leading: Leading
    private let trailing: Trailing
    public init(title: String, subtitle: String? = nil, pills: [KilnPillLevel] = [], @ViewBuilder leading: () -> Leading, @ViewBuilder trailing: () -> Trailing) {
        self.title = title
        self.subtitle = subtitle
        self.pills = pills
        self.leading = leading()
        self.trailing = trailing()
    }
    public var body: some View {
        HStack(spacing: 11) {
            leading
            VStack(alignment: .leading, spacing: 0) {
                Text(title)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(titleInk)
                if !pills.isEmpty {
                    HStack(spacing: 6) {
                        ForEach(Array(pills.enumerated()), id: \.offset) { item in
                            KilnPill(item.element, text: label(for: item.element))
                        }
                    }
                    .padding(.top, 4)
                }
                if let subtitle {
                    Text(subtitle)
                        .font(.system(size: 13))
                        .foregroundStyle(subtitleInk)
                        .padding(.top, 2)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            trailing
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
    private func label(for level: KilnPillLevel) -> String {
        switch level {
        case .trusted: return Strings.Levels.trusted
        case .served: return Strings.Levels.served
        case .synced: return Strings.Levels.synced
        case .warn: return Strings.Levels.warning
        case .err: return Strings.Levels.error
        }
    }
    private var titleInk: Color {
        switch cardKind {
        case .plain: return kiln.ink
        case .warn: return kiln.warnFg
        case .err: return kiln.errFg
        }
    }
    private var subtitleInk: Color {
        switch cardKind {
        case .plain: return kiln.muted
        case .warn: return kiln.warnFg.opacity(0.88)
        case .err: return kiln.errFg.opacity(0.88)
        }
    }
}

public extension KilnRow where Leading == EmptyView {
    init(title: String, subtitle: String? = nil, pills: [KilnPillLevel] = [], @ViewBuilder trailing: () -> Trailing) {
        self.init(title: title, subtitle: subtitle, pills: pills, leading: { EmptyView() }, trailing: trailing)
    }
}
