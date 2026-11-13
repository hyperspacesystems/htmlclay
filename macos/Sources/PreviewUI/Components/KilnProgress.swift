import SwiftUI

public struct KilnProgress: View {
    @Environment(\.kiln) private var kiln
    private let fraction: Double
    public init(fraction: Double) { self.fraction = min(max(fraction, 0), 1) }
    public var body: some View {
        Capsule()
            .fill(kiln.cardSunk)
            .frame(height: 6)
            .overlay(alignment: .leading) {
                GeometryReader { proxy in
                    Capsule().fill(KilnFixed.switchOn).frame(width: proxy.size.width * fraction)
                }
            }
    }
}
