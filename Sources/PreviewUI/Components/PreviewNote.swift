import SwiftUI

public struct PreviewNote: View {
    @Environment(\.kiln) private var kiln
    private let text: String
    public init(_ text: String) { self.text = text }
    public var body: some View {
        Text(text)
            .font(.system(size: 12))
            .foregroundStyle(kiln.muted)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}
