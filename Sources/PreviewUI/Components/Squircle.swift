import SwiftUI

public enum SquircleColor: CaseIterable {
    case slate, sky, amber, cobalt, rose, jade, teal, plum, red
}

public enum SquircleSize {
    case regular, large, extraLarge
}

public struct Squircle: View {
    private let color: SquircleColor
    private let symbol: String
    private let size: SquircleSize
    public init(_ color: SquircleColor, symbol: String, size: SquircleSize = .regular) {
        self.color = color
        self.symbol = symbol
        self.size = size
    }
    public var body: some View {
        Image(systemName: symbol)
            .font(.system(size: symbolSize, weight: .medium))
            .foregroundStyle(KilnFixed.squircleInk)
            .frame(width: side, height: side)
            .background(LinearGradient(colors: gradient, startPoint: .top, endPoint: .bottom), in: RoundedRectangle(cornerRadius: cornerRadius, style: .continuous))
    }
    private var side: CGFloat {
        switch size {
        case .regular: return 24
        case .large: return 32
        case .extraLarge: return 52
        }
    }
    private var cornerRadius: CGFloat {
        switch size {
        case .regular: return 6
        case .large: return 8
        case .extraLarge: return 13
        }
    }
    private var symbolSize: CGFloat {
        switch size {
        case .regular: return 12
        case .large: return 15
        case .extraLarge: return 24
        }
    }
    private var gradient: [Color] {
        switch color {
        case .slate: return KilnFixed.squircleSlate
        case .sky: return KilnFixed.squircleSky
        case .amber: return KilnFixed.squircleAmber
        case .cobalt: return KilnFixed.squircleCobalt
        case .rose: return KilnFixed.squircleRose
        case .jade: return KilnFixed.squircleJade
        case .teal: return KilnFixed.squircleTeal
        case .plum: return KilnFixed.squirclePlum
        case .red: return KilnFixed.squircleRed
        }
    }
}
