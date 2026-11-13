import SwiftUI

public enum KilnFixed {
    public static let primary = Color(css: 0xEC9F2E)
    public static let primaryInk = Color(css: 0x1A140B)
    public static let switchOn = Color(css: 0xE2911C)
    public static let badgeRed = Color(css: 0xDC3E45)
    public static let badgeInk = Color(css: 0xFAF6F6)
}

public extension Color {
    init(css hex: UInt32, alpha: Double = 1) {
        self.init(.sRGB,
                  red: Double((hex >> 16) & 0xFF) / 255,
                  green: Double((hex >> 8) & 0xFF) / 255,
                  blue: Double(hex & 0xFF) / 255,
                  opacity: alpha)
    }
}

private struct KilnKey: EnvironmentKey {
    static let defaultValue = KilnPalette.light
}

public extension EnvironmentValues {
    var kiln: KilnPalette {
        get { self[KilnKey.self] }
        set { self[KilnKey.self] = newValue }
    }
}

public struct KilnRoot<Content: View>: View {
    @Environment(\.colorScheme) private var scheme
    private let content: Content
    public init(@ViewBuilder content: () -> Content) { self.content = content() }
    public var body: some View {
        let palette = scheme == .dark ? KilnPalette.dark : KilnPalette.light
        content
            .environment(\.kiln, palette)
            .foregroundStyle(palette.ink)
            .background(palette.win)
    }
}
