import test from "node:test";
import assert from "node:assert/strict";
import { APIError } from "../.test-build/api.js";
import { DashboardController } from "../.test-build/controller.js";

const NOW = Date.parse("2026-11-04T12:00:00Z");
const session = {
  authenticated: true,
  expires_at: new Date(NOW + 300000).toISOString(),
  csrf_token: "p".repeat(43)
};
const projects = [{
  id: "aab77233-d8ec-45f4-9da9-eaf8e1a29cca",
  name: "Synthetic Cloud",
  environment: "cloud",
  readiness: "unknown"
}];

function fake(overrides = {}) {
  return {
    session: async () => session,
    projects: async () => projects,
    logout: async () => undefined,
    ...overrides
  };
}

test("no authenticated cookie leads to a signed-out, empty catalogue", async () => {
  const model = new DashboardController(fake({
    session: async () => { throw new APIError(401); }
  }), () => NOW);
  await model.load();
  assert.equal(model.current.phase, "signed_out");
  assert.deepEqual(model.current.projects, []);
  assert.equal(model.current.session, undefined);
});

test("verified session only displays current authorised projects", async () => {
  const model = new DashboardController(fake(), () => NOW);
  await model.load();
  assert.equal(model.current.phase, "signed_in");
  assert.equal(model.current.projects[0]?.name, "Synthetic Cloud");
  assert.equal(model.current.session?.csrf_token, "p".repeat(43));
});

test("revoked project access immediately clears previously displayed records", async () => {
  let revoked = false;
  const model = new DashboardController(fake({
    projects: async () => {
      if (revoked) throw new APIError(401);
      return projects;
    }
  }), () => NOW);
  await model.load();
  assert.equal(model.current.projects.length, 1);
  revoked = true;
  await model.load();
  assert.equal(model.current.phase, "expired");
  assert.deepEqual(model.current.projects, []);
  assert.equal(model.current.session, undefined);
});

test("network failure clears project rows and does not display old account data", async () => {
  let disconnected = false;
  const model = new DashboardController(fake({
    session: async () => {
      if (disconnected) throw new Error("offline");
      return session;
    }
  }), () => NOW);
  await model.load();
  assert.equal(model.current.projects.length, 1);
  disconnected = true;
  await model.load();
  assert.equal(model.current.phase, "unavailable");
  assert.deepEqual(model.current.projects, []);
});

test("expiry prevents a late project request from resurrecting stale data", async () => {
  let resolveProject;
  let clock = NOW;
  const model = new DashboardController(fake({
    projects: () => new Promise(resolve => { resolveProject = resolve; })
  }), () => clock);
  const loading = model.load();
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(typeof resolveProject, "function");
  clock = NOW + 300001;
  model.expire(); // The published state is still loading, but it holds no session.
  resolveProject(projects);
  await loading;
  assert.equal(model.current.phase, "expired");
  assert.deepEqual(model.current.projects, []);
});

test("sign-out revokes the server session; no local success on failed CSRF", async () => {
  let fail = true;
  let csrfSeen = "";
  const model = new DashboardController(fake({
    logout: async proof => {
      csrfSeen = proof;
      if (fail) throw new APIError(403);
    }
  }), () => NOW);
  await model.load();
  await model.logout();
  assert.equal(csrfSeen, "p".repeat(43));
  assert.equal(model.current.phase, "signout_failed");
  assert.deepEqual(model.current.projects, []);
  assert.equal(model.current.session?.csrf_token, "p".repeat(43));
  fail = false;
  await model.logout();
  assert.equal(model.current.phase, "signed_out");
  assert.equal(model.current.session, undefined);
});

test("expired sessions cannot populate authorised data", async () => {
  const model = new DashboardController(fake({
    session: async () => ({ ...session, expires_at: new Date(NOW - 1000).toISOString() })
  }), () => NOW);
  await model.load();
  assert.equal(model.current.phase, "expired");
  assert.equal(model.current.projects.length, 0);
});

test("an older account fetch never overwrites a newer session", async () => {
  let oldComplete;
  let sessionNumber = 0;
  const model = new DashboardController(fake({
    session: async () => {
      sessionNumber++;
      return session;
    },
    projects: () => {
      if (sessionNumber === 1) return new Promise(resolve => { oldComplete = resolve; });
      return Promise.resolve([{ ...projects[0], name: "New user project" }]);
    }
  }), () => NOW);
  const older = model.load();
  await Promise.resolve(); await Promise.resolve();
  const newer = model.load();
  await newer;
  oldComplete(projects);
  await older;
  assert.equal(model.current.projects.length, 1);
  assert.equal(model.current.projects[0].name, "New user project");
});
