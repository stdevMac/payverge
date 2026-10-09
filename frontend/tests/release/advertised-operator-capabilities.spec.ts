/**
 * Operator + guest release probes for advertised live capabilities.
 *
 * Each test proves an advertised USER OUTCOME end-to-end against the
 * full API stack (root docker-compose.yml): it registers a fresh owner, provisions the real
 * resource through the same routes the product uses, and asserts the persisted
 * or served result — never a bare HTTP 200, a config flag, a header line, or an
 * empty-math identity. Where the advertised outcome cannot be exercised without
 * a live third party (a translation provider, an on-chain settlement, Postmark),
 * the registry row is `beta` and this file does not claim it as live coverage.
 *
 * Contracts here were captured empirically against the built backend
 * (register -> create business -> exercise -> assert). Money is dollars on the
 * wire (backend MarshalJSON converts cents -> float64).
 *
 * Companion to advertised-live-capabilities.spec.ts (which covers
 * onboarding-state, business-page, plugins-marketplace, TLS).
 */
import { expect, test, type APIRequestContext } from "@playwright/test";
import { localeRegistry } from "../../src/i18n/localeRegistry";

const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";
const ORIGIN = process.env.PLAYWRIGHT_API_ORIGIN || "http://localhost:3000";

let api: APIRequestContext;
let token = "";
let businessId = 0;
let customUrl = "";
let tableCode = "";
let cheddarId = 0;
let staffToken = "";
let staffId = 0;
let seededBillNumber = "";

const authHeaders = () => ({
  Authorization: `Bearer ${token}`,
  Origin: ORIGIN,
});
const staffAuthHeaders = () => ({
  Authorization: `Bearer ${staffToken}`,
  Origin: ORIGIN,
});
const guestHeaders = () => ({ Origin: ORIGIN });
const guestOrderHeaders = (requestId: string) => ({
  ...guestHeaders(),
  "X-Request-Id": requestId,
});

/** ISO timestamp `days` from now (reservations must be within 30 days). */
const soon = (days: number, hour = 19): string => {
  const d = new Date(Date.now() + days * 86_400_000);
  d.setUTCHours(hour, 30, 0, 0);
  return d.toISOString();
};

/** RFC3339 midnight `days` from now — for payroll period bounds. */
const isoDay = (days: number): string => {
  const d = new Date(Date.now() + days * 86_400_000);
  d.setUTCHours(0, 0, 0, 0);
  return d.toISOString();
};

/** YYYY-MM-DD of the next upcoming Monday (schedule week start). */
const nextMonday = (): string => {
  const day = new Date(Date.now()).getUTCDay(); // 0=Sun..6=Sat
  const add = ((8 - day) % 7) || 7; // at least 1 day out, lands on Monday
  const m = new Date(Date.now() + add * 86_400_000);
  return `${m.getUTCFullYear()}-${String(m.getUTCMonth() + 1).padStart(2, "0")}-${String(m.getUTCDate()).padStart(2, "0")}`;
};

/** Provision a fresh table and return its guest QR code. A table holds at most
 * one open bill, so each guest-flow claim needs its own. */
async function newTableCode(label: string): Promise<string> {
  const res = await api.post(
    `${API_BASE}/inside/businesses/${businessId}/tables`,
    { headers: authHeaders(), data: { name: label, capacity: 4 } },
  );
  expect(res.status(), await res.text()).toBe(201);
  return String((await res.json()).table_code);
}

/** Open a fresh bill on `code`, place a Burger order, and get it approved so the
 * bill carries a real total. Returns the bill id + public token + number. */
async function seatOrderedBill(
  code: string,
  quantity = 2,
): Promise<{ billId: number; token: string; number: string; orderId: number }> {
  const billRes = await api.post(`${API_BASE}/guest/table/${code}/bill`, {
    headers: guestHeaders(),
    data: {},
  });
  expect(billRes.status(), await billRes.text()).toBe(201);
  const bill = (await billRes.json()).bill;
  const order = await api.post(`${API_BASE}/guest/table/${code}/order`, {
    headers: guestOrderHeaders(`seat-ordered-bill-${code}`),
    data: { bill_id: Number(bill.id), items: [{ menu_item_name: "Burger", quantity }] },
  });
  expect(order.status(), await order.text()).toBe(201);
  const orderId = Number((await order.json()).order.id);
  const approve = await api.put(
    `${API_BASE}/inside/businesses/${businessId}/orders/${orderId}/status`,
    { headers: authHeaders(), data: { status: "approved" } },
  );
  expect(approve.status(), await approve.text()).toBe(200);
  return {
    billId: Number(bill.id),
    token: String(bill.public_token),
    number: String(bill.bill_number),
    orderId,
  };
}

test.describe.serial("Verified advertised operator + guest outcomes", () => {
  test.beforeAll(async ({ playwright }) => {
    api = await playwright.request.newContext({ ignoreHTTPSErrors: true });
    const stamp = Date.now();
    const email = `operator-${stamp}@payverge.test`;
    const register = await api.post(`${API_BASE}/auth/register`, {
      headers: { Origin: ORIGIN },
      data: { email, password: "E2e!Operator-42", name: "Operator Owner" },
    });
    expect(register.status(), await register.text()).toBe(201);
    token = String((await register.json()).token);

    customUrl = `operator-${stamp}`;
    const business = await api.post(`${API_BASE}/inside/businesses`, {
      headers: authHeaders(),
      data: {
        name: "Operator Capability Bistro",
        owner_name: "Operator Owner",
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

    // Enable kitchen/orders FIRST — guest bill creation is rejected
    // ("ordering_disabled") until this is on.
    const kds = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/toggle-kitchen-orders`,
      { headers: authHeaders(), data: { enabled: true } },
    );
    expect(kds.status(), await kds.text()).toBe(200);

    // Shared fixture: one table + a single priced, available menu item so the
    // guest-flow claims (ordering, splitting, kitchen) have real data.
    const table = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/tables`,
      { headers: authHeaders(), data: { name: "Fixture-1", capacity: 4 } },
    );
    expect(table.status(), await table.text()).toBe(201);
    tableCode = String((await table.json()).table_code);
    expect(tableCode).toMatch(/^[A-Z0-9]{6,}$/);

    const cat = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/menu/categories`,
      { headers: authHeaders(), data: { name: "Mains" } },
    );
    expect(cat.status(), await cat.text()).toBe(201);
    const item = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/menu/items`,
      {
        headers: authHeaders(),
        data: {
          category_index: 0,
          item: { name: "Burger", price: 12.5, is_available: true, description: "Juicy beef burger" },
        },
      },
    );
    expect(item.status(), await item.text()).toBe(201);

    // Inventory ingredient + recipe (Burger -> 0.2kg Cheddar @ $3.50) so the
    // food-cost / waste / menu-engineering analytics have a real BOM to compute.
    const inv = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/inventory/items`,
      {
        headers: authHeaders(),
        data: { name: "Cheddar", unit: "kg", current_quantity: 8, cost_per_unit: 3.5 },
      },
    );
    expect(inv.status(), await inv.text()).toBe(201);
    cheddarId = Number((await inv.json()).id);
    expect(cheddarId).toBeGreaterThan(0);
    const recipe = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/inventory/recipes/Burger`,
      {
        headers: authHeaders(),
        data: {
          menu_item_name: "Burger",
          entries: [{ inventory_item_id: cheddarId, quantity_required: 0.2 }],
        },
      },
    );
    expect(recipe.status(), await recipe.text()).toBe(200);

    // Invite a server-role staff and mint a real limited-role session via the
    // accept-invitation flow. Tokens are not on GET staff; use invitation_url
    // from the invite response. Powers the RBAC + time-clock claims.
    const staffEmail = `server-${stamp}@payverge.test`;
    const invite = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/staff/invite`,
      { headers: authHeaders(), data: { email: staffEmail, name: "Sam Server", role: "server" } },
    );
    expect(invite.status(), await invite.text()).toBe(201);
    const inviteBody = await invite.json();
    const inviteToken = String(
      new URL(String(inviteBody.invitation_url)).searchParams.get("token") ?? "",
    );
    expect(inviteToken.length).toBeGreaterThan(10);
    const accept = await api.post(`${API_BASE}/staff/accept-invitation`, {
      headers: { Origin: ORIGIN },
      data: { token: inviteToken, name: "Sam Server" },
    });
    expect(accept.status(), await accept.text()).toBe(201);
    const accepted = await accept.json();
    staffToken = String(accepted.token);
    staffId = Number(accepted.staff.id);
    expect(staffToken.length).toBeGreaterThan(10);
    expect(staffId).toBeGreaterThan(0);

    // Seed ONE fully settled card sale (2 x $12.50 = $25) so the reporting +
    // prime-cost + menu-engineering + waste analytics read real recognized
    // revenue and a real sold dish. Cash recording now requires an open drawer
    // (#374), so this seed must not go through the silent unassigned-cash path.
    const saleCode = await newTableCode("Seed-Sale");
    const sale = await seatOrderedBill(saleCode, 2);
    seededBillNumber = sale.number;
    const settle = await api.post(
      `${API_BASE}/inside/bills/${sale.billId}/alternative-payment`,
      {
        headers: { ...authHeaders(), "Idempotency-Key": `seed-${stamp}` },
        data: { amount: "25.00", payment_method: "card", business_confirmation: true },
      },
    );
    expect(settle.status(), await settle.text()).toBe(200);
    expect((await settle.json()).payment_breakdown.is_complete).toBe(true);

    // Seed a PAID payroll run overlapping the current week so labor cost is
    // non-zero for the prime-cost claim (period=week window is [today-7, today]).
    const seedRun = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs`,
      {
        headers: authHeaders(),
        data: {
          period_start: isoDay(-3),
          period_end: isoDay(3),
          notes: "labor seed",
          line_items: [
            { payee_type: "contractor", payee_name: "Line Cook", gross_amount: 100, bonus_amount: 0, deduction_amount: 0 },
          ],
        },
      },
    );
    expect(seedRun.status(), await seedRun.text()).toBe(201);
    const seedRunId = Number((await seedRun.json()).data.id);
    const markSeedPaid = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs/${seedRunId}/mark-paid`,
      { headers: authHeaders(), data: {} },
    );
    expect(markSeedPaid.status(), await markSeedPaid.text()).toBe(200);

    // Record an explicit waste movement (-1kg Cheddar) so the waste/variance
    // claim has a clean, attributable tracked loss ($3.50).
    const waste = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/inventory/adjustments`,
      {
        headers: authHeaders(),
        data: { inventory_item_id: cheddarId, movement_type: "waste", quantity_change: -1, reason: "spoilage" },
      },
    );
    expect(waste.status(), await waste.text()).toBe(201);
  });

  test.afterAll(async () => {
    if (businessId && token) {
      await api.delete(`${API_BASE}/inside/businesses/${businessId}`, {
        headers: authHeaders(),
      });
    }
    await api?.dispose();
  });

  test("[claim:table-management] a created table is persisted with a QR code", async () => {
    const create = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/tables`,
      { headers: authHeaders(), data: { name: "Patio-7", capacity: 2 } },
    );
    expect(create.status(), await create.text()).toBe(201);
    const created = await create.json();
    expect(String(created.table_code)).toMatch(/^[A-Z0-9]{6,}$/);

    const list = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/tables`,
      { headers: authHeaders() },
    );
    expect(list.status()).toBe(200);
    const tables = (await list.json()).tables as Array<{ name: string; table_code: string; qr_url: string }>;
    const patio = tables.find((t) => t.name === "Patio-7");
    expect(patio, "created table appears in the operator table list").toBeTruthy();
    expect(patio!.qr_url).toContain(patio!.table_code);
  });

  test("[claim:unlimited-tables] no table cap is imposed", async () => {
    // Marketing promises "unlimited tables". Create a large batch and prove all persist.
    const target = 30;
    for (let i = 0; i < target; i += 1) {
      const res = await api.post(
        `${API_BASE}/inside/businesses/${businessId}/tables`,
        { headers: authHeaders(), data: { name: `Bulk-${i}`, capacity: 2 } },
      );
      expect(res.status(), `table ${i}: ${await res.text()}`).toBe(201);
    }
    const list = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/tables`,
      { headers: authHeaders() },
    );
    const tables = (await list.json()).tables as unknown[];
    expect(tables.length).toBeGreaterThanOrEqual(target);
  });

  test("[claim:reservations] a reservation is booked and returns a confirmation code", async () => {
    const res = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/reservations`,
      {
        headers: authHeaders(),
        data: {
          customer_name: "Ada Lovelace",
          party_size: 3,
          reservation_time: soon(7),
          phone: "+15551230000",
        },
      },
    );
    expect(res.status(), await res.text()).toBe(201);
    const reservation = await res.json();
    expect(String(reservation.confirmation_code)).toMatch(/^[A-Z0-9]{6,}$/);

    const list = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/reservations`,
      { headers: authHeaders() },
    );
    expect(list.status()).toBe(200);
    expect(JSON.stringify(await list.json())).toContain("Ada Lovelace");
  });

  test("[claim:reservations-24-7] a guest self-books online and receives a confirmation code", async () => {
    // The "24/7" promise is the always-OPEN online booking CHANNEL: a guest
    // self-serves and gets an auto-confirmed reservation with NO operator on
    // shift. (Slot times are still bounded by the business's operating hours —
    // it is a round-the-clock intake channel, not round-the-clock seating; the
    // literal "24/7" copy is flagged for P3 review. Booked at an in-hours slot.)
    const settings = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/reservations/settings`,
      {
        headers: authHeaders(),
        data: {
          enabled: true,
          approval_mode: "auto",
          min_advance_minutes: 0,
          max_advance_days: 365,
          min_party_size: 1,
          max_party_size: 20,
          auto_assign_tables: true,
          send_confirmation_email: false,
          allow_waitlist: true,
          max_covers_per_slot: 50,
        },
      },
    );
    expect(settings.status(), await settings.text()).toBe(200);

    const res = await api.post(`${API_BASE}/business/${customUrl}/reservations`, {
      headers: guestHeaders(),
      data: {
        customer_name: "Night Owl",
        party_size: 2,
        reservation_time: soon(8, 16),
        customer_phone: "+15559990000",
        customer_email: "nightowl@example.com",
      },
    });
    expect(res.status(), await res.text()).toBe(201);
    const body = await res.json();
    // Exact confirmation code (12 hex), not a fuzzy substring; auto mode confirms.
    const code = String(body.confirmation_code);
    expect(code).toMatch(/^[0-9A-F]{12}$/);
    expect(body.status).toBe("confirmed");

    // The booking is visible to the operator, matched by its exact code.
    const list = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/reservations`,
      { headers: authHeaders() },
    );
    expect(list.status()).toBe(200);
    const found = (await list.json()).reservations.find(
      (r: { confirmation_code: string }) => r.confirmation_code === code,
    );
    expect(found, "booked reservation appears in the operator list").toBeTruthy();
    expect(found.customer_name).toBe("Night Owl");
  });

  test("[claim:qr-ordering] an unauthenticated guest orders from a table QR and the kitchen receives it", async () => {
    const code = await newTableCode("QR-order");
    // Digital menu is served for the table code — no app, no login.
    const menu = await api.get(`${API_BASE}/guest/table/${code}/menu`, {
      headers: guestHeaders(),
    });
    expect(menu.status(), await menu.text()).toBe(200);
    expect(JSON.stringify(await menu.json())).toContain("Burger");

    // A guest with only an Origin header (no bearer) opens a bill and orders.
    const billRes = await api.post(`${API_BASE}/guest/table/${code}/bill`, {
      headers: guestHeaders(),
      data: {},
    });
    expect(billRes.status(), await billRes.text()).toBe(201);
    const billId = Number((await billRes.json()).bill.id);
    const order = await api.post(`${API_BASE}/guest/table/${code}/order`, {
      headers: guestOrderHeaders(`qr-ordering-${code}`),
      data: { bill_id: billId, items: [{ menu_item_name: "Burger", quantity: 2 }] },
    });
    expect(order.status(), await order.text()).toBe(201);
    const placed = (await order.json()).order;
    const orderId = Number(placed.id);
    expect(orderId).toBeGreaterThan(0);
    expect(placed.status).toBe("pending");
    expect(placed.created_by).toBe("guest");
    // The ordered item is carried on the ticket (items is a JSON-encoded string).
    const placedItems = JSON.parse(String(placed.items));
    expect(placedItems[0].menu_item_name).toBe("Burger");
    expect(Number(placedItems[0].quantity)).toBe(2);

    // The ticket reaches the operator, who approves it (kitchen receives it).
    const orders = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/orders?status=pending`,
      { headers: authHeaders() },
    );
    expect(orders.status()).toBe(200);
    const ticket = (await orders.json()).orders.find(
      (o: { id: number }) => o.id === orderId,
    );
    expect(ticket, "guest ticket is on the operator board").toBeTruthy();
    const approve = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/orders/${orderId}/status`,
      { headers: authHeaders(), data: { status: "approved" } },
    );
    expect(approve.status(), await approve.text()).toBe(200);
    expect((await approve.json()).order.approved_by).toBeTruthy();
  });

  test("[claim:kitchen-display] a placed order surfaces as a ticket on the operator kitchen board", async () => {
    const code = await newTableCode("KDS");
    const billRes = await api.post(`${API_BASE}/guest/table/${code}/bill`, {
      headers: guestHeaders(),
      data: {},
    });
    const billId = Number((await billRes.json()).bill.id);
    const order = await api.post(`${API_BASE}/guest/table/${code}/order`, {
      headers: guestOrderHeaders(`kitchen-display-${code}`),
      data: { bill_id: billId, items: [{ menu_item_name: "Burger", quantity: 3 }] },
    });
    expect(order.status(), await order.text()).toBe(201);
    const orderId = Number((await order.json()).order.id);

    // The KDS board (the orders list) surfaces the actual ticket — item name +
    // quantity + live status — not just a "kitchen is enabled" config flag.
    const board = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/orders?status=pending`,
      { headers: authHeaders() },
    );
    expect(board.status(), await board.text()).toBe(200);
    const ticket = (await board.json()).orders.find(
      (o: { id: number }) => o.id === orderId,
    );
    expect(ticket, "placed order is a ticket on the kitchen board").toBeTruthy();
    expect(ticket.status).toBe("pending");
    const items = JSON.parse(String(ticket.items));
    expect(items[0].menu_item_name).toBe("Burger");
    expect(Number(items[0].quantity)).toBe(3);
  });

  test("[claim:bill-splitting] an approved order's bill splits evenly among guests", async () => {
    // Fresh table + bill + approved order dedicated to the split assertion.
    const code = await newTableCode("Split");
    const seat = await seatOrderedBill(code, 2); // 2 x $12.50 = $25
    const billToken = seat.token;

    const split = await api.post(
      `${API_BASE}/guest/bill/${billToken}/split/equal`,
      { headers: guestHeaders(), data: { num_people: 2 } },
    );
    expect(split.status(), await split.text()).toBe(200);
    const breakdown = (await split.json()).result.breakdown;
    // The $25 bill splits evenly into 2 x $12.50 — the advertised outcome.
    expect(Number(breakdown.num_people)).toBe(2);
    expect(Number(breakdown.amount_per_person)).toBeCloseTo(12.5, 2);
    expect(
      Number(breakdown.amount_per_person) * Number(breakdown.num_people),
    ).toBeCloseTo(25, 2);

    // The split ledger reflects the full bill balance available to divide.
    const state = await api.get(`${API_BASE}/guest/bill/${billToken}/split/state`, {
      headers: guestHeaders(),
    });
    expect(state.status()).toBe(200);
    expect(Number((await state.json()).state.total_amount)).toBeCloseTo(25, 2);
  });

  test("[claim:usdc-stablecoin-payments] a guest bill issues a signed, FX-locked USDC quote", async () => {
    const code = await newTableCode("USDC");
    const seat = await seatOrderedBill(code, 2); // $25 bill
    const quote = await api.post(
      `${API_BASE}/guest/bill/${seat.token}/crypto-quote`,
      { headers: guestHeaders(), data: { amount_paid: 25, chain: "base", token: "USDC" } },
    );
    expect(quote.status(), await quote.text()).toBe(200);
    const q = await quote.json();
    // A locked, HMAC-signed quote (base64 claims '.' 64-hex signature) that pins
    // the USD micro-units at rate 1 — immune to FX drift between quote and pay.
    expect(String(q.quote_token)).toMatch(/^[A-Za-z0-9+/=_-]+\.[0-9a-f]{64}$/);
    expect(Number(q.usd_microunits)).toBe(25_000_000);
    expect(Number(q.rate)).toBe(1);
    expect(Number(q.expires_at)).toBeGreaterThan(0);
    // NOTE: full on-chain settlement to the operator wallet (bill -> paid) is a
    // beta capability: it needs a live RPC verifier + chain sandbox, and guest
    // QR bills do not currently carry a settlement address, so settlement is not
    // exercised here. This journey proves only the accepted, FX-locked quote.
  });

  test("[claim:menu-multilingual] the guest menu is language-aware (translation is a no-op without a provider)", async () => {
    // NOTE: this row is `beta` in the registry — real translated menu text needs
    // a live Google Translate key, absent in the default compose stack. This test
    // is a regression guard for the language-aware read path + the documented
    // no-op, NOT a live coverage proof.
    const translate = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/menu/translate`,
      { headers: authHeaders(), data: { language_code: "es" } },
    );
    expect(translate.status(), await translate.text()).toBe(200);
    expect(String((await translate.json()).language_code)).toBe("es");

    // The guest read is parameterized by ?language= (the backend reads
    // c.Query("language")) and echoes the requested code.
    const en = await api.get(
      `${API_BASE}/guest/table/${tableCode}/menu?language=en`,
      { headers: guestHeaders() },
    );
    const es = await api.get(
      `${API_BASE}/guest/table/${tableCode}/menu?language=es`,
      { headers: guestHeaders() },
    );
    expect(en.status()).toBe(200);
    expect(es.status(), await es.text()).toBe(200);
    const esBody = await es.json();
    expect(esBody.language).toBe("es");
    const enName = (await en.json()).categories[0].items[0].name;
    const esName = esBody.categories[0].items[0].name;
    // With no translation provider configured, translation is a documented
    // no-op: the served name equals English. (Real es text requires the key.)
    expect(esName).toBe(enName);
  });

  test("[claim:offers-bundles] an offer and a priced bundle are created and served back", async () => {
    const offer = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/offers`,
      {
        headers: authHeaders(),
        data: {
          name: "Weekday Special",
          description: "15% off mains",
          discount_type: "percentage",
          discount_value: 15,
        },
      },
    );
    expect(offer.status(), await offer.text()).toBe(201);
    const offers = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/offers`,
      { headers: authHeaders() },
    );
    expect(offers.status()).toBe(200);
    expect(JSON.stringify(await offers.json())).toContain("Weekday Special");

    // A bundle is a distinct entity (priced package of items) — prove it too.
    const bundle = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/bundles`,
      {
        headers: authHeaders(),
        data: {
          name: "Combo Deal",
          description: "Two burgers",
          price: 22.0,
          currency: "USD",
          items: [{ name: "Burger", quantity: 2 }],
          is_active: true,
        },
      },
    );
    expect(bundle.status(), await bundle.text()).toBe(201);
    const bundleId = Number((await bundle.json()).id);
    expect(bundleId).toBeGreaterThan(0);
    const bundles = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bundles`,
      { headers: authHeaders() },
    );
    expect(bundles.status()).toBe(200);
    const list = (await bundles.json()) as Array<{ id: number; name: string; price: number }>;
    const combo = list.find((b) => b.id === bundleId);
    expect(combo, "created bundle is served back to operators").toBeTruthy();
    expect(combo!.name).toBe("Combo Deal");
    expect(Number(combo!.price)).toBe(22);
  });

  test("[claim:inventory-recipes] a menu item is mapped to an ingredient recipe", async () => {
    // The Burger -> 0.2kg Cheddar recipe is seeded in beforeAll; prove the
    // menu-item-to-ingredient mapping persists and reads back (a real BOM,
    // not just that an inventory item exists).
    const recipes = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/inventory/recipes`,
      { headers: authHeaders() },
    );
    expect(recipes.status(), await recipes.text()).toBe(200);
    const rows = (await recipes.json()).recipes as Array<{
      menu_item_id: string;
      menu_item_name: string;
      inventory_item_id: number;
      quantity_required: number;
    }>;
    const burger = rows.find((r) => r.menu_item_name === "Burger");
    expect(burger, "Burger recipe persisted").toBeTruthy();
    expect(burger!.inventory_item_id).toBe(cheddarId);
    expect(Number(burger!.quantity_required)).toBeCloseTo(0.2, 6);

    // And the tracked ingredient itself is listed.
    const items = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/inventory/items`,
      { headers: authHeaders() },
    );
    expect(JSON.stringify(await items.json())).toContain("Cheddar");
  });

  test("[claim:staff-rbac] role-scoped staff sessions are allowed and denied by permission", async () => {
    // The seeded server-role staff session is real (accept-invitation JWT).
    // ALLOWED (server has tables:read):
    const allowed = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/tables`,
      { headers: staffAuthHeaders() },
    );
    expect(allowed.status(), await allowed.text()).toBe(200);

    // DENIED (server lacks financial:read) — a real RBAC rejection, not a 404.
    const deniedPayroll = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs`,
      { headers: staffAuthHeaders() },
    );
    expect(deniedPayroll.status()).toBe(403);
    const denyBody = await deniedPayroll.json();
    expect(denyBody.code).toBe("AUTH_INSUFFICIENT_ROLE");
    expect(denyBody.params.required_permissions).toContain("financial:read");

    // DENIED (server lacks staff:invite):
    const deniedInvite = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/staff/invite`,
      {
        headers: staffAuthHeaders(),
        data: { email: "nope@payverge.test", name: "Nope", role: "server" },
      },
    );
    expect(deniedInvite.status()).toBe(403);

    // POSITIVE CONTROL: the SAME payroll route succeeds for the owner — proving
    // the 403 is role-based, not a broken route.
    const ownerPayroll = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs`,
      { headers: authHeaders() },
    );
    expect(ownerPayroll.status(), await ownerPayroll.text()).toBe(200);
  });

  test("[claim:scheduling-time-clock] a weekly schedule drafts and staff clock in and out", async () => {
    // Scheduling: a weekly schedule is created as a draft.
    const schedule = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/schedule`,
      { headers: authHeaders(), data: { week: nextMonday() } },
    );
    expect(schedule.status(), await schedule.text()).toBe(201);
    const scheduleData = (await schedule.json()).data;
    expect(String(scheduleData.status)).toBe("draft");
    expect(typeof scheduleData.week_start).toBe("string");

    // A real shift is assigned to the seeded staff and the schedule is published
    // — proving the scheduling half (assign + publish + read), not an empty draft.
    const position = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/positions`,
      { headers: authHeaders(), data: { name: "Server" } },
    );
    expect(position.status(), await position.text()).toBe(201);
    const positionId = Number((await position.json()).data.id);
    const weekStart = new Date(scheduleData.week_start).getTime();
    const shift = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/shifts`,
      {
        headers: authHeaders(),
        data: {
          schedule_id: Number(scheduleData.id),
          position_id: positionId,
          staff_id: staffId,
          starts_at: new Date(weekStart + 9 * 3_600_000).toISOString(),
          ends_at: new Date(weekStart + 17 * 3_600_000).toISOString(),
          break_minutes: 30,
        },
      },
    );
    expect(shift.status(), await shift.text()).toBe(201);
    const shiftId = Number((await shift.json()).data.id);
    const publish = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/schedule/${Number(scheduleData.id)}/publish`,
      { headers: authHeaders(), data: {} },
    );
    expect(publish.status(), await publish.text()).toBe(200);

    // The owner sees the published shift assigned to that staff member...
    const ownerRead = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/schedule?week=${nextMonday()}`,
      { headers: authHeaders() },
    );
    expect(ownerRead.status()).toBe(200);
    const publishedShift = (await ownerRead.json()).data.shifts.find(
      (s: { id: number }) => s.id === shiftId,
    );
    expect(publishedShift, "published shift assigned to the staff").toBeTruthy();
    expect(Number(publishedShift.staff_id)).toBe(staffId);
    expect(publishedShift.published).toBe(true);
    // ...and the staff can read their own schedule (server role has schedule:read).
    const staffSchedule = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/schedule?week=${nextMonday()}`,
      { headers: staffAuthHeaders() },
    );
    expect(staffSchedule.status(), await staffSchedule.text()).toBe(200);

    // Time clock: the staff punches in (open entry) then out (pending_review).
    const clockIn = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/me/clock-in`,
      { headers: staffAuthHeaders(), data: {} },
    );
    expect(clockIn.status(), await clockIn.text()).toBe(201);
    const inEntry = (await clockIn.json()).data;
    expect(inEntry.status).toBe("open");
    expect(Number(inEntry.staff_id)).toBe(staffId);
    expect(typeof inEntry.clock_in_at).toBe("string");
    expect(inEntry.clock_out_at).toBeNull();

    const clockOut = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/me/clock-out`,
      { headers: staffAuthHeaders(), data: {} },
    );
    expect(clockOut.status(), await clockOut.text()).toBe(200);
    const outEntry = (await clockOut.json()).data;
    expect(outEntry.status).toBe("pending_review");
    expect(typeof outEntry.clock_out_at).toBe("string");
  });

  test("[claim:team-messaging] an announcement is published to the team", async () => {
    const create = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/announcements`,
      { headers: authHeaders(), data: { title: "All-hands", content: "Team meeting at 4pm" } },
    );
    expect(create.status(), await create.text()).toBe(201);
    const list = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/announcements`,
      { headers: authHeaders() },
    );
    expect(list.status()).toBe(200);
    // Structured retrieval: the announcement is a real row carrying its title +
    // body, not just a substring somewhere in the payload.
    const rows = (await list.json()).data as Array<{ title: string; content: string }>;
    const ann = rows.find((a) => a.title === "All-hands");
    expect(ann, "announcement is retrievable as a structured row").toBeTruthy();
    expect(ann!.content).toBe("Team meeting at 4pm");
  });

  test("[claim:accounting-payroll] an expense entry and a payroll run are recorded and paid", async () => {
    // Accounting / expenses: a manual ledger entry is recorded.
    const entry = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/accounting/entries`,
      {
        headers: authHeaders(),
        data: {
          entry_type: "expense",
          occurred_at: "2026-07-10T00:00:00Z",
          description: "Produce delivery",
          amount: 8000,
          category: "supplies",
        },
      },
    );
    expect(entry.status(), await entry.text()).toBe(201);
    const entries = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/accounting/entries`,
      { headers: authHeaders() },
    );
    expect(JSON.stringify(await entries.json())).toContain("Produce delivery");

    // Payroll: a run is created (draft) with correct dollar-scale net math, then
    // marked paid. Historical period avoids overlapping the labor-seed run.
    const create = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs`,
      {
        headers: authHeaders(),
        data: {
          period_start: isoDay(-40),
          period_end: isoDay(-26),
          notes: "biweekly run",
          line_items: [
            { payee_type: "contractor", payee_name: "Sam Contractor", gross_amount: 1200, bonus_amount: 100, deduction_amount: 50, notes: "biweekly" },
          ],
        },
      },
    );
    expect(create.status(), await create.text()).toBe(201);
    const run = (await create.json()).data;
    const runId = Number(run.id);
    expect(run.status).toBe("draft");
    expect(Number(run.gross_total)).toBe(1200);
    expect(Number(run.net_total)).toBe(1250); // gross + bonus - deduction

    const list = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs`,
      { headers: authHeaders() },
    );
    expect(list.status()).toBe(200);
    const listed = (await list.json()).data.find((r: { id: number }) => r.id === runId);
    expect(listed, "payroll run persisted and lists").toBeTruthy();
    expect(Number(listed.net_total)).toBe(1250);

    const paid = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs/${runId}/mark-paid`,
      { headers: authHeaders(), data: {} },
    );
    expect(paid.status(), await paid.text()).toBe(200);
    const paidRun = (await paid.json()).data;
    expect(paidRun.status).toBe("paid");
    expect(typeof paidRun.paid_at).toBe("string");
  });

  test("[claim:analytics-reporting] a settled sale appears as a row in the exported sales report", async () => {
    const report = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/reports/export?format=csv&period=week`,
      { headers: authHeaders() },
    );
    expect(report.status(), await report.text()).toBe(200);
    const csv = await report.text();
    const lines = csv.trim().split(/\r?\n/).filter(Boolean);
    expect(lines[0]).toContain("Bill Number");
    expect(lines[0]).toContain("Payment Method");
    // The seeded settled sale is a REAL DATA ROW (not just the header line).
    const row = lines.find((l) => l.includes(seededBillNumber));
    expect(row, `CSV must contain the settled bill ${seededBillNumber}`).toBeTruthy();
    const cols = row!.split(",");
    // Columns: Recognized At,Bill Number,Bill ID,Source,Payment Method,Amount,Tip,Bill Status,Bill Total
    expect(cols[1]).toBe(seededBillNumber);
    expect(cols[4]).toBe("cash");
    expect(cols[5]).toBe("25.00");
    expect(cols[7]).toBe("paid");

    // The reporting DASHBOARD (not only the CSV export) reflects the same
    // recognized revenue — the settled $25 sale shows as non-zero week revenue.
    const dashboard = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/analytics/dashboard`,
      { headers: authHeaders() },
    );
    expect(dashboard.status(), await dashboard.text()).toBe(200);
    const summary = (await dashboard.json()).data;
    expect(Number(summary.week.revenue)).toBeGreaterThanOrEqual(25);
    expect(Number(summary.week.transactions)).toBeGreaterThanOrEqual(1);
  });

  test("[claim:crm-guest-profiles] a customer profile persists name and email in the CRM", async () => {
    const toggle = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/crm/toggle`,
      { headers: authHeaders(), data: { enabled: true } },
    );
    expect(toggle.status(), await toggle.text()).toBe(200);
    const customer = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/crm/customers`,
      {
        headers: authHeaders(),
        data: { name: "Grace Hopper", email: "grace@example.com", phone: "+15550001111" },
      },
    );
    expect(customer.status(), await customer.text()).toBe(201);
    const list = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/crm/customers`,
      { headers: authHeaders() },
    );
    expect(list.status()).toBe(200);
    const customers = (await list.json()).customers as Array<{
      customer: { name: string; email: string };
    }>;
    const row = customers.find((c) => c.customer?.email === "grace@example.com");
    expect(row, "created customer is retrievable by email").toBeTruthy();
    expect(row!.customer.name).toBe("Grace Hopper");
    // NOTE: phone is intentionally consent-redacted for operator reads until the
    // guest opts into cross-business PII sharing (crm/consent.go), so it is not
    // asserted here — the advertised, operator-provable outcome is name + email.
  });

  test("[claim:counter-service] a walk-up bill is created against an active counter", async () => {
    const update = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/counters/settings`,
      {
        headers: authHeaders(),
        data: { counter_enabled: true, counter_count: 2, counter_prefix: "C" },
      },
    );
    expect(update.status(), await update.text()).toBe(200);

    const countersRes = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/counters`,
      { headers: authHeaders() },
    );
    expect(countersRes.status()).toBe(200);
    const cbody = await countersRes.json();
    expect(cbody.business.counter_prefix).toBe("C");
    const counter = cbody.counters[0] as { id: number; name: string };
    expect(counter.name).toBe("C1");
    expect(counter.name.startsWith(cbody.business.counter_prefix)).toBe(true);

    // A walk-up bill is created against that active counter (not a table).
    const billRes = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/bills`,
      {
        headers: authHeaders(),
        data: {
          counter_id: counter.id,
          notes: "walk-up",
          items: [{ menu_item_id: "", name: "Burger", price: 12.5, quantity: 1 }],
        },
      },
    );
    expect(billRes.status(), await billRes.text()).toBe(201);
    const bill = (await billRes.json()).bill;
    expect(Number(bill.counter_id)).toBe(counter.id);
    expect(Number(bill.table_id)).toBe(0);
    // A real priced walk-up bill for the single $12.50 Burger (an active
    // percentage offer from an earlier test may discount it, so bound rather
    // than pin the exact total; the counter attribution is the outcome).
    expect(Number(bill.total_amount)).toBeGreaterThan(0);
    expect(Number(bill.total_amount)).toBeLessThanOrEqual(12.5);
    expect(bill.status).toBe("open");

    // It is listed among open bills, attributed to that counter.
    const openRes = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: authHeaders() },
    );
    expect(openRes.status()).toBe(200);
    const listed = (await openRes.json()).bills.find(
      (b: { id: number }) => b.id === Number(bill.id),
    );
    expect(listed, "counter bill appears in open bills").toBeTruthy();
    expect(Number(listed.counter_id)).toBe(counter.id);
  });

  test("[claim:prime-cost-analytics] prime cost composes labor and food cost from real data", async () => {
    const res = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/accounting/labor-cost?period=week`,
      { headers: authHeaders() },
    );
    expect(res.status(), await res.text()).toBe(200);
    const data = (await res.json()).data;
    // Real data: a paid sale (net sales) + a paid payroll run (labor) were
    // seeded, so this is NOT the 0+0 trivial identity.
    expect(data.has_data).toBe(true);
    expect(Number(data.payroll_run_count)).toBeGreaterThanOrEqual(1);
    expect(Number(data.labor_cost)).toBeGreaterThan(0);
    expect(Number(data.net_sales)).toBeGreaterThan(0);
    expect(Number(data.food_cost_pct)).toBeGreaterThan(0);
    // Prime cost is exactly the composition of labor% + food% — the advertised
    // definition, enforced on non-zero inputs.
    expect(Number(data.prime_cost_pct)).toBeCloseTo(
      Number(data.labor_cost_pct) + Number(data.food_cost_pct),
      6,
    );
    expect(Array.isArray(data.contributions)).toBe(true);
    expect(data.contributions.length).toBeGreaterThanOrEqual(1);
  });

  test("[claim:waste-variance-analytics] tracked waste and variance are computed from recipe usage", async () => {
    const res = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/accounting/waste-variance?period=week`,
      { headers: authHeaders() },
    );
    expect(res.status(), await res.text()).toBe(200);
    const data = (await res.json()).data;
    // The explicit -1kg Cheddar waste movement drives a clean, attributable loss
    // of $3.50 (1kg x $3.50) — assert the deterministic tracked loss, not the
    // seed-contaminated raw actual_usage.
    expect(Number(data.tracked_loss_cost)).toBeCloseTo(3.5, 2);
    expect(Array.isArray(data.loss_by_reason)).toBe(true);
    const spoilage = data.loss_by_reason.find(
      (l: { reason: string }) => l.reason === "spoilage",
    );
    expect(spoilage, "waste is attributed to its reason").toBeTruthy();
    expect(Number(spoilage.cost)).toBeCloseTo(3.5, 2);
    // Theoretical usage comes from the recipe x the sold quantity (0.2kg x 2).
    const cheddar = data.ingredients.find(
      (i: { name: string }) => i.name === "Cheddar",
    );
    expect(cheddar, "recipe-linked ingredient is tracked").toBeTruthy();
    expect(cheddar.has_recipe).toBe(true);
    expect(Number(cheddar.theoretical_usage)).toBeCloseTo(0.4, 6);
    expect(Number(data.total_variance_cost)).not.toBe(0);
  });

  test("[claim:menu-engineering] a sold dish is classified into a menu-engineering quadrant", async () => {
    const res = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/accounting/menu-engineering?period=week`,
      { headers: authHeaders() },
    );
    expect(res.status(), await res.text()).toBe(200);
    const data = (await res.json()).data;
    // The seeded paid Burger sale + its recipe cost put a REAL dish into a
    // quadrant with a real classification — not just that the keys exist.
    expect(Array.isArray(data.dishes)).toBe(true);
    const burger = data.dishes.find(
      (d: { menu_item_name: string }) => d.menu_item_name === "Burger",
    );
    expect(burger, "sold dish is classified").toBeTruthy();
    expect(Number(burger.qty_sold)).toBeGreaterThanOrEqual(2);
    expect(["star", "plowhorse", "puzzle", "dog"]).toContain(burger.quadrant);
    expect(typeof burger.action).toBe("string");
    expect(burger.action.length).toBeGreaterThan(0);
    expect(Array.isArray(data.rollups)).toBe(true);
    expect(data.rollups.some((r: { quadrant: string }) => r.quadrant === burger.quadrant)).toBe(true);
  });

  test("[claim:delivery-management] a delivery order is created and its fee is charged to the bill", async () => {
    const update = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/delivery-settings`,
      {
        headers: authHeaders(),
        data: {
          delivery_enabled: true,
          base_delivery_fee: 5.0,
          minimum_order_amount: 10.0,
          payment_mode: "cash_on_delivery",
          delivery_zones: [
            { name: "Zone A", priority: 1, delivery_fee: 5.0, minimum_order: 10.0, estimated_time: 30 },
          ],
        },
      },
    );
    expect(update.status(), await update.text()).toBe(200);

    // Fresh approved bill, then create a real delivery order against it. Read the
    // pre-delivery total first (an active percentage offer may discount it), so
    // the assertion is that the $5 fee is ADDED, independent of the base total.
    const code = await newTableCode("Delivery");
    const seat = await seatOrderedBill(code, 2);
    const before = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: authHeaders() },
    );
    const beforeTotal = Number(
      (await before.json()).bills.find((b: { id: number }) => b.id === seat.billId).total_amount,
    );
    const del = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/delivery-orders`,
      {
        headers: authHeaders(),
        data: {
          bill_id: seat.billId,
          customer_name: "Jane Doe",
          customer_phone: "+15551234567",
          customer_email: "jane@example.com",
          delivery_fee: 5.0,
          delivery_address: { street: "123 Main St", city: "NYC", state: "NY", postal_code: "10001", country: "US" },
          delivery_instructions: "Ring bell",
        },
      },
    );
    expect(del.status(), await del.text()).toBe(201);
    const created = await del.json();
    expect(String(created.delivery_number)).toMatch(/^DEL-[0-9A-F]{16}$/);
    expect(created.status).toBe("pending");
    expect(Number(created.delivery_fee)).toBe(5);
    expect(Number(created.bill_id)).toBe(seat.billId);

    const list = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/deliveries`,
      { headers: authHeaders() },
    );
    expect(list.status()).toBe(200);
    const lb = await list.json();
    expect(Number(lb.total)).toBeGreaterThanOrEqual(1);
    expect(
      lb.deliveries.some(
        (d: { delivery_number: string }) => d.delivery_number === created.delivery_number,
      ),
    ).toBe(true);

    // The $5 fee was actually CHARGED — folded into the bill total (+5).
    const bills = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: authHeaders() },
    );
    const billAfter = (await bills.json()).bills.find(
      (b: { id: number }) => b.id === seat.billId,
    );
    expect(billAfter, "delivery bill is present").toBeTruthy();
    expect(Number(billAfter.total_amount)).toBeCloseTo(beforeTotal + 5, 2);
  });

  test("[claim:setup-in-5-minutes] a self-onboarded business publishes a public storefront", async () => {
    // The "guided setup with review before publishing" promise is a DELIBERATE
    // publish step: prove the transition on a fresh business — the storefront is
    // NOT reachable while it is an unpublished draft, and becomes reachable only
    // after the operator publishes it. (A pre-published fixture would prove
    // nothing about the publish gate.)
    const stamp = Date.now();
    const email = `setup-${stamp}@payverge.test`;
    const reg = await api.post(`${API_BASE}/auth/register`, {
      headers: { Origin: ORIGIN },
      data: { email, password: "E2e!Setup-42", name: "Setup Owner" },
    });
    expect(reg.status(), await reg.text()).toBe(201);
    const setupToken = String((await reg.json()).token);
    const setupHeaders = { Authorization: `Bearer ${setupToken}`, Origin: ORIGIN };
    const setupUrl = `setup-${stamp}`;
    const biz = await api.post(`${API_BASE}/inside/businesses`, {
      headers: setupHeaders,
      data: {
        name: "Setup Storefront Cafe",
        owner_name: "Setup Owner",
        email,
        business_type: "restaurant",
        address: { country: "US" },
        custom_url: setupUrl,
        business_page_enabled: false, // unpublished draft
      },
    });
    expect(biz.status(), await biz.text()).toBe(201);
    const setupBizId = Number((await biz.json()).id);

    // BEFORE publishing: the storefront is not resolvable by a guest (404).
    const draftPage = await api.get(`${API_BASE}/business/${setupUrl}`, {
      headers: guestHeaders(),
    });
    expect(draftPage.status(), "unpublished storefront must not be public").toBe(404);

    // Complete onboarding, then publish (the review -> publish transition).
    const complete = await api.post(
      `${API_BASE}/inside/businesses/${setupBizId}/onboarding-state/complete`,
      { headers: setupHeaders, data: {} },
    );
    expect([200, 201]).toContain(complete.status());
    const publish = await api.put(`${API_BASE}/inside/businesses/${setupBizId}`, {
      headers: setupHeaders,
      data: { business_page_enabled: true },
    });
    expect(publish.status(), await publish.text()).toBe(200);

    // AFTER publishing: the same storefront is now readable by an unauthenticated
    // guest — the completed, guest-facing outcome that was gated before publish.
    const publicPage = await api.get(`${API_BASE}/business/${setupUrl}`, {
      headers: guestHeaders(),
    });
    expect(publicPage.status(), await publicPage.text()).toBe(200);
    expect(JSON.stringify(await publicPage.json())).toContain(setupUrl);

    await api.delete(`${API_BASE}/inside/businesses/${setupBizId}`, {
      headers: setupHeaders,
    });
  });

  test("[claim:zero-lost-tickets] a ticket survives every kitchen status transition", async () => {
    const code = await newTableCode("Ticket");
    const billRes = await api.post(`${API_BASE}/guest/table/${code}/bill`, {
      headers: guestHeaders(),
      data: {},
    });
    const billId = Number((await billRes.json()).bill.id);
    const order = await api.post(`${API_BASE}/guest/table/${code}/order`, {
      headers: guestOrderHeaders(`zero-lost-tickets-${code}`),
      data: { bill_id: billId, items: [{ menu_item_name: "Burger", quantity: 1 }] },
    });
    const orderId = Number((await order.json()).order.id);

    const boardHas = async (status: string) => {
      const res = await api.get(
        `${API_BASE}/inside/businesses/${businessId}/orders`,
        { headers: authHeaders() },
      );
      expect(res.status()).toBe(200);
      const found = (await res.json()).orders.find(
        (o: { id: number }) => o.id === orderId,
      );
      expect(found, `ticket ${orderId} still on the board`).toBeTruthy();
      expect(found.status).toBe(status);
    };

    // The ticket survives EVERY forward transition — never silently dropped.
    await boardHas("pending");
    for (const next of ["approved", "in_kitchen", "ready", "delivered"]) {
      const put = await api.put(
        `${API_BASE}/inside/businesses/${businessId}/orders/${orderId}/status`,
        { headers: authHeaders(), data: { status: next } },
      );
      expect(put.status(), await put.text()).toBe(200);
      await boardHas(next);
    }

    // A rejected illegal transition doesn't lose the ticket either: a fresh
    // pending order can't skip to "ready" (409), and stays on the board.
    const code2 = await newTableCode("Ticket2");
    const bill2 = await api.post(`${API_BASE}/guest/table/${code2}/bill`, {
      headers: guestHeaders(),
      data: {},
    });
    const billId2 = Number((await bill2.json()).bill.id);
    const order2 = await api.post(`${API_BASE}/guest/table/${code2}/order`, {
      headers: guestOrderHeaders(`zero-lost-tickets-${code2}`),
      data: { bill_id: billId2, items: [{ menu_item_name: "Burger", quantity: 1 }] },
    });
    const orderId2 = Number((await order2.json()).order.id);
    const illegal = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/orders/${orderId2}/status`,
      { headers: authHeaders(), data: { status: "ready" } },
    );
    expect(illegal.status()).toBe(409);
    expect((await illegal.json()).code).toBe("CONFLICT");
    const board2 = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/orders?status=pending`,
      { headers: authHeaders() },
    );
    const survivor = (await board2.json()).orders.find(
      (o: { id: number }) => o.id === orderId2,
    );
    expect(survivor, "ticket survives a rejected transition").toBeTruthy();
    expect(survivor.status).toBe("pending");
  });

  test("[claim:guest-locale-count] the platform serves publishable guest locales and labels others beta", async () => {
    const res = await api.get(`${API_BASE}/languages`, { headers: guestHeaders() });
    expect(res.status(), await res.text()).toBe(200);
    const languages = (await res.json()).languages as Array<{ code: string; is_active: boolean }>;
    const active = languages.filter((l) => l.is_active);
    const codes = active.map((l) => l.code);
    const publishableGuestLocales = Object.values(localeRegistry)
      .filter((locale) => locale.guestLocale && locale.publishable)
      .map((locale) => locale.canonical);
    expect(publishableGuestLocales.sort()).toEqual(["en", "es", "es-AR"].sort());
    for (const expected of publishableGuestLocales) {
      expect(codes).toContain(expected);
    }
  });

  test("[claim:onboarding-manual] an owner self-onboards a working business without AI", async () => {
    // Manual onboarding: register -> create business -> immediately operable,
    // no AI wizard involved.
    const stamp = Date.now();
    const email = `manual-onboard-${stamp}@payverge.test`;
    const reg = await api.post(`${API_BASE}/auth/register`, {
      headers: { Origin: ORIGIN },
      data: { email, password: "E2e!Manual-42", name: "Manual Owner" },
    });
    expect(reg.status(), await reg.text()).toBe(201);
    const manualToken = String((await reg.json()).token);
    const biz = await api.post(`${API_BASE}/inside/businesses`, {
      headers: { Authorization: `Bearer ${manualToken}`, Origin: ORIGIN },
      data: {
        name: "Manual Onboard Cafe",
        owner_name: "Manual Owner",
        email,
        business_type: "restaurant",
        address: { country: "US" },
        custom_url: `manual-onboard-${stamp}`,
        business_page_enabled: true,
      },
    });
    expect(biz.status(), await biz.text()).toBe(201);
    const manualBizId = Number((await biz.json()).id);
    // The business is immediately operable — a table can be created right away.
    const table = await api.post(
      `${API_BASE}/inside/businesses/${manualBizId}/tables`,
      { headers: { Authorization: `Bearer ${manualToken}`, Origin: ORIGIN }, data: { name: "M1", capacity: 4 } },
    );
    expect(table.status(), await table.text()).toBe(201);
    await api.delete(`${API_BASE}/inside/businesses/${manualBizId}`, {
      headers: { Authorization: `Bearer ${manualToken}`, Origin: ORIGIN },
    });
  });

  test("[claim:security-access-controls] protected routes reject unauthenticated and cross-tenant access", async ({ playwright }) => {
    // Unauthenticated request to a protected resource is rejected.
    const unauthApi = await playwright.request.newContext({ ignoreHTTPSErrors: true });
    const unauth = await unauthApi.get(
      `${API_BASE}/inside/businesses/${businessId}/staff`,
      { headers: { Origin: ORIGIN } },
    );
    await unauthApi.dispose();
    expect(unauth.status()).toBe(401);

    // A different owner cannot read this business's staff.
    const stamp = Date.now();
    const otherReg = await api.post(`${API_BASE}/auth/register`, {
      headers: { Origin: ORIGIN },
      data: {
        email: `intruder-${stamp}@payverge.test`,
        password: "E2e!Intruder-42",
        name: "Intruder",
      },
    });
    const otherToken = String((await otherReg.json()).token);
    const crossTenant = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/staff`,
      { headers: { Authorization: `Bearer ${otherToken}`, Origin: ORIGIN } },
    );
    expect([403, 404]).toContain(crossTenant.status());
  });

  test("[claim:security-pci-processor] the guest payment path never accepts raw card data", async () => {
    // SAQ-A posture: the guest payment path binds no PAN/CVV fields, so forged
    // card data is silently dropped — never echoed or stored. Card payments go
    // through a processor redirect (return_url/cancel_url), not server capture.
    const code = await newTableCode("PCI");
    const seat = await seatOrderedBill(code, 2);
    const req = await api.post(
      `${API_BASE}/guest/bill/${seat.token}/request-alternative-payment`,
      {
        headers: guestHeaders(),
        data: {
          amount: "25",
          payment_method: "card",
          participant_name: "Probe",
          // Forged card fields — NOT in the bound request struct:
          card_number: "4111111111111111",
          cvv: "123",
          pan: "4111111111111111",
          exp_month: "12",
          exp_year: "2030",
        },
      },
    );
    expect(req.status(), await req.text()).toBe(200);
    const bodyText = await req.text();
    // No card data is reflected back anywhere in the response.
    expect(bodyText).not.toMatch(/4111|cvv|"pan"|card_number|exp_month/);
    const body = JSON.parse(bodyText);
    expect(body.success).toBe(true);
    expect(Number(body.request_id)).toBeGreaterThan(0);
  });

  test("[claim:daily-weekly-automated-reports] daily and weekly report schedules enable, configure, and disable", async () => {
    const catalog = await api.get(`${API_BASE}/inside/plugins`, { headers: authHeaders() });
    expect(catalog.status(), await catalog.text()).toBe(200);
    const plugins = (await catalog.json()).plugins as Array<{ id: number; name: string }>;
    const daily = plugins.find((p) => p.name === "daily_email_report");
    const weekly = plugins.find((p) => p.name === "weekly_email_report");
    expect(daily, "daily_email_report is in the catalog").toBeTruthy();
    expect(weekly, "weekly_email_report is in the catalog").toBeTruthy();

    // Enable + configure the daily report schedule (hour + timezone), read it
    // back, confirm it is listed enabled, then disable it (reversible lifecycle).
    const enable = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/plugins/${daily!.id}/enable`,
      { headers: authHeaders(), data: { config: { enabled: true, hour: 7, timezone: "America/New_York" } } },
    );
    expect(enable.status(), await enable.text()).toBe(200);
    const cfg = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/plugins/${daily!.id}/config`,
      { headers: authHeaders() },
    );
    expect(cfg.status(), await cfg.text()).toBe(200);
    const config = (await cfg.json()).config;
    expect(config.enabled).toBe(true);
    expect(Number(config.hour)).toBe(7);
    expect(config.timezone).toBe("America/New_York");

    const bizPlugins = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/plugins`,
      { headers: authHeaders() },
    );
    const listed = ((await bizPlugins.json()).plugins as Array<{ name: string; is_enabled: boolean }>)
      .find((p) => p.name === "daily_email_report");
    expect(listed?.is_enabled).toBe(true);

    // Weekly enables + configures (hour/timezone/day_of_week) and reads its
    // config back too — proving BOTH cadences are fully configurable, not just daily.
    const enableWeekly = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/plugins/${weekly!.id}/enable`,
      { headers: authHeaders(), data: { config: { enabled: true, hour: 8, timezone: "America/New_York", day_of_week: 1 } } },
    );
    expect(enableWeekly.status(), await enableWeekly.text()).toBe(200);
    const weeklyCfg = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/plugins/${weekly!.id}/config`,
      { headers: authHeaders() },
    );
    expect(weeklyCfg.status(), await weeklyCfg.text()).toBe(200);
    const wconfig = (await weeklyCfg.json()).config;
    expect(wconfig.enabled).toBe(true);
    expect(Number(wconfig.hour)).toBe(8);
    expect(Number(wconfig.day_of_week)).toBe(1);
    expect(wconfig.timezone).toBe("America/New_York");

    // Disable is honored for BOTH cadences — each config read then 404s (reversible).
    for (const pid of [daily!.id, weekly!.id]) {
      const disable = await api.post(
        `${API_BASE}/inside/businesses/${businessId}/plugins/${pid}/disable`,
        { headers: authHeaders(), data: {} },
      );
      expect(disable.status(), await disable.text()).toBe(200);
      const cfgAfter = await api.get(
        `${API_BASE}/inside/businesses/${businessId}/plugins/${pid}/config`,
        { headers: authHeaders() },
      );
      expect(cfgAfter.status()).toBe(404);
    }
    // NOTE: the underlying ReportSchedule row + actual email dispatch (Resend in
    // prod / Postmark) are not exercised here — covered by unit tests. This
    // journey proves the operator-facing schedule enable/configure/disable lifecycle.
  });

  test("[claim:waiter-call] a guest raises a waiter call from the table and the floor resolves it", async () => {
    const code = await newTableCode("Waiter-Call");

    // The reason is a closed enum: it is embedded verbatim into staff-facing
    // alert copy, so guest free text is rejected outright.
    const freeText = await api.post(`${API_BASE}/guest/table/${code}/service-call`, {
      headers: guestHeaders(),
      data: { reason: "bring me a <script>alert(1)</script>" },
    });
    expect(freeText.status(), await freeText.text()).toBe(400);

    // Nothing is in flight for this table yet.
    const idle = await api.get(`${API_BASE}/guest/table/${code}/service-call`, {
      headers: guestHeaders(),
    });
    expect(idle.status(), await idle.text()).toBe(200);
    expect((await idle.json()).status).toBe("none");

    // The guest raises their hand from the QR page.
    const raise = await api.post(`${API_BASE}/guest/table/${code}/service-call`, {
      headers: guestHeaders(),
      data: { reason: "water" },
    });
    expect(raise.status(), await raise.text()).toBe(201);
    expect((await raise.json()).status).toBe("open");

    // The guest's own screen reflects it, and a double-tap does NOT queue a
    // second call — it returns the in-flight one.
    const poll = await api.get(`${API_BASE}/guest/table/${code}/service-call`, {
      headers: guestHeaders(),
    });
    expect((await poll.json()).status).toBe("open");
    const doubleTap = await api.post(`${API_BASE}/guest/table/${code}/service-call`, {
      headers: guestHeaders(),
      data: { reason: "water" },
    });
    expect(doubleTap.status(), await doubleTap.text()).toBe(200);
    expect((await doubleTap.json()).status).toBe("open");

    // Staff see exactly ONE urgent alert that names the table and the reason —
    // the double-tap did not duplicate it.
    const alertsRes = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/alerts?types=service_call`,
      { headers: authHeaders() },
    );
    expect(alertsRes.status(), await alertsRes.text()).toBe(200);
    const calls = (await alertsRes.json()).alerts as Array<{
      id: number;
      status: string;
      priority: string;
      title: string;
      body: string;
      metadata: Record<string, unknown>;
    }>;
    expect(calls.length).toBe(1);
    const call = calls[0];
    expect(call.status).toBe("open");
    expect(call.priority).toBe("urgent");
    expect(call.title).toBe("Service call — Waiter-Call");
    expect(call.body).toContain("water");
    expect(String(call.metadata.table_name)).toBe("Waiter-Call");
    expect(String(call.metadata.reason)).toBe("water");

    // A server resolves it and the guest's screen flips to resolved.
    const resolve = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/alerts/${call.id}/resolve`,
      { headers: authHeaders(), data: { reason: "water delivered" } },
    );
    expect(resolve.status(), await resolve.text()).toBe(200);
    expect((await resolve.json()).status).toBe("resolved");
    const settled = await api.get(`${API_BASE}/guest/table/${code}/service-call`, {
      headers: guestHeaders(),
    });
    expect((await settled.json()).status).toBe("resolved");

    // It also drops off the floor's active queue.
    const afterAlerts = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/alerts?types=service_call`,
      { headers: authHeaders() },
    );
    expect(((await afterAlerts.json()).alerts as unknown[]).length).toBe(0);

    // Calling again immediately is throttled, so one table cannot spam the floor.
    const spam = await api.post(`${API_BASE}/guest/table/${code}/service-call`, {
      headers: guestHeaders(),
      data: { reason: "check" },
    });
    expect(spam.status(), await spam.text()).toBe(429);
    expect(Number(spam.headers()["retry-after"])).toBeGreaterThan(0);
    expect(Number((await spam.json()).retry_after_seconds)).toBeGreaterThan(0);
  });

  test("[claim:operator-alerts] a new order raises a real-time alert that staff claim, resolve, and audit", async () => {
    // A guest orders — the alert is raised synchronously on the order write.
    const code = await newTableCode("Alerts");
    const billRes = await api.post(`${API_BASE}/guest/table/${code}/bill`, {
      headers: guestHeaders(),
      data: {},
    });
    expect(billRes.status(), await billRes.text()).toBe(201);
    const bill = (await billRes.json()).bill;
    const orderRes = await api.post(`${API_BASE}/guest/table/${code}/order`, {
      headers: guestOrderHeaders(`alerts-${code}`),
      data: {
        bill_id: Number(bill.id),
        items: [{ menu_item_name: "Burger", quantity: 1 }],
      },
    });
    expect(orderRes.status(), await orderRes.text()).toBe(201);
    const order = (await orderRes.json()).order;
    const orderId = Number(order.id);
    const orderNumber = String(order.order_number);

    type Alert = {
      id: number;
      status: string;
      priority: string;
      title: string;
      body: string;
      claimed_by_name: string;
      claimed_at: string | null;
      resolved_at: string | null;
      metadata: Record<string, unknown>;
    };
    const listOrderAlerts = async (): Promise<Alert[]> => {
      const res = await api.get(
        `${API_BASE}/inside/businesses/${businessId}/alerts?types=order_new`,
        { headers: authHeaders() },
      );
      expect(res.status(), await res.text()).toBe(200);
      return (await res.json()).alerts as Alert[];
    };

    const raised = (await listOrderAlerts()).find(
      (a) => Number(a.metadata.order_id) === orderId,
    );
    expect(raised, "the new order raised an operator alert").toBeTruthy();
    expect(raised!.status).toBe("open");
    expect(raised!.priority).toBe("urgent");
    // The alert names the real order, not a placeholder.
    expect(raised!.title).toBe(`New order #${orderNumber}`);
    expect(raised!.body).toContain(orderNumber);
    expect(Number(raised!.metadata.bill_id)).toBe(Number(bill.id));

    // A manager claims it — the alert records WHO took it, so two people don't
    // both walk the ticket.
    const claim = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/alerts/${raised!.id}/claim`,
      { headers: authHeaders(), data: { source: "dashboard" } },
    );
    expect(claim.status(), await claim.text()).toBe(200);
    const claimed = (await claim.json()) as Alert;
    expect(claimed.status).toBe("claimed");
    expect(claimed.claimed_by_name.length).toBeGreaterThan(0);
    expect(claimed.claimed_at).toBeTruthy();
    // A claimed alert is still on the board (it is being worked, not gone).
    expect(
      (await listOrderAlerts()).some((a) => a.id === raised!.id),
    ).toBe(true);

    // Resolving clears it from the board.
    const resolve = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/alerts/${raised!.id}/resolve`,
      { headers: authHeaders(), data: { reason: "sent to kitchen" } },
    );
    expect(resolve.status(), await resolve.text()).toBe(200);
    const resolved = (await resolve.json()) as Alert;
    expect(resolved.status).toBe("resolved");
    expect(resolved.resolved_at).toBeTruthy();
    // Resolution preserves the original claim attribution.
    expect(resolved.claimed_by_name).toBe(claimed.claimed_by_name);
    expect(
      (await listOrderAlerts()).some((a) => a.id === raised!.id),
    ).toBe(false);

    // The whole handling is auditable, in order.
    const events = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/alerts/${raised!.id}/events`,
      { headers: authHeaders() },
    );
    expect(events.status(), await events.text()).toBe(200);
    // The feed reads newest-first, so reversing it gives the handling order.
    const trail = ((await events.json()).events as Array<{ event_type: string; actor_name: string }>)
      .map((e) => e.event_type);
    expect(trail).toEqual(["resolved", "claimed", "created"]);
    expect([...trail].reverse()).toEqual(["created", "claimed", "resolved"]);

    // Operators tune how loudly alerts arrive, and the settings persist.
    const settingsRes = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/alerts/settings`,
      { headers: authHeaders() },
    );
    expect(settingsRes.status(), await settingsRes.text()).toBe(200);
    const settings = await settingsRes.json();
    const originalVolume = Number(settings.volume);
    const originalRepeat = Number(settings.repeat_interval_seconds);

    const update = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/alerts/settings`,
      {
        headers: authHeaders(),
        data: { ...settings, sound_enabled: true, volume: 0.4, repeat_interval_seconds: 45 },
      },
    );
    expect(update.status(), await update.text()).toBe(200);
    const readBack = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/alerts/settings`,
      { headers: authHeaders() },
    );
    const saved = await readBack.json();
    expect(Number(saved.volume)).toBeCloseTo(0.4, 5);
    expect(Number(saved.repeat_interval_seconds)).toBe(45);
    expect(saved.sound_enabled).toBe(true);

    // Nonsense cadences are rejected rather than silently clamped.
    const bad = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/alerts/settings`,
      { headers: authHeaders(), data: { ...saved, repeat_interval_seconds: 1 } },
    );
    expect(bad.status(), await bad.text()).toBe(400);

    // Restore so later claims read the shipped defaults.
    await api.put(`${API_BASE}/inside/businesses/${businessId}/alerts/settings`, {
      headers: authHeaders(),
      data: { ...saved, volume: originalVolume, repeat_interval_seconds: originalRepeat },
    });
    // NOTE: Telegram fan-out of these alerts requires the Telegram plugin
    // connected to a live bot (the registry row lists it as a prerequisite);
    // this journey proves the in-app alert lifecycle end to end.
  });

  test("[claim:printing-print-studio] a bill print job renders, routes to a station, and completes the browser print lifecycle", async () => {
    // Register the station an operator binds their browser print tab to.
    const printerRes = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/printers`,
      {
        headers: authHeaders(),
        data: {
          name: "Front Counter",
          role: "bill",
          transport: "browser",
          paper_width_mm: 80,
          enabled: true,
        },
      },
    );
    expect(printerRes.status(), await printerRes.text()).toBe(201);
    const printerId = Number((await printerRes.json()).id);
    expect(printerId).toBeGreaterThan(0);

    // It is discoverable as a bindable station by the print agent.
    const stations = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/print/stations`,
      { headers: authHeaders() },
    );
    expect(stations.status(), await stations.text()).toBe(200);
    const station = ((await stations.json()).items as Array<{
      id: number;
      role: string;
      paper_width_mm: number;
    }>).find((s) => s.id === printerId);
    expect(station, "the printer is offered as a bindable browser station").toBeTruthy();
    expect(station!.role).toBe("bill");
    expect(station!.paper_width_mm).toBe(80);

    // A real seated bill (2 x Burger) is queued for printing.
    const code = await newTableCode("Print-Station");
    const seat = await seatOrderedBill(code, 2);
    const openBills = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: authHeaders() },
    );
    const billRow = ((await openBills.json()).bills as Array<{ id: number; total_amount: number }>)
      .find((b) => b.id === seat.billId);
    expect(billRow, "the seated bill is open on the floor").toBeTruthy();
    const billTotal = Number(billRow!.total_amount);
    expect(billTotal).toBeGreaterThan(0);

    const create = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs`,
      {
        headers: authHeaders(),
        data: { kind: "bill", source_type: "bill", source_id: seat.billId, language: "en" },
      },
    );
    expect(create.status(), await create.text()).toBe(201);
    const job = await create.json();
    const jobId = Number(job.id);
    // The document is actually RENDERED and ROUTED to the bill station — the
    // payload carries this bill's number, table, item and total, not a stub.
    expect(job.status).toBe("routed");
    expect(Number(job.printer_id)).toBe(printerId);
    const html = String(job.payload_html);
    expect(html).toContain("Burger");
    expect(html).toContain(seat.number);
    expect(html).toContain("Print-Station");
    expect(html).toContain(`>${billTotal.toFixed(2)}<`);

    // Cross-tenant / bogus sources cannot be rendered.
    const bogus = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs`,
      {
        headers: authHeaders(),
        data: { kind: "bill", source_type: "bill", source_id: 99_999_999 },
      },
    );
    expect(bogus.status()).toBe(404);

    // The bound browser tab sees exactly one job waiting for it.
    const pending = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/pending-count?printer_id=${printerId}`,
      { headers: authHeaders() },
    );
    expect(pending.status(), await pending.text()).toBe(200);
    expect(Number((await pending.json()).count)).toBe(1);

    // Claim → present → confirm: the real browser-agent lease lifecycle.
    const clientId = "8f14e45f-ce0a-4d0b-9c3e-1f1b2c3d4e5f";
    const claim = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/claim`,
      { headers: authHeaders(), data: { client_id: clientId, printer_id: printerId } },
    );
    expect(claim.status(), await claim.text()).toBe(200);
    const claimed = await claim.json();
    expect(Number(claimed.job.id)).toBe(jobId);
    expect(String(claimed.job.payload_html)).toContain("Burger");
    expect(String(claimed.job.claimed_by)).toBe(clientId);
    expect(claimed.job.lease_expires_at).toBeTruthy();
    // Claiming took it off the queue, so a second tab cannot double-print it.
    expect(Number(claimed.pending_count)).toBe(0);

    const presented = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/${jobId}/presented`,
      { headers: authHeaders(), data: { client_id: clientId } },
    );
    expect(presented.status(), await presented.text()).toBe(200);
    const confirm = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/${jobId}/confirm`,
      { headers: authHeaders(), data: { client_id: clientId } },
    );
    expect(confirm.status(), await confirm.text()).toBe(200);
    expect((await confirm.json()).ok).toBe(true);

    // Another tab holding a stale lease cannot hijack a finished job.
    const hijack = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/${jobId}/confirm`,
      {
        headers: authHeaders(),
        data: { client_id: "1c1b0d2e-3a4f-4b5c-8d9e-0f1a2b3c4d5e" },
      },
    );
    expect(hijack.status()).toBe(409);

    // The queue history shows the completed print with its timestamps.
    type JobRow = {
      id: number;
      status: string;
      kind: string;
      presented_at: string | null;
      printed_at: string | null;
    };
    const history = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs?printer_id=${printerId}`,
      { headers: authHeaders() },
    );
    expect(history.status(), await history.text()).toBe(200);
    const printed = ((await history.json()).items as JobRow[]).find((j) => j.id === jobId);
    expect(printed, "the job is in the station's history").toBeTruthy();
    expect(printed!.status).toBe("printed");
    expect(printed!.presented_at).toBeTruthy();
    expect(printed!.printed_at).toBeTruthy();

    // Reprint re-renders the same source as a NEW auditable job (a printed job
    // is never silently reused), and it can be cancelled before it goes out.
    const reprint = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/${jobId}/reprint`,
      { headers: authHeaders(), data: {} },
    );
    expect(reprint.status(), await reprint.text()).toBe(201);
    const reJob = await reprint.json();
    expect(Number(reJob.id)).not.toBe(jobId);
    expect(reJob.status).toBe("routed");
    expect(Number(reJob.printer_id)).toBe(printerId);
    expect(String(reJob.payload_html)).toContain(seat.number);
    const cancel = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/${reJob.id}/cancel`,
      { headers: authHeaders(), data: {} },
    );
    expect(cancel.status(), await cancel.text()).toBe(200);
    const drained = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/pending-count?printer_id=${printerId}`,
      { headers: authHeaders() },
    );
    expect(Number((await drained.json()).count)).toBe(0);
    // NOTE: the Menu Print Studio's document rendering (page geometry, family
    // typography) is a client-side renderer covered by the dedicated visual
    // suite (playwright.menu-print.config.ts). This journey proves the
    // server-side receipt/kitchen print pipeline an operator actually depends on.
  });

  test("[claim:cash-register] a cash drawer session tracks tenders, movements, and closes with a real variance", async () => {
    // Cash recording is refused until a drawer is open — no silent Unassigned
    // cash path (#374). The seeded sale above is card, so unassigned stays empty.
    const refused = await api.post(
      `${API_BASE}/inside/bills/${(await seatOrderedBill(await newTableCode("No-Drawer"), 1)).billId}/alternative-payment`,
      {
        headers: { ...authHeaders(), "Idempotency-Key": `no-drawer-${Date.now()}` },
        data: { amount: "12.50", payment_method: "cash", business_confirmation: true },
      },
    );
    expect(refused.status(), await refused.text()).toBe(409);
    expect((await refused.json()).code).toBe("cash_session_required");

    const unassignedBefore = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/unassigned`,
      { headers: authHeaders() },
    );
    expect(unassignedBefore.status(), await unassignedBefore.text()).toBe(200);
    const unassigned = await unassignedBefore.json();
    expect(Number(unassigned.count)).toBe(0);
    expect(Number(unassigned.total)).toBeCloseTo(0, 2);

    // No drawer is open yet.
    const idle = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/current`,
      { headers: authHeaders() },
    );
    expect(idle.status(), await idle.text()).toBe(200);
    expect((await idle.json()).session).toBeFalsy();

    // Open the shift with a counted float.
    const open = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions`,
      {
        headers: authHeaders(),
        data: { opening_float: "100.00", opening_note: "morning float" },
      },
    );
    expect(open.status(), await open.text()).toBe(201);
    const session = await open.json();
    const sessionId = Number(session.id);
    expect(session.status).toBe("open");
    expect(Number(session.opening_float)).toBeCloseTo(100, 2);
    expect(Number(session.cash_sales)).toBe(0);
    // A closed drawer's expected/variance is not published while it is open.
    expect(session.variance).toBeUndefined();

    // Two drawers cannot be open at once.
    const second = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions`,
      { headers: authHeaders(), data: { opening_float: "50.00" } },
    );
    expect(second.status()).toBe(409);

    // A real cash sale lands in the OPEN drawer automatically.
    const code = await newTableCode("Register");
    const seat = await seatOrderedBill(code, 2);
    const openBills = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: authHeaders() },
    );
    const billRow = ((await openBills.json()).bills as Array<{ id: number; total_amount: number }>)
      .find((b) => b.id === seat.billId);
    expect(billRow, "the seated bill is open on the floor").toBeTruthy();
    const saleDollars = Number(billRow!.total_amount);
    expect(saleDollars).toBeGreaterThan(0);
    const settle = await api.post(`${API_BASE}/inside/bills/${seat.billId}/alternative-payment`, {
      headers: { ...authHeaders(), "Idempotency-Key": `register-${seat.billId}` },
      data: {
        amount: saleDollars.toFixed(2),
        payment_method: "cash",
        business_confirmation: true,
      },
    });
    expect(settle.status(), await settle.text()).toBe(200);
    expect((await settle.json()).payment_breakdown.is_complete).toBe(true);

    // A payout out of the drawer needs a reason, and only real tenders may
    // create a sale line — an operator cannot hand-write cash sales.
    const noReason = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions/${sessionId}/movements`,
      { headers: authHeaders(), data: { movement_type: "cash_out", amount: "10.00" } },
    );
    expect(noReason.status(), await noReason.text()).toBe(400);
    const forgedSale = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions/${sessionId}/movements`,
      {
        headers: authHeaders(),
        data: { movement_type: "cash_sale", amount: "500.00", reason: "forged" },
      },
    );
    expect(forgedSale.status(), await forgedSale.text()).toBe(400);

    const payout = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions/${sessionId}/movements`,
      {
        headers: authHeaders(),
        data: {
          movement_type: "cash_out",
          amount: "10.00",
          reason: "produce run",
          note: "corner market",
        },
      },
    );
    expect(payout.status(), await payout.text()).toBe(201);
    const afterPayout = (await payout.json()).session;
    expect(Number(afterPayout.cash_sales)).toBeCloseTo(saleDollars, 2);
    expect(Number(afterPayout.cash_out)).toBeCloseTo(10, 2);

    // The drawer ledger shows the automatic sale AND the manual payout.
    const detail = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions/${sessionId}`,
      { headers: authHeaders() },
    );
    expect(detail.status(), await detail.text()).toBe(200);
    const movements = (await detail.json()).movements as Array<{
      movement_type: string;
      amount: number;
      reason: string;
      bill_id: number | null;
    }>;
    const autoSale = movements.find((m) => m.movement_type === "cash_sale");
    expect(autoSale, "the cash tender posted itself to the open drawer").toBeTruthy();
    expect(Number(autoSale!.amount)).toBeCloseTo(saleDollars, 2);
    expect(Number(autoSale!.bill_id)).toBe(seat.billId);
    const manualOut = movements.find((m) => m.movement_type === "cash_out");
    expect(manualOut!.reason).toBe("produce run");
    expect(Number(manualOut!.amount)).toBeCloseTo(10, 2);

    // Close on a deliberately SHORT count: expected = 100 + sale - 10, and the
    // variance is the real shortfall, not an empty-math zero.
    const expectedDollars = 100 + saleDollars - 10;
    const countedDollars = expectedDollars - 3;
    const close = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions/${sessionId}/close`,
      {
        headers: authHeaders(),
        data: { counted_cash: countedDollars.toFixed(2), closing_note: "short by three" },
      },
    );
    expect(close.status(), await close.text()).toBe(200);
    const closed = await close.json();
    expect(closed.status).toBe("closed");
    expect(Number(closed.expected_cash)).toBeCloseTo(expectedDollars, 2);
    expect(Number(closed.counted_cash)).toBeCloseTo(countedDollars, 2);
    expect(Number(closed.variance)).toBeCloseTo(-3, 2);
    expect(closed.closed_at).toBeTruthy();

    // A closed drawer is closed: no late movements, and the floor is clear.
    const late = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions/${sessionId}/movements`,
      {
        headers: authHeaders(),
        data: { movement_type: "cash_in", amount: "5.00", reason: "too late" },
      },
    );
    expect(late.status()).toBe(409);
    const after = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/current`,
      { headers: authHeaders() },
    );
    expect((await after.json()).session).toBeFalsy();

    // The shift is retrievable in the session history for reconciliation.
    const historyRes = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/sessions`,
      { headers: authHeaders() },
    );
    expect(historyRes.status(), await historyRes.text()).toBe(200);
    const history = await historyRes.json();
    expect(Number(history.total)).toBe(1);
    const past = (history.sessions as Array<{ id: number; variance: number }>)
      .find((s) => s.id === sessionId);
    expect(Number(past!.variance)).toBeCloseTo(-3, 2);

    // The sale taken inside the session is assigned to the drawer — nothing
    // is left in Unassigned cash.
    const unassignedAfter = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/cash-register/unassigned`,
      { headers: authHeaders() },
    );
    const stillOrphan = await unassignedAfter.json();
    expect(Number(stillOrphan.count)).toBe(0);
    expect(Number(stillOrphan.total)).toBeCloseTo(0, 2);
  });

  test("[claim:digital-tipping] a tip is collected at checkout, kept out of bill revenue, and reported in tip analytics", async () => {
    const code = await newTableCode("Tips");
    const seat = await seatOrderedBill(code, 2);
    const openBills = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: authHeaders() },
    );
    const billRow = ((await openBills.json()).bills as Array<{ id: number; total_amount: number }>)
      .find((b) => b.id === seat.billId);
    expect(billRow, "the seated bill is open on the floor").toBeTruthy();
    const billDollars = Number(billRow!.total_amount);
    expect(billDollars).toBeGreaterThan(0);
    const billCents = Math.round(billDollars * 100);

    // Checkout: the guest adds a $5 tip on top of the bill.
    const settle = await api.post(`${API_BASE}/inside/bills/${seat.billId}/alternative-payment`, {
      headers: { ...authHeaders(), "Idempotency-Key": `tip-${seat.billId}` },
      data: {
        amount: billDollars.toFixed(2),
        tip_amount: "5.00",
        payment_method: "card",
        participant_name: "Tipping Guest",
        business_confirmation: true,
      },
    });
    expect(settle.status(), await settle.text()).toBe(200);
    const breakdown = (await settle.json()).payment_breakdown;
    // The tip does NOT inflate what the bill owed: the bill is settled exactly.
    expect(Number(breakdown.total_amount)).toBeCloseTo(billDollars, 2);
    expect(Number(breakdown.alternative_paid)).toBeCloseTo(billDollars, 2);
    expect(Number(breakdown.remaining)).toBeCloseTo(0, 2);
    expect(breakdown.is_complete).toBe(true);

    // The guest's own bill view shows the tip recorded alongside the tender.
    const guestView = await api.get(
      `${API_BASE}/guest/bill/${seat.token}/alternative-payments`,
      { headers: guestHeaders() },
    );
    expect(guestView.status(), await guestView.text()).toBe(200);
    const tenders = (await guestView.json()).alternative_payments as Array<{
      amount: number;
      tip_amount_cents: number;
      bill_amount_cents: number;
      payment_method: string;
      status: string;
      participant_name: string;
    }>;
    expect(tenders.length).toBe(1);
    const tender = tenders[0];
    expect(tender.status).toBe("confirmed");
    expect(tender.payment_method).toBe("card");
    expect(tender.participant_name).toBe("Tipping Guest");
    expect(Number(tender.amount)).toBeCloseTo(billDollars, 2);
    expect(Number(tender.bill_amount_cents)).toBe(billCents);
    expect(Number(tender.tip_amount_cents)).toBe(500);

    // Tip analytics reports it — this business's ONLY tipped tender, so the
    // math is exact rather than a moving aggregate.
    const tipsRes = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/analytics/tips?period=week`,
      { headers: authHeaders() },
    );
    expect(tipsRes.status(), await tipsRes.text()).toBe(200);
    const report = (await tipsRes.json()).data as {
      total_tips: number;
      tip_count: number;
      average_tip: number;
      average_tip_rate: number;
      tip_distribution: Record<string, number>;
    };
    expect(Number(report.total_tips)).toBeCloseTo(5, 2);
    expect(Number(report.tip_count)).toBe(1);
    expect(Number(report.average_tip)).toBeCloseTo(5, 2);
    // Rate is tip / tendered bill amount, so it excludes the tip itself.
    expect(Number(report.average_tip_rate)).toBeCloseTo((500 / billCents) * 100, 1);
    // $5 lands in the $5–$10 band.
    expect(Number(report.tip_distribution["5_10"])).toBe(1);
    // NOTE: the untipped $25 cash sale seeded in beforeAll is deliberately not
    // counted here — tip_count is positive-tip tenders only.
  });

  test("[claim:loyalty-rewards] a loyalty program publishes its tiers and redemption rate and guards point redemption", async () => {
    // The operator turns on loyalty with a real tier ladder.
    const configure = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/crm/loyalty`,
      {
        headers: authHeaders(),
        data: {
          enabled: true,
          points_per_dollar: 1,
          redemption_points_per_dollar: 20,
          tiers: [
            { name: "Bronze", min_lifetime_spent: 0, sort_order: 0 },
            { name: "Gold", min_lifetime_spent: 250, sort_order: 1 },
          ],
        },
      },
    );
    expect(configure.status(), await configure.text()).toBe(200);

    // A ladder with duplicate names is rejected, so tiers stay meaningful.
    const dupe = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/crm/loyalty`,
      {
        headers: authHeaders(),
        data: {
          enabled: true,
          points_per_dollar: 1,
          redemption_points_per_dollar: 20,
          tiers: [
            { name: "Gold", min_lifetime_spent: 0, sort_order: 0 },
            { name: "gold", min_lifetime_spent: 250, sort_order: 1 },
          ],
        },
      },
    );
    expect(dupe.status(), await dupe.text()).toBe(400);

    // It persists, with thresholds on the wire in dollars.
    const read = await api.get(`${API_BASE}/inside/businesses/${businessId}/crm/loyalty`, {
      headers: authHeaders(),
    });
    expect(read.status(), await read.text()).toBe(200);
    const loyalty = await read.json();
    expect(loyalty.program.enabled).toBe(true);
    expect(Number(loyalty.program.points_per_dollar)).toBe(1);
    expect(Number(loyalty.program.redemption_points_per_dollar)).toBe(20);
    const tiers = (loyalty.tiers as Array<{ name: string; min_lifetime_spent: number }>)
      .slice()
      .sort((a, b) => a.min_lifetime_spent - b.min_lifetime_spent);
    expect(tiers.map((t) => t.name)).toEqual(["Bronze", "Gold"]);
    expect(Number(tiers[1].min_lifetime_spent)).toBeCloseTo(250, 2);

    // The guest QR page reads the live redeem rate before anyone logs in, so the
    // discount estimate a guest sees is the operator's actual redeem rate.
    const code = await newTableCode("Loyalty");
    const seat = await seatOrderedBill(code, 2);
    const publicRate = await api.get(`${API_BASE}/guest/table/${code}/loyalty-rate`, {
      headers: guestHeaders(),
    });
    expect(publicRate.status(), await publicRate.text()).toBe(200);
    expect(Number((await publicRate.json()).points_per_dollar)).toBe(20);

    // A real diner enrolls and signs in.
    const stamp = Date.now();
    const customerEmail = `diner-${stamp}@payverge.test`;
    const register = await api.post(`${API_BASE}/crm/register`, {
      headers: { Origin: ORIGIN },
      data: { email: customerEmail, password: "E2e!Diner-42", name: "Loyal Diner" },
    });
    expect(register.status(), await register.text()).toBe(201);
    const login = await api.post(`${API_BASE}/crm/login`, {
      headers: { Origin: ORIGIN },
      data: { email: customerEmail, password: "E2e!Diner-42" },
    });
    expect(login.status(), await login.text()).toBe(200);
    const customerToken = String((await login.json()).token);
    expect(customerToken.length).toBeGreaterThan(10);
    const customerHeaders = { Authorization: `Bearer ${customerToken}`, Origin: ORIGIN };

    // Checking in at the table links them to this restaurant AND to the open
    // bill, which is what makes the points balance addressable at checkout.
    const checkIn = await api.post(`${API_BASE}/customer/table/${code}/check-in`, {
      headers: customerHeaders,
      data: {},
    });
    expect(checkIn.status(), await checkIn.text()).toBe(200);
    const checkedIn = await checkIn.json();
    expect(checkedIn.bill_attached).toBe(true);
    expect(Number(checkedIn.bill_id)).toBe(seat.billId);
    expect(Number(checkedIn.customer_business.business_id)).toBe(businessId);
    expect(Number(checkedIn.customer_business.loyalty_points)).toBe(0);

    // Redemption is balance-guarded: a diner with no points cannot discount the
    // bill, and the bill total is provably untouched by the attempt.
    const beforeRedeem = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: authHeaders() },
    );
    const totalBefore = Number(
      ((await beforeRedeem.json()).bills as Array<{ id: number; total_amount: number }>)
        .find((b) => b.id === seat.billId)!.total_amount,
    );
    const overdraw = await api.post(`${API_BASE}/guest/table/${code}/redeem-points`, {
      headers: customerHeaders,
      data: { points: 500 },
    });
    expect(overdraw.status(), await overdraw.text()).toBe(422);
    expect((await overdraw.json()).code).toBe("insufficient_points");
    const afterRedeem = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: authHeaders() },
    );
    const totalAfter = Number(
      ((await afterRedeem.json()).bills as Array<{ id: number; total_amount: number }>)
        .find((b) => b.id === seat.billId)!.total_amount,
    );
    expect(totalAfter).toBeCloseTo(totalBefore, 2);

    // Turning the program off takes the offer off the guest page immediately.
    const disable = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/crm/loyalty`,
      {
        headers: authHeaders(),
        data: {
          enabled: false,
          points_per_dollar: 1,
          redemption_points_per_dollar: 20,
          tiers: [],
        },
      },
    );
    expect(disable.status(), await disable.text()).toBe(200);
    const rateOff = await api.get(`${API_BASE}/guest/table/${code}/loyalty-rate`, {
      headers: guestHeaders(),
    });
    expect(Number((await rateOff.json()).points_per_dollar)).toBe(0);
    // NOTE: point ACCRUAL from a settled bill runs synchronously only for
    // crypto/cross-chain/plugin-webhook settlements; cash/card tenders accrue
    // via the periodic ReconcileCRMSettlementVisits job, which this
    // request-scoped journey cannot wait on. Accrual is covered by backend
    // tests; this journey proves the operator-configured program, the
    // guest-facing rate, enrollment, table check-in, and the redemption guard.
  });
});
