import Foundation
import SBMNativeAuth

enum LocalConfigurationError: Error {
    case missing
    case unsafeFile
    case invalid
}

/// This contains PUBLIC identity-provider metadata and a PUBLIC RSA verification
/// key only. No access tokens, client secrets or signing private keys are loaded.
enum LocalIdentityConfiguration {
    private struct PublicSettings: Decodable {
        let issuer: String
        let authorizationEndpoint: String
        let tokenEndpoint: String
        let redirectURI: String
        let apiBaseURL: String
        let clientID: String
        let apiScope: String
        let requiredACR: String
        let signingKeyID: String
        let idTokenRSAPublicKeyDERBase64: String
    }

    static let registeredScheme = "org.example.supabasebackupmanager"

    static func configurationURL() throws -> URL {
        guard let base = FileManager.default.urls(
            for: .applicationSupportDirectory, in: .userDomainMask
        ).first else { throw LocalConfigurationError.missing }
        return base.appendingPathComponent("SupabaseBackupManager", isDirectory: true)
            .appendingPathComponent("oidc.json", isDirectory: false)
    }

    static func load() throws -> NativeAuthConfiguration {
        let url = try configurationURL()
        guard FileManager.default.fileExists(atPath: url.path) else {
            throw LocalConfigurationError.missing
        }
        let resource = try url.resourceValues(forKeys: [
            .isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey
        ])
        guard resource.isRegularFile == true,
              resource.isSymbolicLink != true,
              let size = resource.fileSize, size > 0, size <= 16384,
              let attributes = try FileManager.default.attributesOfItem(
                atPath: url.path
              )[.posixPermissions] as? NSNumber,
              attributes.intValue & 0o077 == 0 else {
            throw LocalConfigurationError.unsafeFile
        }
        let data = try Data(contentsOf: url)
        guard data.count <= 16384,
              let settings = try? JSONDecoder().decode(PublicSettings.self, from: data),
              let issuer = URL(string: settings.issuer),
              let authorization = URL(string: settings.authorizationEndpoint),
              let token = URL(string: settings.tokenEndpoint),
              let callback = URL(string: settings.redirectURI),
              let api = URL(string: settings.apiBaseURL),
              callback.scheme == registeredScheme,
              Bundle.main.bundleIdentifier == registeredScheme,
              let publicKey = Data(base64Encoded: settings.idTokenRSAPublicKeyDERBase64)
        else {
            throw LocalConfigurationError.invalid
        }

        return try NativeAuthConfiguration(
            issuer: issuer,
            authorizationEndpoint: authorization,
            tokenEndpoint: token,
            redirectURI: callback,
            apiBaseURL: api,
            clientID: settings.clientID,
            apiScope: settings.apiScope,
            requiredACR: settings.requiredACR,
            signingKeyID: settings.signingKeyID,
            idTokenRSAPublicKeyDER: publicKey
        )
    }
}
