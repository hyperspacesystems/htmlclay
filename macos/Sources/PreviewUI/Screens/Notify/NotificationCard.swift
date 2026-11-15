import SwiftUI
import PreviewCore

public struct NotificationCard: View {
    @Environment(\.kiln) private var kiln
    private let title: String
    private let message: String
    private let footnote: String
    private let action: (label: String, run: () -> Void)?

    public init(note: SampleNotification) {
        self.init(title: Strings.Notify.title(note), body: Strings.Notify.body(note), footnote: Strings.Notify.fallbackNote)
    }

    public init(title: String, body: String, footnote: String, action: (label: String, run: () -> Void)? = nil) {
        self.title = title
        self.message = body
        self.footnote = footnote
        self.action = action
    }
    public var body: some View {
        HStack(alignment: .top, spacing: 11) {
            KilnAppIcon(size: 38)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.system(size: 13.5, weight: .bold))
                    .foregroundStyle(kiln.ink)
                    .fixedSize(horizontal: false, vertical: true)
                Text(message)
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.ink)
                    .fixedSize(horizontal: false, vertical: true)
                Text(footnote)
                    .font(.system(size: 12))
                    .foregroundStyle(kiln.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 4)
                if let action {
                    Button(action.label, action: action.run)
                        .buttonStyle(KilnButtonStyle(.plain))
                        .padding(.top, 6)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.horizontal, 13)
        .padding(.vertical, 11)
        .frame(width: 368)
        .background(kiln.win, in: RoundedRectangle(cornerRadius: 13))
        .overlay(RoundedRectangle(cornerRadius: 13).strokeBorder(kiln.cardBorder, lineWidth: 1))
    }
}
