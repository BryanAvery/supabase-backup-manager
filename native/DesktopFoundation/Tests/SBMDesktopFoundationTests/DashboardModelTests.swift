import XCTest
import Foundation
import SBMNativeAuth
@testable import SBMDesktopFoundation

@MainActor
private final class MockDesktopClient: DesktopClient {
    var nextIdentity: NativeIdentity?
    var nextProjects: [ProjectSummary] = []
    var restoreFailure: Error?
    var signInFailure: Error?
    var projectFailure: Error?
    var logoutFailure: Error?
    var remoteRevoked = true
    var projectRequests = 0
    var logoutRequests = 0

    func signIn() async throws -> NativeIdentity {
        if let signInFailure { throw signInFailure }
        guard let nextIdentity else { throw NativeAuthError.invalidToken }
        return nextIdentity
    }

    func restoreSession() async throws -> NativeIdentity? {
        if let restoreFailure { throw restoreFailure }
        return nextIdentity
    }

    func authorisedProjects() async throws -> [ProjectSummary] {
        projectRequests += 1
        if let projectFailure { throw projectFailure }
        return nextProjects
    }

    func signOut() async throws -> Bool {
        logoutRequests += 1
        if let logoutFailure { throw logoutFailure }
        return remoteRevoked
    }
}

@MainActor
final class DashboardModelTests: XCTestCase {
    private func identity(expiry: TimeInterval = 600) -> NativeIdentity {
        NativeIdentity(userID: "fixture-user", subject: "fixture-subject",
                       expiresAt: Date().addingTimeInterval(expiry))
    }

    private func project(_ name: String) -> ProjectSummary {
        ProjectSummary(id: UUID(), name: name, environment: .cloud, readiness: .unknown)
    }

    func testNoConfigurationOrCredentialProducesSignedOutState() async {
        let client = MockDesktopClient()
        let model = DashboardModel(client: client)
        await model.restore()
        XCTAssertEqual(model.phase, .signedOut)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertNil(model.identity)
        XCTAssertEqual(client.projectRequests, 0)
    }

    func testRestoreOnlyShowsServerAuthorisedProjects() async {
        let client = MockDesktopClient()
        client.nextIdentity = identity()
        client.nextProjects = [project("Zulu"), project("Alpha")]
        let model = DashboardModel(client: client)
        await model.restore()
        XCTAssertEqual(model.phase, .signedIn)
        XCTAssertEqual(model.projects.map(\.name), ["Alpha", "Zulu"])
        XCTAssertEqual(client.projectRequests, 1)
        XCTAssertEqual(model.identity?.userID, "fixture-user")
    }

    func testSignInAndEmptyProjectListNeverClaimBackupSuccess() async {
        let client = MockDesktopClient()
        client.nextIdentity = identity()
        let model = DashboardModel(client: client)
        await model.signIn()
        XCTAssertEqual(model.phase, .signedIn)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertNil(model.message)
    }

    func testExpiredOrRevokedRefreshClearsOldProjects() async {
        let client = MockDesktopClient()
        client.nextIdentity = identity()
        client.nextProjects = [project("Private")]
        let model = DashboardModel(client: client)
        await model.signIn()
        XCTAssertEqual(model.projects.count, 1)
        client.projectFailure = NativeAuthError.unauthorised
        await model.refresh()
        XCTAssertEqual(model.phase, .expired)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertNil(model.identity)
        XCTAssertNotNil(model.message)
    }

    func testFailedProjectRefreshNeverDisplaysStaleData() async {
        let client = MockDesktopClient()
        client.nextIdentity = identity()
        client.nextProjects = [project("Old")]
        let model = DashboardModel(client: client)
        await model.signIn()
        client.projectFailure = NativeAuthError.invalidServerResponse
        await model.refresh()
        XCTAssertEqual(model.phase, .signedIn)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertNotNil(model.message)
    }

    func testTimerExpiresSessionAndClearsProjects() async {
        let client = MockDesktopClient()
        client.nextIdentity = identity()
        client.nextProjects = [project("Expired")]
        let model = DashboardModel(client: client)
        await model.signIn()
        model.checkExpiry(now: Date().addingTimeInterval(1000))
        XCTAssertEqual(model.phase, .expired)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertNil(model.identity)
    }

    func testExpiredRestoreDoesNotLoadProjects() async {
        let client = MockDesktopClient()
        client.nextIdentity = identity(expiry: -10)
        client.nextProjects = [project("Should not show")]
        let model = DashboardModel(client: client)
        await model.restore()
        XCTAssertEqual(model.phase, .expired)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertEqual(client.projectRequests, 0)
    }

    func testLocalSignOutWithoutRemoteConfirmationShowsWarning() async {
        let client = MockDesktopClient()
        client.nextIdentity = identity()
        client.nextProjects = [project("Visible")]
        client.remoteRevoked = false
        let model = DashboardModel(client: client)
        await model.signIn()
        await model.signOut()
        XCTAssertEqual(model.phase, .signedOut)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertNil(model.identity)
        XCTAssertTrue(model.message?.contains("could not be confirmed") == true)
        XCTAssertEqual(client.logoutRequests, 1)
    }

    func testKeychainClearFailureDoesNotClaimSignOut() async {
        let client = MockDesktopClient()
        client.nextIdentity = identity()
        client.nextProjects = [project("Visible")]
        client.logoutFailure = NativeAuthError.keychainFailure(-25293)
        let model = DashboardModel(client: client)
        await model.signIn()
        await model.signOut()
        XCTAssertEqual(model.phase, .signedIn)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertNotNil(model.identity)
        XCTAssertTrue(model.message?.contains("Could not clear") == true)
    }

    func testRestoreRejectedIdentityNeverDisplaysOldData() async {
        let client = MockDesktopClient()
        client.restoreFailure = NativeAuthError.unauthorised
        let model = DashboardModel(client: client)
        await model.restore()
        XCTAssertEqual(model.phase, .expired)
        XCTAssertTrue(model.projects.isEmpty)
        XCTAssertNil(model.identity)
    }
}
