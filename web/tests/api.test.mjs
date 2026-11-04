import test from "node:test";
import assert from "node:assert/strict";
import { APIError, BrowserAPI, parseProjects, parseSession } from "../.test-build/api.js";

const NOW = Date.parse("2026-11-04T12:00:00Z");
const GOOD_SESSION = {
  authenticated: true,
  expires_at: "2026-11-04T12:10:00Z",
  csrf_token: "a".repeat(43)
};
const PROJECT_ID = "aab77233-d8ec-45f4-9da9-eaf8e1a29cca";
const PROJECT = { id: PROJECT_ID, name: "Synthetic", environment: "cloud" };

test("accepts only a future authenticated session with high-entropy CSRF proof", () => {
  const session = parseSession(GOOD_SESSION, NOW);
  assert.equal(session.csrf_token.length, 43);
  for (const wrong of [
    null, {}, { ...GOOD_SESSION, authenticated: false },
    { ...GOOD_SESSION, expires_at: "2026-11-04T11:00:00Z" },
    { ...GOOD_SESSION, csrf_token: "x" },
    { ...GOOD_SESSION, csrf_token: "<script>" },
    { ...GOOD_SESSION, expires_at: "unparseable" }
  ]) {
    assert.throws(() => parseSession(wrong, NOW));
  }
});

test("rejects fabricated project readiness and invalid permissions data", () => {
  assert.deepEqual(parseProjects([PROJECT])[0], { ...PROJECT, readiness: "unknown" });
  assert.deepEqual(parseProjects([{ ...PROJECT, environment: "self_hosted", readiness: "at_risk" }])[0].readiness, "at_risk");
  for (const invalid of [
    { ...PROJECT, id: "not-a-uuid" },
    { ...PROJECT, name: "" },
    { ...PROJECT, environment: "administrator" },
    { ...PROJECT, readiness: "backup_success" },
    { ...PROJECT, name: "x".repeat(129) },
    { ...PROJECT, readiness: {} }
  ]) {
    assert.throws(() => parseProjects([invalid]));
  }
  assert.throws(() => parseProjects([PROJECT, PROJECT]));
  assert.throws(() => parseProjects({ projects: [PROJECT] }));
  assert.throws(() => parseProjects(new Array(1001).fill(PROJECT)));
  assert.deepEqual(parseProjects([]), []);
});

test("browser API uses only fixed same-origin URLs and never puts bearer tokens in JavaScript", async () => {
  const calls = [];
  const fakeFetch = async (url, options) => {
    calls.push({ url, options });
    if (url === "/web/session") {
      return new Response(JSON.stringify({
        ...GOOD_SESSION, expires_at: "2099-11-04T12:10:00Z"
      }), {
        status: 200, headers: { "Content-Type": "application/json" }
      });
    }
    if (url === "/web/projects") {
      return new Response(JSON.stringify([PROJECT]), {
        status: 200, headers: { "Content-Type": "application/json" }
      });
    }
    return new Response(null, { status: 204 });
  };
  const api = new BrowserAPI(fakeFetch);
  // The browser API must accept a valid future session at any test date.
  assert.equal((await api.session()).authenticated, true);
  assert.equal((await api.projects())[0].name, "Synthetic");
  await api.logout("a".repeat(43));
  assert.deepEqual(calls.map(c => c.url), ["/web/session", "/web/projects", "/web/logout"]);
  for (const call of calls) {
    assert.equal(call.options.credentials, "same-origin");
    assert.equal(call.options.mode, "same-origin");
    assert.equal(call.options.redirect, "error");
    assert.equal(call.options.cache, "no-store");
    assert.equal(call.options.referrerPolicy, "no-referrer");
    assert.equal(call.options.headers.Authorization, undefined);
  }
  assert.equal(calls[2].options.method, "POST");
  assert.equal(calls[2].options.headers["X-CSRF-Token"], "a".repeat(43));
});

test("untrusted response MIME, size and denied access never become projects", async () => {
  const malformed = new BrowserAPI(async () =>
    new Response(JSON.stringify([PROJECT]), { status: 200, headers: { "Content-Type": "text/html" } }));
  await assert.rejects(() => malformed.projects(), /content type/);
  const oversized = new BrowserAPI(async () =>
    new Response("x".repeat(520000), { status: 200, headers: { "Content-Type": "application/json" } }));
  await assert.rejects(() => oversized.projects(), /too large/);
  const denied = new BrowserAPI(async () =>
    new Response(JSON.stringify({ error: "request_denied" }), { status: 401, headers: { "Content-Type": "application/json" } }));
  await assert.rejects(() => denied.projects(), error => error instanceof APIError && error.status === 401);
});

test("logout requires valid CSRF and a successful 204 response", async () => {
  const api = new BrowserAPI(async () => new Response(JSON.stringify({ error: "request_denied" }), { status: 403 }));
  await assert.rejects(() => api.logout("not-csrf"));
  await assert.rejects(() => api.logout("a".repeat(43)), error => error instanceof APIError && error.status === 403);
});
