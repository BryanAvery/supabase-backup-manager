import { useEffect, useMemo, useState, type ReactNode } from "react";
import { BrowserAPI, type Project, type Readiness } from "./api.js";
import { DashboardController, type DashboardState } from "./controller.js";
import "./styles.css";

function Icon({ name, size = 20 }: { name: "shield" | "grid" | "layers" | "lock" | "refresh" | "log-out" | "clock" | "info" | "cloud" | "server" | "arrow"; size?: number }) {
  const paths: Record<typeof name, ReactNode> = {
    shield: <><path d="m12 2 8 4v5c0 5-3.4 8.8-8 11-4.6-2.2-8-6-8-11V6l8-4Z"/><path d="m9 12 2 2 4-4"/></>,
    grid: <><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></>,
    layers: <><path d="m12 2 9 5-9 5-9-5 9-5Z"/><path d="m3 12 9 5 9-5M3 17l9 5 9-5"/></>,
    lock: <><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></>,
    refresh: <><path d="M20 11a8 8 0 0 0-14-5L4 8"/><path d="M4 3v5h5M4 13a8 8 0 0 0 14 5l2-2"/><path d="M20 21v-5h-5"/></>,
    "log-out": <><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5M21 12H9"/></>,
    clock: <><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></>,
    info: <><circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/></>,
    cloud: <path d="M19 18H7a5 5 0 1 1 1-9.9A7 7 0 0 1 21 12a3 3 0 0 1-2 6Z"/>,
    server: <><rect x="4" y="3" width="16" height="8" rx="2"/><rect x="4" y="13" width="16" height="8" rx="2"/><path d="M8 7h.01M8 17h.01"/></>,
    arrow: <><path d="M5 12h14M13 6l6 6-6 6"/></>
  };
  return <svg aria-hidden="true" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>;
}

function ReadinessLabel({ value }: { value: Readiness }) {
  const labels: Record<Readiness, string> = {
    unknown: "Not assessed",
    manual_steps: "Manual steps",
    at_risk: "At risk",
    recoverable: "Catalogue: recoverable"
  };
  return <span className={`readiness readiness--${value}`}>{labels[value]}</span>;
}

function ProjectTable({ projects }: { projects: readonly Project[] }) {
  if (projects.length === 0) {
    return <div className="empty">
      <div className="empty__icon"><Icon name="layers" size={28}/></div>
      <h3>No authorised projects</h3>
      <p>The local service has not returned any projects you can access. Nothing has been created or changed.</p>
    </div>;
  }
  return <div className="table-scroll"><table className="projects-table">
    <thead><tr><th scope="col">Project</th><th scope="col">Environment</th><th scope="col">Recovery readiness</th></tr></thead>
    <tbody>{projects.map(project => <tr key={project.id}>
      <td><div className="project-name">{project.name}</div><div className="project-id">{project.id}</div></td>
      <td><span className="environment"><Icon name={project.environment === "cloud" ? "cloud" : "server"} size={17}/>{project.environment === "cloud" ? "Supabase Cloud" : "Self-hosted"}</span></td>
      <td><ReadinessLabel value={project.readiness}/></td>
    </tr>)}</tbody>
  </table></div>;
}

function Gate({ state }: { state: DashboardState }) {
  const title: Record<DashboardState["phase"], string> = {
    loading: "Checking secure access",
    signed_out: "Sign in to view your projects",
    signed_in: "Signed in",
    expired: "Your session has expired",
    unavailable: "Local service unavailable",
    signout_failed: "Sign-out needs attention"
  };
  return <div className="gate" role="status">
    <div className="gate__mark"><Icon name={state.phase === "expired" ? "clock" : "lock"} size={32}/></div>
    <h2>{title[state.phase]}</h2>
    <p>{state.message ??
      (state.phase === "loading"
        ? "Checking your session with the local service."
        : "Your account and project permissions are verified by the local backup manager.")}
    </p>
    {state.phase === "loading"
      ? <div className="spinner" aria-label="Loading" />
      : state.phase !== "signout_failed"
        ? <a className="button button--primary" href="/web/login"><Icon name="lock" size={17}/> Sign in securely <Icon name="arrow" size={17}/></a>
        : null}
    {state.phase !== "loading" && <p className="gate__note">Only approved, pre-provisioned accounts can sign in. No backup or restore actions are available.</p>}
  </div>;
}

export default function App() {
  const controller = useMemo(() => new DashboardController(new BrowserAPI()), []);
  const [state, setState] = useState<DashboardState>(controller.current);

  useEffect(() => {
    const unsubscribe = controller.subscribe(setState);
    void controller.load();
    const timer = window.setInterval(() => controller.expire(), 3000);
    return () => {
      window.clearInterval(timer);
      unsubscribe();
    };
  }, [controller]);

  const signedIn = state.phase === "signed_in";
  const expiredAt = state.session ? Date.parse(state.session.expires_at) : NaN;
  const expiry = Number.isFinite(expiredAt)
    ? new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date(expiredAt))
    : "";
  const cloudCount = state.projects.filter(x => x.environment === "cloud").length;
  const selfHostedCount = state.projects.length - cloudCount;
  const visibleSession = signedIn || state.phase === "signout_failed";

  return <div className="app-shell">
    <aside className="sidebar" aria-label="Main navigation">
      <div className="brand">
        <span className="brand__icon"><Icon name="layers" size={23}/></span>
        <span><strong>Backup Manager</strong><small>SUPABASE · LOCAL</small></span>
      </div>

      <div className="nav-group-label">WORKSPACE</div>
      <nav aria-label="Main sections">
        <span className="nav-link nav-link--active" aria-current="page"><Icon name="grid" size={19}/>Overview</span>
        <span className="nav-link nav-link--muted"><Icon name="layers" size={19}/>Project catalogue</span>
        <div className="nav-group-label nav-group-label--secondary">COMING IN LATER RELEASES</div>
        <span className="nav-link nav-link--disabled"><Icon name="clock" size={19}/>Backup schedules <small>Later</small></span>
        <span className="nav-link nav-link--disabled"><Icon name="refresh" size={19}/>Recovery <small>Later</small></span>
      </nav>

      <div className="sidebar__bottom">
        <div className="security-note"><Icon name="shield" size={19}/><div><strong>Private by default</strong><span>Local TLS access only</span></div></div>
        <div className="build-label">Platform foundation · M1</div>
      </div>
    </aside>

    <main className="main">
      <header className="topbar">
        <div><span className="breadcrumb">Workspace</span><span className="breadcrumb-sep">/</span><span className="breadcrumb-current">Overview</span></div>
        <span className="topbar-status"><span className="status-dot"/>Local dashboard</span>
      </header>

      <div className="content">
        <section className="welcome" aria-label="Dashboard heading">
          <div><div className="eyebrow">PROJECT OVERVIEW</div><h1>Project catalogue</h1>
            <p>See the Supabase projects your account is permitted to access. All information comes from the local management service.</p></div>
          {signedIn && <button className="button button--secondary" onClick={() => void controller.load()}><Icon name="refresh" size={16}/> Refresh projects</button>}
        </section>

        {signedIn ? <>
          <section className="metric-grid" aria-label="Authorised project summary">
            <div className="metric"><span className="metric__label">Authorised projects</span><strong>{state.projects.length}</strong><span className="metric__hint">Returned by the local service</span></div>
            <div className="metric"><span className="metric__label">Supabase Cloud</span><strong>{cloudCount}</strong><span className="metric__hint">Managed environments</span></div>
            <div className="metric"><span className="metric__label">Self-hosted</span><strong>{selfHostedCount}</strong><span className="metric__hint">Private deployments</span></div>
          </section>
          <section className="panel" aria-labelledby="projects-heading">
            <div className="panel__header"><div><h2 id="projects-heading">Your projects</h2><p>Access is checked against your current session.</p></div><span className="pill pill--neutral">Read-only</span></div>
            <ProjectTable projects={state.projects}/>
          </section>
        </> : <section className="panel panel--gate"><Gate state={state}/></section>}

        {state.phase === "signout_failed" && <div className="retry-panel"><button className="button button--primary" onClick={() => void controller.logout()}>Retry sign-out</button></div>}
        {visibleSession && <section className="session-bar" aria-label="Session information">
          <div className="session-bar__left"><Icon name="shield" size={20}/><div><strong>{signedIn ? "Authenticated session" : "Session may still be active"}</strong><span>Access expires at {expiry || "an unknown time"}. Permissions are checked by the server.</span></div></div>
          <button className="button button--ghost" onClick={() => void controller.logout()}><Icon name="log-out" size={18}/>Sign out</button>
        </section>}

        <div className="notice" role="note"><Icon name="info" size={20}/><span>Recovery readiness is catalogue metadata, not evidence of a tested backup. Backup and restore controls are not available in this foundation release.</span></div>
        <footer className="footer">Supabase Backup Manager <span>·</span> M1 foundation <span>·</span> Secure, read-only project access</footer>
      </div>
    </main>
  </div>;
}
