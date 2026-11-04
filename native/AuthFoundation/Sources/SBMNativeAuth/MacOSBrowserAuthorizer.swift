import AppKit
import AuthenticationServices
import Foundation

/// Uses the system authentication browser, never an embedded web view.
/// The calling SwiftUI app must register its reverse-domain callback scheme
/// in CFBundleURLTypes and provide a real application window as the anchor.
@MainActor
public final class MacOSBrowserAuthorizer: NSObject,
    BrowserAuthorizer, ASWebAuthenticationPresentationContextProviding {

    private let window: NSWindow
    private var activeSession: ASWebAuthenticationSession?

    public init(window: NSWindow) {
        self.window = window
        super.init()
    }

    public func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        window
    }

    public func authorize(_ url: URL, callbackScheme: String) async throws -> URL {
        guard activeSession == nil else { throw NativeAuthError.invalidCallback }
        return try await withCheckedThrowingContinuation { continuation in
            let browser = ASWebAuthenticationSession(
                url: url, callbackURLScheme: callbackScheme
            ) { [weak self] result, error in
                Task { @MainActor in
                    self?.activeSession = nil
                    if let result {
                        continuation.resume(returning: result)
                    } else {
                        continuation.resume(throwing: error ?? NativeAuthError.invalidCallback)
                    }
                }
            }
            browser.presentationContextProvider = self
            browser.prefersEphemeralWebBrowserSession = true
            activeSession = browser
            if !browser.start() {
                activeSession = nil
                continuation.resume(throwing: NativeAuthError.credentialUnavailable)
            }
        }
    }
}
