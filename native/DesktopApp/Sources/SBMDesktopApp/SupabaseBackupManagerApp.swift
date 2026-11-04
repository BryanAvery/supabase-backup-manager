import AppKit
import Foundation
import SwiftUI
import SBMDesktopFoundation
import SBMNativeAuth

@MainActor
final class ApplicationCoordinator: ObservableObject {
    @Published private(set) var dashboard: DashboardModel?
    @Published private(set) var setupMessage: String?
    private var configured = false

    func connect(window: NSWindow) {
        guard !configured else { return }
        configured = true
        do {
            let settings = try LocalIdentityConfiguration.load()
            let keychain = try KeychainCredentialStore(
                service: LocalIdentityConfiguration.registeredScheme
            )
            let browser = MacOSBrowserAuthorizer(window: window)
            let native = NativeSignInClient(
                configuration: settings, browser: browser, credentials: keychain
            )
            let model = DashboardModel(client: native)
            dashboard = model
            Task { await model.restore() }
        } catch LocalConfigurationError.missing {
            setupMessage = "Sign-in has not been configured. Install the approved public identity-provider settings before using this app."
        } catch {
            setupMessage = "Sign-in is unavailable because the local configuration could not be verified. No accounts or projects have been loaded."
        }
    }
}

/// Resolves the real NSWindow once. The system browser requires a concrete
/// presentation anchor and cannot safely attach to an invented dummy window.
private struct WindowReader: NSViewRepresentable {
    let onWindow: @MainActor (NSWindow) -> Void

    func makeNSView(context: Context) -> WindowProbe {
        WindowProbe(onWindow: onWindow)
    }

    func updateNSView(_ view: WindowProbe, context: Context) {}

    final class WindowProbe: NSView {
        let onWindow: @MainActor (NSWindow) -> Void

        init(onWindow: @escaping @MainActor (NSWindow) -> Void) {
            self.onWindow = onWindow
            super.init(frame: .zero)
        }

        required init?(coder: NSCoder) { nil }

        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            guard window != nil else { return }
            DispatchQueue.main.async { [weak self] in
                guard let self, let window = self.window else { return }
                self.onWindow(window)
            }
        }
    }
}

struct DesktopRootView: View {
    @ObservedObject var coordinator: ApplicationCoordinator

    var body: some View {
        Group {
            if let dashboard = coordinator.dashboard {
                ProjectDashboardView(model: dashboard)
            } else {
                VStack(spacing: 18) {
                    Image(systemName: "lock.shield")
                        .font(.system(size: 42))
                        .foregroundStyle(.secondary)
                    Text("Supabase Backup Manager")
                        .font(.title2.bold())
                    Text(coordinator.setupMessage ?? "Preparing secure sign-in…")
                        .multilineTextAlignment(.center)
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: 470)
                    Text("No backup or recovery operations are available in this build.")
                        .font(.footnote)
                        .foregroundStyle(.tertiary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .frame(minWidth: 820, minHeight: 540)
        .background(WindowReader(onWindow: { window in
            coordinator.connect(window: window)
        }).frame(width: 0, height: 0))
    }
}

@main
struct SupabaseBackupManagerApp: App {
    @StateObject private var coordinator = ApplicationCoordinator()

    var body: some Scene {
        Window("Supabase Backup Manager", id: "main") {
            DesktopRootView(coordinator: coordinator)
        }
        .defaultSize(width: 960, height: 640)
    }
}
