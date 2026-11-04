import Foundation

@MainActor
public protocol BrowserAuthorizer {
    func authorize(_ url: URL, callbackScheme: String) async throws -> URL
}

public protocol SecureHTTPTransport {
    func perform(_ request: URLRequest) async throws -> (Data, HTTPURLResponse)
}

private final class NoRedirectDelegate: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask,
                    willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest,
                    completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}

/// Trusts only certificates accepted by the operating system's TLS trust store.
/// Disables redirects so codes and bearer credentials cannot leave pinned URLs.
public final class StrictTLSHTTPClient: SecureHTTPTransport {
    private let delegate = NoRedirectDelegate()
    private let session: URLSession

    public init() {
        let settings = URLSessionConfiguration.ephemeral
        settings.timeoutIntervalForRequest = 8
        settings.timeoutIntervalForResource = 12
        settings.httpCookieAcceptPolicy = .never
        settings.httpShouldSetCookies = false
        session = URLSession(configuration: settings, delegate: delegate,
                             delegateQueue: nil)
    }

    public func perform(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        guard request.url?.scheme == "https" else {
            throw NativeAuthError.invalidConfiguration
        }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, data.count <= 16384 else {
            throw NativeAuthError.invalidServerResponse
        }
        return (data, http)
    }
}

/// Contract-checked representation of the M1 project-list response.
public enum ProjectEnvironment: String, Codable, Sendable {
    case cloud
    case selfHosted = "self_hosted"
}

public enum ProjectReadiness: String, Codable, Sendable {
    case recoverable
    case manualSteps = "manual_steps"
    case atRisk = "at_risk"
    case unknown
}

public struct ProjectSummary: Decodable, Identifiable, Equatable, Sendable {
    public let id: UUID
    public let name: String
    public let environment: ProjectEnvironment
    public let readiness: ProjectReadiness?

    public init(id: UUID, name: String,
                environment: ProjectEnvironment,
                readiness: ProjectReadiness?) {
        self.id = id
        self.name = name
        self.environment = environment
        self.readiness = readiness
    }
}

public struct NativeIdentity {
    public let userID: String
    public let subject: String
    public let expiresAt: Date
}

private struct TokenResponse: Decodable {
    let token_type: String
    let access_token: String
    let id_token: String
    let expires_in: Int
}

private struct WhoAmI: Decodable {
    let user_id: String
    let subject: String
}

/// Designed to be retained by the eventual SwiftUI application shell.
/// No user can sign in without explicit, reviewed provider registration.
@MainActor
public final class NativeSignInClient {
    private let configuration: NativeAuthConfiguration
    private let flow: PKCEFlow
    private let verifier: IDTokenVerifier
    private let browser: BrowserAuthorizer
    private let transport: SecureHTTPTransport
    private let credentials: CredentialStorage
    private var authenticating = false

    public init(configuration: NativeAuthConfiguration,
                browser: BrowserAuthorizer,
                transport: SecureHTTPTransport = StrictTLSHTTPClient(),
                credentials: CredentialStorage) {
        self.configuration = configuration
        self.flow = PKCEFlow(configuration: configuration)
        self.verifier = IDTokenVerifier(configuration: configuration)
        self.browser = browser
        self.transport = transport
        self.credentials = credentials
    }

    public func signIn() async throws -> NativeIdentity {
        guard !authenticating else { throw NativeAuthError.invalidCallback }
        authenticating = true
        defer { authenticating = false }

        // A failed account switch must not leave an earlier account active.
        // Clear before launching the external authentication browser.
        try credentials.clear()
        let loginURL = try flow.begin()
        let callback = try await browser.authorize(loginURL,
                                   callbackScheme: configuration.redirectURI.scheme!)
        let authorisation = try flow.consume(callback: callback)
        let form = [
            URLQueryItem(name: "grant_type", value: "authorization_code"),
            URLQueryItem(name: "client_id", value: configuration.clientID),
            URLQueryItem(name: "redirect_uri", value: configuration.redirectURI.absoluteString),
            URLQueryItem(name: "code", value: authorisation.value),
            URLQueryItem(name: "code_verifier", value: authorisation.verifier)
        ]
        var fields = URLComponents()
        fields.queryItems = form
        var exchange = URLRequest(url: configuration.tokenEndpoint)
        exchange.httpMethod = "POST"
        exchange.httpBody = fields.percentEncodedQuery?.data(using: .utf8)
        exchange.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        exchange.setValue("application/json", forHTTPHeaderField: "Accept")

        let (bytes, response) = try await transport.perform(exchange)
        guard response.statusCode == 200,
              response.value(forHTTPHeaderField: "Content-Type")?.lowercased()
                .hasPrefix("application/json") == true,
              bytes.count <= 16384,
              let token = try? JSONDecoder().decode(TokenResponse.self, from: bytes),
              token.token_type.lowercased() == "bearer",
              token.expires_in > 0 && token.expires_in <= 900,
              !token.access_token.isEmpty && token.access_token.count <= 8192,
              !token.id_token.isEmpty && token.id_token.count <= 8192 else {
            throw NativeAuthError.invalidServerResponse
        }

        let verifiedID = try verifier.verify(token.id_token, expectedNonce: authorisation.nonce)
        let (who, status) = try await authenticatedRequest(
            path: "me", token: token.access_token, method: "GET"
        )
        guard status == 200,
              let user = try? JSONDecoder().decode(WhoAmI.self, from: who),
              !user.user_id.isEmpty,
              user.subject == verifiedID.subject else {
            throw NativeAuthError.identityMismatch
        }

        // Never retain ID tokens, authorization codes, verifiers or refresh tokens.
        let expiry = min(Date().addingTimeInterval(TimeInterval(token.expires_in)),
                         verifiedID.expiresAt).addingTimeInterval(-5)
        guard expiry > Date() else { throw NativeAuthError.invalidToken }
        try credentials.save(AccessCredential(accessToken: token.access_token,
                                              expiresAt: expiry))
        return NativeIdentity(userID: user.user_id, subject: user.subject,
                              expiresAt: expiry)
    }

    private func authenticatedRequest(path: String, token: String,
                                      method: String) async throws -> (Data, Int) {
        let url = configuration.apiBaseURL.appendingPathComponent(path)
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        let (data, response) = try await transport.perform(request)
        return (data, response.statusCode)
    }

    /// The Go service rechecks identity and project membership on each call.
    public func authorisedProjectsJSON() async throws -> Data {
        guard let token = try credentials.load() else {
            throw NativeAuthError.credentialUnavailable
        }
        let (body, status) = try await authenticatedRequest(
            path: "projects", token: token.accessToken, method: "GET"
        )
        if status == 401 {
            try credentials.clear()
            throw NativeAuthError.unauthorised
        }
        guard status == 200 else { throw NativeAuthError.invalidServerResponse }
        return body
    }

    /// On app relaunch, recheck the stored token against the live Go verifier.
    /// No previous user or project state is trusted from local preferences.
    public func restoreSession() async throws -> NativeIdentity? {
        guard let credential = try credentials.load() else { return nil }
        let (body, status) = try await authenticatedRequest(
            path: "me", token: credential.accessToken, method: "GET"
        )
        if status == 401 || status == 403 {
            try credentials.clear()
            throw NativeAuthError.unauthorised
        }
        guard status == 200,
              let user = try? JSONDecoder().decode(WhoAmI.self, from: body),
              !user.user_id.isEmpty, !user.subject.isEmpty else {
            throw NativeAuthError.invalidServerResponse
        }
        return NativeIdentity(userID: user.user_id, subject: user.subject,
                              expiresAt: credential.expiresAt)
    }

    /// The first M1 desktop shell is read-only. Strictly decode the SRS/OpenAPI
    /// project contract; never substitute fixture projects on network failure.
    public func authorisedProjects() async throws -> [ProjectSummary] {
        let data = try await authorisedProjectsJSON()
        return try JSONDecoder().decode([ProjectSummary].self, from: data)
    }

    /// Always clears local credentials. Returns false if the server could not
    /// confirm revocation; the remote bearer may remain usable until expiry.
    @discardableResult
    public func signOut() async throws -> Bool {
        guard let token = try credentials.load() else {
            try credentials.clear()
            return false
        }
        let confirmed: Bool
        if let (_, status) = try? await authenticatedRequest(
            path: "sessions/revoke", token: token.accessToken, method: "POST"
        ) {
            confirmed = status == 204
        } else {
            confirmed = false
        }
        try credentials.clear()
        return confirmed
    }
}
