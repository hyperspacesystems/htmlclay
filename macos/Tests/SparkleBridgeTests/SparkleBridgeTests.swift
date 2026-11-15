import XCTest
import SparkleBridge

final class SparkleBridgeTests: XCTestCase {
    func testMissingFrameworkIsASentence() {
        let sentence = HCSparkleBridge.loadFramework(atPath: "/nonexistent/Sparkle.framework")
        XCTAssertNotNil(sentence)
        XCTAssertTrue(sentence?.contains("/nonexistent/Sparkle.framework") ?? false)
    }

    func testInstallWithNothingPreparedFails() {
        let received = expectation(description: "install failed")
        let bridge = HCSparkleBridge(events: { event, text in
            XCTAssertEqual(event, .installFailed)
            XCTAssertEqual(text, "Sparkle has no prepared update")
            received.fulfill()
        }, skipReason: { _ in nil })
        bridge.installNow()
        wait(for: [received], timeout: 2)
    }

    func testCheckBeforeStartIsSkipped() {
        let received = expectation(description: "check skipped")
        let bridge = HCSparkleBridge(events: { event, text in
            XCTAssertEqual(event, .checkSkipped)
            XCTAssertEqual(text, "Sparkle is not started")
            received.fulfill()
        }, skipReason: { _ in nil })
        bridge.checkNow()
        wait(for: [received], timeout: 2)
    }
}
