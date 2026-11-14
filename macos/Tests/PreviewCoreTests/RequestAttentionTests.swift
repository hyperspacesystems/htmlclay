import XCTest
@testable import PreviewCore

final class RequestAttentionTests: XCTestCase {
    private func request(_ id: String, attention: Int? = nil) -> ProgramRequest {
        ProgramRequest(id: id, kind: .found, document: "notes.htmlclay", program: "resize", attention: attention)
    }

    private func snapshot(_ requests: [ProgramRequest]) -> HostSnapshot {
        HostSnapshot(requests: requests)
    }

    func testARaisedQuestionComesBack() {
        let raised = RequestAttention.raised(from: snapshot([request("r1")]), to: snapshot([request("r1", attention: 1)]))
        XCTAssertEqual(raised?.id, "r1")
    }

    func testAQuestionAlreadyShownIsNotRaisedAgain() {
        XCTAssertNil(RequestAttention.raised(from: snapshot([request("r1", attention: 2)]),
                                            to: snapshot([request("r1", attention: 2)])))
    }

    func testAQuestionNobodyRaisedIsNotShown() {
        XCTAssertNil(RequestAttention.raised(from: snapshot([]), to: snapshot([request("r1", attention: 0)])))
        XCTAssertNil(RequestAttention.raised(from: snapshot([]), to: snapshot([request("r1")])))
    }

    func testAQuestionThatIsGoneRaisesNothing() {
        XCTAssertNil(RequestAttention.raised(from: snapshot([request("r1", attention: 1)]), to: snapshot([])))
    }

    func testTheNewestRaiseIsTheOneShown() {
        let raised = RequestAttention.raised(from: snapshot([request("r1"), request("r2", attention: 3)]),
                                            to: snapshot([request("r1"), request("r2", attention: 4)]))
        XCTAssertEqual(raised?.id, "r2")
    }
}
