import Combine
import Foundation
import SwiftUI
import SBMDesktopFoundation
import SBMNativeAuth

struct ProjectDashboardView: View {
    @ObservedObject var model: DashboardModel

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .center, spacing: 14) {
                Image(systemName: "externaldrive.fill")
                    .font(.system(size: 28))
                    .foregroundStyle(.blue)
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 3) {
                    Text("Supabase Backup Manager")
                        .font(.title2.bold())
                    Text("Local catalogue • Platform foundation")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                if model.phase == .signedIn {
                    Button {
                        Task { await model.refresh() }
                    } label: {
                        Label("Refresh", systemImage: "arrow.clockwise")
                    }
                    .disabled(!model.canRefresh)
                    Button {
                        Task { await model.signOut() }
                    } label: {
                        Label("Sign out", systemImage: "rectangle.portrait.and.arrow.right")
                    }
                    .disabled(model.isBusy)
                }
            }
            .padding(26)

            Divider()

            Group {
                if model.phase == .signedIn {
                    projectArea
                } else {
                    signedOutArea
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)

            Divider()
            HStack {
                Label("Local access only", systemImage: "lock.shield")
                Spacer()
                Text("Backup and restore functions are not enabled in M1.")
            }
            .font(.footnote)
            .foregroundStyle(.secondary)
            .padding(.horizontal, 26)
            .padding(.vertical, 14)
        }
        .onReceive(Timer.publish(every: 5, on: .main, in: .common).autoconnect()) { _ in
            model.checkExpiry()
        }
    }

    private var signedOutArea: some View {
        VStack(spacing: 16) {
            Image(systemName: model.phase == .expired ?
                  "clock.badge.exclamationmark" : "person.crop.circle.badge.checkmark")
                .font(.system(size: 46))
                .foregroundStyle(.secondary)
                .accessibilityHidden(true)

            Text(model.phase == .expired ? "Session expired" : "Sign in securely")
                .font(.title2.bold())
            Text(model.phase == .expired
                 ? "Sign in again to refresh your project permissions."
                 : "Use the approved identity provider to access your local project catalogue.")
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
                .frame(maxWidth: 450)

            if model.isBusy {
                ProgressView("Checking your session…")
            } else {
                Button {
                    Task { await model.signIn() }
                } label: {
                    Label("Sign in", systemImage: "lock.open")
                        .padding(.horizontal, 8)
                }
                .buttonStyle(.borderedProminent)
                .disabled(!model.canSignIn)

                Button("Check previous session") {
                    Task { await model.restore() }
                }
                .buttonStyle(.link)
            }

            if let message = model.message {
                messageView(message)
            }
        }
        .padding(32)
    }

    private var projectArea: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack {
                VStack(alignment: .leading, spacing: 5) {
                    Text("Your projects")
                        .font(.title3.bold())
                    if let user = model.identity {
                        Text("Verified account: \(user.userID)")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                            .textSelection(.enabled)
                        Text("Session expires: \(user.expiresAt.formatted(date: .abbreviated, time: .shortened))")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
                Spacer()
                Text("Read-only")
                    .font(.caption.weight(.medium))
                    .foregroundStyle(.secondary)
            }

            if let message = model.message {
                messageView(message)
            }

            if model.isBusy {
                ProgressView("Checking authorised projects…")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if model.projects.isEmpty {
                VStack(spacing: 10) {
                    Image(systemName: "folder.badge.questionmark")
                        .font(.largeTitle)
                        .foregroundStyle(.secondary)
                    Text("No projects to display")
                        .font(.headline)
                    Text("No authorised projects were returned by the local service.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List(model.projects) { project in
                    HStack(spacing: 14) {
                        Image(systemName: "square.stack.3d.up")
                            .foregroundStyle(.secondary)
                            .font(.title3)
                        VStack(alignment: .leading, spacing: 5) {
                            Text(project.name)
                                .font(.headline)
                            Text(project.id.uuidString.lowercased())
                                .font(.caption.monospaced())
                                .foregroundStyle(.secondary)
                                .textSelection(.enabled)
                        }
                        Spacer()
                        VStack(alignment: .trailing, spacing: 5) {
                            Text(project.environment == .cloud ? "Supabase Cloud" : "Self-hosted")
                                .font(.subheadline)
                            Text(readinessLabel(project.readiness))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .padding(.vertical, 10)
                    .accessibilityElement(children: .combine)
                }
                .listStyle(.inset)
            }
        }
        .padding(26)
    }

    private func readinessLabel(_ readiness: ProjectReadiness?) -> String {
        switch readiness ?? .unknown {
        case .unknown:
            return "Recovery not assessed"
        case .recoverable:
            return "Catalogue assessment: recoverable"
        case .manualSteps:
            return "Manual recovery steps"
        case .atRisk:
            return "Recovery at risk"
        }
    }

    private func messageView(_ message: String) -> some View {
        Label(message, systemImage: "info.circle")
            .font(.callout)
            .foregroundStyle(.secondary)
            .padding(12)
            .frame(maxWidth: 540, alignment: .leading)
            .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10))
            .accessibilityLabel(message)
    }
}
