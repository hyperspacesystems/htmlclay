import SwiftUI

public struct KilnCard<Content: View>: View {
    @Environment(\.kiln) private var kiln
    private let content: Content
    public init(@ViewBuilder content: () -> Content) { self.content = content() }
    public var body: some View {
        VStack(spacing: 0) { content }
            .background(kiln.card, in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(kiln.cardBorder, lineWidth: 1))
    }
}

public struct KilnRow<Trailing: View>: View {
    @Environment(\.kiln) private var kiln
    private let title: String
    private let subtitle: String?
    private let trailing: Trailing
    public init(title: String, subtitle: String? = nil, @ViewBuilder trailing: () -> Trailing) {
        self.title = title; self.subtitle = subtitle; self.trailing = trailing()
    }
    public var body: some View {
        HStack(spacing: 11) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.system(size: 14, weight: .medium))
                if let subtitle { Text(subtitle).font(.system(size: 13)).foregroundStyle(kiln.muted) }
            }
            Spacer(minLength: 0)
            trailing
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
}

public struct KilnPrimaryButtonStyle: ButtonStyle {
    public init() {}
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 14, weight: .semibold))
            .foregroundStyle(KilnFixed.primaryInk)
            .padding(.horizontal, 16)
            .padding(.vertical, 7)
            .frame(maxWidth: .infinity)
            .background(KilnFixed.primary.opacity(configuration.isPressed ? 0.85 : 1), in: RoundedRectangle(cornerRadius: 9))
    }
}

public struct KilnSwitchStyle: ToggleStyle {
    @Environment(\.kiln) private var kiln
    public init() {}
    public func makeBody(configuration: Configuration) -> some View {
        Button { configuration.isOn.toggle() } label: {
            Capsule()
                .fill(configuration.isOn ? KilnFixed.switchOn : kiln.switchOff)
                .frame(width: 38, height: 22)
                .overlay(alignment: configuration.isOn ? .trailing : .leading) {
                    Circle().fill(Color(css: 0xF6F7F9)).frame(width: 18, height: 18).padding(2)
                }
        }
        .buttonStyle(.plain)
    }
}

public struct KilnBadge: View {
    private let count: Int
    public init(_ count: Int) { self.count = count }
    public var body: some View {
        Text(String(count))
            .font(.system(size: 12, weight: .bold).monospacedDigit())
            .foregroundStyle(KilnFixed.badgeInk)
            .padding(.horizontal, 6)
            .frame(minWidth: 20, minHeight: 19)
            .background(KilnFixed.badgeRed, in: Capsule())
    }
}
