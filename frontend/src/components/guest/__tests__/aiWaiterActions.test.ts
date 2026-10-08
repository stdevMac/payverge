import type { Bundle, MenuCategory } from "@/api/business";
import { parseAiWaiterChatResponse } from "@/api/aiWaiter";
import {
  resolveCartAction,
  resolveLegacyCartAction,
} from "@/components/guest/aiWaiterActions";
import type { AssistantResponse } from "@/types/assistant";

const ITEM_ID = "018f99f8-4f0d-74ac-bbee-91b2f7b57e04";

const menu = (
  overrides: Partial<MenuCategory["items"][number]> = {},
): MenuCategory[] => [
  {
    id: "category-main",
    name: "Principales",
    description: "",
    items: [
      {
        id: ITEM_ID,
        name: "Bowl de la cosecha",
        description: "Verduras de estación",
        price: 14.5 as MenuCategory["items"][number]["price"],
        currency: "USD",
        is_available: true,
        ...overrides,
      },
    ],
  },
];

const bundles = (overrides: Partial<Bundle> = {}): Bundle[] => [
  {
    id: 42,
    name: "Menú para dos",
    description: "",
    price: 29 as Bundle["price"],
    currency: "USD",
    items: [],
    is_active: true,
    ...overrides,
  },
];

const v2Response = (
  target: {
    kind: "menu_item" | "bundle";
    id: string;
    quantity?: number;
    notes?: string;
  } = { kind: "menu_item", id: ITEM_ID, quantity: 2, notes: "sin cebolla" },
): AssistantResponse => ({
  version: 2,
  response_id: "waiter-response-1",
  answer: { format: "plain_text", content: "Te ayudo a continuar en el menú." },
  sections: [],
  steps: [],
  actions: [
    {
      id: "add-cart-1",
      type: "add_cart_item",
      label: "Agregar Harvest Bowl",
      target: { ...target, href: "" },
      state: "ready",
      confirmation: "none",
      disabled_reason: null,
      expires_at: null,
    },
  ],
  sources: [],
  entities: [],
  follow_ups: [],
  workflow: null,
  notices: [],
  status: "complete",
});

describe("AI Waiter V2 response parsing", () => {
  it("parses a present valid response_v2 as the authoritative response", () => {
    const parsed = parseAiWaiterChatResponse({
      id: 17,
      role: "model",
      parts: [{ text: "legacy compatibility copy" }],
      response_v2: v2Response(),
    });

    expect(parsed.response_v2.response_id).toBe("waiter-response-1");
    expect(parsed.response_v2.answer.content).toBe(
      "Te ayudo a continuar en el menú.",
    );
  });

  it("fails closed when response_v2 is present but invalid", () => {
    expect(() =>
      parseAiWaiterChatResponse({
        role: "model",
        parts: [{ text: "valid legacy copy must not be used" }],
        response_v2: { ...v2Response(), version: 1 },
      }),
    ).toThrow();
  });

  it.each([null, undefined])(
    "does not downgrade an own response_v2 value of %p",
    (responseV2) => {
      expect(() =>
        parseAiWaiterChatResponse({
          role: "model",
          parts: [{ text: "legacy copy" }],
          response_v2: responseV2,
        }),
      ).toThrow();
    },
  );

  it("rejects unknown fields in a present response_v2", () => {
    expect(() =>
      parseAiWaiterChatResponse({
        role: "model",
        parts: [{ text: "legacy copy" }],
        response_v2: { ...v2Response(), injected: true },
      }),
    ).toThrow();
  });

  it("rejects an unsafe outer assistant message ID", () => {
    expect(() =>
      parseAiWaiterChatResponse({
        id: Number.MAX_SAFE_INTEGER + 1,
        role: "model",
        parts: [{ text: "compatibility copy" }],
        response_v2: v2Response(),
      }),
    ).toThrow("invalid_ai_waiter_response_id");
  });

  it("adapts legacy text only when response_v2 is absent", () => {
    const parsed = parseAiWaiterChatResponse({
      id: 18,
      role: "model",
      parts: [{ text: "Legacy answer" }],
    });

    expect(parsed.response_v2.answer.content).toBe("Legacy answer");
    expect(parsed.response_v2.actions).toEqual([]);
  });
});

describe("resolveCartAction", () => {
  it("resolves a menu item only by its exact UUID and returns live canonical metadata", () => {
    const result = resolveCartAction(v2Response(), menu(), bundles());

    expect(result).toEqual({
      ok: true,
      source: "v2_stable_id",
      request: {
        itemType: "menu_item",
        menuItemId: ITEM_ID,
        itemName: "Bowl de la cosecha",
        price: 14.5,
        currency: "USD",
        quantity: 2,
        notes: "sin cebolla",
      },
    });
  });

  it("resolves a string V2 bundle target to the live numeric bundle ID", () => {
    const result = resolveCartAction(
      v2Response({ kind: "bundle", id: "42", quantity: 3 }),
      menu(),
      bundles(),
    );

    expect(result).toEqual({
      ok: true,
      source: "v2_stable_id",
      request: {
        itemType: "bundle",
        bundleId: 42,
        itemName: "Menú para dos",
        price: 29,
        currency: "USD",
        quantity: 3,
      },
    });
  });

  it.each([
    ["missing ID", "", "invalid_target"],
    ["unknown ID", "unknown-item", "menu_item_not_found"],
  ])(
    "fails honestly for a %s without falling back to the translated name",
    (_case, id, error) => {
      const result = resolveCartAction(
        v2Response({ kind: "menu_item", id, quantity: 1 }),
        menu(),
        bundles(),
      );

      expect(result).toEqual({ ok: false, error });
    },
  );

  it("rejects an unavailable exact item", () => {
    const result = resolveCartAction(
      v2Response(),
      menu({ is_available: false }),
      bundles(),
    );

    expect(result).toEqual({ ok: false, error: "menu_item_unavailable" });
  });

  it("rejects an inactive exact bundle", () => {
    const result = resolveCartAction(
      v2Response({ kind: "bundle", id: "42", quantity: 1 }),
      menu(),
      bundles({ is_active: false }),
    );

    expect(result).toEqual({ ok: false, error: "bundle_unavailable" });
  });

  it.each([
    ["disabled", { state: "disabled", confirmation: "none" }],
    ["confirmation required", { state: "ready", confirmation: "required" }],
  ])("rejects a %s cart action", (_case, actionState) => {
    const response = v2Response();
    Object.assign(response.actions[0], actionState);

    expect(resolveCartAction(response, menu(), bundles())).toEqual({
      ok: false,
      error: "cart_action_not_ready",
    });
  });

  it("rejects a response with no cart action", () => {
    const response = v2Response();
    response.actions = [];

    expect(resolveCartAction(response, menu(), bundles())).toEqual({
      ok: false,
      error: "cart_action_missing",
    });
  });

  it("does not treat another action type as a cart request", () => {
    const response = v2Response();
    response.actions[0] = {
      ...response.actions[0],
      type: "navigate",
      target: { kind: "dashboard_area", id: "menu", href: "/menu" },
    };

    expect(resolveCartAction(response, menu(), bundles())).toEqual({
      ok: false,
      error: "cart_action_missing",
    });
  });

  it("rejects multiple eligible cart actions", () => {
    const response = v2Response();
    response.actions.push({ ...response.actions[0], id: "add-cart-2" });

    expect(resolveCartAction(response, menu(), bundles())).toEqual({
      ok: false,
      error: "cart_action_ambiguous",
    });
  });

  it("ignores an ineligible cart action when exactly one eligible action remains", () => {
    const response = v2Response();
    response.actions.push({
      ...response.actions[0],
      id: "add-cart-disabled",
      state: "disabled",
      disabled_reason: "No disponible",
    });

    expect(resolveCartAction(response, menu(), bundles())).toMatchObject({
      ok: true,
      source: "v2_stable_id",
    });
  });

  it("does not execute an action carrying a disabled reason", () => {
    const response = v2Response();
    response.actions[0].disabled_reason = "Not actually available";

    expect(resolveCartAction(response, menu(), bundles())).toEqual({
      ok: false,
      error: "cart_action_not_ready",
    });
  });

  it.each([
    [
      "a non-empty href",
      { target: { ...v2Response().actions[0].target, href: "/menu" } },
    ],
    ["an expiry", { expires_at: "2026-08-08T12:00:00Z" }],
  ])("rejects a ready cart action with %s", (_case, mutation) => {
    const response = v2Response();
    Object.assign(response.actions[0], mutation);

    expect(resolveCartAction(response, menu(), bundles())).toEqual({
      ok: false,
      error: "cart_action_not_ready",
    });
  });

  it.each([" 42", "42 ", "042", "4.2", "4e1", "+42"])(
    "does not coerce a noncanonical bundle target ID %p",
    (id) => {
      expect(
        resolveCartAction(
          v2Response({ kind: "bundle", id, quantity: 1 }),
          menu(),
          bundles(),
        ),
      ).toEqual({ ok: false, error: "invalid_target" });
    },
  );

  it("fails closed on duplicate live stable IDs regardless of order", () => {
    const duplicateMenu = [
      ...menu(),
      {
        ...menu()[0],
        id: "category-other",
        items: [{ ...menu()[0].items[0], name: "Duplicate" }],
      },
    ];
    const duplicateBundles = [
      ...bundles(),
      { ...bundles()[0], name: "Duplicate bundle" },
    ];

    expect(resolveCartAction(v2Response(), duplicateMenu, bundles())).toEqual({
      ok: false,
      error: "menu_item_not_found",
    });
    expect(
      resolveCartAction(
        v2Response({ kind: "bundle", id: "42", quantity: 1 }),
        menu(),
        duplicateBundles.reverse(),
      ),
    ).toEqual({ ok: false, error: "bundle_not_found" });
  });

  it("rejects padded/control-bearing item identities even if live data repeats them", () => {
    for (const id of [" padded-id ", "padded\u202eid"]) {
      expect(
        resolveCartAction(
          v2Response({ kind: "menu_item", id, quantity: 1 }),
          menu({ id }),
          bundles(),
        ),
      ).toEqual({ ok: false, error: "invalid_target" });
    }
  });

  it("rejects non-positive and unsafe live numeric bundle identities", () => {
    for (const id of [0, -1, Number.MAX_SAFE_INTEGER + 1]) {
      expect(
        resolveCartAction(
          v2Response({ kind: "bundle", id: String(id), quantity: 1 }),
          menu(),
          bundles({ id }),
        ),
      ).toEqual({ ok: false, error: "invalid_target" });
    }
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY, -0.01])(
    "rejects a noncanonical live menu price %p",
    (price) => {
      expect(
        resolveCartAction(
          v2Response(),
          menu({ price: price as MenuCategory["items"][number]["price"] }),
          bundles(),
        ),
      ).toEqual({ ok: false, error: "invalid_target" });
    },
  );

  it.each([null, "", "   ", "x".repeat(301), "spoof\u202ename"])(
    "rejects malformed live item name metadata %p",
    (name) => {
      expect(
        resolveCartAction(
          v2Response(),
          menu({ name: name as never }),
          bundles(),
        ),
      ).toEqual({ ok: false, error: "invalid_target" });
    },
  );

  it.each([7, "", " ", "X".repeat(101), "US\u202eD"])(
    "rejects malformed live bundle currency metadata %p",
    (currency) => {
      expect(
        resolveCartAction(
          v2Response({ kind: "bundle", id: "42", quantity: 1 }),
          menu(),
          bundles({ currency: currency as never }),
        ),
      ).toEqual({ ok: false, error: "invalid_target" });
    },
  );

  it("returns trimmed canonical live presentation metadata", () => {
    expect(
      resolveCartAction(
        v2Response(),
        menu({ name: "  Bowl de la cosecha  ", currency: " USD " }),
        bundles(),
      ),
    ).toMatchObject({
      ok: true,
      request: { itemName: "Bowl de la cosecha", currency: "USD" },
    });
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY, -0.01])(
    "rejects a noncanonical live bundle price %p",
    (price) => {
      expect(
        resolveCartAction(
          v2Response({ kind: "bundle", id: "42", quantity: 1 }),
          menu(),
          bundles({ price: price as Bundle["price"] }),
        ),
      ).toEqual({ ok: false, error: "invalid_target" });
    },
  );

  it("defensively rejects malformed V2 quantity, notes, and runtime ID types", () => {
    for (const target of [
      { kind: "menu_item", id: ITEM_ID, quantity: 0 },
      { kind: "menu_item", id: ITEM_ID, quantity: 1.5 },
      { kind: "menu_item", id: ITEM_ID, quantity: 1, notes: "x".repeat(201) },
      { kind: "menu_item", id: ITEM_ID, quantity: 1, notes: "spoof\u202e" },
      { kind: "menu_item", id: 123, quantity: 1 },
    ]) {
      const response = v2Response();
      response.actions[0].target = { ...target, href: "" } as never;
      expect(resolveCartAction(response, menu(), bundles())).toEqual({
        ok: false,
        error: "invalid_target",
      });
    }
  });

  it("does not cross the target kind domain", () => {
    expect(
      resolveCartAction(
        v2Response({ kind: "bundle", id: ITEM_ID, quantity: 1 }),
        menu(),
        bundles(),
      ),
    ).toEqual({ ok: false, error: "invalid_target" });
  });

  it("does not use a matching V2 label as an identity fallback", () => {
    const response = v2Response({
      kind: "menu_item",
      id: "not-the-live-id",
      quantity: 1,
    });
    response.actions[0].label = "Agregar Bowl de la cosecha";

    expect(resolveCartAction(response, menu(), bundles())).toEqual({
      ok: false,
      error: "menu_item_not_found",
    });
  });

  it("keeps exact legacy name resolution in an explicit compatibility-only path", () => {
    const legacy = resolveLegacyCartAction(
      {
        item_name: "  bowl DE LA cosecha ",
        item_type: "menu_item",
        quantity: 1,
      },
      menu(),
      bundles(),
    );

    expect(legacy).toEqual({
      ok: true,
      source: "v1_name_compatibility",
      request: {
        itemType: "menu_item",
        menuItemId: ITEM_ID,
        itemName: "Bowl de la cosecha",
        price: 14.5,
        currency: "USD",
        quantity: 1,
      },
    });
  });

  it("never fuzzy-matches a partial legacy name", () => {
    expect(
      resolveLegacyCartAction(
        { item_name: "cosecha", quantity: 1 },
        menu(),
        bundles(),
      ),
    ).toEqual({ ok: false, error: "legacy_item_not_found" });
  });

  it("fails closed on duplicate exact legacy names in both input orders", () => {
    const duplicate = [
      ...menu(),
      {
        ...menu()[0],
        id: "category-two",
        items: [
          {
            ...menu()[0].items[0],
            id: "018f99f8-4f0d-74ac-bbee-91b2f7b57e05",
          },
        ],
      },
    ];
    const call = { item_name: "Bowl de la cosecha", quantity: 1 };

    expect(resolveLegacyCartAction(call, duplicate, bundles())).toEqual({
      ok: false,
      error: "legacy_item_ambiguous",
    });
    expect(
      resolveLegacyCartAction(call, duplicate.slice().reverse(), bundles()),
    ).toEqual({ ok: false, error: "legacy_item_ambiguous" });
  });

  it("fails closed on duplicate exact legacy bundle names in both input orders", () => {
    const duplicate = [...bundles(), { ...bundles()[0], id: 43 }];
    const call = {
      item_name: "Menú para dos",
      item_type: "bundle",
      quantity: 1,
    };

    expect(resolveLegacyCartAction(call, menu(), duplicate)).toEqual({
      ok: false,
      error: "legacy_item_ambiguous",
    });
    expect(
      resolveLegacyCartAction(call, menu(), duplicate.slice().reverse()),
    ).toEqual({ ok: false, error: "legacy_item_ambiguous" });
  });

  it("honors the legacy type hint and rejects a cross-type match", () => {
    expect(
      resolveLegacyCartAction(
        {
          item_name: "Bowl de la cosecha",
          item_type: "bundle",
          quantity: 1,
        },
        menu(),
        bundles(),
      ),
    ).toEqual({ ok: false, error: "legacy_item_not_found" });
  });

  it("normalizes a safe legacy bundle type hint but returns its stable numeric ID", () => {
    expect(
      resolveLegacyCartAction(
        {
          item_name: "Menú para dos",
          item_type: " Bundle ",
          quantity: 2,
        },
        menu(),
        bundles(),
      ),
    ).toMatchObject({
      ok: true,
      source: "v1_name_compatibility",
      request: { itemType: "bundle", bundleId: 42, quantity: 2 },
    });
  });

  it("rejects unavailable legacy item and bundle matches", () => {
    expect(
      resolveLegacyCartAction(
        { item_name: "Bowl de la cosecha", quantity: 1 },
        menu({ is_available: false }),
        bundles(),
      ),
    ).toEqual({ ok: false, error: "menu_item_unavailable" });
    expect(
      resolveLegacyCartAction(
        { item_name: "Menú para dos", item_type: "bundle", quantity: 1 },
        menu(),
        bundles({ is_active: false }),
      ),
    ).toEqual({ ok: false, error: "bundle_unavailable" });
  });

  it.each([
    null,
    [],
    { item_name: 7, quantity: 1 },
    { item_name: "Bowl de la cosecha", item_type: 7, quantity: 1 },
    { item_name: "Bowl de la cosecha", quantity: 1, notes: 7 },
    { item_name: "Bowl de la cosecha", quantity: null },
    { item_name: "Bowl de la cosecha", quantity: 1, notes: null },
  ])(
    "returns an honest error for malformed runtime legacy input: %p",
    (call) => {
      expect(() =>
        resolveLegacyCartAction(call as never, menu(), bundles()),
      ).not.toThrow();
      expect(resolveLegacyCartAction(call as never, menu(), bundles())).toEqual(
        {
          ok: false,
          error: "legacy_args_invalid",
        },
      );
    },
  );

  it.each([
    { item_name: "Bowl de la cosecha", quantity: 0 },
    { item_name: "Bowl de la cosecha", quantity: 21 },
    { item_name: "Bowl de la cosecha", quantity: 1.5 },
    { item_name: "Bowl de la cosecha", quantity: 1, notes: "x".repeat(201) },
    { item_name: "Bowl de la cosecha", quantity: 1, notes: "bad\u0000note" },
    { item_name: "Bowl de la cosecha", quantity: 1, notes: "bad\u202enote" },
  ])("rejects malformed legacy quantity or notes: %p", (call) => {
    expect(resolveLegacyCartAction(call, menu(), bundles())).toEqual({
      ok: false,
      error: "legacy_args_invalid",
    });
  });
});
