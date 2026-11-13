import SwiftUI
import PreviewCore

public struct NotificationCard: View {
    @Environment(\.kiln) private var kiln
    private let note: SampleNotification
    public init(note: SampleNotification) { self.note = note }
    public var body: some View {
        HStack(alignment: .top, spacing: 11) {
            KilnAppIcon(size: 38)
            VStack(alignment: .leading, spacing: 2) {
                Text(Strings.Notify.title(note))
                    .font(.system(size: 13.5, weight: .bold))
                    .foregroundStyle(kiln.ink)
                    .fixedSize(horizontal: false, vertical: true)
                Text(Strings.Notify.body(note))
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.ink)
                    .fixedSize(horizontal: false, vertical: true)
                Text(Strings.Notify.fallbackNote)
                    .font(.system(size: 12))
                    .foregroundStyle(kiln.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 4)
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
