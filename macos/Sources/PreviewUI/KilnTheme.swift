import SwiftUI

public enum KilnFixed {
    public static let nestMidOnBorder = Color(css: 0x3A9BF0, alpha: 0.44)
    public static let nestInnerOnBorder = Color(css: 0x3BC16A, alpha: 0.44)
    public static let primary = Color(css: 0xEC9F2E)
    public static let primaryInk = Color(css: 0x1A140B)
    public static let primaryBorder = Color(css: 0x6E4008, alpha: 0.32)
    public static let openDashTop = Color(css: 0xEEA63B)
    public static let openDashBottom = Color(css: 0xE29424)
    public static let openDashBorder = Color(css: 0x6E4008, alpha: 0.28)
    public static let switchOn = Color(css: 0xE2911C)
    public static let checkOnBorder = Color(css: 0xC4780C)
    public static let checkInk = Color(css: 0x191208)
    public static let badgeRed = Color(css: 0xDC3E45)
    public static let badgeInk = Color(css: 0xFAF6F6)
    public static let squircleInk = Color(css: 0xF6F7F9)
    public static let squircleSlate = [Color(css: 0x6E7382), Color(css: 0x505564)]
    public static let squircleSky = [Color(css: 0x3A9BF0), Color(css: 0x1F78CB)]
    public static let squircleAmber = [Color(css: 0xF09826), Color(css: 0xCC720E)]
    public static let squircleCobalt = [Color(css: 0x5282F6), Color(css: 0x315EE0)]
    public static let squircleRose = [Color(css: 0xEC5E87), Color(css: 0xC83C66)]
    public static let squircleJade = [Color(css: 0x3BC16A), Color(css: 0x23964C)]
    public static let squircleTeal = [Color(css: 0x2EB8B0), Color(css: 0x1B8E88)]
    public static let squirclePlum = [Color(css: 0x8677A2), Color(css: 0x63557D)]
    public static let squircleRed = [Color(css: 0xE55353), Color(css: 0xBE3232)]
    public static let statusGreen = Color(css: 0x36B864)
    public static let statusAmber = Color(css: 0xE69620)
    public static let statusBlue = Color(css: 0x3882F6)
    public static let nestOuterBorder = Color(css: 0xE2911C, alpha: 0.44)
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
