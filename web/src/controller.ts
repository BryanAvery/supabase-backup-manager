import { APIError, type DashboardAPI, type Project, type Session } from "./api.js";

export type Phase =
  | "loading"
  | "signed_out"
  | "signed_in"
  | "expired"
  | "unavailable"
  | "signout_failed";

export interface DashboardState {
  readonly phase: Phase;
  readonly projects: readonly Project[];
  readonly session?: Session;
  readonly message?: string;
}

const initial: DashboardState = { phase: "loading", projects: [] };

function isAccessDenied(error: unknown): boolean {
  return error instanceof APIError && (error.status === 401 || error.status === 403);
}

// The controller is intentionally independent of React for negative-path tests.
// No user, CSRF value or project is cached outside this in-memory instance.
export class DashboardController {
  private state: DashboardState = initial;
  private listeners = new Set<(value: DashboardState) => void>();
  private generation = 0;

  constructor(private readonly api: DashboardAPI, private readonly now = Date.now) {}

  get current(): DashboardState { return this.state; }

  subscribe(onChange: (value: DashboardState) => void): () => void {
    this.listeners.add(onChange);
    onChange(this.state);
    return () => { this.listeners.delete(onChange); };
  }

  private publish(value: DashboardState): void {
    this.state = value;
    for (const listener of this.listeners) listener(value);
  }

  private clear(phase: Phase, message?: string): void {
    this.publish({ phase, projects: [], ...(message ? { message } : {}) });
  }

  expire(): void {
    const expiry = this.state.session ? Date.parse(this.state.session.expires_at) : NaN;
    if (this.state.session && (!Number.isFinite(expiry) || expiry <= this.now())) {
      this.generation++;
      this.clear("expired", "Your session has expired. Sign in again to continue.");
    }
  }

  async load(): Promise<void> {
    const ticket = ++this.generation;
    this.clear("loading");
    let session: Session;
    try {
      session = await this.api.session();
    } catch (error: unknown) {
      if (ticket !== this.generation) return;
      if (isAccessDenied(error)) this.clear("signed_out");
      else this.clear("unavailable", "The local service could not verify your session. No project data is shown.");
      return;
    }
    if (ticket !== this.generation) return;
    if (Date.parse(session.expires_at) <= this.now()) {
      this.clear("expired", "Your session has expired. Sign in again.");
      return;
    }

    try {
      const projects = await this.api.projects();
      if (ticket !== this.generation) return;
      if (Date.parse(session.expires_at) <= this.now()) {
        this.clear("expired", "Your session has expired. Sign in again.");
        return;
      }
      this.publish({
        phase: "signed_in",
        session,
        projects: [...projects].sort((a, b) => a.name.localeCompare(b.name, "en-GB"))
      });
    } catch (error: unknown) {
      if (ticket !== this.generation) return;
      if (isAccessDenied(error)) {
        this.clear("expired", "Access has expired or been revoked. Sign in again.");
      } else {
        this.clear("unavailable", "Unable to load authorised projects. No cached records are displayed.");
      }
    }
  }

  async logout(): Promise<void> {
    const session = this.state.session;
    if (!session || this.state.phase !== "signed_in" &&
        this.state.phase !== "signout_failed") return;

    const ticket = ++this.generation;
    this.publish({ phase: "loading", projects: [], session });
    try {
      await this.api.logout(session.csrf_token);
      if (ticket !== this.generation) return;
      this.clear("signed_out", "Signed out. The server session has been revoked.");
    } catch (error: unknown) {
      if (ticket !== this.generation) return;
      if (isAccessDenied(error)) {
        // A failed CSRF proof is not a confirmed logout.
        if (error instanceof APIError && error.status === 403) {
          this.publish({
            phase: "signout_failed",
            projects: [],
            session,
            message: "Sign-out could not be confirmed. Retry or close the browser; the server session may still be active."
          });
        } else {
          this.clear("expired", "The server session was no longer valid.");
        }
      } else {
        this.publish({
          phase: "signout_failed",
          projects: [],
          session,
          message: "Sign-out could not be confirmed. Retry or close the browser; the session may still be active."
        });
      }
    }
  }
}
