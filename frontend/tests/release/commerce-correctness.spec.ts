/**
 * Commerce correctness acceptance against the full API stack (root docker-compose.yml).
 *
 * The public/operator quote APIs intentionally do not accept an `at_time`
 * override: letting a client choose the pricing clock would let it revive an
 * expired offer. Exact Friday/Saturday and DST instants therefore remain in
 * the deterministic Go evaluator tests. This image-level journey supplies the
 * strongest safe wiring proof: it chooses a business timezone whose local day
 * differs from UTC, proves the UTC weekday is excluded, proves the local day
 * is included, and then proves an overnight window around the server instant.
 *
 * The same fixture also locks the PV-PROD-011 money boundary: 15% of $18.50 is
 * $2.775, so the server must round the discount once to $2.78 and publish the
 * exact same quote to guest and operator clients. A fixed offer is then
 * stacked, followed by an over-subtotal offer that proves the zero floor.
 */
import {
  expect,
  test,
  type APIRequestContext,
  type APIResponse,
} from "@playwright/test";

const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://127.0.0.1:8080/api/v1";

type QuoteLine = {
  key: string;
  line_type: string;
  unit_price: number;
  quantity: number;
  subtotal: number;
};

type QuoteBody = {
  subtotal: number;
  discount: number;
  net_subtotal: number;
  tax: number;
  service_fee: number;
  tip: number;
  total: number;
  lines: QuoteLine[];
};

type BusinessClock = {
  timezone: string;
  weekday: number;
  minute: number;
};

const WEEKDAYS = new Map([
  ["Sun", 0],
  ["Mon", 1],
  ["Tue", 2],
  ["Wed", 3],
  ["Thu", 4],
  ["Fri", 5],
  ["Sat", 6],
]);

function localClock(at: Date, timezone: string): BusinessClock {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: timezone,
    weekday: "short",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(at);
  const value = (type: Intl.DateTimeFormatPartTypes): string =>
    parts.find((part) => part.type === type)?.value ?? "";
  const weekday = WEEKDAYS.get(value("weekday"));
  const hour = Number(value("hour"));
  const minute = Number(value("minute"));
  if (weekday == null || !Number.isInteger(hour) || !Number.isInteger(minute)) {
    throw new Error(`could not resolve ${timezone} business clock`);
  }
  return { timezone, weekday, minute: hour * 60 + minute };
}

function chooseBoundaryTimezone(at: Date): BusinessClock {
  const utcWeekday = at.getUTCDay();
  const candidates = ["Pacific/Kiritimati", "Pacific/Pago_Pago"]
    .map((timezone) => localClock(at, timezone))
    .filter(({ weekday, minute }) => {
      const distanceFromMidnight = Math.min(minute, 1440 - minute);
      return weekday !== utcWeekday && distanceFromMidnight >= 30;
    });
  if (candidates.length === 0) {
    throw new Error(
      "could not select a stable business timezone on the opposite UTC calendar day",
    );
  }
  return candidates[0];
}

function weekdayMask(weekday: number): number {
  return 1 << weekday;
}

function toCents(value: unknown, label: string): number {
  expect(typeof value, `${label} must be numeric`).toBe("number");
  const scaled = Number(value) * 100;
  expect(
    Math.abs(scaled - Math.round(scaled)),
    `${label} must resolve to whole cents`,
  ).toBeLessThan(1e-6);
  return Math.round(scaled);
}

function financialProjection(quote: QuoteBody) {
  return {
    subtotal: toCents(quote.subtotal, "subtotal"),
    discount: toCents(quote.discount, "discount"),
    netSubtotal: toCents(quote.net_subtotal, "net_subtotal"),
    tax: toCents(quote.tax, "tax"),
    serviceFee: toCents(quote.service_fee, "service_fee"),
    tip: toCents(quote.tip, "tip"),
    total: toCents(quote.total, "total"),
    lines: quote.lines.map((line) => ({
      key: line.key,
      lineType: line.line_type,
      unitPrice: toCents(line.unit_price, `${line.key}.unit_price`),
      quantity: line.quantity,
      subtotal: toCents(line.subtotal, `${line.key}.subtotal`),
    })),
  };
}

async function expectResponse(response: APIResponse, status: number) {
  expect(response.status(), await response.text()).toBe(status);
}

async function createOffer(
  api: APIRequestContext,
  businessId: number,
  authHeaders: { Authorization: string },
  data: Record<string, unknown>,
): Promise<number> {
  const response = await api.post(
    `${API_BASE}/inside/businesses/${businessId}/offers`,
    { headers: authHeaders, data },
  );
  await expectResponse(response, 201);
  const id = Number((await response.json()).id);
  expect(id, "created offer id").toBeGreaterThan(0);
  return id;
}

async function quotePair(
  api: APIRequestContext,
  businessId: number,
  tableCode: string,
  authHeaders: { Authorization: string },
  itemId: string,
): Promise<{ guest: QuoteBody; operator: QuoteBody }> {
  const data = {
    items: [
      {
        menu_item_name: "Boundary Bowl",
        menu_item_id: itemId,
        quantity: 1,
        price: 18.5,
      },
    ],
  };
  const [guestResponse, operatorResponse] = await Promise.all([
    api.post(`${API_BASE}/guest/table/${tableCode}/order/quote`, { data }),
    api.post(`${API_BASE}/inside/businesses/${businessId}/orders/quote`, {
      headers: authHeaders,
      data,
    }),
  ]);
  await expectResponse(guestResponse, 200);
  await expectResponse(operatorResponse, 200);
  return {
    guest: (await guestResponse.json()) as QuoteBody,
    operator: (await operatorResponse.json()) as QuoteBody,
  };
}

async function guestOfferNames(
  api: APIRequestContext,
  tableCode: string,
): Promise<string[]> {
  const response = await api.get(`${API_BASE}/guest/table/${tableCode}/menu`);
  await expectResponse(response, 200);
  const body = (await response.json()) as {
    offers?: Array<{ name?: string }>;
  };
  return (body.offers ?? []).map((offer) => String(offer.name ?? ""));
}

test.describe("Commerce correctness release acceptance", () => {
  test("PV-PROD-003 schedules in business time and PV-PROD-011 quotes match at cent boundaries", async ({
    request: api,
  }) => {
    test.setTimeout(120_000);
    const stamp = Date.now();
    const email = `commerce-release-${stamp}@payverge.test`;
    const password = "E2e!Commerce-Quote-42";
    let businessId = 0;
    let token = "";

    try {
      const registration = await api.post(`${API_BASE}/auth/register`, {
        data: {
          email,
          password,
          name: "Commerce Release Owner",
        },
      });
      await expectResponse(registration, 201);
      const registrationBody = await registration.json();
      token = String(registrationBody.token ?? "");
      expect(token, "registration bearer token").toBeTruthy();
      const authHeaders = { Authorization: `Bearer ${token}` };

      const responseDate = registration.headers().date;
      const serverNow = new Date(responseDate || Date.now());
      expect(Number.isNaN(serverNow.getTime()), "server Date header").toBe(
        false,
      );
      const businessClock = chooseBoundaryTimezone(serverNow);
      expect(businessClock.weekday).not.toBe(serverNow.getUTCDay());

      const businessResponse = await api.post(`${API_BASE}/inside/businesses`, {
        headers: authHeaders,
        data: {
          name: `Commerce Boundary Bistro ${stamp}`,
          owner_name: "Commerce Release Owner",
          email,
          business_type: "restaurant",
          address: { country: "US" },
          custom_url: `commerce-boundary-${stamp}`,
          business_page_enabled: true,
          timezone: businessClock.timezone,
          tax_rate: 8.5,
          service_fee_rate: 8.5,
        },
      });
      await expectResponse(businessResponse, 201);
      businessId = Number((await businessResponse.json()).id);
      expect(businessId, "created business id").toBeGreaterThan(0);

      const kitchenResponse = await api.post(
        `${API_BASE}/inside/businesses/${businessId}/toggle-kitchen-orders`,
        { headers: authHeaders, data: { enabled: true } },
      );
      expect(kitchenResponse.ok(), await kitchenResponse.text()).toBeTruthy();

      const categoryResponse = await api.post(
        `${API_BASE}/inside/businesses/${businessId}/menu/categories`,
        {
          headers: authHeaders,
          data: { name: "Boundary Mains", description: "" },
        },
      );
      await expectResponse(categoryResponse, 201);
      const itemResponse = await api.post(
        `${API_BASE}/inside/businesses/${businessId}/menu/items`,
        {
          headers: authHeaders,
          data: {
            category_index: 0,
            item: {
              name: "Boundary Bowl",
              description: "Half-cent quote fixture",
              price: 18.5,
              currency: "USD",
              is_available: true,
            },
          },
        },
      );
      await expectResponse(itemResponse, 201);
      const tableResponse = await api.post(
        `${API_BASE}/inside/businesses/${businessId}/tables`,
        {
          headers: authHeaders,
          data: { name: `CB-${stamp % 10000}`, capacity: 2 },
        },
      );
      await expectResponse(tableResponse, 201);
      const tableCode = String((await tableResponse.json()).table_code ?? "");
      expect(tableCode, "created table code").toBeTruthy();

      const menuResponse = await api.get(
        `${API_BASE}/guest/table/${tableCode}/menu`,
      );
      await expectResponse(menuResponse, 200);
      const menuBody = (await menuResponse.json()) as {
        categories?: Array<{
          items?: Array<{ id?: string; name?: string }>;
        }>;
      };
      const itemId = String(
        (menuBody.categories ?? [])
          .flatMap((category) => category.items ?? [])
          .find((item) => item.name === "Boundary Bowl")?.id ?? "",
      );
      expect(itemId, "boundary menu item id").toBeTruthy();

      // The business-local day differs from UTC. A UTC-day-only offer must not
      // leak into either the guest menu or the authoritative guest quote.
      await createOffer(api, businessId, authHeaders, {
        name: "Wrong UTC Day",
        description: "Must be inactive on the business-local calendar day",
        discount_type: "fixed",
        discount_value: 5,
        weekday_mask: weekdayMask(serverNow.getUTCDay()),
        is_active: true,
        applicable_to: "all",
      });
      expect(await guestOfferNames(api, tableCode)).not.toContain(
        "Wrong UTC Day",
      );
      const excluded = await quotePair(
        api,
        businessId,
        tableCode,
        authHeaders,
        itemId,
      );
      expect(financialProjection(excluded.guest).discount).toBe(0);
      expect(financialProjection(excluded.operator)).toEqual(
        financialProjection(excluded.guest),
      );

      // 15% of $18.50 is the historical half-cent failure ($2.775). Both APIs
      // must publish one rounded $2.78 discount and the same settlement cents.
      await createOffer(api, businessId, authHeaders, {
        name: "Business Day 15 Percent",
        description: "Active only on the business-local weekday",
        discount_type: "percentage",
        discount_value: 15,
        weekday_mask: weekdayMask(businessClock.weekday),
        is_active: true,
        applicable_to: "all",
      });
      expect(await guestOfferNames(api, tableCode)).toContain(
        "Business Day 15 Percent",
      );
      const halfCent = await quotePair(
        api,
        businessId,
        tableCode,
        authHeaders,
        itemId,
      );
      expect(financialProjection(halfCent.operator)).toEqual(
        financialProjection(halfCent.guest),
      );
      expect(financialProjection(halfCent.guest)).toMatchObject({
        subtotal: 1850,
        discount: 278,
        netSubtotal: 1572,
        tax: 134,
        serviceFee: 134,
        tip: 0,
        total: 1840,
      });

      // Construct an overnight window that contains the current business
      // minute with at least a 15-minute cushion. Before noon the window began
      // on the previous local weekday; after noon it began on the current day.
      const beforeNoon = businessClock.minute < 720;
      const overnightStart = beforeNoon
        ? businessClock.minute + 30
        : businessClock.minute - 15;
      const overnightEnd = beforeNoon
        ? businessClock.minute + 15
        : businessClock.minute - 30;
      const overnightOwnerDay = beforeNoon
        ? (businessClock.weekday + 6) % 7
        : businessClock.weekday;
      expect(overnightStart).toBeGreaterThan(overnightEnd);
      await createOffer(api, businessId, authHeaders, {
        name: "Business Overnight Fixed",
        description: "Cross-midnight business-time fixture",
        discount_type: "fixed",
        discount_value: 1.03,
        weekday_mask: weekdayMask(overnightOwnerDay),
        start_minute: overnightStart,
        end_minute: overnightEnd,
        is_active: true,
        applicable_to: "all",
      });
      expect(await guestOfferNames(api, tableCode)).toContain(
        "Business Overnight Fixed",
      );
      const stacked = await quotePair(
        api,
        businessId,
        tableCode,
        authHeaders,
        itemId,
      );
      expect(financialProjection(stacked.operator)).toEqual(
        financialProjection(stacked.guest),
      );
      expect(financialProjection(stacked.guest)).toMatchObject({
        subtotal: 1850,
        discount: 381,
        netSubtotal: 1469,
        tax: 125,
        serviceFee: 125,
        tip: 0,
        total: 1719,
      });

      // A final fixed offer exceeds the remaining subtotal. The shared quote
      // engine must cap stacked discounts at the subtotal, never go negative,
      // and preserve guest/operator equality at the zero boundary.
      await createOffer(api, businessId, authHeaders, {
        name: "Subtotal Cap Boundary",
        description: "Must floor the payable quote at zero",
        discount_type: "fixed",
        discount_value: 100,
        weekday_mask: weekdayMask(businessClock.weekday),
        is_active: true,
        applicable_to: "all",
      });
      const capped = await quotePair(
        api,
        businessId,
        tableCode,
        authHeaders,
        itemId,
      );
      expect(financialProjection(capped.operator)).toEqual(
        financialProjection(capped.guest),
      );
      expect(financialProjection(capped.guest)).toMatchObject({
        subtotal: 1850,
        discount: 1850,
        netSubtotal: 0,
        tax: 0,
        serviceFee: 0,
        tip: 0,
        total: 0,
      });
    } finally {
      if (businessId > 0 && token) {
        try {
          await api.delete(`${API_BASE}/inside/businesses/${businessId}`, {
            headers: { Authorization: `Bearer ${token}` },
          });
        } catch {
          // Cleanup must not hide the acceptance failure.
        }
      }
    }
  });
});
