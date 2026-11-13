import SwiftUI

public struct NestRows<Content: View>: View {
    @Environment(\.kiln) private var kiln
    private let inOuter: Bool
    private let content: Content
    public init(inOuter: Bool = false, @ViewBuilder content: () -> Content) {
        self.inOuter = inOuter
        self.content = content()
    }
    public var body: some View {
        VStack(spacing: 0) { content }
            .background(fill, in: RoundedRectangle(cornerRadius: 8))
            .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(kiln.cardBorder, lineWidth: 1))
            .padding(.top, 11)
    }
    private var fill: Color {
        inOuter ? kiln.card2 : kiln.card
    }
}
