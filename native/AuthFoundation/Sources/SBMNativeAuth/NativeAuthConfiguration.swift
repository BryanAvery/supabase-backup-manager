import Foundation

public enum NativeAuthError: Error, Equatable {
    case invalidConfiguration
    case noPendingLogin
    case invalidCallback
    case expiredLogin
    case invalidToken
    case identityMismatch
    case unauthorised
    case invalidServerResponse
    case credentialUnavailable
    case keychainFailure(OSStatus)
}

public struct NativeAuthConfiguration {
    public let issuer: URL
    public let authorizationEndpoint: URL
    public let tokenEndpoint: URL
    public let redirectURI: URL
    public let apiBaseURL: URL
    public let clientID: String
    public let apiScope: String
    public let requiredACR: String
    public let signingKeyID: String
    /// PKCS#1 DER RSA public key for the pinned OIDC ID-token signing key.
    /// Never supply an IdP private key.
    public let idTokenRSAPublicKeyDER: Data

    public init(
        issuer: URL,
        authorizationEndpoint: URL,
        tokenEndpoint: URL,
        redirectURI: URL,
        apiBaseURL: URL,
        clientID: String,
        apiScope: String,
        requiredACR: String,
        signingKeyID: String,
        idTokenRSAPublicKeyDER: Data
    ) throws {
        func httpsURL(_ url: URL) -> Bool {
            guard let u = URLComponents(url: url, resolvingAgainstBaseURL: false) else { return false }
            return u.scheme == "https" && u.host != nil &&
                u.user == nil && u.password == nil &&
                u.query == nil && u.fragment == nil
        }
        guard httpsURL(issuer), httpsURL(authorizationEndpoint),
              httpsURL(tokenEndpoint), httpsURL(apiBaseURL) else {
            throw NativeAuthError.invalidConfiguration
        }
        guard authorizationEndpoint.host == issuer.host,
              authorizationEndpoint.port == issuer.port,
              tokenEndpoint.host == issuer.host,
              tokenEndpoint.port == issuer.port,
              apiBaseURL.path == "/api/v1",
              let host = apiBaseURL.host,
              (host == "127.0.0.1" || host == "::1"),
              apiBaseURL.port != nil else {
            throw NativeAuthError.invalidConfiguration
        }
        guard let callback = URLComponents(url: redirectURI, resolvingAgainstBaseURL: false),
              let scheme = callback.scheme,
              scheme.contains("."), scheme.count >= 10,
              scheme.range(of: "^[a-z][a-z0-9.+-]+$", options: .regularExpression) != nil,
              callback.host == nil, callback.path == "/oauth/callback",
              callback.user == nil, callback.password == nil,
              callback.query == nil, callback.fragment == nil else {
            throw NativeAuthError.invalidConfiguration
        }
        let fields = [clientID, apiScope, requiredACR, signingKeyID]
        guard fields.allSatisfy({ !$0.isEmpty && $0.count <= 256 &&
            $0.rangeOfCharacter(from: .whitespacesAndNewlines) == nil }),
            apiScope != "openid", apiScope != "offline_access",
            !apiScope.contains("openid"), !apiScope.contains("offline_access"),
            !idTokenRSAPublicKeyDER.isEmpty else {
            throw NativeAuthError.invalidConfiguration
        }
        self.issuer = issuer
        self.authorizationEndpoint = authorizationEndpoint
        self.tokenEndpoint = tokenEndpoint
        self.redirectURI = redirectURI
        self.apiBaseURL = apiBaseURL
        self.clientID = clientID
        self.apiScope = apiScope
        self.requiredACR = requiredACR
        self.signingKeyID = signingKeyID
        self.idTokenRSAPublicKeyDER = idTokenRSAPublicKeyDER
    }
}
