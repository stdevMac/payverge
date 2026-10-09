import {
  guestOrderErrorInfo,
  presentGuestOrderError,
  isOpenBillConflict,
  GUEST_ORDER_QUANTITY_CAP,
  GUEST_ORDER_LINE_CAP,
  GUEST_ORDER_TEXT_CAP,
} from "@/lib/guestOrderErrors";

// Real axios error shape: axios.isAxiosError() keys on `isAxiosError: true`.
const axiosError = (
  status: number | undefined,
  data?: {
    error?: string;
    code?: string;
    details?: {
      items?: Array<{ menu_item_id?: unknown; reason?: unknown }>;
    };
  },
) =>
  Object.assign(new Error("request failed"), {
    isAxiosError: true,
    response: status === undefined ? undefined : { status, data },
  });

// What the shared axiosInstance interceptor actually delivers: a sanitized
// plain Error with the axios markers copied but NO `isAxiosError` flag.
const sanitizedError = (
  status: number | undefined,
  data?: { error?: string; code?: string },
) =>
  Object.assign(new Error("request failed"), {
    status,
    code: data?.code,
    response: status === undefined ? undefined : { status, data },
  });

describe("guestOrderErrorInfo", () => {
  it("extracts status, code and message from an axios error body", () => {
    const info = guestOrderErrorInfo(
      axiosError(409, { error: "Bill is not open", code: "bill_not_open" }),
    );
    expect(info).toEqual({
      status: 409,
      code: "bill_not_open",
      message: "Bill is not open",
      isNetwork: false,
    });
  });

  it("flags network failures (no response) as isNetwork", () => {
    expect(guestOrderErrorInfo(axiosError(undefined)).isNetwork).toBe(true);
  });

  it("treats non-axios throws as non-network", () => {
    expect(guestOrderErrorInfo(new TypeError("boom")).isNetwork).toBe(false);
  });

  it("extracts from a SANITIZED plain error (the interceptor's real output)", () => {
    const info = guestOrderErrorInfo(
      sanitizedError(409, { error: "Bill is not open", code: "bill_not_open" }),
    );
    expect(info).toEqual({
      status: 409,
      code: "bill_not_open",
      message: "Bill is not open",
      isNetwork: false,
    });
  });

  it("extracts authoritative blocked item IDs and reasons from item_not_orderable", () => {
    const info = guestOrderErrorInfo(
      axiosError(409, {
        error: "One or more items are unavailable",
        code: "item_not_orderable",
        details: {
          items: [
            { menu_item_id: "steak", reason: "inventory_out" },
            { menu_item_id: "salad", reason: "manual_disabled" },
            { menu_item_id: 42, reason: "inventory_out" },
          ],
        },
      }),
    );
    expect(info.blockedItems).toEqual([
      { menuItemId: "steak", reason: "inventory_out" },
      { menuItemId: "salad", reason: "manual_disabled" },
    ]);
  });
});

describe("presentGuestOrderError mapping table", () => {
  const ctx = (path: "create-bill" | "add-items", orderRequestSent = true) => ({
    path,
    orderRequestSent,
  });

  const cases: Array<{
    name: string;
    err: unknown;
    context: { path: "create-bill" | "add-items"; orderRequestSent: boolean };
    messageKey: string;
    offerRemoveItem?: boolean;
    maybePlaced?: boolean;
    params?: Record<string, string | number>;
  }> = [
    {
      name: "network after order request: 'order placed, but…' survives ONLY here (create-bill)",
      err: axiosError(undefined),
      context: ctx("create-bill"),
      messageKey: "menu.orderApprovalWarning",
      maybePlaced: true,
    },
    {
      name: "network after order request (add-items)",
      err: axiosError(undefined),
      context: ctx("add-items"),
      messageKey: "menu.additionalApprovalWarning",
      maybePlaced: true,
    },
    {
      name: "network BEFORE the order request was sent is NOT 'maybe placed'",
      err: axiosError(undefined),
      context: ctx("create-bill", false),
      messageKey: "menu.orderErrorGeneric",
    },
    {
      name: "403 business_unavailable (admin-locked venue)",
      err: axiosError(403, { error: "x", code: "business_unavailable" }),
      context: ctx("create-bill"),
      messageKey: "menu.orderingDisabled",
    },
    {
      name: "409 ordering_disabled",
      err: axiosError(409, { error: "x", code: "ordering_disabled" }),
      context: ctx("add-items"),
      messageKey: "menu.orderingDisabled",
    },
    {
      name: "409 business_closed top-level code maps to closed banner copy",
      err: axiosError(409, {
        error: "business is closed",
        code: "business_closed",
      }),
      context: ctx("add-items"),
      messageKey: "menu.businessClosed",
    },
    {
      name: "409 bill_not_open",
      err: axiosError(409, { error: "x", code: "bill_not_open" }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorBillClosed",
    },
    {
      name: "400 item_unavailable offers tap-to-remove",
      err: axiosError(400, {
        error: "Item 'Burger' is unavailable",
        code: "item_unavailable",
      }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorItemUnavailable",
      offerRemoveItem: true,
    },
    {
      name: "400 item_not_found reuses unavailable copy",
      err: axiosError(400, { error: "x", code: "item_not_found" }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorItemUnavailable",
      offerRemoveItem: true,
    },
    {
      name: "400 bundle_not_found reuses unavailable copy",
      err: axiosError(400, { error: "x", code: "bundle_not_found" }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorItemUnavailable",
      offerRemoveItem: true,
    },
    {
      name: "409 item_not_orderable inventory block offers authoritative removal",
      err: axiosError(409, {
        error: "x",
        code: "item_not_orderable",
        details: {
          items: [{ menu_item_id: "burger", reason: "inventory_out" }],
        },
      }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorItemUnavailable",
      offerRemoveItem: true,
    },
    {
      name: "409 item_not_orderable business closure does not remove valid cart lines",
      err: axiosError(409, {
        error: "x",
        code: "item_not_orderable",
        details: {
          items: [{ menu_item_id: "burger", reason: "business_closed" }],
        },
      }),
      context: ctx("add-items"),
      messageKey: "menu.businessClosed",
    },
    {
      name: "400 option_not_found",
      err: axiosError(400, { error: "x", code: "option_not_found" }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorOptionNotFound",
    },
    {
      name: "400 quantity_exceeded carries the cap",
      err: axiosError(400, { error: "x", code: "quantity_exceeded" }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorQuantityExceeded",
      params: { max: GUEST_ORDER_QUANTITY_CAP },
    },
    {
      name: "400 too_many_items carries the line cap",
      err: axiosError(400, { error: "x", code: "too_many_items" }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorTooManyItems",
      params: { max: GUEST_ORDER_LINE_CAP },
    },
    {
      name: "400 text_too_long carries the char cap",
      err: axiosError(400, { error: "x", code: "text_too_long" }),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorTextTooLong",
      params: { max: GUEST_ORDER_TEXT_CAP },
    },
    {
      name: "500 with no code is generic and truthful",
      err: axiosError(500, { error: "internal" }),
      context: ctx("create-bill"),
      messageKey: "menu.orderErrorGeneric",
    },
    {
      name: "non-axios throw is generic",
      err: new Error("boom"),
      context: ctx("add-items"),
      messageKey: "menu.orderErrorGeneric",
    },
  ];

  it.each(cases)(
    "$name",
    ({ err, context, messageKey, offerRemoveItem, maybePlaced, params }) => {
      const p = presentGuestOrderError(guestOrderErrorInfo(err), context);
      expect(p.messageKey).toBe(messageKey);
      expect(p.preserveCart).toBe(true); // EVERY failure mode preserves the cart
      expect(p.offerRemoveItem).toBe(Boolean(offerRemoveItem));
      expect(p.maybePlaced).toBe(Boolean(maybePlaced));
      if (params) expect(p.params).toEqual(params);
    },
  );
});

describe("isOpenBillConflict (G-2 detection)", () => {
  it("detects the create-bill 409 by status + stable message string", () => {
    expect(
      isOpenBillConflict(
        axiosError(409, { error: "Table already has an open bill" }),
      ),
    ).toBe(true);
  });

  it("accepts a forward-compat code if the backend adds one", () => {
    expect(
      isOpenBillConflict(
        axiosError(409, { error: "x", code: "bill_already_exists" }),
      ),
    ).toBe(true);
  });

  it("rejects other 409s and other statuses", () => {
    expect(
      isOpenBillConflict(axiosError(409, { error: "Bill is not open" })),
    ).toBe(false);
    expect(
      isOpenBillConflict(
        axiosError(400, { error: "Table already has an open bill" }),
      ),
    ).toBe(false);
    expect(isOpenBillConflict(new Error("boom"))).toBe(false);
  });
});
