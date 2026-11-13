import SwiftUI

public struct KilnNoteRow<Trailing: View>: View {
    @Environment(\.kiln) private var kiln
    private let text: String
    private let trailing: Trailing
    public init(_ text: String, @ViewBuilder trailing: () -> Trailing) {
        self.text = text
        self.trailing = trailing()
    }
    public var body: some View {
        HStack(spacing: 11) {
            Text(text)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .frame(maxWidth: .infinity, alignment: .leading)
            trailing
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
}

public extension KilnNoteRow where Trailing == EmptyView {
    init(_ text: String) { self.init(text) { EmptyView() } }
}
