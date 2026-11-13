import SwiftUI

public struct KilnCheck: View {
    @Environment(\.kiln) private var kiln
    private let on: Bool
    public init(on: Bool) { self.on = on }
    public var body: some View {
        RoundedRectangle(cornerRadius: 4)
            .fill(on ? KilnFixed.switchOn : kiln.inputBg)
            .overlay(RoundedRectangle(cornerRadius: 4).strokeBorder(on ? KilnFixed.checkOnBorder : kiln.muted, lineWidth: 1.5))
            .overlay {
                if on {
                    Image(systemName: Symbols.check)
                        .font(.system(size: 9, weight: .bold))
                        .foregroundStyle(KilnFixed.checkInk)
                }
            }
            .frame(width: 16, height: 16)
    }
}
