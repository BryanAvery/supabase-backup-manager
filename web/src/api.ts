// All endpoints are fixed, relative paths on the Go daemon's HTTPS origin.
// Browser JavaScript never handles bearer credentials or the session cookie.
export type Environment = "cloud" | "self_hosted";
export type Readiness = "recoverable" | "manual_steps" | "at_risk" | "unknown";

export interface Project {
  readonly id: string;
  readonly name: string;
  readonly environment: Environment;
  readonly readiness: Readiness;
}

export interface Session {
  readonly authenticated: true;
  readonly expires_at: string;
  readonly csrf_token: string;
}

export class APIError extends Error {
  constructor(readonly status: number) {
    super(status === 401 ? "Authentication required" : `API request rejected (${status})`);
    this.name = "APIError";
  }
}

export interface DashboardAPI {
  session(): Promise<Session>;
  projects(): Promise<Project[]>;
  logout(csrf: string): Promise<void>;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const CSRF = /^[A-Za-z0-9_-]{32,128}$/;
const knownReadiness = new Set<Readiness>([
  "recoverable", "manual_steps", "at_risk", "unknown"
]);

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("Unexpected API response shape");
  }
  return value as Record<string, unknown>;
}

export function parseSession(value: unknown, now = Date.now()): Session {
  const data = record(value);
  const expiry = typeof data.expires_at === "string" ? Date.parse(data.expires_at) : NaN;
  if (data.authenticated !== true ||
      !Number.isFinite(expiry) || expiry <= now ||
      typeof data.csrf_token !== "string" || !CSRF.test(data.csrf_token)) {
    throw new Error("Unverified or expired browser session");
  }
  return {
    authenticated: true,
    expires_at: data.expires_at as string,
    csrf_token: data.csrf_token
  };
}

export function parseProjects(value: unknown): Project[] {
  if (!Array.isArray(value) || value.length > 1000) {
    throw new Error("Unexpected project collection");
  }
  const ids = new Set<string>();
  return value.map((item: unknown) => {
    const data = record(item);
    const id = data.id;
    const name = data.name;
    const env = data.environment;
    const readiness = data.readiness ?? "unknown";
    if (typeof id !== "string" || !UUID.test(id) || ids.has(id.toLowerCase()) ||
        typeof name !== "string" || name.trim().length < 1 || name.length > 128 ||
        (env !== "cloud" && env !== "self_hosted") ||
        typeof readiness !== "string" ||
        !knownReadiness.has(readiness as Readiness)) {
      throw new Error("Unverified project catalogue entry");
    }
    ids.add(id.toLowerCase());
    return {
      id,
      name,
      environment: env,
      readiness: readiness as Readiness
    };
  });
}

async function boundedJSON(response: Response): Promise<unknown> {
  const mime = response.headers.get("content-type")?.split(";")[0]?.trim().toLowerCase();
  const length = Number(response.headers.get("content-length") ?? "0");
  if (mime !== "application/json" || !Number.isFinite(length) || length > 512 * 1024) {
    throw new Error("Unexpected response content type or size");
  }
  const text = await response.text();
  if (text.length > 512 * 1024) throw new Error("Response too large");
  return JSON.parse(text) as unknown;
}

export class BrowserAPI implements DashboardAPI {
  constructor(private readonly request: typeof fetch = fetch) {}

  private async call(path: "/web/session" | "/web/projects" | "/web/logout",
                     method: "GET" | "POST" = "GET", csrf?: string): Promise<Response> {
    const headers: Record<string, string> = { "Accept": "application/json" };
    if (csrf !== undefined) headers["X-CSRF-Token"] = csrf;
    return this.request(path, {
      method,
      headers,
      credentials: "same-origin",
      mode: "same-origin",
      cache: "no-store",
      redirect: "error",
      referrerPolicy: "no-referrer"
    });
  }

  async session(): Promise<Session> {
    const response = await this.call("/web/session");
    if (!response.ok) throw new APIError(response.status);
    return parseSession(await boundedJSON(response));
  }

  async projects(): Promise<Project[]> {
    const response = await this.call("/web/projects");
    if (!response.ok) throw new APIError(response.status);
    return parseProjects(await boundedJSON(response));
  }

  async logout(csrf: string): Promise<void> {
    if (!CSRF.test(csrf)) throw new Error("Missing same-origin CSRF proof");
    const response = await this.call("/web/logout", "POST", csrf);
    if (response.status !== 204) throw new APIError(response.status);
  }
}
