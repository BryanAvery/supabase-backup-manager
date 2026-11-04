import Foundation
import CryptoKit
import Security

public struct AuthorisationCode {
    public let value: String
    public let verifier: String
    public let nonce: String
}

@MainActor
public final class PKCEFlow {
    private struct Pending {
        let state: String
        let nonce: String
        let verifier: String
        let expires: Date
    }

    private let configuration: NativeAuthConfiguration
    private var pending: Pending?

    public init(configuration: NativeAuthConfiguration) {
        self.configuration = configuration
    }

    private static func secureRandom() throws -> String {
        var bytes = [UInt8](repeating: 0, count: 32)
        let result = bytes.withUnsafeMutableBytes {
            SecRandomCopyBytes(kSecRandomDefault, $0.count, $0.baseAddress!)
        }
        guard result == errSecSuccess else { throw NativeAuthError.credentialUnavailable }
        return Data(bytes).base64URLEncoded()
    }

    /// Replaces any outstanding transaction. No concurrent login flows.
    public func begin(now: Date = Date()) throws -> URL {
        pending = nil
        let state = try Self.secureRandom()
        let nonce = try Self.secureRandom()
        let verifier = try Self.secureRandom()
        pending = Pending(state: state, nonce: nonce, verifier: verifier,
                          expires: now.addingTimeInterval(300))
        let digest = SHA256.hash(data: Data(verifier.utf8))
        var url = URLComponents(url: configuration.authorizationEndpoint,
                                resolvingAgainstBaseURL: false)!
        url.queryItems = [
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "client_id", value: configuration.clientID),
            URLQueryItem(name: "redirect_uri", value: configuration.redirectURI.absoluteString),
            URLQueryItem(name: "scope", value: "openid " + configuration.apiScope),
            URLQueryItem(name: "state", value: state),
            URLQueryItem(name: "nonce", value: nonce),
            URLQueryItem(name: "acr_values", value: configuration.requiredACR),
            URLQueryItem(name: "code_challenge_method", value: "S256"),
            URLQueryItem(name: "code_challenge", value: Data(digest).base64URLEncoded())
        ]
        guard let result = url.url else { throw NativeAuthError.invalidConfiguration }
        return result
    }

    /// Consume-before-verify prevents replay even if callback verification fails.
    public func consume(callback: URL, now: Date = Date()) throws -> AuthorisationCode {
        guard let transaction = pending else { throw NativeAuthError.noPendingLogin }
        pending = nil
        guard now < transaction.expires else { throw NativeAuthError.expiredLogin }
        guard let received = URLComponents(url: callback, resolvingAgainstBaseURL: false),
              let expected = URLComponents(url: configuration.redirectURI,
                                           resolvingAgainstBaseURL: false),
              received.scheme == expected.scheme,
              received.host == expected.host, received.path == expected.path,
              received.user == nil, received.password == nil,
              received.fragment == nil else { throw NativeAuthError.invalidCallback }
        let groups = Dictionary(grouping: received.queryItems ?? [], by: \.name)
        func exactlyOne(_ key: String) -> String? {
            guard let items = groups[key], items.count == 1,
                  let value = items[0].value, !value.isEmpty else { return nil }
            return value
        }
        guard groups["error"] == nil,
              let code = exactlyOne("code"), code.count <= 4096,
              exactlyOne("state") == transaction.state,
              exactlyOne("iss") == configuration.issuer.absoluteString else {
            throw NativeAuthError.invalidCallback
        }
        return AuthorisationCode(value: code, verifier: transaction.verifier,
                                 nonce: transaction.nonce)
    }
}

extension Data {
    func base64URLEncoded() -> String {
        base64EncodedString().replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }
    init?(base64URL: String) {
        let converted = base64URL.replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        let padding = String(repeating: "=", count: (4 - converted.count % 4) % 4)
        self.init(base64Encoded: converted + padding)
    }
}
