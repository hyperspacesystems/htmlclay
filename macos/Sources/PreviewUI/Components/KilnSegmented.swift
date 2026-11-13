import SwiftUI

public struct KilnSegmented<Value: Hashable>: View {
    @Environment(\.kiln) private var kiln
    @Binding private var selection: Value
    private let options: [(value: Value, label: String)]
    public init(selection: Binding<Value>, options: [(value: Value, label: String)]) {
        self._selection = selection
        self.options = options
    }
    public var body: some View {
        HStack(spacing: 2) {
            ForEach(options.indices, id: \.self) { index in
                let option = options[index]
                Button { selection = option.value } label: {
                    Text(option.label)
                        .font(.system(size: 13, weight: isSelected(option.value) ? .semibold : .regular))
                        .foregroundStyle(isSelected(option.value) ? kiln.ink : kiln.ink2)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 4)
                        .background(isSelected(option.value) ? kiln.segOn : Color.clear, in: RoundedRectangle(cornerRadius: 5))
                }
                .buttonStyle(.plain)
            }
        }
        .padding(2)
        .background(kiln.segBg, in: RoundedRectangle(cornerRadius: 7))
        .overlay(RoundedRectangle(cornerRadius: 7).strokeBorder(kiln.cardBorder, lineWidth: 1))
    }
    private func isSelected(_ value: Value) -> Bool { value == selection }
}
