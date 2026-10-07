import assert from "node:assert/strict";
import test from "node:test";

import { PayvergeAdminClient } from "../src/backendClient.mjs";
import { createOwnerSession, decodeJwtExpiry, redactEmail } from "../src/ownerSession.mjs";
import { API_BASE, fakeJwt, loginRoute, mockBackend, reply, sequence } from "./mockBackend.mjs";

const EMAIL = "owner@example.test";
const PASSWORD = "correct horse";

function session(backend, extra = {}) {
  return createOwnerSession({ baseUrl: API_BASE, email: EMAIL, password: PASSWORD, fetchImpl: backend.fetch, ...extra });
}

test("logs in once and shares one login between concurrent callers", async () => {
  const backend = mockBackend({ "POST /auth/login": loginRoute() });
  const owner = session(backend);

  const [a, b, c] = await Promise.all([owner.getToken(), owner.getToken(), owner.getToken()]);
  assert.equal(a, b);
  assert.equal(b, c);
  assert.equal(await owner.getToken(), a, "a fresh token is reused");
  assert.deepEqual(backend.calls(), ["POST /auth/login"]);
  assert.deepEqual(backend.requests[0].body, { email: EMAIL, password: PASSWORD });
  assert.equal(owner.describe().logins, 1);
});

test("re-logs in when the JWT is within 60 seconds of expiry", async () => {
  let clock = Date.parse("2026-10-03T12:00:00Z");
  const now = () => clock;
  const backend = mockBackend({ "POST /auth/login": loginRoute({ ttlSeconds: 900, now }) });
  const owner = session(backend, { now });

  const first = await owner.getToken();
  clock += 800 * 1000; // 100 s left: still fresh
  assert.equal(await owner.getToken(), first);
  clock += 60 * 1000; // 40 s left: inside the skew window
  const second = await owner.getToken();
  assert.notEqual(second, first);
  assert.equal(backend.calls().length, 2);
});

test("wrong credentials are sticky: no second login attempt, no password in the error", async () => {
  const backend = mockBackend({ "POST /auth/login": loginRoute({ password: "something else" }) });
  const owner = session(backend);

  await assert.rejects(owner.getToken(), (err) => {
    assert.equal(err.code, "owner_invalid_credentials");
    assert.equal(err.status, 401);
    assert.equal(err.requestId, "req-login-401");
    assert.match(err.hint, /reset-password/);
    assert.ok(!JSON.stringify(err).includes(PASSWORD));
    assert.ok(!err.message.includes(PASSWORD));
    return true;
  });
  await assert.rejects(owner.getToken(), { code: "owner_invalid_credentials" });
  assert.equal(backend.calls().length, 1, "a rejected password must not be retried (account lockout)");
  assert.equal(owner.describe().blocked, "owner_invalid_credentials");
});

test("a lockout (429) is sticky too", async () => {
  const backend = mockBackend({ "POST /auth/login": reply(429, { error: "Too many failed attempts" }) });
  const owner = session(backend);
  await assert.rejects(owner.getToken(), { code: "owner_login_locked", status: 429 });
  await assert.rejects(owner.getToken(), { code: "owner_login_locked" });
  assert.equal(backend.calls().length, 1);
});

test("an unverified email is reported without echoing the verification link", async () => {
  const backend = mockBackend({
    "POST /auth/login": reply(403, {
      error: "Please verify your email",
      params: { requires_email_verification: true },
      verification_url: "https://pos.example.test/verify?token=secret-verification-token",
    }),
  });
  const owner = session(backend);
  await assert.rejects(owner.getToken(), (err) => {
    assert.equal(err.code, "owner_email_unverified");
    assert.ok(!JSON.stringify(err).includes("secret-verification-token"));
    return true;
  });
});

test("server errors are not sticky", async () => {
  const backend = mockBackend({
    "POST /auth/login": sequence(reply(502, { error: "bad gateway" }), loginRoute()),
  });
  const owner = session(backend);
  await assert.rejects(owner.getToken(), { code: "owner_login_failed", status: 502 });
  assert.ok(await owner.getToken());
  assert.equal(backend.calls().length, 2);
});

test("an unreachable backend is reported with a base-URL hint", async () => {
  const owner = createOwnerSession({
    baseUrl: API_BASE,
    email: EMAIL,
    password: PASSWORD,
    fetchImpl: async () => {
      throw new Error("connect ECONNREFUSED");
    },
  });
  await assert.rejects(owner.getToken(), (err) => {
    assert.equal(err.code, "owner_login_unreachable");
    assert.match(err.hint, /PAYVERGE_API_BASE_URL/);
    return true;
  });
});

test("the API client re-logs in once on 401 and retries the request", async () => {
  let tokenSeen = [];
  const backend = mockBackend({
    "POST /auth/login": loginRoute(),
    "GET /inside/businesses": (request) => {
      tokenSeen.push(request.headers.authorization);
      return tokenSeen.length === 1 ? reply(401, { error: "Authentication token has expired" }) : [{ id: 1 }];
    },
  });
  const owner = session(backend);
  const client = new PayvergeAdminClient({ baseUrl: API_BASE, tokenProvider: owner, fetchImpl: backend.fetch });

  assert.deepEqual(await client.get("/inside/businesses"), [{ id: 1 }]);
  assert.deepEqual(backend.calls(), ["POST /auth/login", "GET /inside/businesses", "POST /auth/login", "GET /inside/businesses"]);
  assert.notEqual(tokenSeen[0], tokenSeen[1], "the retry carries the new token");
});

test("a 401 that survives a fresh login is returned, not looped", async () => {
  const backend = mockBackend({
    "POST /auth/login": loginRoute(),
    "GET /inside/businesses": reply(401, { error: "nope" }),
  });
  const client = new PayvergeAdminClient({ baseUrl: API_BASE, tokenProvider: session(backend), fetchImpl: backend.fetch });
  await assert.rejects(client.get("/inside/businesses"), { status: 401 });
  assert.equal(backend.calls().filter((call) => call === "GET /inside/businesses").length, 2);
});

test("a static owner token is used as-is and reports expiry instead of logging in", async () => {
  let clock = Date.parse("2026-10-03T12:00:00Z");
  const token = fakeJwt(Math.floor(clock / 1000) + 600);
  const backend = mockBackend({});
  const owner = createOwnerSession({ baseUrl: API_BASE, staticToken: token, fetchImpl: backend.fetch, now: () => clock });
  assert.equal(owner.mode, "static_token");
  assert.equal(await owner.getToken(), token);
  clock += 600 * 1000;
  await assert.rejects(owner.getToken(), { code: "owner_token_expired" });
  assert.equal(backend.calls().length, 0);

  const other = createOwnerSession({ baseUrl: API_BASE, staticToken: "opaque-token", fetchImpl: backend.fetch });
  assert.equal(await other.getToken(), "opaque-token");
  other.invalidate("opaque-token");
  await assert.rejects(other.getToken(), { code: "owner_token_rejected" });
});

test("describe() never exposes the password or the token", async () => {
  const backend = mockBackend({ "POST /auth/login": loginRoute() });
  const owner = session(backend);
  const token = await owner.getToken();
  const described = JSON.stringify(owner.describe());
  assert.ok(!described.includes(PASSWORD));
  assert.ok(!described.includes(token));
  assert.ok(!described.includes(EMAIL));
  assert.match(described, /o\*\*\*@example\.test/);
});

test("helpers", () => {
  assert.equal(decodeJwtExpiry(fakeJwt(1234)), 1234);
  assert.equal(decodeJwtExpiry("not-a-jwt"), undefined);
  assert.equal(redactEmail("maria@resto.example"), "m***@resto.example");
  assert.equal(redactEmail("nope"), "***");
  assert.throws(() => createOwnerSession({ baseUrl: API_BASE, email: EMAIL }), /PAYVERGE_OWNER_PASSWORD/);
});
