/** Claim-specific release probes that need the full API stack (root docker-compose.yml). */
import { expect, test, type APIRequestContext } from "@playwright/test";

const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";
let api: APIRequestContext;
let token = "";
let businessId = 0;
let customUrl = "";

test.describe.serial("Verified advertised outcomes", () => {
  test.beforeAll(async ({ playwright }) => {
    api = await playwright.request.newContext({ ignoreHTTPSErrors: true });
    const stamp = Date.now();
    const email = `advertised-${stamp}@payverge.test`;
    const register = await api.post(`${API_BASE}/auth/register`, {
      data: {
        email,
        password: "E2e!Advertised-42",
        name: "Advertised Contract Owner",
      },
    });
    expect(register.status(), await register.text()).toBe(201);
    token = String((await register.json()).token);
    customUrl = `advertised-${stamp}`;
    const business = await api.post(`${API_BASE}/inside/businesses`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: "Advertised Contract Bistro",
        owner_name: "Advertised Contract Owner",
        email,
        business_type: "restaurant",
        address: { country: "US" },
        custom_url: customUrl,
        business_page_enabled: true,
      },
    });
    expect(business.status(), await business.text()).toBe(201);
    businessId = Number((await business.json()).id);
    expect(businessId).toBeGreaterThan(0);
  });

  test.afterAll(async () => {
    if (businessId && token) {
      await api.delete(`${API_BASE}/inside/businesses/${businessId}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
    }
    await api?.dispose();
  });

  test("[claim:onboarding-dashboard-setup] setup status is readable", async () => {
    const response = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/setup-status`,
      { headers: { Authorization: `Bearer ${token}` } },
    );
    expect(response.status(), await response.text()).toBe(200);
    expect(await response.json()).toBeTruthy();
  });

  test("[claim:business-page-editor] published business edits reach the guest page", async () => {
    const response = await api.get(`${API_BASE}/business/${customUrl}`);
    expect(response.status(), await response.text()).toBe(200);
    expect(JSON.stringify(await response.json())).toContain(
      "Advertised Contract Bistro",
    );
  });

  test("[claim:plugins-marketplace] active plugin catalog is served by the built backend", async () => {
    const response = await api.get(`${API_BASE}/inside/plugins`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(response.status(), await response.text()).toBe(200);
    const catalog = JSON.stringify(await response.json()).toLowerCase();
    for (const plugin of ["stripe", "paypal", "mercadopago"]) {
      expect(catalog).toContain(plugin);
    }
  });

  test("[claim:security-tls-in-transit] release edge serves the privacy page over TLS", async ({
    page,
  }) => {
    await page.goto("/privacy", { waitUntil: "domcontentloaded" });
    expect(new URL(page.url()).protocol).toBe("https:");
    await expect(page.locator("body")).toContainText(/security/i);
  });
});
