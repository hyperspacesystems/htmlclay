import XCTest
@testable import PreviewCore

final class PersonaAllowListTests: XCTestCase {
    func testSampleOnlyPersonasStayInsideTheAllowList() {
        let environment = ProcessInfo.processInfo.environment
        guard let listed = environment["SAMPLE_ONLY_PERSONAS"] else {
            if environment["HTMLCLAY_REQUIRE_LIVE_HOST"] == "1" {
                XCTFail("SAMPLE_ONLY_PERSONAS is missing while HTMLCLAY_REQUIRE_LIVE_HOST=1")
            }
            return
        }
        let allowed = Set(listed.split(separator: " ").map(String.init))
        let unlisted = Persona.sampleOnly.map(\.rawValue).filter { !allowed.contains($0) }
        XCTAssertEqual(unlisted, [],
                       "\(unlisted) went back to sample data; SAMPLE_ONLY_PERSONAS may only shrink")
    }
}
