import SwiftUI

public struct KilnBadge: View {
    private let count: Int
    public init(_ count: Int) { self.count = count }
    public var body: some View {
        Text(String(count))
            .font(.system(size: 12, weight: .bold).monospacedDigit())
            .foregroundStyle(KilnFixed.badgeInk)
            .padding(.horizontal, 6)
            .frame(minWidth: 20, minHeight: 19)
            .background(KilnFixed.badgeRed, in: Capsule())
    }
}
