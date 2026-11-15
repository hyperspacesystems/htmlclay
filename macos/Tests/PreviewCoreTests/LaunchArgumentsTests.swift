import XCTest
@testable import PreviewCore

final class LaunchArgumentsTests: XCTestCase {
    private let existing: Set<String> = ["/tmp/a.htmlclay", "notes.html", "serve"]

    private func role(_ rest: [String]) -> LaunchRole {
        LaunchArguments.role(["/Applications/HTMLClay.app/Contents/MacOS/htmlclay"] + rest, fileExists: { self.existing.contains($0) })
    }

    func testTheAppKeepsItsOwnLaunches() {
        XCTAssertEqual(role([]), .app(open: []))
        XCTAssertEqual(role(["--profile", "newPerson"]), .app(open: []))
        XCTAssertEqual(role(["--no-welcome"]), .app(open: []))
        XCTAssertEqual(role(["-NSDocumentRevisionsDebugMode", "YES"]), .app(open: []))
        XCTAssertEqual(role(["-psn_0_12345"]), .app(open: []))
        XCTAssertEqual(role(["/tmp/a.htmlclay"]), .app(open: ["/tmp/a.htmlclay"]))
        XCTAssertEqual(role(["/tmp/a.htmlclay", "notes.html"]), .app(open: ["/tmp/a.htmlclay", "notes.html"]))
    }

    func testEveryOtherLaunchGoesToTheHost() {
        XCTAssertEqual(role(["--no-tray"]), .host)
        XCTAssertEqual(role(["--finish-update"]), .host)
        XCTAssertEqual(role(["serve", "/tmp/site"]), .host)
        XCTAssertEqual(role(["serve"]), .host)
        XCTAssertEqual(role(["wire", "status"]), .host)
        XCTAssertEqual(role(["update-info"]), .host)
        XCTAssertEqual(role(["/tmp/missing.htmlclay"]), .host)
        XCTAssertEqual(role(["/tmp/a.htmlclay", "--no-tray"]), .host)
    }

    func testUpdateNowAndUnregisterAreNamed() {
        XCTAssertEqual(role(["update-now", "--version", "1.2.3"]), .updateNow(["--version", "1.2.3"]))
        XCTAssertEqual(role(["--unregister"]), .unregister)
    }
}
