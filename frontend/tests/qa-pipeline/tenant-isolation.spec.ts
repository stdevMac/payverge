/**
 * GAP-2 — cross-account tenant isolation probe (two genuine owner accounts).
 *
 * Not a defect fix: coverage work. "Nothing found" is success.
 * If a real leak is found: STOP and escalate (do not fix in Session Q).
 *
 * Setup: the local stack with demo data and owner A (admin@local.test, whose
 * password comes from the owner-session env). The spec registers a fresh
 * owner B on every run through the real path (admin invite batch -> register
 * -> email verify) with a random @example.invalid address and password, then
 * creates B's business. Every probe must be denied with 401/403/404: a 2xx is
 * a leak and a 5xx proves nothing, so both fail the run.
 *
 *   PLAYWRIGHT_RUN_QA_PIPELINE=1 \
 *   npx playwright test --config=playwright.qa-pipeline.config.ts \
 *     tenant-isolation.spec.ts --project=chromium
 */
import { test, expect, request as apiRequestFactory } from "@playwright/test";
import { randomBytes } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { API_BASE, listOwnerBusinesses, OWNER_EMAIL, OWNER_PASSWORD, OWNER_PASSWORD_MISSING_REASON } from "./owner-session";
import {
  buildCrossTenantMatrix,
  classifyIsolationStatus,
  type IsolationOutcome,
} from "@/lib/tenantIsolationProbe";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";
// Owner B is created fresh by the spec on every run.
const OWNER_B_EMAIL = `owner-b-${Date.now()}@example.invalid`;
const OWNER_B_PASSWORD = randomBytes(24).toString("base64url");

async function loginEmail(
  email: string,
  password: string,
): Promise<{ token: string; api: Awaited<ReturnType<typeof apiRequestFactory.newContext>> }> {
  const apiOrigin = new URL(API_BASE).origin;
  const api = await apiRequestFactory.newContext({
    baseURL: apiOrigin,
    extraHTTPHeaders: {
      Origin: process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000",
    },
  });
  const resp = await api.post(`${API_BASE}/auth/login`, {
    data: { email, password },
  });
  if (!resp.ok()) {
    const body = await resp.text();
    await api.dispose();
    throw new Error(`login ${email} failed: ${resp.status()} ${body}`);
  }
  const body = (await resp.json()) as { token?: string };
  if (!body.token) {
    await api.dispose();
    throw new Error(`login ${email} returned no token`);
  }
  return { token: body.token, api };
}

/**
 * Register owner B through the real path:
 *   admin invite-batch -> auth/register (invite_code) -> auth/email/verify
 * The email is @example.invalid so the sanitize-clone.sql PII check stays green.
 */
async function registerOwnerB(
  adminToken: string,
  email: string,
  password: string,
): Promise<void> {
  const apiOrigin = new URL(API_BASE).origin;
  const api = await apiRequestFactory.newContext({
    baseURL: apiOrigin,
    extraHTTPHeaders: {
      Origin: process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000",
    },
  });
  try {
    const expires = new Date(Date.now() + 7 * 24 * 3600 * 1000).toISOString();
    const batch = await api.post(
      `${API_BASE}/admin/runtime-controls/invite-batches`,
      {
        headers: { Authorization: `Bearer ${adminToken}` },
        data: {
          name: `gap2-${Date.now()}`,
          cohort_cap: 2,
          owner: "session-q",
          reason: "GAP-2 tenant isolation probe",
          expires_at: expires,
        },
      },
    );
    if (!batch.ok()) {
      throw new Error(
        `invite batch failed: ${batch.status()} ${await batch.text()}`,
      );
    }
    const batchJson = (await batch.json()) as { invite_code?: string };
    if (!batchJson.invite_code) {
      throw new Error("invite batch returned no invite_code");
    }

    const reg = await api.post(`${API_BASE}/auth/register`, {
      data: {
        email,
        password,
        name: "GAP2 Owner B",
        invite_code: batchJson.invite_code,
      },
    });
    if (!reg.ok()) {
      throw new Error(
        `register owner B failed: ${reg.status()} ${await reg.text()}`,
      );
    }
    const regJson = (await reg.json().catch(() => ({}))) as {
      verification_url?: string;
    };
    let vtoken: string | null = null;
    if (regJson.verification_url) {
      vtoken = new URL(regJson.verification_url).searchParams.get("token");
    }
    if (!vtoken) {
      // Unverified login answers 403 with the verification URL.
      const again = await api.post(`${API_BASE}/auth/login`, {
        data: { email, password },
      });
      if (again.ok()) return;
      const againJson = (await again.json().catch(() => ({}))) as {
        params?: { verification_url?: string };
      };
      const vu = againJson.params?.verification_url;
      if (vu) vtoken = new URL(vu).searchParams.get("token");
    }
    if (!vtoken) {
      throw new Error("owner B registered but no verification token surfaced");
    }
    const v = await api.post(`${API_BASE}/auth/email/verify`, {
      data: { token: vtoken },
    });
    if (!v.ok()) {
      throw new Error(`email verify failed: ${v.status()} ${await v.text()}`);
    }
  } finally {
    await api.dispose();
  }
}

test.describe("GAP-2 tenant isolation matrix", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");
  test.skip(!OWNER_PASSWORD, OWNER_PASSWORD_MISSING_REASON);
  test.describe.configure({ mode: "serial", timeout: 10 * 60_000 });

  test("owner A cannot read/mutate owner B resources by id", async () => {
    const ownerA = await loginEmail(OWNER_EMAIL, OWNER_PASSWORD);
    await registerOwnerB(ownerA.token, OWNER_B_EMAIL, OWNER_B_PASSWORD);
    const ownerB = await loginEmail(OWNER_B_EMAIL, OWNER_B_PASSWORD);

    let bizA = await listOwnerBusinesses(ownerA.api, ownerA.token);
    let bizB = await listOwnerBusinesses(ownerB.api, ownerB.token);

    // If B has no business yet, create one via the inside API.
    if (!bizB.length) {
      const create = await ownerB.api.post(`${API_BASE}/inside/businesses`, {
        headers: { Authorization: `Bearer ${ownerB.token}` },
        data: {
          name: "GAP2 Tenant B Cafe",
          custom_url: `gap2-b-${Date.now()}`,
        },
      });
      if (!create.ok()) {
        const body = await create.text();
        await ownerA.api.dispose();
        await ownerB.api.dispose();
        throw new Error(
          `could not create business for owner B: ${create.status()} ${body}`,
        );
      }
      bizB = await listOwnerBusinesses(ownerB.api, ownerB.token);
    }

    if (!bizA.length) {
      await ownerA.api.dispose();
      await ownerB.api.dispose();
      throw new Error("owner A has no businesses — seed the local stack with demo data");
    }
    if (!bizB.length) {
      await ownerA.api.dispose();
      await ownerB.api.dispose();
      throw new Error("owner B still has no businesses after create attempt");
    }

    // Ensure A and B are different tenants.
    const aIds = new Set(bizA.map((b) => b.id));
    const foreign = bizB.find((b) => !aIds.has(b.id));
    expect(
      foreign,
      "owner B business id must not equal owner A businesses",
    ).toBeTruthy();

    const matrix = buildCrossTenantMatrix({
      otherBusinessId: foreign!.id,
    });
    const outcomes: IsolationOutcome[] = [];

    for (const row of matrix) {
      const resp = await ownerA.api.fetch(`${API_BASE}${row.path}`, {
        method: row.method,
        headers: {
          Authorization: `Bearer ${ownerA.token}`,
          "Content-Type": "application/json",
        },
        data:
          row.method === "PUT" || row.method === "POST" || row.method === "PATCH"
            ? { name: "should-not-apply" }
            : undefined,
      });
      const status = resp.status();
      const { isolated, leaked, failed } = classifyIsolationStatus(status);
      outcomes.push({
        owner: "A",
        method: row.method,
        path: row.path,
        status,
        isolated,
        leaked,
        failed,
        note: row.label,
      });
    }

    // Reverse direction: B → A primary business
    const matrixBA = buildCrossTenantMatrix({
      otherBusinessId: bizA[0].id,
    });
    for (const row of matrixBA) {
      const resp = await ownerB.api.fetch(`${API_BASE}${row.path}`, {
        method: row.method,
        headers: {
          Authorization: `Bearer ${ownerB.token}`,
          "Content-Type": "application/json",
        },
        data:
          row.method === "PUT" || row.method === "POST" || row.method === "PATCH"
            ? { name: "should-not-apply" }
            : undefined,
      });
      const status = resp.status();
      const { isolated, leaked, failed } = classifyIsolationStatus(status);
      outcomes.push({
        owner: "B",
        method: row.method,
        path: row.path,
        status,
        isolated,
        leaked,
        failed,
        note: row.label,
      });
    }

    await ownerA.api.dispose();
    await ownerB.api.dispose();

    const outDir =
      process.env.QA_REPORT_DIR ||
      path.resolve(__dirname, "../../test-results/qa-pipeline");
    fs.mkdirSync(outDir, { recursive: true });
    const outPath = path.join(outDir, "gap2-tenant-matrix.json");
    fs.writeFileSync(
      outPath,
      JSON.stringify(
        {
          generatedAt: new Date().toISOString(),
          ownerA: OWNER_EMAIL,
          ownerB: OWNER_B_EMAIL,
          businessA: bizA.map((b) => b.id),
          businessB: bizB.map((b) => b.id),
          outcomes,
        },
        null,
        2,
      ),
    );
    // eslint-disable-next-line no-console
    console.log(`[gap-2] wrote matrix → ${outPath}`);

    const leaks = outcomes.filter((o) => o.leaked);
    if (leaks.length) {
      // Security finding — stop and escalate; do not fix here.
      // eslint-disable-next-line no-console
      console.error(
        "[gap-2] TENANT LEAK DETECTED — escalate to coordinator:\n",
        JSON.stringify(leaks, null, 2),
      );
    }
    expect(
      leaks,
      "cross-tenant 2xx responses are security leaks — escalate, do not fix in Q",
    ).toEqual([]);
    expect(
      outcomes.filter((o) => !o.isolated),
      "every cross-tenant probe must be denied with 401/403/404; 2xx is a leak and 5xx proves nothing",
    ).toEqual([]);
  });
});
