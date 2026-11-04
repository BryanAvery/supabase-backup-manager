import XCTest
import Foundation
import CryptoKit
import Security
@testable import SBMNativeAuth

final class MemoryCredentialStore: CredentialStorage {
    var saved: AccessCredential?
    func save(_ credential: AccessCredential) throws { saved = credential }
    func load() throws -> AccessCredential? { saved }
    func clear() throws { saved = nil }
}

final class SimulatedIdentityProvider {
    var nonce = ""
    var challenge = ""
    var wrongSubject = false
    var wrongNonce = false
    var badSignature = false
    var wrongMFA = false
    var denyAPI = false
    var revokeAccepted = true
    var lastExchangeIsPKCE = false
    var lastProjectRequestIsBearer = false
}

@MainActor
final class SimulatedNativeBrowser: BrowserAuthorizer {
    let provider: SimulatedIdentityProvider
    let callback: URL
    init(provider: SimulatedIdentityProvider, callback: URL) {
        self.provider = provider
        self.callback = callback
    }
    func authorize(_ url: URL, callbackScheme: String) async throws -> URL {
        XCTAssertEqual(callbackScheme, callback.scheme)
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)!.queryItems!
        func item(_ name: String) -> String {
            items.first(where: { $0.name == name })!.value!
        }
        provider.nonce = item("nonce")
        provider.challenge = item("code_challenge")
        XCTAssertEqual(item("code_challenge_method"), "S256")
        XCTAssertEqual(item("acr_values"), "urn:example:mfa")
        var result = URLComponents(url: callback, resolvingAgainstBaseURL: false)!
        result.queryItems = [
            URLQueryItem(name: "code", value: "synthetic-code"),
            URLQueryItem(name: "state", value: item("state")),
            URLQueryItem(name: "iss", value: "https://id.example.invalid/")
        ]
        return result.url!
    }
}

final class SimulatedTransport: SecureHTTPTransport {
    let provider: SimulatedIdentityProvider
    let signingKey: SecKey

    init(provider: SimulatedIdentityProvider, signingKey: SecKey) {
        self.provider = provider
        self.signingKey = signingKey
    }

    private func signedIDToken() throws -> String {
        let now = Int(Date().timeIntervalSince1970)
        let header: [String: Any] = [
            "alg": "RS256", "typ": "JWT", "kid": "fixture-rsa"
        ]
        let payload: [String: Any] = [
            "iss": "https://id.example.invalid/",
            "sub": provider.wrongSubject ? "different-user" : "fixture-user",
            "aud": "native-fixture-client",
            "nonce": provider.wrongNonce ? "wrong-nonce" : provider.nonce,
            "acr": provider.wrongMFA ? "pwd" : "urn:example:mfa",
            "iat": now - 10,
            "exp": now + 300
        ]
        let head = try JSONSerialization.data(withJSONObject: header).base64URLEncoded()
        let body = try JSONSerialization.data(withJSONObject: payload).base64URLEncoded()
        let signed = Data((head + "." + body).utf8)
        guard let cfSignature = SecKeyCreateSignature(
            signingKey, .rsaSignatureMessagePKCS1v15SHA256, signed as CFData, nil
        ) else { throw NativeAuthError.invalidToken }
        let signature = (cfSignature as Data).base64URLEncoded()
        if provider.badSignature { return head + "." + body + "." + String(repeating: "A", count: signature.count) }
        return head + "." + body + "." + signature
    }

    func perform(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        guard let url = request.url, url.scheme == "https" else {
            throw NativeAuthError.invalidConfiguration
        }
        func reply(_ body: Data, status: Int) -> (Data, HTTPURLResponse) {
            let response = HTTPURLResponse(
                url: url, statusCode: status, httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            return (body, response)
        }
        if url.path == "/token" {
            XCTAssertEqual(request.httpMethod, "POST")
            let form = String(data: request.httpBody!, encoding: .utf8)!
            let params = URLComponents(string: "https://id.example.invalid/?" + form)!.queryItems!
            func field(_ name: String) -> String {
                params.first(where: { $0.name == name })!.value!
            }
            let calculated = Data(SHA256.hash(data: Data(field("code_verifier").utf8))).base64URLEncoded()
            provider.lastExchangeIsPKCE = calculated == provider.challenge
            XCTAssertTrue(provider.lastExchangeIsPKCE)
            XCTAssertEqual(field("code"), "synthetic-code")
            XCTAssertEqual(field("grant_type"), "authorization_code")
            XCTAssertEqual(field("client_id"), "native-fixture-client")
            let token: [String: Any] = [
                "token_type": "Bearer",
                "access_token": "synthetic-access-token",
                "id_token": try signedIDToken(),
                "expires_in": 240
            ]
            return reply(try JSONSerialization.data(withJSONObject: token), status: 200)
        }
        guard request.value(forHTTPHeaderField: "Authorization") == "Bearer synthetic-access-token" else {
            return reply(Data(), status: 401)
        }
        if url.path == "/api/v1/me" {
            let json = provider.denyAPI ?
                #"{"error":"authentication_required"}"# :
                #"{"user_id":"local-user","subject":"fixture-user"}"#
            return reply(Data(json.utf8), status: provider.denyAPI ? 401 : 200)
        }
        if url.path == "/api/v1/projects" {
            provider.lastProjectRequestIsBearer = true
            return reply(Data(#"[{"id":"aab77233-d8ec-45f4-9da9-eaf8e1a29cca","name":"fixture-project","environment":"cloud","readiness":"unknown"}]"#.utf8), status: 200)
        }
        if url.path == "/api/v1/sessions/revoke" {
            XCTAssertEqual(request.httpMethod, "POST")
            return reply(Data(), status: provider.revokeAccepted ? 204 : 503)
        }
        return reply(Data(), status: 404)
    }
}

@MainActor
final class NativeSignInClientTests: XCTestCase {
    private func fixture() throws -> (
        NativeSignInClient, MemoryCredentialStore, SimulatedIdentityProvider
    ) {
        let attrs: [String: Any] = [
            kSecAttrKeyType as String: kSecAttrKeyTypeRSA,
            kSecAttrKeySizeInBits as String: 2048
        ]
        guard let signingKey = SecKeyCreateRandomKey(attrs as CFDictionary, nil),
              let publicKey = SecKeyCopyPublicKey(signingKey),
              let raw = SecKeyCopyExternalRepresentation(publicKey, nil) else {
            throw NativeAuthError.invalidToken
        }
        let config = try NativeAuthConfiguration(
            issuer: URL(string: "https://id.example.invalid/")!,
            authorizationEndpoint: URL(string: "https://id.example.invalid/authorize")!,
            tokenEndpoint: URL(string: "https://id.example.invalid/token")!,
            redirectURI: URL(string: "org.example.backupmanager:/oauth/callback")!,
            apiBaseURL: URL(string: "https://127.0.0.1:8765/api/v1")!,
            clientID: "native-fixture-client",
            apiScope: "api.read",
            requiredACR: "urn:example:mfa",
            signingKeyID: "fixture-rsa",
            idTokenRSAPublicKeyDER: raw as Data
        )
        let provider = SimulatedIdentityProvider()
        let browser = SimulatedNativeBrowser(provider: provider, callback: config.redirectURI)
        let credentials = MemoryCredentialStore()
        let client = NativeSignInClient(
            configuration: config, browser: browser,
            transport: SimulatedTransport(provider: provider, signingKey: signingKey),
            credentials: credentials
        )
        return (client, credentials, provider)
    }

    func testVerifiedSignInProjectsAndRemoteLogout() async throws {
        let (client, credentials, provider) = try fixture()
        let identity = try await client.signIn()
        XCTAssertEqual(identity.subject, "fixture-user")
        XCTAssertEqual(identity.userID, "local-user")
        XCTAssertTrue(provider.lastExchangeIsPKCE)
        XCTAssertEqual(try credentials.load()?.accessToken, "synthetic-access-token")
        XCTAssertTrue(identity.expiresAt > Date())
        let projects = try await client.authorisedProjectsJSON()
        XCTAssertTrue(provider.lastProjectRequestIsBearer)
        XCTAssertTrue(String(data: projects, encoding: .utf8)!.contains("fixture-project"))
        let revoked = try await client.signOut()
        XCTAssertTrue(revoked)
        XCTAssertNil(try credentials.load())
        await expectAuthFailure { try await client.authorisedProjectsJSON() }
    }

    func testWrongIDSubjectNeverStoresCredential() async throws {
        let (client, credentials, provider) = try fixture()
        provider.wrongSubject = true
        await expectAuthFailure { try await client.signIn() }
        XCTAssertNil(try credentials.load())
    }

    func testInvalidNonceOrSignatureOrMFAFailsClosed() async throws {
        for violation in ["nonce", "signature", "MFA", "API"] {
            let (client, credentials, provider) = try fixture()
            switch violation {
            case "nonce": provider.wrongNonce = true
            case "signature": provider.badSignature = true
            case "MFA": provider.wrongMFA = true
            default: provider.denyAPI = true
            }
            await expectAuthFailure { try await client.signIn() }
            XCTAssertNil(try credentials.load(), "saved token for invalid \(violation)")
        }
    }

    func testFailedAccountSwitchClearsOldKeychainCredential() async throws {
        let (client, credentials, provider) = try fixture()
        try credentials.save(AccessCredential(
            accessToken: "previous-user-token",
            expiresAt: Date().addingTimeInterval(120)
        ))
        provider.wrongMFA = true
        await expectAuthFailure { try await client.signIn() }
        XCTAssertNil(try credentials.load(), "failed account switch retained old credentials")
    }

    func testRestoreRequiresLiveVerifiedAPIIdentity() async throws {
        let (client, credentials, provider) = try fixture()
        try credentials.save(AccessCredential(
            accessToken: "synthetic-access-token",
            expiresAt: Date().addingTimeInterval(120)
        ))
        let restored = try await client.restoreSession()
        XCTAssertEqual(restored?.userID, "local-user")
        XCTAssertEqual(restored?.subject, "fixture-user")
        let projects = try await client.authorisedProjects()
        XCTAssertEqual(projects.count, 1)
        XCTAssertEqual(projects[0].environment, .cloud)
        XCTAssertEqual(projects[0].readiness, .unknown)
        provider.denyAPI = true
        await expectAuthFailure { try await client.restoreSession() }
        XCTAssertNil(try credentials.load())
    }

    func testExpiredStoredBearerIsNeverRestoredOrUsed() async throws {
        let (client, credentials, _) = try fixture()
        try credentials.save(AccessCredential(
            accessToken: "synthetic-access-token",
            expiresAt: Date().addingTimeInterval(-30)
        ))
        let restored = try await client.restoreSession()
        XCTAssertNil(restored)
        XCTAssertNil(try credentials.load())
        await expectAuthFailure { try await client.authorisedProjects() }
    }

    func testLocalLogoutEvenIfRemoteRevocationFails() async throws {
        let (client, credentials, provider) = try fixture()
        _ = try await client.signIn()
        provider.revokeAccepted = false
        let revoked = try await client.signOut()
        XCTAssertFalse(revoked)
        XCTAssertNil(try credentials.load())
    }
}

@MainActor
func expectAuthFailure<T>(_ action: () async throws -> T,
                          file: StaticString = #filePath, line: UInt = #line) async {
    do {
        _ = try await action()
        XCTFail("Expected secure operation to fail", file: file, line: line)
    } catch {}
}
