import SwiftUI

public struct KilnGroupTitle: View {
    @Environment(\.kiln) private var kiln
    private let text: String
    public init(_ text: String) { self.text = text }
    public var body: some View {
        Text(text)
            .font(.system(size: 13, weight: .semibold))
            .foregroundStyle(kiln.ink2)
            .padding(.horizontal, 3)
            .padding(.top, 16)
            .padding(.bottom, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}
