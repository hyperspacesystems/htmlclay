import SwiftUI

public enum NestLevel {
    case outer, mid, inner
}

public struct NestCard<Content: View>: View {
    @Environment(\.kiln) private var kiln
    private let level: NestLevel
    private let on: Bool
    private let content: Content
    public init(level: NestLevel, on: Bool = false, @ViewBuilder content: () -> Content) {
        self.level = level
        self.on = on
        self.content = content()
    }
    public var body: some View {
        content
            .padding(inset)
            .background(fill, in: RoundedRectangle(cornerRadius: cornerRadius))
            .overlay(RoundedRectangle(cornerRadius: cornerRadius).strokeBorder(line, lineWidth: 1.5))
            .padding(.top, topMargin)
    }
    private var inset: CGFloat {
        switch level {
        case .outer: return 14
        case .mid: return 13
        case .inner: return 12
        }
    }
    private var cornerRadius: CGFloat {
        switch level {
        case .outer: return 12
        case .mid: return 10
        case .inner: return 9
        }
    }
    private var topMargin: CGFloat {
        switch level {
        case .outer: return 0
        case .mid, .inner: return 12
        }
    }
    private var fill: Color {
        switch level {
        case .outer: return kiln.card
        case .mid: return kiln.card2
        case .inner: return kiln.cardSunk
        }
    }
    private var line: Color {
        switch level {
        case .outer: return KilnFixed.nestOuterBorder
        case .mid: return on ? KilnFixed.nestMidOnBorder : kiln.cardBorder
        case .inner: return on ? KilnFixed.nestInnerOnBorder : kiln.cardBorder
        }
    }
}
