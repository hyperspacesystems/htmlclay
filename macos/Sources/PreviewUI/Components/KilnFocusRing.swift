import SwiftUI

/// A focus ring that follows a button's own corners, drawn only while the
/// button has keyboard focus.
public struct KilnFocusRing: ViewModifier {
    private let focused: Bool
    private let cornerRadius: CGFloat

    public init(focused: Bool, cornerRadius: CGFloat) {
        self.focused = focused
        self.cornerRadius = cornerRadius
    }

    public func body(content: Content) -> some View {
        content.overlay {
            if focused {
                RoundedRectangle(cornerRadius: cornerRadius)
                    .strokeBorder(KilnFixed.primary, lineWidth: 2)
            }
        }
    }
}

public extension View {
    func focusRing(focused: Bool, cornerRadius: CGFloat) -> some View {
        modifier(KilnFocusRing(focused: focused, cornerRadius: cornerRadius))
    }
}
