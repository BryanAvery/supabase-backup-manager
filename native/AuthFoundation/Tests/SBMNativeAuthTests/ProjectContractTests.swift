import Foundation
import XCTest
@testable import SBMNativeAuth

final class ProjectContractTests: XCTestCase {
    func testValidOpenAPIProjectList() throws {
        let json = """
        [
          {
            "id": "aab77233-d8ec-45f4-9da9-eaf8e1a29cca",
            "name": "Synthetic Cloud",
            "environment": "cloud",
            "readiness": "unknown"
          },
          {
            "id": "182811f6-387c-42cf-970e-9b5e7312dbe8",
            "name": "Synthetic Self Hosted",
            "environment": "self_hosted"
          }
        ]
        """
        let projects = try JSONDecoder().decode([ProjectSummary].self, from: Data(json.utf8))
        XCTAssertEqual(projects.count, 2)
        XCTAssertEqual(projects[0].name, "Synthetic Cloud")
        XCTAssertEqual(projects[0].environment, .cloud)
        XCTAssertEqual(projects[0].readiness, .unknown)
        XCTAssertEqual(projects[1].environment, .selfHosted)
        XCTAssertNil(projects[1].readiness)
    }

    func testMalformedProjectCannotBePresented() {
        for json in [
            #"[{"id":"invalid","name":"Fake","environment":"cloud"}]"#,
            #"[{"id":"aab77233-d8ec-45f4-9da9-eaf8e1a29cca","name":"Fake","environment":"admin"}]"#,
            #"[{"id":"aab77233-d8ec-45f4-9da9-eaf8e1a29cca","name":"Fake","environment":"cloud","readiness":"completed_backup"}]"#,
            #"[{"id":"aab77233-d8ec-45f4-9da9-eaf8e1a29cca","environment":"cloud"}]"#
        ] {
            XCTAssertThrowsError(try JSONDecoder().decode([ProjectSummary].self, from: Data(json.utf8)))
        }
    }

    func testNoProjectsIsEmptyArrayNotSyntheticDemoData() throws {
        let result = try JSONDecoder().decode([ProjectSummary].self, from: Data("[]".utf8))
        XCTAssertTrue(result.isEmpty)
    }
}
