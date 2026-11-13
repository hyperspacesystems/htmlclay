import SwiftUI

public struct KilnBackButton: View {
    @Environment(\.kiln) private var kiln
    private let action: () -> Void
    public init(action: @escaping () -> Void) { self.action = action }
    public var body: some View {
        Button(action: action) {
            Image(systemName: Symbols.back)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(kiln.ink2)
                .frame(width: 27, height: 27)
                .background(kiln.btnPlainBg, in: RoundedRectangle(cornerRadius: 7))
                .overlay(RoundedRectangle(cornerRadius: 7).strokeBorder(kiln.cardBorder, lineWidth: 1))
        }
        .buttonStyle(.plain)
    }
}
