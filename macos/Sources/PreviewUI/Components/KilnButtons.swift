import SwiftUI

public enum KilnButtonKind {
    case plain, large, primary, danger, openDashboard
}

public struct KilnButtonStyle: ButtonStyle {
    @Environment(\.kiln) private var kiln
    private let kind: KilnButtonKind
    public init(_ kind: KilnButtonKind = .plain) { self.kind = kind }
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: fontSize, weight: fontWeight))
            .foregroundStyle(ink)
            .padding(.vertical, verticalPadding)
            .padding(.horizontal, horizontalPadding)
            .frame(maxWidth: isFullWidth ? .infinity : nil)
            .background(fill, in: RoundedRectangle(cornerRadius: cornerRadius))
            .overlay(RoundedRectangle(cornerRadius: cornerRadius).strokeBorder(line, lineWidth: 1))
            .opacity(configuration.isPressed ? 0.85 : 1)
    }
    private var isFullWidth: Bool { kind == .openDashboard }
    private var fontSize: CGFloat {
        switch kind {
        case .plain, .primary, .danger: return 13.5
        case .large, .openDashboard: return 14
        }
    }
    private var fontWeight: Font.Weight {
        switch kind {
        case .plain, .large, .danger: return .medium
        case .primary, .openDashboard: return .semibold
        }
    }
    private var verticalPadding: CGFloat {
        switch kind {
        case .plain, .primary, .danger: return 5
        case .large: return 7
        case .openDashboard: return 9
        }
    }
    private var horizontalPadding: CGFloat {
        switch kind {
        case .plain, .primary, .danger: return 13
        case .large: return 16
        case .openDashboard: return 14
        }
    }
    private var cornerRadius: CGFloat {
        switch kind {
        case .plain, .primary, .danger: return 7
        case .large: return 8
        case .openDashboard: return 9
        }
    }
    private var fill: AnyShapeStyle {
        switch kind {
        case .plain, .large:
            return AnyShapeStyle(LinearGradient(colors: kiln.btnBg, startPoint: .top, endPoint: .bottom))
        case .primary:
            return AnyShapeStyle(KilnFixed.primary)
        case .danger:
            return AnyShapeStyle(kiln.dangerBg)
        case .openDashboard:
            return AnyShapeStyle(LinearGradient(colors: [KilnFixed.openDashTop, KilnFixed.openDashBottom], startPoint: .top, endPoint: .bottom))
        }
    }
    private var line: Color {
        switch kind {
        case .plain, .large: return kiln.btnBorder
        case .primary: return KilnFixed.primaryBorder
        case .danger: return kiln.dangerBorder
        case .openDashboard: return KilnFixed.openDashBorder
        }
    }
    private var ink: Color {
        switch kind {
        case .plain, .large: return kiln.btnFg
        case .primary, .openDashboard: return KilnFixed.primaryInk
        case .danger: return kiln.danger
        }
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
