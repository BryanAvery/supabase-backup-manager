import XCTest
import Foundation
import Security
@testable import SBMNativeAuth

@MainActor
final class NativePKCETests: XCTestCase {
    func configuration(
        key: Data = Data([1, 2, 3]),
        redirect: String = "org.example.backupmanager:/oauth/callback",
        api: String = "https://127.0.0.1:8765/api/v1"
    ) throws -> NativeAuthConfiguration {
        try NativeAuthConfiguration(
            issuer: URL(string: "https://id.example.invalid/")!,
            authorizationEndpoint: URL(string: "https://id.example.invalid/authorize")!,
            tokenEndpoint: URL(string: "https://id.example.invalid/token")!,
            redirectURI: URL(string: redirect)!,
            apiBaseURL: URL(string: api)!,
            clientID: "native-fixture-client",
            apiScope: "api.read",
            requiredACR: "urn:example:mfa",
            signingKeyID: "fixture-rsa",
            idTokenRSAPublicKeyDER: key
        )
    }

    func testPKCES256NonceIssuerAndOneUseState() throws {
        let flow = PKCEFlow(configuration: try configuration())
        let start = try flow.begin()
        let fields = URLComponents(url: start, resolvingAgainstBaseURL: false)!
        let values = Dictionary(uniqueKeysWithValues:
            fields.queryItems!.map { ($0.name, $0.value ?? "") })
        XCTAssertEqual(values["response_type"], "code")
        XCTAssertEqual(values["scope"], "openid api.read")
        XCTAssertEqual(values["code_challenge_method"], "S256")
        XCTAssertEqual(values["acr_values"], "urn:example:mfa")
        XCTAssertEqual(values["redirect_uri"], "org.example.backupmanager:/oauth/callback")
        XCTAssertEqual(values["code_challenge"]?.count, 43)
        XCTAssertEqual(values["state"]?.count, 43)
        XCTAssertEqual(values["nonce"]?.count, 43)
        XCTAssertNotEqual(values["state"], values["nonce"])

        let callback = "org.example.backupmanager:/oauth/callback" +
            "?state=\(values["state"]!)&code=fixture-code&iss=" +
            "https%3A%2F%2Fid.example.invalid%2F"
        let obtained = try flow.consume(callback: URL(string: callback)!)
        XCTAssertEqual(obtained.value, "fixture-code")
        XCTAssertEqual(obtained.nonce, values["nonce"])
        let challenge = Data(SHA256.hash(data: Data(obtained.verifier.utf8))).base64URLEncoded()
        XCTAssertEqual(challenge, values["code_challenge"])
        XCTAssertThrowsError(try flow.consume(callback: URL(string: callback)!))
    }

    func testCrossTransactionStateAndDuplicateStateRejected() throws {
        let flow = PKCEFlow(configuration: try configuration())
        let old = try flow.begin()
        let oldState = URLComponents(url: old, resolvingAgainstBaseURL: false)!
            .queryItems!.first { $0.name == "state" }!.value!
        _ = try flow.begin()
        let stolen = URL(string: "org.example.backupmanager:/oauth/callback" +
            "?state=\(oldState)&code=fixture-code&iss=https%3A%2F%2Fid.example.invalid%2F")!
        XCTAssertThrowsError(try flow.consume(callback: stolen))

        let newStart = try flow.begin()
        let state = URLComponents(url: newStart, resolvingAgainstBaseURL: false)!
            .queryItems!.first { $0.name == "state" }!.value!
        let duplicates = URL(string: "org.example.backupmanager:/oauth/callback" +
            "?state=\(state)&state=\(state)&code=fixture-code&iss=https%3A%2F%2Fid.example.invalid%2F")!
        XCTAssertThrowsError(try flow.consume(callback: duplicates))
    }

    func testExpiredStateAndCallbackSubstitution() throws {
        let flow = PKCEFlow(configuration: try configuration())
        let now = Date()
        let start = try flow.begin(now: now)
        let state = URLComponents(url: start, resolvingAgainstBaseURL: false)!
            .queryItems!.first { $0.name == "state" }!.value!
        let callback = URL(string: "org.example.backupmanager:/oauth/callback" +
            "?state=\(state)&code=fixture-code&iss=https%3A%2F%2Fid.example.invalid%2F")!
        XCTAssertThrowsError(try flow.consume(callback: callback,
                                               now: now.addingTimeInterval(301)))
        let fresh = try flow.begin()
        let freshState = URLComponents(url: fresh, resolvingAgainstBaseURL: false)!
            .queryItems!.first { $0.name == "state" }!.value!
        let otherScheme = URL(string: "org.evil.backupmanager:/oauth/callback" +
            "?state=\(freshState)&code=fixture-code&iss=https%3A%2F%2Fid.example.invalid%2F")!
        XCTAssertThrowsError(try flow.consume(callback: otherScheme))
    }

    func testStrictEndpointValidation() throws {
        XCTAssertThrowsError(try configuration(api: "http://127.0.0.1:8765/api/v1"))
        XCTAssertThrowsError(try configuration(api: "https://0.0.0.0:8765/api/v1"))
        XCTAssertThrowsError(try configuration(api: "https://localhost:8765/api/v1"))
        XCTAssertThrowsError(try configuration(redirect: "backupmanager:/oauth/callback"))
        XCTAssertThrowsError(try configuration(redirect: "org.example.backupmanager:/oauth/other"))
        XCTAssertThrowsError(try configuration(redirect: "org.example.backupmanager:/oauth/callback?next=evil"))
    }
}
