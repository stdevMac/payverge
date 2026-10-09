import type {
  APIRequestContext,
  BrowserContext,
  PlaywrightWorkerArgs,
} from "@playwright/test";

import { loginStaff } from "./staff-login";

export async function authenticatedPrintAPI(
  playwright: PlaywrightWorkerArgs["playwright"],
  context: BrowserContext,
  apiBase: string,
  staffEmail: string,
): Promise<APIRequestContext> {
  const api = await playwright.request.newContext({ baseURL: apiBase });
  await loginStaff(api, staffEmail);
  const state = await api.storageState();
  await context.addCookies(state.cookies);
  return api;
}

export async function ensureBrowserPrinter(
  api: APIRequestContext,
  businessId: number,
  apiBase: string,
): Promise<number> {
  const list = await api.get(
    `${apiBase}/inside/businesses/${businessId}/printers`,
  );
  if (!list.ok()) throw new Error(`printer list failed: ${list.status()}`);
  const body = await list.json();
  const items: Array<{ id: number; transport: string; enabled: boolean }> =
    body.items ?? body ?? [];
  const existing = items.find(
    (printer) => printer.transport === "browser" && printer.enabled,
  );
  if (existing) return existing.id;

  const created = await api.post(
    `${apiBase}/inside/businesses/${businessId}/printers`,
    {
      data: {
        name: "E2E Browser Station",
        role: "bill",
        transport: "browser",
        paper_width_mm: 80,
      },
    },
  );
  if (!created.ok()) {
    throw new Error(`printer create failed: ${created.status()}`);
  }
  const printer = await created.json();
  return Number(printer.id);
}
