import SwiftUI

public struct KilnRadio: View {
    @Environment(\.kiln) private var kiln
    private let on: Bool
    public init(on: Bool) { self.on = on }
    public var body: some View {
        Circle()
            .fill(kiln.inputBg)
            .overlay(Circle().strokeBorder(on ? KilnFixed.switchOn : kiln.muted, lineWidth: on ? 5 : 1.5))
            .frame(width: 15, height: 15)
    }
}
