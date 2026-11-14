import SwiftUI
import PreviewCore

public struct StatusDot: View {
    @Environment(\.kiln) private var kiln
    private let kind: StatusDotKind
    public init(_ kind: StatusDotKind) { self.kind = kind }
    public var body: some View {
        Circle()
            .fill(fill)
            .frame(width: 8, height: 8)
    }
    private var fill: Color {
        switch kind {
        case .green: return KilnFixed.statusGreen
        case .amber: return KilnFixed.statusAmber
        case .blue: return KilnFixed.statusBlue
        case .red: return kiln.danger
        case .muted: return kiln.faint
        }
    }
}
