import assert from "node:assert/strict";
import test from "node:test";

import { createRuntime, loadConfig } from "../src/config.mjs";
import { CONFIG_TOOL_NAMES } from "../src/configTools.mjs";
import { API_BASE, loginRoute, mockBackend, reply, sequence } from "./mockBackend.mjs";

const OWNER_ENV = {
  PAYVERGE_API_BASE_URL: API_BASE,
  PAYVERGE_OWNER_EMAIL: "owner@example.test",
  PAYVERGE_OWNER_PASSWORD: "correct horse",
};

function runtime(routes, env = {}) {
  const backend = mockBackend({ "POST /auth/login": loginRoute(), ...routes });
  const { tools } = createRuntime(loadConfig({ ...OWNER_ENV, ...env }), { fetchImpl: backend.fetch });
  return { backend, tools };
}

const call = (tools, name, args) => tools.callTool(name, args);

function menuFixture(version = 3) {
  return {
    id: 5,
    version,
    parsed_categories: [
      {
        id: "cat-1",
        name: "Mains",
        description: "",
        sort_order: 2,
        items: [
          {
            id: "item-1",
            name: "Burger",
            price: 12.5,
            // effective availability (out of stock) vs the stored flag
            is_available: false,
            manual_available: true,
            inventory_status: "out_of_stock",
            allergens: ["gluten"],
            dietary_tags: [],
            options: [],
          },
        ],
      },
      { id: "cat-2", name: "Drinks", description: "", sort_order: 0, items: [{ id: "item-2", name: "Lemonade", price: 3, is_available: true }] },
    ],
  };
}

// ---------------------------------------------------------------------------
// Schemas and registration

test("every config tool has a closed JSON schema; mutating tools expose dry_run", () => {
  const { tools } = runtime({});
  const definitions = tools.listTools();
  assert.deepEqual(definitions.map((tool) => tool.name), CONFIG_TOOL_NAMES, "admin tools are hidden without an admin token");
  assert.equal(CONFIG_TOOL_NAMES.length, 24);
  assert.equal(new Set(CONFIG_TOOL_NAMES).size, CONFIG_TOOL_NAMES.length);
  for (const tool of definitions) {
    assert.equal(tool.inputSchema.type, "object", tool.name);
    assert.equal(tool.inputSchema.additionalProperties, false, tool.name);
    assert.ok(tool.description.length > 20, tool.name);
    assert.equal(typeof tool.annotations.readOnlyHint, "boolean", tool.name);
    if (tool.annotations.readOnlyHint === false) {
      assert.equal(tool.inputSchema.properties.dry_run?.type, "boolean", `${tool.name} needs dry_run`);
      assert.equal(tool.inputSchema.properties.dry_run.default, true, `${tool.name} defaults to a preview`);
    } else {
      assert.equal(tool.inputSchema.properties.dry_run, undefined, `${tool.name} is read-only`);
    }
  }
  const replace = definitions.find((tool) => tool.name === "payverge_import_menu");
  assert.equal(replace.annotations.destructiveHint, true);
  assert.equal(replace.inputSchema.properties.confirm.type, "boolean");
  const hours = definitions.find((tool) => tool.name === "payverge_set_opening_hours");
  assert.equal(hours.annotations.destructiveHint, true);
  assert.equal(hours.annotations.idempotentHint, true);
  assert.equal(hours.inputSchema.properties.confirm.type, "boolean");
  assert.match(hours.description, /confirm=true/);
});

test("unknown or mistyped arguments are rejected before any request", async () => {
  const { backend, tools } = runtime({});
  const unknown = await call(tools, "payverge_get_business_profile", { business_id: 1, bogus: true });
  assert.equal(unknown.isError, true);
  assert.equal(unknown.structuredContent.error.code, "invalid_arguments");
  assert.ok(unknown.structuredContent.error.details.includes("arguments.bogus is not a known field"));

  const mistyped = await call(tools, "payverge_update_menu_item", { business_id: "1", changes: { price: -2 } });
  assert.equal(mistyped.structuredContent.error.code, "invalid_arguments");
  assert.ok(mistyped.structuredContent.error.details.some((problem) => problem.includes("business_id must be integer")));
  assert.ok(mistyped.structuredContent.error.details.some((problem) => problem.includes("changes.price must be > 0")));

  const badEnum = await call(tools, "payverge_invite_staff", { business_id: 1, email: "x@y.co", name: "X", role: "owner" });
  assert.equal(badEnum.structuredContent.error.code, "invalid_arguments");

  assert.equal(backend.requests.length, 0);
});

test("owner tools explain how to configure owner auth when it is missing", async () => {
  const backend = mockBackend({});
  const { tools } = createRuntime(loadConfig({ PAYVERGE_API_BASE_URL: API_BASE }), { fetchImpl: backend.fetch });
  const result = await call(tools, "payverge_list_businesses", {});
  assert.equal(result.isError, true);
  assert.equal(result.structuredContent.error.code, "owner_auth_not_configured");
  assert.match(result.structuredContent.error.hint, /PAYVERGE_OWNER_EMAIL/);
  assert.equal(backend.requests.length, 0);
});

test("a rejected owner password comes back as a structured, non-leaking error", async () => {
  const { tools } = runtime({}, { PAYVERGE_OWNER_PASSWORD: "wrong" });
  const result = await call(tools, "payverge_list_businesses", {});
  assert.equal(result.isError, true);
  assert.equal(result.structuredContent.error.code, "owner_invalid_credentials");
  assert.equal(result.structuredContent.error.request_id, "req-login-401");
  assert.ok(!result.content[0].text.includes("wrong"));
});

// ---------------------------------------------------------------------------
// Instance status

test("instance status works without credentials and points at the next step", async () => {
  const backend = mockBackend({
    "GET /instance": {
      product_name: "Payverge",
      public_url: "https://pos.example.test",
      features: { ai: false, email: true },
    },
    "GET /health/live": { status: "ok" },
    "GET /health/ready": { status: "ready" },
  });
  const { tools } = createRuntime(loadConfig({ PAYVERGE_API_BASE_URL: API_BASE }), { fetchImpl: backend.fetch });
  const result = await call(tools, "payverge_instance_status", {});
  assert.equal(result.isError, undefined);
  const out = result.structuredContent;
  assert.equal(out.instance.product_name, "Payverge");
  assert.equal("billing" in out, false);
  assert.equal(out.health.live.ok, true);
  assert.equal(out.mcp.owner_auth.mode, "none");
  assert.equal(out.mcp.admin_token_configured, false);
  assert.equal(out.public_url, "http://payverge.localhost");
  assert.ok(out.next_steps.some((step) => step.includes("OPENROUTER_API_KEY")));
  assert.ok(out.next_steps.some((step) => step.includes("PAYVERGE_OWNER_EMAIL")));
  assert.ok(backend.requests.every((request) => request.headers.authorization === undefined), "public probes are anonymous");
});

test("instance status degrades gracefully against a backend without /instance", async () => {
  const backend = mockBackend({ "GET /health/live": { status: "ok" }, "GET /health/ready": reply(503, { status: "not ready" }) });
  const { tools } = createRuntime(loadConfig(OWNER_ENV), { fetchImpl: backend.fetch });
  const out = (await call(tools, "payverge_instance_status", {})).structuredContent;
  assert.equal(out.instance.available, false);
  assert.equal(out.instance.status, 404);
  assert.equal(out.health.ready.ok, false);
  assert.equal(out.mcp.owner_auth.mode, "email_password");
  assert.ok(out.next_steps.some((step) => step.includes("not ready")));
  assert.ok(out.next_steps.some((step) => step.includes("/api/v1/instance")));
});

// ---------------------------------------------------------------------------
// Least privilege: allow-list and read-only mode

test("PAYVERGE_MCP_BUSINESS_IDS filters listings and blocks other businesses", async () => {
  const { backend, tools } = runtime(
    { "GET /inside/businesses": [{ id: 1, name: "Cafe Uno" }, { id: 2, name: "Bar Dos" }] },
    { PAYVERGE_MCP_BUSINESS_IDS: "1" },
  );
  const listed = (await call(tools, "payverge_list_businesses", {})).structuredContent;
  assert.equal(listed.count, 1);
  assert.equal(listed.hidden_by_allow_list, 1);
  assert.equal(listed.businesses[0].name, "Cafe Uno");

  const blocked = await call(tools, "payverge_get_business_profile", { business_id: 2 });
  assert.equal(blocked.structuredContent.error.code, "business_not_allowed");

  const create = await call(tools, "payverge_create_business", { name: "Tres" });
  assert.equal(create.structuredContent.error.code, "business_not_allowed");
  assert.ok(!backend.calls().some((entry) => entry.includes("/businesses/2")));
  assert.equal(backend.writes().length, 0);
});

test("read-only mode keeps previews and refuses every write", async () => {
  const { backend, tools } = runtime(
    { "GET /inside/businesses/1": { id: 1, name: "Old" } },
    { PAYVERGE_MCP_READ_ONLY: "true" },
  );
  const preview = await call(tools, "payverge_update_business_profile", { business_id: 1, changes: { name: "New" } });
  assert.equal(preview.structuredContent.dry_run, true);
  const applied = await call(tools, "payverge_update_business_profile", { business_id: 1, changes: { name: "New" }, dry_run: false });
  assert.equal(applied.structuredContent.error.code, "read_only_mode");
  const aliased = await call(tools, "payverge_bulk_create_tables", { business_id: 1, count: 2, dryRun: false });
  assert.equal(aliased.structuredContent.error.code, "read_only_mode");
  assert.equal(backend.writes().length, 0);
});

// ---------------------------------------------------------------------------
// Business profile and AI settings

test("profile updates preview a diff, merge the address and never print payout wallets", async () => {
  const current = {
    id: 1,
    name: "Old name",
    tax_rate: 21,
    address: { street: "1 Main St", city: "Buenos Aires", state: "", postal_code: "C1000", country: "AR" },
    settlement_address: "0xSETTLEMENTWALLET",
    tipping_address: "",
  };
  const { backend, tools } = runtime({
    "GET /inside/businesses/1": current,
    "PUT /inside/businesses/1": (request) => ({ ...current, ...request.body }),
  });
  const changes = { name: "New name", tax_rate: 21, address: { city: "Cordoba" } };

  const profile = await call(tools, "payverge_get_business_profile", { business_id: 1 });
  assert.deepEqual(profile.structuredContent.business.payout_wallets, { settlement_configured: true, tipping_configured: false });
  assert.ok(!profile.content[0].text.includes("0xSETTLEMENTWALLET"));

  const preview = (await call(tools, "payverge_update_business_profile", { business_id: 1, changes })).structuredContent;
  assert.equal(preview.dry_run, true);
  assert.deepEqual(preview.request.body.address, { street: "1 Main St", city: "Cordoba", state: "", postal_code: "C1000", country: "AR" });
  assert.deepEqual(
    preview.diff.map(({ field, changed }) => [field, changed]),
    [["name", true], ["tax_rate", false], ["address", true]],
  );
  assert.equal(backend.writes().length, 0);

  const applied = await call(tools, "payverge_update_business_profile", { business_id: 1, changes, dry_run: false });
  assert.equal(applied.structuredContent.changed, true);
  assert.equal(applied.structuredContent.business.name, "New name");
  assert.ok(!applied.content[0].text.includes("0xSETTLEMENTWALLET"));
  assert.deepEqual(backend.writes().map((request) => request.body.address.city), ["Cordoba"]);
});

test("AI settings map to the backend fields and warn when no LLM is configured", async () => {
  const business = { id: 1, ai_settings: { ai_enabled: false, ai_name: "Sage", ai_priority: "balanced", special_instructions: "" } };
  const { backend, tools } = runtime({
    "GET /inside/businesses/1": business,
    "GET /instance": { features: { ai: false } },
    "PUT /inside/businesses/1": (request) => ({
      ...business,
      ai_settings: { ...business.ai_settings, ai_enabled: request.body.ai_enabled, special_instructions: request.body.ai_special_instructions },
    }),
  });

  const read = (await call(tools, "payverge_get_ai_settings", { business_id: 1 })).structuredContent;
  assert.equal(read.ai_settings.name, "Sage");
  assert.equal(read.server.llm_provider_configured, false);

  const args = { business_id: 1, enabled: true, special_instructions: "Always mention the daily special", dry_run: false };
  const applied = (await call(tools, "payverge_update_ai_settings", args)).structuredContent;
  assert.deepEqual(backend.writes()[0].body, { ai_enabled: true, ai_special_instructions: "Always mention the daily special" });
  assert.equal(applied.applied, true);
  assert.equal(applied.ai_settings.enabled, true);
  assert.equal(applied.ai_settings.special_instructions, "Always mention the daily special");
  assert.equal(applied.warnings.length, 1);
  assert.match(applied.warnings[0], /OPENROUTER_API_KEY/);

  const empty = await call(tools, "payverge_update_ai_settings", { business_id: 1, dry_run: true });
  assert.equal(empty.structuredContent.error.code, "invalid_arguments");
});

test("AI settings report fields a manager token cannot change", async () => {
  const { tools } = runtime({
    "GET /inside/businesses/1": { id: 1, ai_settings: { ai_enabled: false } },
    "PUT /inside/businesses/1": { id: 1, ai_settings: { ai_enabled: false }, skipped_fields: ["ai_enabled"] },
  });
  const out = (await call(tools, "payverge_update_ai_settings", { business_id: 1, enabled: true, dry_run: false })).structuredContent;
  assert.equal(out.applied, false);
  assert.deepEqual(out.skipped_fields, ["ai_enabled"]);
  assert.ok(out.warnings.some((warning) => warning.includes("owner-only")));
});

// ---------------------------------------------------------------------------
// Menu

test("get_menu returns stored availability next to what is sellable now", async () => {
  const { tools } = runtime({ "GET /inside/businesses/1/menu": menuFixture() });
  const out = (await call(tools, "payverge_get_menu", { business_id: 1 })).structuredContent;
  assert.equal(out.version, 3);
  assert.equal(out.item_count, 2);
  const burger = out.categories[0].items[0];
  assert.equal(burger.is_available, true);
  assert.equal(burger.sellable_now, false);
  assert.equal(burger.inventory_status, "out_of_stock");

  const empty = runtime({ "GET /inside/businesses/1/menu": { id: 0, categories: [] } });
  const none = (await call(empty.tools, "payverge_get_menu", { business_id: 1 })).structuredContent;
  assert.equal(none.menu_exists, false);
  assert.equal(none.category_count, 0);
});

test("add_menu_item resolves the category by name and retries once on a version conflict", async () => {
  const { backend, tools } = runtime({
    // preview read, apply read, re-read after the conflict
    "GET /inside/businesses/1/menu": sequence(menuFixture(3), menuFixture(3), menuFixture(4)),
    "POST /inside/businesses/1/menu/items": sequence(
      reply(409, { error: "Menu changed", code: "MENU_VERSION_CONFLICT" }),
      { version: 5, item: { id: "item-9", name: "Fries", price: 4, is_available: true } },
    ),
  });
  const preview = (await call(tools, "payverge_add_menu_item", { business_id: 1, category_name: " mains ", item: { name: "Fries", price: 4 } })).structuredContent;
  assert.equal(preview.dry_run, true);
  assert.deepEqual(preview.category, { id: "cat-1", name: "Mains" });
  assert.equal(preview.request.body.item.is_available, true, "new items default to available");

  const out = (await call(tools, "payverge_add_menu_item", { business_id: 1, category_name: "Mains", item: { name: "Fries", price: 4 }, dry_run: false })).structuredContent;
  assert.equal(out.saved, true);
  assert.equal(out.menu_version, 5);
  assert.equal(out.item.id, "item-9");
  assert.deepEqual(backend.writes().map((request) => request.body.version), [3, 4]);
  assert.ok(backend.writes().every((request) => request.body.category_id === "cat-1"));
});

test("a second version conflict is reported, not retried forever", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/menu": menuFixture(),
    "POST /inside/businesses/1/menu/items": reply(409, { error: "Menu changed", code: "MENU_VERSION_CONFLICT" }),
  });
  const out = await call(tools, "payverge_add_menu_item", { business_id: 1, category_id: "cat-2", item: { name: "Tea", price: 2 }, dry_run: false });
  assert.equal(out.structuredContent.error.code, "conflict");
  assert.equal(out.structuredContent.error.backend_code, "MENU_VERSION_CONFLICT");
  assert.equal(backend.writes().length, 2);
});

test("unknown and ambiguous category names are explained", async () => {
  const menu = menuFixture();
  menu.parsed_categories.push({ id: "cat-3", name: "mains", items: [] });
  const { tools } = runtime({ "GET /inside/businesses/1/menu": menu });
  const missing = await call(tools, "payverge_add_menu_item", { business_id: 1, category_name: "Desserts", item: { name: "Flan", price: 5 } });
  assert.equal(missing.structuredContent.error.code, "not_found");
  assert.deepEqual(missing.structuredContent.error.details.available, ["Mains", "Drinks", "mains"]);
  const ambiguous = await call(tools, "payverge_add_menu_item", { business_id: 1, category_name: "MAINS", item: { name: "Flan", price: 5 } });
  assert.equal(ambiguous.structuredContent.error.code, "ambiguous_category");
});

test("update_menu_item writes the stored availability, not the inventory-derived one", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/menu": menuFixture(),
    "PUT /inside/businesses/1/menu/items": (request) => ({ version: 4, item: { ...request.body.item, id: "item-1" } }),
  });
  const preview = (await call(tools, "payverge_update_menu_item", { business_id: 1, item_name: "burger", changes: { price: 13 } })).structuredContent;
  assert.deepEqual(preview.diff, [{ field: "price", from: 12.5, to: 13, changed: true }]);

  await call(tools, "payverge_update_menu_item", { business_id: 1, item_name: "burger", changes: { price: 13 }, dry_run: false });
  const [write] = backend.writes();
  assert.equal(write.body.item_id, "item-1");
  assert.equal(write.body.category_id, "cat-1");
  assert.equal(write.body.version, 3);
  assert.equal(write.body.item.price, 13);
  assert.equal(write.body.item.is_available, true, "an out-of-stock item must not be saved as manually disabled");
  assert.equal(write.body.item.inventory_status, undefined);
  assert.equal(write.body.item.manual_available, undefined);

  const noop = (await call(tools, "payverge_update_menu_item", { business_id: 1, item_id: "item-1", changes: { price: 12.5 }, dry_run: false })).structuredContent;
  assert.equal(noop.changed, false);
  assert.equal(backend.writes().length, 1);
});

test("update_menu_category resends the items and warns that sort_order resets", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/menu": menuFixture(),
    "PUT /inside/businesses/1/menu/category/cat-1": (request) => ({ version: 4, category: { id: "cat-1", name: request.body.name, items: request.body.items } }),
  });
  const out = (await call(tools, "payverge_update_menu_category", { business_id: 1, category_id: "cat-1", name: "Main courses", dry_run: false })).structuredContent;
  assert.equal(out.saved, true);
  assert.ok(out.warnings.some((warning) => warning.includes("sort_order")));
  const [write] = backend.writes();
  assert.equal(write.body.items.length, 1);
  assert.equal(write.body.items[0].is_available, true);
});

// The backend answers 200 + requires_confirmation and saves nothing. Every menu
// write must surface that as the documented sanitization_review_required error,
// never as a success-shaped result an agent could read as "saved".
const SANITIZATION_REVIEW = { message: "Review dropped menu fields before saving", requires_confirmation: true, sanitization: { dropped_allergens: 1 } };

function assertSanitizationError(result, label) {
  assert.equal(result.isError, true, `${label}: isError`);
  const { error } = result.structuredContent;
  assert.equal(error.code, "sanitization_review_required", label);
  assert.deepEqual(error.details, { dropped_allergens: 1 }, label);
  assert.match(error.hint, /confirm_sanitization=true/, label);
  assert.equal(result.structuredContent.saved, undefined, `${label}: no success-shaped fields`);
}

test("sanitization reviews fail every menu write with sanitization_review_required", async () => {
  const cases = [
    ["payverge_add_menu_category", "POST /inside/businesses/1/menu/categories", { business_id: 1, name: "Desserts", dry_run: false }],
    ["payverge_add_menu_item", "POST /inside/businesses/1/menu/items", { business_id: 1, category_id: "cat-2", item: { name: "Tea", price: 2 }, dry_run: false }],
    ["payverge_update_menu_item", "PUT /inside/businesses/1/menu/items", { business_id: 1, item_name: "burger", changes: { price: 13 }, dry_run: false }],
    ["payverge_import_menu", "POST /inside/businesses/1/menu", { business_id: 1, menu: IMPORT_DOC, mode: "replace", dry_run: false, confirm: true }],
  ];
  for (const [name, route, args] of cases) {
    const { tools } = runtime({ "GET /inside/businesses/1/menu": menuFixture(), [route]: SANITIZATION_REVIEW });
    assertSanitizationError(await call(tools, name, args), name);
  }
});

test("import_menu (append) stops on a sanitization review and lists completed steps", async () => {
  const { tools } = runtime({
    "GET /inside/businesses/1/menu": menuFixture(),
    "POST /inside/businesses/1/menu/categories": { version: 4, category: { id: "cat-3", name: "Desserts", items: [{}] } },
    "POST /inside/businesses/1/menu/items": SANITIZATION_REVIEW,
  });
  const result = await call(tools, "payverge_import_menu", { business_id: 1, menu: IMPORT_DOC, dry_run: false });
  assertSanitizationError(result, "import append");
  assert.deepEqual(result.structuredContent.partial.completed, [{ created_category: "Desserts", id: "cat-3", items: 1 }]);
});

const IMPORT_DOC = {
  categories: [
    {
      name: "mains",
      items: [
        { name: "Burger", price: 12 },
        { name: "Salad", price: 8, allergens: ["mustard"] },
        { name: "salad ", price: 8 },
      ],
    },
    { name: "Desserts", description: "Sweet", items: [{ name: "Flan", price: 5 }] },
  ],
};

test("import_menu (append) plans new categories and items and skips what exists", async () => {
  const { backend, tools } = runtime({ "GET /inside/businesses/1/menu": menuFixture() });
  const { plan, dry_run: dryRun } = (await call(tools, "payverge_import_menu", { business_id: 1, menu: IMPORT_DOC })).structuredContent;
  assert.equal(dryRun, true);
  assert.deepEqual(plan.create_categories, [{ name: "Desserts", items: ["Flan"] }]);
  assert.deepEqual(plan.add_items, [{ category: "Mains", item: "Salad", price: 8 }]);
  assert.deepEqual(plan.skipped, [
    { category: "Mains", item: "Burger", reason: "already on the menu" },
    { category: "Mains", item: "salad ", reason: "duplicate in document" },
  ]);
  assert.deepEqual(plan.totals, { categories_to_create: 1, items_to_create: 2, items_skipped: 2, requests: 2 });
  assert.equal(backend.writes().length, 0);
});

test("import_menu reports completed steps when a later step fails", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/menu": menuFixture(),
    "POST /inside/businesses/1/menu/categories": { version: 4, category: { id: "cat-3", name: "Desserts", items: [{}] } },
    "POST /inside/businesses/1/menu/items": reply(500, { error: "database is down" }, { "x-request-id": "req-500" }),
  });
  const result = await call(tools, "payverge_import_menu", { business_id: 1, menu: IMPORT_DOC, dry_run: false });
  assert.equal(result.isError, true);
  const { error, partial } = result.structuredContent;
  assert.equal(error.code, "import_stopped");
  assert.equal(error.details.code, "backend_error");
  assert.equal(error.details.request_id, "req-500");
  assert.deepEqual(partial, {
    completed: [{ created_category: "Desserts", id: "cat-3", items: 1 }],
    failed_step: { category: "Mains", item: "Salad" },
    remaining_steps: 0,
  });
  const [categoryWrite, itemWrite] = backend.writes();
  assert.equal(categoryWrite.body.version, 3);
  assert.equal(itemWrite.body.version, 4, "the next step uses the version the previous write returned");
  assert.deepEqual(itemWrite.body.item.allergens, ["mustard"]);
});

test("import_menu (replace) needs confirm=true before deleting the menu", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/menu": menuFixture(),
    "POST /inside/businesses/1/menu": { version: 4 },
  });
  const preview = (await call(tools, "payverge_import_menu", { business_id: 1, menu: IMPORT_DOC, mode: "replace" })).structuredContent;
  assert.deepEqual(preview.plan.deletes, { categories: 2, items: 2 });
  assert.match(preview.next, /confirm=true/);

  const refused = await call(tools, "payverge_import_menu", { business_id: 1, menu: IMPORT_DOC, mode: "replace", dry_run: false });
  assert.equal(refused.structuredContent.error.code, "confirmation_required");
  assert.equal(backend.writes().length, 0);

  const done = (await call(tools, "payverge_import_menu", { business_id: 1, menu: IMPORT_DOC, mode: "replace", dry_run: false, confirm: true })).structuredContent;
  assert.equal(done.saved, true);
  assert.equal(done.menu_version, 4);
  const [write] = backend.writes();
  assert.equal(write.body.version, 3);
  assert.equal(write.body.categories.length, 2);
  assert.equal(write.body.categories[0].items.length, 3, "replace writes the document as given");
});

// ---------------------------------------------------------------------------
// Tables, staff, plugins

test("bulk table creation skips existing names and returns absolute QR links", async () => {
  let nextId = 10;
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/tables": { tables: [{ id: 1, name: "Table 1", table_code: "aaa111", qr_url: "/t/aaa111" }] },
    "POST /inside/businesses/1/tables": (request) => {
      nextId += 1;
      return { id: nextId, name: request.body.name, capacity: request.body.capacity, table_code: `code${nextId}` };
    },
  });
  const preview = (await call(tools, "payverge_bulk_create_tables", { business_id: 1, count: 3 })).structuredContent;
  assert.deepEqual(preview.to_create, ["Table 2", "Table 3"]);
  assert.deepEqual(preview.skipped, [{ name: "Table 1", reason: "already exists" }]);

  const out = (await call(tools, "payverge_bulk_create_tables", { business_id: 1, count: 3, capacity: 2, dry_run: false })).structuredContent;
  assert.equal(out.created_count, 2);
  assert.deepEqual(out.created.map((table) => table.guest_url), ["http://payverge.localhost/t/code11", "http://payverge.localhost/t/code12"]);
  assert.ok(backend.writes().every((request) => request.body.capacity === 2));

  const listed = (await call(tools, "payverge_list_tables", { business_id: 1 })).structuredContent;
  assert.equal(listed.tables[0].guest_url, "http://payverge.localhost/t/aaa111");

  const both = await call(tools, "payverge_bulk_create_tables", { business_id: 1, count: 2, names: ["Bar"] });
  assert.equal(both.structuredContent.error.code, "invalid_arguments");
});

test("PAYVERGE_PUBLIC_URL overrides the guest-link origin", async () => {
  const { tools } = runtime(
    { "GET /inside/businesses/1/tables": { tables: [{ id: 1, name: "Patio", table_code: "zz9" }] } },
    { PAYVERGE_PUBLIC_URL: "https://pos.example.test/" },
  );
  const listed = (await call(tools, "payverge_list_tables", { business_id: 1 })).structuredContent;
  assert.equal(listed.tables[0].guest_url, "https://pos.example.test/t/zz9");
});

test("staff invitations flag conflicts in preview and redact the link unless asked", async () => {
  const invite = { invitation_id: 9, invitation_url: "https://pos.example.test/staff/accept?token=secret-invite-token", email_sent: false };
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/staff": {
      staff: [{ id: 3, email: "ana@example.test", name: "Ana", role: "server" }],
      pending_invitations: [{ id: 4, email: "luis@example.test", role: "host" }],
    },
    "POST /inside/businesses/1/staff/invite": invite,
    "PUT /inside/businesses/1/staff/3/role": { message: "Role updated" },
  });
  const preview = (await call(tools, "payverge_invite_staff", { business_id: 1, email: "Luis@Example.test", name: "Luis", role: "host" })).structuredContent;
  assert.equal(preview.conflicts.length, 1);
  assert.equal(preview.request.body.email, "luis@example.test");
  assert.match(preview.notes.join(" "), /EMAIL_LOG_CONTENT=true/);

  const sent = await call(tools, "payverge_invite_staff", { business_id: 1, email: "eva@example.test", name: "Eva", role: "kitchen", dry_run: false });
  assert.ok(!sent.content[0].text.includes("secret-invite-token"));
  assert.match(sent.structuredContent.invitation_url, /redacted/);
  assert.ok(sent.structuredContent.hint);

  const revealed = await call(tools, "payverge_invite_staff", { business_id: 1, email: "eva@example.test", name: "Eva", role: "kitchen", reveal_invitation_url: true, dry_run: false });
  assert.equal(revealed.structuredContent.invitation_url, invite.invitation_url);

  const role = (await call(tools, "payverge_change_staff_role", { business_id: 1, staff_id: 3, new_role: "manager" })).structuredContent;
  assert.deepEqual(role.change, { from: "server", to: "manager", changed: true });
  const missing = await call(tools, "payverge_change_staff_role", { business_id: 1, staff_id: 99, new_role: "manager" });
  assert.equal(missing.structuredContent.error.code, "not_found");
  await call(tools, "payverge_change_staff_role", { business_id: 1, staff_id: 3, new_role: "manager", reason: "promotion", dry_run: false });
  assert.deepEqual(backend.writes().at(-1).body, { new_role: "manager", reason: "promotion" });
});

test("payment plugin status never returns plugin configuration", async () => {
  const { tools } = runtime({
    "GET /inside/businesses/1/plugins": {
      payments_ready: true,
      enabled_payment_count: 1,
      plugins: [
        {
          plugin_id: "stripe",
          name: "stripe",
          display_name: "Stripe",
          category: "payment",
          is_enabled: true,
          config: { secret_key: "sk_live_SHOULD_NOT_LEAK" },
          credentials: "whsec_SHOULD_NOT_LEAK",
          last_error: "x".repeat(1000),
        },
      ],
    },
    "GET /inside/plugins": { plugins: [{ id: "mercadopago", name: "mercadopago", category: "payment", config_schema: { access_token: "string" } }] },
  });
  const result = await call(tools, "payverge_payment_plugin_status", { business_id: 1, include_catalog: true });
  assert.ok(!result.content[0].text.includes("SHOULD_NOT_LEAK"));
  const [plugin] = result.structuredContent.plugins;
  assert.equal(plugin.is_enabled, true);
  assert.equal(plugin.config, undefined);
  assert.ok(plugin.last_error.length <= 300);
  assert.deepEqual(result.structuredContent.catalog.map((entry) => entry.plugin_id), ["mercadopago"]);
  assert.equal(result.structuredContent.catalog[0].config_schema, undefined);
});

// ---------------------------------------------------------------------------
// Reservations and opening hours

test("reservation settings validate party sizes against the stored values", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/reservations/settings": { enabled: false, min_party_size: 2, max_party_size: 8, internal_note: "x" },
    "PUT /inside/businesses/1/reservations/settings": (request) => ({ enabled: true, min_party_size: 2, max_party_size: 8, ...request.body }),
  });
  const read = (await call(tools, "payverge_get_reservation_settings", { business_id: 1 })).structuredContent;
  assert.equal(read.settings.internal_note, undefined, "only documented fields are projected");
  const bad = await call(tools, "payverge_update_reservation_settings", { business_id: 1, changes: { max_party_size: 1 } });
  assert.equal(bad.structuredContent.error.code, "invalid_arguments");
  const out = (await call(tools, "payverge_update_reservation_settings", { business_id: 1, changes: { enabled: true }, dry_run: false })).structuredContent;
  assert.equal(out.settings.enabled, true);
  assert.deepEqual(backend.writes()[0].body, { enabled: true });
});

const HOURS = [
  { day_of_week: 1, open_time: "12:00", close_time: "15:00", is_closed: false },
  { day_of_week: 2, open_time: "09:00", close_time: "17:00", is_closed: true },
];

test("opening hours merge only the days passed and write closed days as placeholders", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/operating-hours": HOURS,
    "PUT /inside/businesses/1/operating-hours": { message: "ok" },
  });
  const read = (await call(tools, "payverge_get_opening_hours", { business_id: 1 })).structuredContent;
  assert.deepEqual(read.schedule.monday, { closed: false, periods: [{ open: "12:00", close: "15:00" }] });
  assert.deepEqual(read.schedule.tuesday, { closed: true });
  assert.deepEqual(read.schedule.friday, { unset: true });

  const schedule = { wednesday: { periods: [{ open: "19:00", close: "23:30", kitchen_close: "23:00" }] }, sunday: { closed: true } };
  const out = (await call(tools, "payverge_set_opening_hours", { business_id: 1, schedule, dry_run: false })).structuredContent;
  assert.equal(out.changed, true);
  assert.deepEqual(out.diff.map((entry) => entry.day), ["sunday", "wednesday"]);
  assert.deepEqual(backend.writes()[0].body, [
    { day_of_week: 0, open_time: "09:00", close_time: "17:00", kitchen_close_time: null, is_closed: true },
    { day_of_week: 1, open_time: "12:00", close_time: "15:00", kitchen_close_time: null, is_closed: false },
    { day_of_week: 2, open_time: "09:00", close_time: "17:00", kitchen_close_time: null, is_closed: true },
    { day_of_week: 3, open_time: "19:00", close_time: "23:30", kitchen_close_time: "23:00", is_closed: false },
  ]);
});

test("opening hours replace mode clears the days not passed", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/operating-hours": HOURS,
    "PUT /inside/businesses/1/operating-hours": { message: "ok" },
  });
  await call(tools, "payverge_set_opening_hours", {
    business_id: 1,
    mode: "replace",
    confirm: true,
    schedule: { friday: { periods: [{ open: "20:00", close: "02:00" }] } },
    dry_run: false,
  });
  assert.deepEqual(backend.writes()[0].body, [
    { day_of_week: 5, open_time: "20:00", close_time: "02:00", kitchen_close_time: null, is_closed: false },
  ]);
});

test("opening hours replace dry-run lists the days it clears", async () => {
  const { tools } = runtime({
    "GET /inside/businesses/1/operating-hours": HOURS,
  });
  const preview = (await call(tools, "payverge_set_opening_hours", {
    business_id: 1,
    mode: "replace",
    schedule: { friday: { periods: [{ open: "20:00", close: "02:00" }] } },
  })).structuredContent;
  assert.deepEqual(preview.clears, ["monday", "tuesday"]);
  assert.match(preview.next, /confirm=true/);
});

test("opening hours replace needs confirm=true before clearing omitted days", async () => {
  const { backend, tools } = runtime({
    "GET /inside/businesses/1/operating-hours": HOURS,
    "PUT /inside/businesses/1/operating-hours": { message: "ok" },
  });
  const refused = await call(tools, "payverge_set_opening_hours", {
    business_id: 1,
    mode: "replace",
    schedule: { friday: { periods: [{ open: "20:00", close: "02:00" }] } },
    dry_run: false,
  });
  assert.equal(refused.isError, true);
  assert.equal(refused.structuredContent.error.code, "confirmation_required");
  assert.deepEqual(refused.structuredContent.error.details.clears, ["monday", "tuesday"]);
  assert.equal(backend.writes().length, 0);
});

test("opening hours reject contradictory or empty days and bad times", async () => {
  const { backend, tools } = runtime({ "GET /inside/businesses/1/operating-hours": HOURS });
  const contradictory = await call(tools, "payverge_set_opening_hours", {
    business_id: 1,
    schedule: { monday: { closed: true, periods: [{ open: "12:00", close: "15:00" }] }, friday: {} },
  });
  assert.equal(contradictory.structuredContent.error.code, "invalid_arguments");
  assert.deepEqual(contradictory.structuredContent.error.details, [
    "monday: a closed day cannot have periods",
    "friday: pass closed=true or at least one period",
  ]);
  const badTime = await call(tools, "payverge_set_opening_hours", { business_id: 1, schedule: { monday: { periods: [{ open: "25:00", close: "15:00" }] } } });
  assert.equal(badTime.structuredContent.error.code, "invalid_arguments");
  const noChange = (await call(tools, "payverge_set_opening_hours", { business_id: 1, schedule: { tuesday: { closed: true } }, dry_run: false })).structuredContent;
  assert.equal(noChange.changed, false);
  assert.equal(backend.writes().length, 0);
});

// ---------------------------------------------------------------------------
// Error mapping

test("backend errors map to stable codes with the request id", async () => {
  const { tools } = runtime({
    "GET /inside/businesses/1/staff": reply(403, { error: "Insufficient permissions" }, { "x-request-id": "req-403" }),
    "GET /inside/businesses/2/staff": reply(403, { error: "This business is suspended", code: "business_suspended" }),
    "GET /inside/businesses/3/staff": reply(503, { error: "AI is not configured", code: "ai_not_configured" }),
    "GET /inside/businesses/4/staff": reply(503, {
      error: "This operation is temporarily unavailable.",
      code: "RUNTIME_CONTROL_DISABLED",
      control: "maintenance_mode",
    }),
  });
  const forbidden = (await call(tools, "payverge_list_staff", { business_id: 1 })).structuredContent.error;
  assert.equal(forbidden.code, "forbidden");
  assert.equal(forbidden.status, 403);
  assert.equal(forbidden.request_id, "req-403");
  assert.equal(forbidden.request, "GET /inside/businesses/1/staff");
  assert.equal(forbidden.message, "Insufficient permissions");

  const suspended = (await call(tools, "payverge_list_staff", { business_id: 2 })).structuredContent.error;
  assert.equal(suspended.code, "business_suspended");
  assert.match(suspended.hint, /suspended or closed/);

  const ai = (await call(tools, "payverge_list_staff", { business_id: 3 })).structuredContent.error;
  assert.equal(ai.code, "ai_not_configured");

  const paused = (await call(tools, "payverge_list_staff", { business_id: 4 })).structuredContent.error;
  assert.equal(paused.code, "runtime_control_disabled");
  assert.equal(paused.control, "maintenance_mode");
  assert.equal(paused.backend_code, "RUNTIME_CONTROL_DISABLED");
  assert.match(paused.hint, /runtime-launch-controls\.md/);

  const unreachable = createRuntime(loadConfig(OWNER_ENV), {
    fetchImpl: async (url) => {
      if (String(url).endsWith("/auth/login")) throw new Error("ECONNREFUSED");
      throw new Error("unreachable");
    },
  });
  const down = (await call(unreachable.tools, "payverge_list_businesses", {})).structuredContent.error;
  assert.equal(down.code, "owner_login_unreachable");
});

test("create_business sends a stable Idempotency-Key", async () => {
  const { backend, tools } = runtime({
    "POST /inside/businesses": (request) => ({ id: 7, name: request.body.name, settlement_address: "0xWALLET" }),
  });
  const args = { name: "Cafe Siete", business_type: "cafe", address: { country: "AR" } };
  const preview = (await call(tools, "payverge_create_business", args)).structuredContent;
  const key = preview.request.headers["Idempotency-Key"];
  assert.match(key, /^mcp-create-business-[0-9a-f]{40}$/);
  const created = await call(tools, "payverge_create_business", { ...args, dry_run: false });
  assert.equal(created.structuredContent.business.id, 7);
  assert.ok(!created.content[0].text.includes("0xWALLET"));
  assert.equal(backend.writes()[0].headers["idempotency-key"], key);
});
