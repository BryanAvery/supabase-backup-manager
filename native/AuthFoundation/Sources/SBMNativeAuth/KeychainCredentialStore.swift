import Foundation
import Security

public struct AccessCredential: Codable {
    public let accessToken: String
    public let expiresAt: Date

    public init(accessToken: String, expiresAt: Date) {
        self.accessToken = accessToken
        self.expiresAt = expiresAt
    }
}

public protocol CredentialStorage {
    func save(_ credential: AccessCredential) throws
    func load() throws -> AccessCredential?
    func clear() throws
}

/// Device-local Keychain item, inaccessible while the macOS user is locked.
/// No iCloud synchronisation, shared preference files, refresh tokens or
/// application-bundled credentials are used.
public final class KeychainCredentialStore: CredentialStorage {
    private let service: String
    private let account = "api-access-token"

    public init(service: String) throws {
        guard !service.isEmpty, service.contains(".") else {
            throw NativeAuthError.invalidConfiguration
        }
        self.service = service
    }

    private func query() -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecUseDataProtectionKeychain as String: true
        ]
    }

    public func save(_ credential: AccessCredential) throws {
        guard credential.expiresAt > Date(),
              !credential.accessToken.isEmpty else {
            throw NativeAuthError.invalidToken
        }
        let data = try JSONEncoder().encode(credential)
        var attributes = query()
        attributes[kSecValueData as String] = data
        attributes[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        attributes[kSecAttrSynchronizable as String] = false
        let status = SecItemAdd(attributes as CFDictionary, nil)
        if status == errSecDuplicateItem {
            let update = [kSecValueData as String: data]
            let result = SecItemUpdate(query() as CFDictionary, update as CFDictionary)
            guard result == errSecSuccess else { throw NativeAuthError.keychainFailure(result) }
        } else if status != errSecSuccess {
            throw NativeAuthError.keychainFailure(status)
        }
    }

    public func load() throws -> AccessCredential? {
        var request = query()
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(request as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let data = result as? Data else {
            throw NativeAuthError.keychainFailure(status)
        }
        let credential = try JSONDecoder().decode(AccessCredential.self, from: data)
        if credential.expiresAt <= Date() {
            try clear()
            return nil
        }
        return credential
    }

    public func clear() throws {
        let status = SecItemDelete(query() as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw NativeAuthError.keychainFailure(status)
        }
    }
}
