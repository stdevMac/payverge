// frontend/tests/spaces-tables.spec.ts
//
// Spaces & Tables e2e — manual publish journey + unauthorized cross-tenant API.
// Runs in the journeys job when PLAYWRIGHT_RUN_JOURNEYS_E2E=1.
import { test, expect, request as apiRequestFactory } from "@playwright/test";

import {
  API_BASE,
  MARKETING_BUSINESS_SLUG,
  journeysEnabled,
  newManagerApi,
  resolveBusinessId,
} from "./helpers/journeys";

test.describe("spaces & tables", () => {
  test.skip(
    !journeysEnabled,
    "Set PLAYWRIGHT_RUN_JOURNEYS_E2E=1 (requires the compose stack + demo seed)",
  );

  let businessId = 0;
  let foreignId = 0;

  test.beforeAll(async () => {
    businessId = await resolveBusinessId();
    foreignId = await resolveBusinessId(MARKETING_BUSINESS_SLUG);
    expect(businessId).not.toBe(foreignId);
  });

  test("manual create → draft → publish journey", async ({ playwright }) => {
    test.setTimeout(120_000);

    const api = await newManagerApi(playwright);
    try {
      const createResp = await api.post(
        `${API_BASE}/inside/businesses/${businessId}/spaces`,
        {
          data: {
            name: `E2E Space ${Date.now()}`,
            space_type: "indoor",
            measurement_unit: "m",
          },
        },
      );
      expect(createResp.ok(), await createResp.text()).toBeTruthy();
      const space = await createResp.json();
      const spaceId = space.id as number;
      expect(spaceId).toBeTruthy();

      // Put a valid empty-ish draft (no tables) and publish
      const layout = {
        schema_version: 1,
        width_mm: 8000,
        height_mm: 6000,
        measurement_unit: "m",
        tables: [],
        elements: [],
        regions: [],
      };
      const draftResp = await api.put(
        `${API_BASE}/inside/businesses/${businessId}/spaces/${spaceId}/layout/draft`,
        {
          data: {
            expected_revision: space.draft_revision ?? 1,
            layout,
          },
        },
      );
      expect(draftResp.ok(), await draftResp.text()).toBeTruthy();
      const draftBody = await draftResp.json();
      const rev = draftBody.draft_revision as number;

      const publishResp = await api.post(
        `${API_BASE}/inside/businesses/${businessId}/spaces/${spaceId}/layout/publish`,
        {
          data: { expected_revision: rev },
        },
      );
      expect(publishResp.ok(), await publishResp.text()).toBeTruthy();
      const published = await publishResp.json();
      expect(
        published.space?.status || published.status || "published",
      ).toMatch(/published|ok/i);
    } finally {
      await api.dispose();
    }
  });

  test("unauthorized cross-tenant space access is denied", async ({
    playwright,
  }) => {
    test.setTimeout(60_000);

    const api = await newManagerApi(playwright);
    try {
      const cross = await api.get(
        `${API_BASE}/inside/businesses/${foreignId}/spaces`,
      );
      expect([403, 404]).toContain(cross.status());

      // Unauthenticated request must not succeed.
      const anon = await apiRequestFactory.newContext({ baseURL: API_BASE });
      try {
        const anonResp = await anon.get(
          `${API_BASE}/inside/businesses/${businessId}/spaces`,
        );
        expect([401, 403]).toContain(anonResp.status());
      } finally {
        await anon.dispose();
      }
    } finally {
      await api.dispose();
    }
  });
});
