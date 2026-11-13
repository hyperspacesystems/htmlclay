import SwiftUI

public struct KilnRowDivider: View {
    @Environment(\.kiln) private var kiln
    public init() {}
    public var body: some View {
        Rectangle()
            .fill(kiln.rowSep)
            .frame(height: 1)
    }
}
