import SwiftUI

/// Matches the Phosphor regular copy icon (256x256 viewBox).
public struct CopyPathIcon: Shape {
    public init() {}

    public func path(in rect: CGRect) -> Path {
        let sx = rect.width / 256.0
        let sy = rect.height / 256.0
        func pt(_ x: CGFloat, _ y: CGFloat) -> CGPoint {
            CGPoint(x: rect.minX + x * sx, y: rect.minY + y * sy)
        }

        var path = Path()
        // Outer silhouette of the two overlapping squares.
        path.move(to: pt(216, 32))
        path.addLine(to: pt(88, 32))
        path.addQuadCurve(to: pt(80, 40), control: pt(80, 32))
        path.addLine(to: pt(80, 80))
        path.addLine(to: pt(40, 80))
        path.addQuadCurve(to: pt(32, 88), control: pt(32, 80))
        path.addLine(to: pt(32, 216))
        path.addQuadCurve(to: pt(40, 224), control: pt(32, 224))
        path.addLine(to: pt(168, 224))
        path.addQuadCurve(to: pt(176, 216), control: pt(176, 224))
        path.addLine(to: pt(176, 176))
        path.addLine(to: pt(216, 176))
        path.addQuadCurve(to: pt(224, 168), control: pt(224, 176))
        path.addLine(to: pt(224, 40))
        path.addQuadCurve(to: pt(216, 32), control: pt(224, 32))
        path.closeSubpath()

        // Front square cutout.
        path.addRect(CGRect(x: rect.minX + 48 * sx, y: rect.minY + 96 * sy, width: 112 * sx, height: 112 * sy))

        // Back square L-shaped cutout.
        path.move(to: pt(208, 160))
        path.addLine(to: pt(176, 160))
        path.addLine(to: pt(176, 88))
        path.addQuadCurve(to: pt(168, 80), control: pt(176, 80))
        path.addLine(to: pt(96, 80))
        path.addLine(to: pt(96, 48))
        path.addLine(to: pt(208, 48))
        path.closeSubpath()

        return path
    }
}
