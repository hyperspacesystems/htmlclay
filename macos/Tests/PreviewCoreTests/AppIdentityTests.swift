import XCTest
@testable import PreviewCore

final class AppIdentityTests: XCTestCase {
    func testOnlyTheProductIDIsTheProduct() {
        XCTAssertEqual(AppIdentity.of(bundleID: "com.htmlclay.HTMLClay"), .product)
        XCTAssertEqual(AppIdentity.of(bundleID: "com.htmlclay.HTMLClayCandidate"), .candidate)
        XCTAssertEqual(AppIdentity.of(bundleID: "com.htmlclay.UpgradeCheck"), .candidate)
        XCTAssertEqual(AppIdentity.of(bundleID: nil), .candidate)
    }

    func testTheProductHostRunsOnTheRealConfigFolder() {
        let candidateHome = ["HOME": "/tmp/hc-1", "HTMLCLAY_CONFIG_BASE": "/tmp/hc-1/c"]
        XCTAssertEqual(AppIdentity.product.hostEnvironment(candidateHome: candidateHome), [:])
        XCTAssertEqual(AppIdentity.candidate.hostEnvironment(candidateHome: candidateHome), candidateHome)
        XCTAssertFalse(AppIdentity.product.usesCandidateHome)
        XCTAssertTrue(AppIdentity.candidate.usesCandidateHome)
    }

    func testAnotherInstanceIsTheSameBundleAtTheSamePath() {
        let own = "/Applications/HTMLClay.app"
        let copy = "/Users/x/Library/Application Support/htmlclay/updates/previous/abc/HTMLClay.app"
        XCTAssertNil(AppIdentity.otherInstance(pid: 10, bundlePath: own, among: [.init(pid: 10, bundlePath: own)]))
        XCTAssertNil(AppIdentity.otherInstance(pid: 10, bundlePath: own, among: [.init(pid: 20, bundlePath: copy)]))
        XCTAssertNil(AppIdentity.otherInstance(pid: 10, bundlePath: own, among: [.init(pid: 20, bundlePath: nil)]))
        XCTAssertEqual(AppIdentity.otherInstance(pid: 10, bundlePath: own, among: [.init(pid: 20, bundlePath: copy), .init(pid: 30, bundlePath: own + "/")]), 30)
    }
}
