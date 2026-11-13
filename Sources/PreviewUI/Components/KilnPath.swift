import SwiftUI

public struct KilnPath: View {
    @Environment(\.kiln) private var kiln
    private let text: String
    public init(_ text: String) { self.text = text }
    public var body: some View {
        Text(text)
            .font(.system(size: 13))
            .foregroundStyle(kiln.muted)
            .padding(.top, -4)
            .padding(.bottom, 14)
    }
}
