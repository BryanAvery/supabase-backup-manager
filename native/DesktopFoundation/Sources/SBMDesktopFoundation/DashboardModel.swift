import Foundation
import Combine
import SBMNativeAuth

@MainActor
public protocol DesktopClient: AnyObject {
    func signIn() async throws -> NativeIdentity
    func restoreSession() async throws -> NativeIdentity?
    func authorisedProjects() async throws -> [ProjectSummary]
    func signOut() async throws -> Bool
}

extension NativeSignInClient: DesktopClient {}

public enum DesktopPhase: Equatable {
    case signedOut
    case signedIn
    case expired
}

/// The UI is not an authority. Only the Go API returns authorised projects.
/// Every failure clears project rows so stale or synthetic data cannot appear.
@MainActor
public final class DashboardModel: ObservableObject {
    @Published public private(set) var phase: DesktopPhase = .signedOut
    @Published public private(set) var projects: [ProjectSummary] = []
    @Published public private(set) var identity: NativeIdentity?
    @Published public private(set) var isBusy = false
    @Published public private(set) var message: String?

    private let client: any DesktopClient

    public init(client: any DesktopClient) {
        self.client = client
    }

    public var canSignIn: Bool { !isBusy && phase != .signedIn }
    public var canRefresh: Bool { !isBusy && phase == .signedIn }

    private func begin() -> Bool {
        guard !isBusy else { return false }
        isBusy = true
        message = nil
        return true
    }

    public func restore() async {
        guard begin() else { return }
        defer { isBusy = false }
        clearSessionUI()
        do {
            guard let restored = try await client.restoreSession() else {
                phase = .signedOut
                return
            }
            guard restored.expiresAt > Date() else {
                phase = .expired
                return
            }
            identity = restored
            phase = .signedIn
            await fetchProjects()
        } catch {
            clearSessionUI()
            if isAuthFailure(error) {
                phase = .expired
                message = "Your session has expired. Sign in again."
            } else {
                phase = .signedOut
                message = "Could not verify the previous session. Check the local service and try again."
            }
        }
    }

    public func signIn() async {
        guard begin() else { return }
        defer { isBusy = false }
        clearSessionUI()
        do {
            let verified = try await client.signIn()
            guard verified.expiresAt > Date() else {
                phase = .expired
                message = "The new session has already expired."
                return
            }
            identity = verified
            phase = .signedIn
            await fetchProjects()
        } catch {
            clearSessionUI()
            phase = .signedOut
            message = "Sign-in could not be completed. Check your identity provider and try again."
        }
    }

    public func refresh() async {
        guard phase == .signedIn && begin() else { return }
        defer { isBusy = false }
        await fetchProjects()
    }

    public func signOut() async {
        guard begin() else { return }
        defer { isBusy = false }
        do {
            let confirmed = try await client.signOut()
            clearSessionUI()
            phase = .signedOut
            if !confirmed {
                message = "Signed out on this Mac. Server revocation could not be confirmed; the token will expire shortly."
            }
        } catch {
            // Do not claim logout if the local Keychain could not be cleared.
            projects = []
            message = "Could not clear stored credentials. Please retry sign-out."
        }
    }

    public func checkExpiry(now: Date = Date()) {
        guard let expiry = identity?.expiresAt, now >= expiry else { return }
        clearSessionUI()
        phase = .expired
        message = "Your session has expired. Sign in again."
    }

    private func fetchProjects() async {
        projects = []
        do {
            let fresh = try await client.authorisedProjects()
            // Show only the current response, never a previous account's rows.
            if phase == .signedIn {
                projects = fresh.sorted {
                    $0.name.localizedStandardCompare($1.name) == .orderedAscending
                }
            }
        } catch {
            projects = []
            if isAuthFailure(error) {
                clearSessionUI()
                phase = .expired
                message = "Access has expired or been revoked. Sign in again."
            } else {
                message = "Projects could not be loaded. No cached project data is shown."
            }
        }
    }

    private func clearSessionUI() {
        identity = nil
        projects = []
        phase = .signedOut
    }

    private func isAuthFailure(_ error: Error) -> Bool {
        guard let authError = error as? NativeAuthError else { return false }
        switch authError {
        case .unauthorised, .credentialUnavailable, .invalidToken:
            return true
        default:
            return false
        }
    }
}
