import Foundation
import Security

public struct VerifiedIDToken {
    public let subject: String
    public let expiresAt: Date
}

/// Verifies OIDC ID-token signatures using a *pinned*, public PKCS#1 RSA key.
/// Untrusted JWT headers never cause network key retrieval.
public struct IDTokenVerifier {
    private let configuration: NativeAuthConfiguration

    public init(configuration: NativeAuthConfiguration) {
        self.configuration = configuration
    }

    public func verify(_ raw: String, expectedNonce: String,
                       now: Date = Date()) throws -> VerifiedIDToken {
        guard raw.utf8.count <= 8192 else { throw NativeAuthError.invalidToken }
        let parts = raw.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count == 3,
              let headerData = Data(base64URL: String(parts[0])),
              let payloadData = Data(base64URL: String(parts[1])),
              let signature = Data(base64URL: String(parts[2])),
              headerData.count <= 2048, payloadData.count <= 4096,
              let header = try? JSONSerialization.jsonObject(with: headerData) as? [String: Any],
              let claims = try? JSONSerialization.jsonObject(with: payloadData) as? [String: Any],
              header["alg"] as? String == "RS256",
              header["kid"] as? String == configuration.signingKeyID,
              (header["typ"] == nil || header["typ"] as? String == "JWT") else {
            throw NativeAuthError.invalidToken
        }
        for untrusted in ["jwk", "jku", "x5u", "x5c", "crit"] {
            guard header[untrusted] == nil else { throw NativeAuthError.invalidToken }
        }
        let attributes: [String: Any] = [
            kSecAttrKeyType as String: kSecAttrKeyTypeRSA,
            kSecAttrKeyClass as String: kSecAttrKeyClassPublic,
            kSecAttrKeySizeInBits as String: 2048
        ]
        guard let publicKey = SecKeyCreateWithData(
            configuration.idTokenRSAPublicKeyDER as CFData,
            attributes as CFDictionary, nil
        ), SecKeyIsAlgorithmSupported(
            publicKey, .verify, .rsaSignatureMessagePKCS1v15SHA256
        ) else { throw NativeAuthError.invalidToken }
        let message = Data((String(parts[0]) + "." + String(parts[1])).utf8)
        guard SecKeyVerifySignature(publicKey, .rsaSignatureMessagePKCS1v15SHA256,
                                     message as CFData, signature as CFData, nil) else {
            throw NativeAuthError.invalidToken
        }
        guard let issuer = claims["iss"] as? String,
              issuer == configuration.issuer.absoluteString,
              let subject = claims["sub"] as? String,
              !subject.isEmpty, subject.count <= 256,
              let nonce = claims["nonce"] as? String,
              nonce == expectedNonce,
              claims["acr"] as? String == configuration.requiredACR,
              let issuedAt = claims["iat"] as? NSNumber,
              let expiry = claims["exp"] as? NSNumber else {
            throw NativeAuthError.invalidToken
        }
        let issued = Date(timeIntervalSince1970: issuedAt.doubleValue)
        let expires = Date(timeIntervalSince1970: expiry.doubleValue)
        guard issued <= now.addingTimeInterval(30),
              issued <= expires, expires > now else {
            throw NativeAuthError.invalidToken
        }
        let audiences: [String]
        if let array = claims["aud"] as? [String] { audiences = array }
        else if let single = claims["aud"] as? String { audiences = [single] }
        else { throw NativeAuthError.invalidToken }
        guard audiences.contains(configuration.clientID) else {
            throw NativeAuthError.invalidToken
        }
        let azp = claims["azp"] as? String
        if audiences.count > 1 && azp != configuration.clientID {
            throw NativeAuthError.invalidToken
        }
        if let azp, azp != configuration.clientID {
            throw NativeAuthError.invalidToken
        }
        return VerifiedIDToken(subject: subject, expiresAt: expires)
    }
}
