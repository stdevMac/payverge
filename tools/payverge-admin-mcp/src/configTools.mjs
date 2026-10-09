// Configuration tools: set up a restaurant through the Payverge HTTP API.
//
// Every tool here wraps an endpoint that already exists in the backend. Most
// routes live under /api/v1/inside/businesses/:id and need an owner session
// (see ownerSession.mjs). instance_status uses only public routes.
//
// Conventions shared by every tool:
//   - inputs are validated against the declared JSON schema before any
//     request is made (unknown fields are rejected);
//   - mutating tools default to dry_run=true and return a preview of the exact
//     request; set dry_run=false to apply. dryRun is accepted as an alias;
//   - destructive operations (menu replace, opening-hours replace) also need confirm=true;
//   - failures come back as an MCP tool error (isError=true) whose
//     structuredContent.error has {code, message, hint, status, request_id};
//   - secrets never appear in output: plugin configs are not read, payout
//     wallet addresses are reduced to booleans, and invitation links are
//     redacted unless explicitly requested.

import { createHash } from "node:crypto";

import { MCP_SERVER_VERSION } from "./mcpProtocol.mjs";

const ALLERGENS = [
  "celery", "crustaceans", "dairy", "eggs", "fish", "gluten", "lupin",
  "mollusc", "mustard", "peanut", "sesame", "so2", "soya", "treenuts",
];
const DIETARY_TAGS = [
  "vegan", "vegetarian", "gluten-free", "dairy-free", "nut-free", "mild", "low-sodium",
];
const STAFF_ROLES = ["manager", "server", "host", "kitchen"];
const BUSINESS_TYPES = [
  "restaurant", "cafe", "bar", "quick_service", "food_truck", "bakery", "fine_dining", "other",
];
const AI_PRIORITIES = ["balanced", "upselling", "service"];
// Index = backend day_of_week (0 = Sunday).
const WEEKDAYS = ["sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"];
const HHMM = "^([01][0-9]|2[0-3]):[0-5][0-9]$";
const CURRENCY = "^[A-Z]{3}$";
const HEX_COLOR = "^#[0-9a-fA-F]{6}$";

const MAX_IMPORT_ITEMS = 500;
const MAX_IMPORT_CATEGORIES = 100;
const MAX_BULK_TABLES = 200;
const MAX_ERROR_TEXT = 300;

export class ToolError extends Error {
  constructor(code, message, { details, hint, partial } = {}) {
    super(message);
    this.name = "ToolError";
    this.code = code;
    this.details = details;
    this.hint = hint;
    this.partial = partial;
  }
}

// ---------------------------------------------------------------------------
// Schema fragments

const businessIdSchema = {
  type: "integer",
  minimum: 1,
  description: "Numeric business id, from payverge_list_businesses.",
};

const dryRunProperties = {
  dry_run: {
    type: "boolean",
    default: true,
    description: "Default true: return a preview of the exact request without writing. Set false to apply.",
  },
  dryRun: { type: "boolean", description: "Alias of dry_run." },
};

const confirmSanitizationSchema = {
  type: "boolean",
  default: false,
  description: "Save even when the backend reports fields it would drop (unknown allergen/dietary values, out-of-range prices).",
};

const addressSchema = {
  type: "object",
  additionalProperties: false,
  minProperties: 1,
  properties: {
    street: { type: "string", maxLength: 255 },
    city: { type: "string", maxLength: 255 },
    state: { type: "string", maxLength: 255 },
    postal_code: { type: "string", maxLength: 32 },
    country: { type: "string", maxLength: 64, description: "ISO 3166-1 alpha-2 code, e.g. AR, ES, US." },
  },
};

const menuOptionSchema = {
  type: "object",
  additionalProperties: false,
  required: ["name"],
  properties: {
    name: { type: "string", minLength: 1, maxLength: 200 },
    price_change: { type: "number", minimum: -10000, maximum: 10000, description: "Surcharge in major units (1.5 = 1.50)." },
    is_required: { type: "boolean" },
  },
};

const menuItemProperties = {
  name: { type: "string", minLength: 1, maxLength: 200 },
  description: { type: "string", maxLength: 2000 },
  price: {
    type: "number",
    exclusiveMinimum: 0,
    exclusiveMaximum: 10000,
    description: "Price in major units of the business currency (12.5 = 12.50). Must be > 0 and < 10000.",
  },
  cogs: { type: "number", minimum: 0, description: "Cost of goods, major units." },
  image: { type: "string", maxLength: 2048, description: "Image URL." },
  allergens: {
    type: "array",
    maxItems: ALLERGENS.length,
    items: { type: "string", enum: ALLERGENS },
    description: `EU-14 allergen ids: ${ALLERGENS.join(", ")}.`,
  },
  dietary_tags: {
    type: "array",
    maxItems: DIETARY_TAGS.length,
    items: { type: "string", enum: DIETARY_TAGS },
    description: `Dietary tag ids: ${DIETARY_TAGS.join(", ")}.`,
  },
  options: { type: "array", maxItems: 50, items: menuOptionSchema },
  is_available: { type: "boolean", description: "Default true." },
};

const menuItemSchema = {
  type: "object",
  additionalProperties: false,
  required: ["name", "price"],
  properties: menuItemProperties,
};

const menuItemPatchSchema = {
  type: "object",
  additionalProperties: false,
  minProperties: 1,
  properties: menuItemProperties,
};

const categoryRefProperties = {
  category_id: { type: "string", minLength: 1, description: "Category id from payverge_get_menu." },
  category_name: { type: "string", minLength: 1, description: "Category name (case-insensitive), used when category_id is not given." },
};

const periodSchema = {
  type: "object",
  additionalProperties: false,
  required: ["open", "close"],
  properties: {
    open: { type: "string", pattern: HHMM, description: "HH:MM, 24h." },
    close: { type: "string", pattern: HHMM, description: "HH:MM, 24h. Earlier than open means the period runs past midnight." },
    kitchen_close: { type: "string", pattern: HHMM, description: "Optional last-order time for the kitchen." },
  },
};

const daySchema = {
  type: "object",
  additionalProperties: false,
  properties: {
    closed: { type: "boolean", description: "true marks the day closed (no periods allowed)." },
    periods: { type: "array", maxItems: 2, items: periodSchema, description: "One or two open periods (split shifts)." },
  },
};

const BUSINESS_PROFILE_FIELDS = [
  "id", "business_id", "name", "logo", "address", "description", "custom_url", "phone", "email",
  "website", "business_page_enabled", "show_reviews", "google_reviews_enabled", "tax_rate",
  "service_fee_rate", "tax_inclusive", "service_inclusive", "default_currency", "display_currency",
  "default_language", "source_language", "timezone", "service_day_start_minute", "business_type",
  "counter_enabled", "kitchen_enabled", "orders_enabled", "welcome_message", "about_story",
  "show_welcome_message", "show_about_story", "show_gallery", "show_operating_hours",
  "show_special_features", "default_qr_logo_url", "default_qr_foreground_color",
  "default_qr_background_color", "default_qr_logo_size", "default_qr_show_business_name",
  "default_qr_show_table_name", "default_qr_text_font", "is_active", "created_at", "updated_at",
];

const profileChangeProperties = {
  name: { type: "string", minLength: 1, maxLength: 255 },
  logo: { type: "string", minLength: 1, maxLength: 2048, description: "Logo URL." },
  address: { ...addressSchema, description: "Merged into the current address; fields you omit keep their value." },
  description: { type: "string", maxLength: 5000 },
  custom_url: { type: "string", pattern: "^[a-zA-Z0-9-]{2,63}$", description: "Public slug for the business page." },
  phone: { type: "string", maxLength: 64 },
  website: { type: "string", maxLength: 2048 },
  tax_rate: { type: "number", minimum: 0, maximum: 100, description: "Percent, e.g. 21 for 21%." },
  service_fee_rate: { type: "number", minimum: 0, maximum: 100, description: "Percent." },
  tax_inclusive: { type: "boolean", description: "Menu prices already include tax." },
  service_inclusive: { type: "boolean", description: "Menu prices already include the service fee." },
  default_currency: { type: "string", pattern: CURRENCY, description: "ISO 4217, e.g. ARS, EUR, USD." },
  display_currency: { type: "string", pattern: CURRENCY },
  timezone: { type: "string", minLength: 1, maxLength: 64, description: "IANA zone, e.g. America/Argentina/Buenos_Aires." },
  service_day_start_minute: { type: "integer", minimum: 0, maximum: 1439, description: "Minutes after local midnight when the service day starts (e.g. 300 = 05:00)." },
  business_type: { type: "string", enum: BUSINESS_TYPES },
  business_page_enabled: { type: "boolean" },
  show_reviews: { type: "boolean" },
  google_reviews_enabled: { type: "boolean" },
  welcome_message: { type: "string", maxLength: 2000 },
  about_story: { type: "string", maxLength: 10000 },
  show_welcome_message: { type: "boolean" },
  show_about_story: { type: "boolean" },
  show_gallery: { type: "boolean" },
  show_operating_hours: { type: "boolean" },
  show_special_features: { type: "boolean" },
  default_qr_logo_url: { type: "string", maxLength: 2048 },
  default_qr_foreground_color: { type: "string", pattern: HEX_COLOR },
  default_qr_background_color: { type: "string", pattern: HEX_COLOR },
  default_qr_logo_size: { type: "integer", minimum: 0, maximum: 100 },
  default_qr_show_business_name: { type: "boolean" },
  default_qr_show_table_name: { type: "boolean" },
  default_qr_text_font: { type: "string", maxLength: 64 },
};

const reservationSettingsProperties = {
  enabled: { type: "boolean" },
  approval_mode: { type: "string", enum: ["auto", "manual"] },
  min_party_size: { type: "integer", minimum: 1, maximum: 500 },
  max_party_size: { type: "integer", minimum: 1, maximum: 500 },
  max_advance_days: { type: "integer", minimum: 1, maximum: 730 },
  min_advance_minutes: { type: "integer", minimum: 0, maximum: 100000 },
  default_duration: { type: "integer", minimum: 15, maximum: 1440, description: "Minutes per reservation." },
  slot_interval_minutes: { type: "integer", minimum: 5, maximum: 1440 },
  service_buffer_minutes: { type: "integer", minimum: 0, maximum: 1440 },
  max_covers_per_slot: { type: "integer", minimum: 0, maximum: 10000, description: "0 = no cap." },
  auto_assign_tables: { type: "boolean" },
  allow_waitlist: { type: "boolean" },
  hold_duration_minutes: { type: "integer", minimum: 0, maximum: 1440 },
  allow_cancellation: { type: "boolean" },
  cancellation_deadline: { type: "integer", minimum: 0, maximum: 8760, description: "Hours before the reservation." },
  no_show_grace_minutes: { type: "integer", minimum: 0, maximum: 1440 },
  send_confirmation_email: { type: "boolean" },
  send_reminder_email: { type: "boolean" },
  reminder_hours_before: { type: "integer", minimum: 0, maximum: 720 },
};

function objectSchema(properties, required = [], extra = {}) {
  return { type: "object", additionalProperties: false, properties, required, ...extra };
}

const readAnnotations = { readOnlyHint: true, openWorldHint: false };
function writeAnnotations({ destructive = false, idempotent = false } = {}) {
  return { readOnlyHint: false, destructiveHint: destructive, idempotentHint: idempotent, openWorldHint: false };
}

// ---------------------------------------------------------------------------
// Tool table

const TOOLS = [
  {
    name: "payverge_instance_status",
    title: "Instance status",
    description:
      "Check the Payverge server this MCP points at: public instance identity and feature flags (GET /api/v1/instance), liveness/readiness, and how this MCP server is authenticated. Start here; needs no credentials.",
    inputSchema: objectSchema({}),
    annotations: readAnnotations,
    auth: "public",
    handler: instanceStatus,
  },
  {
    name: "payverge_list_businesses",
    title: "List businesses",
    description: "List the businesses the owner account can manage (id, name, type, currency, timezone). Filtered by PAYVERGE_MCP_BUSINESS_IDS when set.",
    inputSchema: objectSchema({}),
    annotations: readAnnotations,
    auth: "owner",
    handler: listBusinesses,
  },
  {
    name: "payverge_get_business_profile",
    title: "Get business profile",
    description: "Read a business profile and settings: address, currency, tax, timezone, public page, QR defaults, AI settings. Billing data and payout wallet addresses are left out.",
    inputSchema: objectSchema({ business_id: businessIdSchema }, ["business_id"]),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: getBusinessProfile,
  },
  {
    name: "payverge_get_setup_status",
    title: "Get setup checklist",
    description: "Read the first-run setup checklist for a business (profile, tables, menu, staff, payments, layout) with what is done and what is missing.",
    inputSchema: objectSchema({ business_id: businessIdSchema }, ["business_id"]),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: getSetupStatus,
  },
  {
    name: "payverge_create_business",
    title: "Create business",
    description:
      "Create a new business owned by the MCP owner account (POST /inside/businesses). Uses an Idempotency-Key so a retried call does not create a duplicate. Refused when PAYVERGE_MCP_BUSINESS_IDS is set. Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        name: { type: "string", minLength: 1, maxLength: 255 },
        business_type: { type: "string", enum: BUSINESS_TYPES },
        address: addressSchema,
        timezone: profileChangeProperties.timezone,
        default_currency: profileChangeProperties.default_currency,
        display_currency: profileChangeProperties.display_currency,
        tax_rate: profileChangeProperties.tax_rate,
        service_fee_rate: profileChangeProperties.service_fee_rate,
        tax_inclusive: profileChangeProperties.tax_inclusive,
        service_inclusive: profileChangeProperties.service_inclusive,
        description: profileChangeProperties.description,
        phone: profileChangeProperties.phone,
        website: profileChangeProperties.website,
        idempotency_key: { type: "string", minLength: 8, maxLength: 200, description: "Optional. Defaults to a hash of the request body." },
        ...dryRunProperties,
      },
      ["name"],
    ),
    annotations: writeAnnotations({ idempotent: true }),
    auth: "owner",
    mutating: true,
    handler: createBusiness,
  },
  {
    name: "payverge_update_business_profile",
    title: "Update business profile",
    description:
      "Change business profile and settings fields (PUT /inside/businesses/:id, partial). The preview shows a field-by-field diff. Payout wallets and email are not editable here; AI settings have their own tool. Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        changes: { type: "object", additionalProperties: false, minProperties: 1, properties: profileChangeProperties },
        ...dryRunProperties,
      },
      ["business_id", "changes"],
    ),
    annotations: writeAnnotations({ idempotent: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: updateBusinessProfile,
  },
  {
    name: "payverge_get_ai_settings",
    title: "Get AI settings",
    description: "Read a business's AI waiter settings and whether this server has an LLM provider configured.",
    inputSchema: objectSchema({ business_id: businessIdSchema }, ["business_id"]),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: getAiSettings,
  },
  {
    name: "payverge_update_ai_settings",
    title: "Update AI settings",
    description:
      "Turn the guest AI waiter on or off and set its name, priority and house instructions (PUT /inside/businesses/:id). Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        enabled: { type: "boolean", description: "AI waiter on the table (guest) page." },
        business_page_enabled: { type: "boolean", description: "AI waiter on the public business page (/b/<slug>)." },
        name: { type: "string", minLength: 1, maxLength: 40, description: "Display name of the waiter, e.g. Sage." },
        priority: { type: "string", enum: AI_PRIORITIES, description: "balanced (default), upselling, or service (brief, no unprompted pairings)." },
        special_instructions: { type: "string", maxLength: 4000, description: "House rules for the waiter, e.g. 'Always mention the daily special'." },
        ...dryRunProperties,
      },
      ["business_id"],
      { minProperties: 2 },
    ),
    annotations: writeAnnotations({ idempotent: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: updateAiSettings,
  },
  {
    name: "payverge_get_menu",
    title: "Get menu",
    description: "Read the menu in its source language: categories with ids, items with prices (major units), allergens, dietary tags, options and stored availability. Also returns the menu version used for safe writes.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        include_items: { type: "boolean", default: true, description: "false returns categories with item counts only." },
      },
      ["business_id"],
    ),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: getMenu,
  },
  {
    name: "payverge_add_menu_category",
    title: "Add menu category",
    description: "Add a menu category, optionally with its items, in one versioned write. Creates the menu if the business has none. Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        name: { type: "string", minLength: 1, maxLength: 200 },
        description: { type: "string", maxLength: 2000 },
        items: { type: "array", maxItems: MAX_IMPORT_ITEMS, items: menuItemSchema },
        confirm_sanitization: confirmSanitizationSchema,
        ...dryRunProperties,
      },
      ["business_id", "name"],
    ),
    annotations: writeAnnotations(),
    auth: "owner",
    business: true,
    mutating: true,
    handler: addMenuCategory,
  },
  {
    name: "payverge_update_menu_category",
    title: "Update menu category",
    description: "Rename a menu category or change its description. Its items are kept. Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        ...categoryRefProperties,
        name: { type: "string", minLength: 1, maxLength: 200, description: "New name." },
        description: { type: "string", maxLength: 2000, description: "New description." },
        ...dryRunProperties,
      },
      ["business_id"],
    ),
    annotations: writeAnnotations({ idempotent: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: updateMenuCategory,
  },
  {
    name: "payverge_add_menu_item",
    title: "Add menu item",
    description: "Add one item to an existing category (by id or name). Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        ...categoryRefProperties,
        item: menuItemSchema,
        confirm_sanitization: confirmSanitizationSchema,
        ...dryRunProperties,
      },
      ["business_id", "item"],
    ),
    annotations: writeAnnotations(),
    auth: "owner",
    business: true,
    mutating: true,
    handler: addMenuItem,
  },
  {
    name: "payverge_update_menu_item",
    title: "Update menu item",
    description: "Change fields of one menu item (price, name, allergens, availability, ...). Fields you omit keep their value. Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        ...categoryRefProperties,
        item_id: { type: "string", minLength: 1, description: "Item id from payverge_get_menu." },
        item_name: { type: "string", minLength: 1, description: "Item name (case-insensitive), used when item_id is not given." },
        changes: menuItemPatchSchema,
        confirm_sanitization: confirmSanitizationSchema,
        ...dryRunProperties,
      },
      ["business_id", "changes"],
    ),
    annotations: writeAnnotations({ idempotent: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: updateMenuItem,
  },
  {
    name: "payverge_import_menu",
    title: "Import menu from JSON",
    description:
      `Bulk-load a menu from a JSON document {categories:[{name, description?, items:[{name, price, description?, allergens?, dietary_tags?, options?}]}]}. mode=append (default) adds new categories and items and skips items whose name already exists in that category; mode=replace overwrites the whole menu and also needs confirm=true. At most ${MAX_IMPORT_ITEMS} items. Defaults to dry_run=true and returns the plan.`,
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        menu: objectSchema(
          {
            categories: {
              type: "array",
              minItems: 1,
              maxItems: MAX_IMPORT_CATEGORIES,
              items: objectSchema(
                {
                  name: { type: "string", minLength: 1, maxLength: 200 },
                  description: { type: "string", maxLength: 2000 },
                  items: { type: "array", maxItems: MAX_IMPORT_ITEMS, items: menuItemSchema },
                },
                ["name"],
              ),
            },
          },
          ["categories"],
        ),
        mode: { type: "string", enum: ["append", "replace"], default: "append" },
        skip_existing: { type: "boolean", default: true, description: "append mode: skip items whose name already exists in the target category." },
        confirm: { type: "boolean", default: false, description: "Required (true) to apply mode=replace, which deletes every current category and item." },
        confirm_sanitization: confirmSanitizationSchema,
        ...dryRunProperties,
      },
      ["business_id", "menu"],
    ),
    annotations: writeAnnotations({ destructive: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: importMenu,
  },
  {
    name: "payverge_list_tables",
    title: "List tables and QR links",
    description: "List tables with capacity, table code, and the absolute guest link each QR code opens (PUBLIC_URL/t/<code>).",
    inputSchema: objectSchema({ business_id: businessIdSchema }, ["business_id"]),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: listTables,
  },
  {
    name: "payverge_bulk_create_tables",
    title: "Create tables in bulk",
    description:
      `Create several tables at once, from explicit names or from prefix+start+count (e.g. "Table 1".."Table 12"). Names that already exist are skipped. At most ${MAX_BULK_TABLES} per call. Each table gets a QR code. Defaults to dry_run=true.`,
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        names: { type: "array", minItems: 1, maxItems: MAX_BULK_TABLES, items: { type: "string", minLength: 1, maxLength: 64 } },
        prefix: { type: "string", maxLength: 50, default: "Table ", description: "Used with count. Include a trailing space if you want one." },
        start: { type: "integer", minimum: 0, maximum: 100000, default: 1 },
        count: { type: "integer", minimum: 1, maximum: MAX_BULK_TABLES },
        capacity: { type: "integer", minimum: 1, maximum: 100, default: 4, description: "Seats per table." },
        ...dryRunProperties,
      },
      ["business_id"],
    ),
    annotations: writeAnnotations({ idempotent: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: bulkCreateTables,
  },
  {
    name: "payverge_list_staff",
    title: "List staff and invitations",
    description: "List staff members (role, active, last login) and pending invitations for a business.",
    inputSchema: objectSchema({ business_id: businessIdSchema }, ["business_id"]),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: listStaff,
  },
  {
    name: "payverge_invite_staff",
    title: "Invite staff member",
    description:
      `Invite a person to a business with a role (${STAFF_ROLES.join(", ")}). The backend emails the invitation. The invitation link is a credential and is redacted unless reveal_invitation_url=true. Defaults to dry_run=true.`,
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        email: { type: "string", pattern: "^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$", maxLength: 254 },
        name: { type: "string", minLength: 1, maxLength: 200 },
        role: { type: "string", enum: STAFF_ROLES },
        reveal_invitation_url: { type: "boolean", default: false, description: "Include the one-time invitation link in the result (treat it like a password)." },
        ...dryRunProperties,
      },
      ["business_id", "email", "name", "role"],
    ),
    annotations: writeAnnotations(),
    auth: "owner",
    business: true,
    mutating: true,
    handler: inviteStaff,
  },
  {
    name: "payverge_change_staff_role",
    title: "Change staff role",
    description: `Change a staff member's role (${STAFF_ROLES.join(", ")}). The preview shows the current role. Defaults to dry_run=true.`,
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        staff_id: { type: "integer", minimum: 1, description: "Staff id from payverge_list_staff." },
        new_role: { type: "string", enum: STAFF_ROLES },
        reason: { type: "string", maxLength: 500, description: "Recorded in the staff audit log." },
        ...dryRunProperties,
      },
      ["business_id", "staff_id", "new_role"],
    ),
    annotations: writeAnnotations({ idempotent: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: changeStaffRole,
  },
  {
    name: "payverge_payment_plugin_status",
    title: "Payment plugin status",
    description: "Show which payment and integration plugins are installed and enabled for a business, their last status and error, and whether payments are ready. Never returns plugin credentials.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        include_catalog: { type: "boolean", default: false, description: "Also list every plugin this server offers." },
      },
      ["business_id"],
    ),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: paymentPluginStatus,
  },
  {
    name: "payverge_get_reservation_settings",
    title: "Get reservation settings",
    description: "Read reservation settings: on/off, party sizes, slot length, approval mode, cancellation and reminder rules.",
    inputSchema: objectSchema({ business_id: businessIdSchema }, ["business_id"]),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: getReservationSettings,
  },
  {
    name: "payverge_update_reservation_settings",
    title: "Update reservation settings",
    description: "Change reservation settings (partial). The preview shows a field-by-field diff. Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        changes: { type: "object", additionalProperties: false, minProperties: 1, properties: reservationSettingsProperties },
        ...dryRunProperties,
      },
      ["business_id", "changes"],
    ),
    annotations: writeAnnotations({ idempotent: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: updateReservationSettings,
  },
  {
    name: "payverge_get_opening_hours",
    title: "Get opening hours",
    description: "Read the weekly opening hours as a schedule keyed by weekday (monday..sunday), with up to two periods per day.",
    inputSchema: objectSchema({ business_id: businessIdSchema }, ["business_id"]),
    annotations: readAnnotations,
    auth: "owner",
    business: true,
    handler: getOpeningHours,
  },
  {
    name: "payverge_set_opening_hours",
    title: "Set opening hours",
    description:
      "Set weekly opening hours. schedule maps weekday -> {closed:true} or {periods:[{open:'12:00', close:'15:30', kitchen_close?:'15:00'}, ...]} (max 2 periods). mode=merge (default) changes only the days you pass; mode=replace clears the days you omit and needs confirm=true. Defaults to dry_run=true.",
    inputSchema: objectSchema(
      {
        business_id: businessIdSchema,
        schedule: objectSchema(Object.fromEntries(WEEKDAYS.map((day) => [day, daySchema])), [], { minProperties: 1 }),
        mode: { type: "string", enum: ["merge", "replace"], default: "merge" },
        confirm: { type: "boolean", default: false, description: "Required (true) to apply mode=replace, which clears every weekday not in schedule." },
        ...dryRunProperties,
      },
      ["business_id", "schedule"],
    ),
    annotations: writeAnnotations({ destructive: true, idempotent: true }),
    auth: "owner",
    business: true,
    mutating: true,
    handler: setOpeningHours,
  },
];

export const CONFIG_TOOL_NAMES = TOOLS.map((tool) => tool.name);

/**
 * @param {object} deps
 * @param {import("./backendClient.mjs").PayvergeAdminClient} [deps.ownerClient]  owner-session client
 * @param {import("./backendClient.mjs").PayvergeAdminClient} [deps.publicClient] anonymous client
 * @param {object} [deps.options]
 *   readOnly, businessAllowList (number[]), publicUrl, apiBaseUrl,
 *   ownerSession ({describe()}), adminConfigured (boolean)
 */
export function createConfigTools({ ownerClient, publicClient, options = {} } = {}) {
  const byName = new Map(TOOLS.map((tool) => [tool.name, tool]));
  const ctxBase = {
    owner: ownerClient,
    pub: publicClient,
    options: {
      ...options,
      publicUrl: trimSlash(options.publicUrl ?? ""),
      businessAllowList: Array.isArray(options.businessAllowList) && options.businessAllowList.length > 0
        ? options.businessAllowList.map(Number)
        : undefined,
    },
  };

  return {
    definitions: TOOLS.map(({ name, title, description, inputSchema, annotations }) => ({
      name,
      title,
      description,
      inputSchema,
      annotations,
    })),

    has(name) {
      return byName.has(name);
    },

    async call(name, rawArgs) {
      const tool = byName.get(name);
      if (!tool) throw new Error(`Unknown tool: ${name}`);
      const args = rawArgs ?? {};
      try {
        const problems = validateArgs(tool.inputSchema, args);
        if (problems.length > 0) {
          throw new ToolError("invalid_arguments", `Invalid arguments for ${name}`, {
            details: problems,
            hint: "Fix the listed fields; the input schema is in tools/list.",
          });
        }
        if (tool.auth === "owner" && !ownerClient) {
          throw new ToolError("owner_auth_not_configured", "Owner credentials are not configured for this MCP server", {
            hint: "Set PAYVERGE_OWNER_EMAIL and PAYVERGE_OWNER_PASSWORD (recommended) or PAYVERGE_OWNER_TOKEN in the MCP server env, then restart it. See tools/payverge-admin-mcp/README.md#authentication.",
          });
        }
        if (tool.auth === "public" && !publicClient) {
          throw new ToolError("not_configured", "No API client is configured for public routes");
        }
        if (tool.business) assertBusinessAllowed(ctxBase.options, args.business_id);

        let dryRun = false;
        if (tool.mutating) {
          dryRun = resolveDryRun(args);
          if (!dryRun && ctxBase.options.readOnly) {
            throw new ToolError("read_only_mode", "PAYVERGE_MCP_READ_ONLY is set, so changes are refused", {
              hint: "Previews (dry_run=true) still work. Unset PAYVERGE_MCP_READ_ONLY and restart the MCP server to apply changes.",
            });
          }
        }

        const result = await tool.handler(args, { ...ctxBase, dryRun });
        return toolResult(result);
      } catch (err) {
        return errorResult(err);
      }
    },
  };
}

export function resolveDryRun(args = {}) {
  if (typeof args.dry_run === "boolean") return args.dry_run;
  if (typeof args.dryRun === "boolean") return args.dryRun;
  return true;
}

// ---------------------------------------------------------------------------
// Handlers: instance and businesses

async function instanceStatus(_args, ctx) {
  const [instance, live, ready] = await Promise.all([
    probe(() => ctx.pub.get("/instance")),
    probe(() => ctx.pub.get("/health/live")),
    probe(() => ctx.pub.get("/health/ready")),
  ]);

  const owner = ctx.options.ownerSession?.describe?.() ?? { mode: "none" };
  const nextSteps = [];
  if (!live.ok) {
    nextSteps.push("The backend did not answer /health/live. Check PAYVERGE_API_BASE_URL (it ends in /api/v1) and that the stack is running.");
  } else if (!ready.ok) {
    nextSteps.push("The backend is up but not ready (database or migrations). Check the backend logs.");
  }
  if (instance.status === 404) {
    nextSteps.push("This backend has no GET /api/v1/instance yet (older build). Feature detection falls back to the other probes.");
  }
  if (instance.ok && instance.body?.features && instance.body.features.ai === false) {
    nextSteps.push("In-app AI is off: no LLM provider is configured. Set OPENROUTER_API_KEY, or LLM_BASE_URL (+ LLM_API_KEY) for a self-hosted model, and restart the backend.");
  }
  if (owner.mode === "none") {
    nextSteps.push("Configuration tools need owner credentials: set PAYVERGE_OWNER_EMAIL and PAYVERGE_OWNER_PASSWORD in the MCP server env.");
  } else if (owner.blocked) {
    nextSteps.push(`Owner sign-in is blocked (${owner.blocked}); fix the credentials and restart the MCP server.`);
  }
  if (nextSteps.length === 0) {
    nextSteps.push("Ready. Next: payverge_list_businesses, then payverge_get_setup_status for the business you are configuring.");
  }

  return {
    api_base_url: ctx.options.apiBaseUrl,
    public_url: ctx.options.publicUrl || undefined,
    instance: instance.ok ? instance.body : { available: false, status: instance.status, error: instance.error },
    health: {
      live: { ok: live.ok, status: live.status },
      ready: { ok: ready.ok, status: ready.status, body: ready.body },
    },
    mcp: {
      version: MCP_SERVER_VERSION,
      owner_auth: owner,
      admin_token_configured: Boolean(ctx.options.adminConfigured),
      read_only: Boolean(ctx.options.readOnly),
      business_allow_list: ctx.options.businessAllowList ?? null,
    },
    next_steps: nextSteps,
  };
}

async function listBusinesses(_args, ctx) {
  const rows = asArray(await ctx.owner.get("/inside/businesses"));
  const allow = ctx.options.businessAllowList;
  const visible = allow ? rows.filter((row) => allow.includes(Number(row.id))) : rows;
  return {
    count: visible.length,
    ...(allow ? { hidden_by_allow_list: rows.length - visible.length } : {}),
    businesses: visible.map((row) => ({
      id: row.id,
      business_id: row.business_id,
      name: row.name,
      business_type: row.business_type,
      is_active: row.is_active,
      default_currency: row.default_currency,
      timezone: row.timezone,
      custom_url: row.custom_url || undefined,
      city: row.address?.city || undefined,
      country: row.address?.country || undefined,
    })),
  };
}

async function getBusinessProfile(args, ctx) {
  const business = await ctx.owner.get(businessPath(args.business_id));
  return { business: projectBusiness(business) };
}

async function getSetupStatus(args, ctx) {
  return ctx.owner.get(businessPath(args.business_id, "/setup-status"));
}

async function createBusiness(args, ctx) {
  if (ctx.options.businessAllowList) {
    throw new ToolError("business_not_allowed", "Creating businesses is disabled while PAYVERGE_MCP_BUSINESS_IDS is set", {
      hint: "Unset PAYVERGE_MCP_BUSINESS_IDS (or create the business in the dashboard) and add its id to the list afterwards.",
    });
  }
  const { idempotency_key: explicitKey, dry_run: _d, dryRun: _a, ...fields } = args;
  const body = { ...fields };
  const idempotencyKey = explicitKey ?? `mcp-create-business-${sha256(canonicalJson(body)).slice(0, 40)}`;
  const request = { method: "POST", path: "/inside/businesses", headers: { "Idempotency-Key": idempotencyKey }, body };
  if (ctx.dryRun) {
    return {
      dry_run: true,
      request,
      notes: [
        "The business is owned by the MCP owner account.",
        "Repeating this exact call returns the same business instead of creating a second one.",
        "Defaults the backend fills in when omitted: currency and timezone from the address country, then USD/UTC.",
      ],
      next: "Call again with dry_run=false to create it.",
    };
  }
  const created = await ctx.owner.post("/inside/businesses", body, { headers: { "Idempotency-Key": idempotencyKey } });
  return {
    dry_run: false,
    business: projectBusiness(created),
    next: "Run payverge_get_setup_status with the new id to see what is left to configure.",
  };
}

async function updateBusinessProfile(args, ctx) {
  const path = businessPath(args.business_id);
  const current = await ctx.owner.get(path);
  const body = { ...args.changes };
  if (body.address) body.address = { ...emptyAddress(), ...(current.address ?? {}), ...body.address };
  const diff = diffFields(current, body);
  if (ctx.dryRun) {
    return { dry_run: true, request: { method: "PUT", path, body }, diff, next: "Call again with dry_run=false to apply." };
  }
  if (diff.every((entry) => !entry.changed)) {
    return { dry_run: false, changed: false, diff, message: "Nothing to change; no request sent." };
  }
  const updated = await ctx.owner.put(path, body);
  return {
    dry_run: false,
    changed: true,
    diff,
    business: projectBusiness(updated),
    ...(Array.isArray(updated?.skipped_fields) && updated.skipped_fields.length > 0
      ? { skipped_fields: updated.skipped_fields }
      : {}),
  };
}

async function getAiSettings(args, ctx) {
  const [business, instance] = await Promise.all([
    ctx.owner.get(businessPath(args.business_id)),
    ctx.pub ? probe(() => ctx.pub.get("/instance")) : Promise.resolve({ ok: false }),
  ]);
  return {
    business_id: args.business_id,
    ai_settings: projectAiSettings(business.ai_settings),
    server: aiServerState(instance),
  };
}

async function updateAiSettings(args, ctx) {
  const path = businessPath(args.business_id);
  const mapping = {
    enabled: "ai_enabled",
    business_page_enabled: "business_page_ai_enabled",
    name: "ai_name",
    priority: "ai_priority",
    special_instructions: "ai_special_instructions",
  };
  const body = {};
  for (const [input, field] of Object.entries(mapping)) {
    if (args[input] !== undefined) body[field] = args[input];
  }
  if (Object.keys(body).length === 0) {
    throw new ToolError("invalid_arguments", "Pass at least one of enabled, business_page_enabled, name, priority, special_instructions");
  }

  const [business, instance] = await Promise.all([
    ctx.owner.get(path),
    ctx.pub ? probe(() => ctx.pub.get("/instance")) : Promise.resolve({ ok: false }),
  ]);
  const current = projectAiSettings(business.ai_settings);
  const currentByField = {
    ai_enabled: current.enabled,
    business_page_ai_enabled: current.business_page_enabled,
    ai_name: current.name,
    ai_priority: current.priority,
    ai_special_instructions: current.special_instructions,
  };
  const diff = Object.entries(body).map(([field, to]) => ({
    field,
    from: currentByField[field],
    to,
    changed: !sameValue(currentByField[field], to),
  }));
  const server = aiServerState(instance);
  const warnings = [];
  if ((body.ai_enabled || body.business_page_ai_enabled) && server.llm_provider_configured === false) {
    warnings.push("No LLM provider is configured on this server: the guest waiter runs in its no-key fallback mode, and the ops assistant and director console answer 503 ai_not_configured. Set OPENROUTER_API_KEY or LLM_BASE_URL and restart the backend.");
  }

  if (ctx.dryRun) {
    return { dry_run: true, request: { method: "PUT", path, body }, diff, server, warnings, next: "Call again with dry_run=false to apply." };
  }
  const updated = await ctx.owner.put(path, body);
  const skipped = Array.isArray(updated?.skipped_fields) ? updated.skipped_fields : [];
  if (skipped.length > 0) {
    // UpdateBusiness drops AI fields for staff (manager) tokens and lists them.
    warnings.push(`The backend ignored ${skipped.join(", ")}: AI settings are owner-only, and this session is a staff (manager) token. Use the owner's email and password.`);
  }
  return {
    dry_run: false,
    applied: skipped.length === 0,
    diff,
    ai_settings: projectAiSettings(updated?.ai_settings),
    ...(skipped.length > 0 ? { skipped_fields: skipped } : {}),
    server,
    warnings,
  };
}

// ---------------------------------------------------------------------------
// Handlers: menu

async function getMenu(args, ctx) {
  const menu = await readMenu(ctx, args.business_id);
  const includeItems = args.include_items !== false;
  let itemCount = 0;
  const categories = menu.categories.map((category) => {
    const items = Array.isArray(category.items) ? category.items : [];
    itemCount += items.length;
    return {
      id: category.id,
      name: category.name,
      description: category.description || undefined,
      sort_order: category.sort_order,
      item_count: items.length,
      ...(includeItems ? { items: items.map(projectMenuItem) } : {}),
    };
  });
  return {
    business_id: args.business_id,
    menu_exists: menu.exists,
    version: menu.version,
    category_count: categories.length,
    item_count: itemCount,
    categories,
  };
}

async function addMenuCategory(args, ctx) {
  const menu = await readMenu(ctx, args.business_id);
  const items = (args.items ?? []).map((item) => toMenuItemPayload(item));
  const duplicate = findCategoriesByName(menu.categories, args.name);
  const warnings = duplicate.length > 0 ? [`A category named "${args.name}" already exists (id ${duplicate[0].id}); this adds a second one.`] : [];
  const path = businessPath(args.business_id, "/menu/categories");
  const body = {
    name: args.name,
    description: args.description ?? "",
    items,
    version: menu.version,
    ...(args.confirm_sanitization ? { confirm_sanitization: true } : {}),
  };
  if (ctx.dryRun) {
    return { dry_run: true, request: { method: "POST", path, body }, warnings, next: "Call again with dry_run=false to add it." };
  }
  const response = await withMenuVersionRetry(ctx, args.business_id, menu.version, (version) =>
    ctx.owner.post(path, { ...body, version }),
  );
  return menuWriteResult(response, { category: response?.category ? projectCategory(response.category) : undefined, warnings });
}

async function updateMenuCategory(args, ctx) {
  if (args.name === undefined && args.description === undefined) {
    throw new ToolError("invalid_arguments", "Pass name and/or description to change");
  }
  const menu = await readMenu(ctx, args.business_id);
  const category = resolveCategory(menu.categories, args);
  const path = businessPath(args.business_id, `/menu/category/${encodeURIComponent(category.id)}`);
  const body = {
    name: args.name ?? category.name,
    description: args.description ?? category.description ?? "",
    // The backend replaces the whole category, items included: resend them
    // with their stored (not inventory-derived) availability.
    items: (category.items ?? []).map(storedMenuItem),
    version: menu.version,
  };
  const diff = [
    { field: "name", from: category.name, to: body.name, changed: category.name !== body.name },
    { field: "description", from: category.description ?? "", to: body.description, changed: (category.description ?? "") !== body.description },
  ];
  const warnings = [];
  if (category.sort_order) {
    warnings.push(`The backend's category update does not carry sort_order, so this category's stored sort_order (${category.sort_order}) resets to 0. The AI waiter orders categories by sort_order.`);
  }
  if (ctx.dryRun) {
    return {
      dry_run: true,
      request: { method: "PUT", path, body: { ...body, items: `[${body.items.length} existing items, unchanged]` } },
      diff,
      warnings,
      next: "Call again with dry_run=false to apply.",
    };
  }
  const response = await withMenuVersionRetry(ctx, args.business_id, menu.version, async (version, fresh) => {
    const target = fresh ? resolveCategory(fresh.categories, { category_id: category.id }) : category;
    return ctx.owner.put(path, { ...body, items: (target.items ?? []).map(storedMenuItem), version });
  });
  return menuWriteResult(response, { diff, warnings, category: response?.category ? projectCategory(response.category) : undefined });
}

async function addMenuItem(args, ctx) {
  const menu = await readMenu(ctx, args.business_id);
  const category = resolveCategory(menu.categories, args);
  const item = toMenuItemPayload(args.item);
  const path = businessPath(args.business_id, "/menu/items");
  const body = {
    category_id: category.id,
    item,
    version: menu.version,
    ...(args.confirm_sanitization ? { confirm_sanitization: true } : {}),
  };
  const warnings = findItemsByName(category.items, item.name).length > 0
    ? [`"${item.name}" already exists in "${category.name}"; this adds a second item with the same name.`]
    : [];
  if (ctx.dryRun) {
    return { dry_run: true, request: { method: "POST", path, body }, category: { id: category.id, name: category.name }, warnings, next: "Call again with dry_run=false to add it." };
  }
  const response = await withMenuVersionRetry(ctx, args.business_id, menu.version, (version) =>
    ctx.owner.post(path, { ...body, version }),
  );
  return menuWriteResult(response, { item: response?.item ? projectMenuItem(response.item) : undefined, warnings });
}

async function updateMenuItem(args, ctx) {
  const menu = await readMenu(ctx, args.business_id);
  const { category, item } = resolveItem(menu.categories, args);
  const before = storedMenuItem(item);
  const after = { ...before };
  for (const [field, value] of Object.entries(args.changes)) {
    after[field] = field === "options" ? value.map(toOptionPayload) : value;
  }
  const diff = Object.keys(args.changes).map((field) => ({
    field,
    from: before[field],
    to: after[field],
    changed: !sameValue(before[field], after[field]),
  }));
  const path = businessPath(args.business_id, "/menu/items");
  const body = {
    category_id: category.id,
    item_id: item.id,
    item: after,
    version: menu.version,
    ...(args.confirm_sanitization ? { confirm_sanitization: true } : {}),
  };
  if (ctx.dryRun) {
    return { dry_run: true, request: { method: "PUT", path, body }, diff, next: "Call again with dry_run=false to apply." };
  }
  if (diff.every((entry) => !entry.changed)) {
    return { dry_run: false, changed: false, diff, message: "Nothing to change; no request sent." };
  }
  const response = await withMenuVersionRetry(ctx, args.business_id, menu.version, (version) =>
    ctx.owner.put(path, { ...body, version }),
  );
  return menuWriteResult(response, { diff, item: response?.item ? projectMenuItem(response.item) : undefined });
}

async function importMenu(args, ctx) {
  const doc = args.menu;
  const totalItems = doc.categories.reduce((sum, category) => sum + (category.items?.length ?? 0), 0);
  if (totalItems > MAX_IMPORT_ITEMS) {
    throw new ToolError("invalid_arguments", `The document has ${totalItems} items; the limit is ${MAX_IMPORT_ITEMS} per call`, {
      hint: "Split the import into several calls (for example one call per group of categories).",
    });
  }
  const mode = args.mode ?? "append";
  const menu = await readMenu(ctx, args.business_id);

  if (mode === "replace") return replaceMenu(args, ctx, menu, totalItems);

  const skipExisting = args.skip_existing !== false;
  const newCategories = [];
  const newByName = new Map();
  const itemAdds = [];
  const skipped = [];
  const warnings = [];

  for (const docCategory of doc.categories) {
    const key = normalizeName(docCategory.name);
    const matches = findCategoriesByName(menu.categories, docCategory.name);
    if (matches.length > 1) {
      warnings.push(`Several categories are named "${docCategory.name}"; items go to the first one (id ${matches[0].id}).`);
    }
    const existing = matches[0];
    if (!existing) {
      let planned = newByName.get(key);
      if (!planned) {
        planned = { name: docCategory.name, description: docCategory.description ?? "", items: [] };
        newByName.set(key, planned);
        newCategories.push(planned);
      }
      for (const item of docCategory.items ?? []) {
        if (findItemsByName(planned.items, item.name).length > 0) {
          skipped.push({ category: docCategory.name, item: item.name, reason: "duplicate in document" });
          continue;
        }
        planned.items.push(toMenuItemPayload(item));
      }
      continue;
    }
    const plannedHere = [];
    for (const item of docCategory.items ?? []) {
      if (skipExisting && findItemsByName(existing.items, item.name).length > 0) {
        skipped.push({ category: existing.name, item: item.name, reason: "already on the menu" });
        continue;
      }
      if (findItemsByName(plannedHere, item.name).length > 0) {
        skipped.push({ category: existing.name, item: item.name, reason: "duplicate in document" });
        continue;
      }
      const payload = toMenuItemPayload(item);
      plannedHere.push(payload);
      itemAdds.push({ category_id: existing.id, category_name: existing.name, item: payload });
    }
  }

  const plan = {
    mode: "append",
    menu_version: menu.version,
    create_categories: newCategories.map((category) => ({ name: category.name, items: category.items.map((item) => item.name) })),
    add_items: itemAdds.map((entry) => ({ category: entry.category_name, item: entry.item.name, price: entry.item.price })),
    skipped,
    totals: {
      categories_to_create: newCategories.length,
      items_to_create: newCategories.reduce((sum, category) => sum + category.items.length, 0) + itemAdds.length,
      items_skipped: skipped.length,
      requests: newCategories.length + itemAdds.length,
    },
    warnings,
  };

  if (ctx.dryRun) {
    return { dry_run: true, plan, next: plan.totals.requests > 0 ? "Call again with dry_run=false to run this plan." : "Nothing to import." };
  }

  const completed = [];
  let version = menu.version;
  const steps = [
    ...newCategories.map((category) => ({ kind: "category", category })),
    ...itemAdds.map((entry) => ({ kind: "item", entry })),
  ];
  for (let index = 0; index < steps.length; index += 1) {
    const step = steps[index];
    try {
      let response;
      if (step.kind === "category") {
        response = await withMenuVersionRetry(ctx, args.business_id, version, (v) =>
          ctx.owner.post(businessPath(args.business_id, "/menu/categories"), {
            name: step.category.name,
            description: step.category.description,
            items: step.category.items,
            version: v,
            ...(args.confirm_sanitization ? { confirm_sanitization: true } : {}),
          }),
        );
      } else {
        response = await withMenuVersionRetry(ctx, args.business_id, version, (v) =>
          ctx.owner.post(businessPath(args.business_id, "/menu/items"), {
            category_id: step.entry.category_id,
            item: step.entry.item,
            version: v,
            ...(args.confirm_sanitization ? { confirm_sanitization: true } : {}),
          }),
        );
      }
      throwIfSanitizationReview(
        response,
        "This step was not saved. Fix the reported values, or rerun with confirm_sanitization=true to save without them. Steps already completed are listed in partial.",
      );
      if (typeof response?.version === "number") version = response.version;
      completed.push(
        step.kind === "category"
          ? { created_category: step.category.name, id: response?.category?.id, items: step.category.items.length }
          : { added_item: step.entry.item.name, category: step.entry.category_name, id: response?.item?.id },
      );
    } catch (err) {
      const failedStep = step.kind === "category" ? { category: step.category.name } : { category: step.entry.category_name, item: step.entry.item.name };
      if (err instanceof ToolError) {
        err.partial = { completed, failed_step: failedStep, remaining_steps: steps.length - index - 1 };
        throw err;
      }
      const wrapped = new ToolError("import_stopped", `Import stopped at step ${index + 1} of ${steps.length}`, {
        details: explainError(err),
        hint: "Completed steps are saved. Fix the cause and rerun the same import: items already on the menu are skipped.",
      });
      wrapped.partial = { completed, failed_step: failedStep, remaining_steps: steps.length - index - 1 };
      throw wrapped;
    }
  }

  return { dry_run: false, mode: "append", menu_version: version, completed, skipped, warnings };
}

async function replaceMenu(args, ctx, menu, totalItems) {
  const existingItems = menu.categories.reduce((sum, category) => sum + (category.items?.length ?? 0), 0);
  const categories = args.menu.categories.map((category) => ({
    name: category.name,
    description: category.description ?? "",
    items: (category.items ?? []).map((item) => toMenuItemPayload(item)),
  }));
  const path = businessPath(args.business_id, "/menu");
  const body = {
    categories,
    ...(menu.exists ? { version: menu.version } : {}),
    ...(args.confirm_sanitization ? { confirm_sanitization: true } : {}),
  };
  const plan = {
    mode: "replace",
    menu_version: menu.version,
    deletes: { categories: menu.categories.length, items: existingItems },
    creates: { categories: categories.length, items: totalItems },
    warning: "Replace deletes every current category and item (with their ids, images and translations) and writes the document as the whole menu.",
  };
  if (ctx.dryRun) {
    return {
      dry_run: true,
      plan,
      request: { method: "POST", path, body: { ...body, categories: `[${categories.length} categories]` } },
      next: "To apply, call again with dry_run=false and confirm=true.",
    };
  }
  if (args.confirm !== true) {
    throw new ToolError("confirmation_required", "mode=replace deletes the current menu; pass confirm=true together with dry_run=false", {
      details: plan,
    });
  }
  const response = await withMenuVersionRetry(ctx, args.business_id, menu.version, (version, fresh) =>
    ctx.owner.post(path, { ...body, ...((fresh ?? menu).exists ? { version } : {}) }),
  );
  throwIfSanitizationReview(response, "Nothing was saved and the current menu is unchanged. Fix the reported values, or rerun with confirm_sanitization=true to save without them.");
  return {
    dry_run: false,
    saved: true,
    mode: "replace",
    menu_version: typeof response?.version === "number" ? response.version : undefined,
    created: plan.creates,
    deleted: plan.deletes,
    sanitization: response?.sanitization,
  };
}

// ---------------------------------------------------------------------------
// Handlers: tables

async function listTables(args, ctx) {
  const response = await ctx.owner.get(businessPath(args.business_id, "/tables"));
  const tables = asArray(response?.tables ?? response).map((table) => projectTable(table, ctx.options.publicUrl));
  return {
    business_id: args.business_id,
    count: tables.length,
    public_url: ctx.options.publicUrl || undefined,
    tables,
    note: ctx.options.publicUrl
      ? "guest_url is what each table's QR code opens. Print QR codes from the dashboard (Tables > QR)."
      : "Set PAYVERGE_PUBLIC_URL to get absolute guest links.",
  };
}

async function bulkCreateTables(args, ctx) {
  const hasNames = Array.isArray(args.names);
  const hasCount = args.count !== undefined;
  if (hasNames === hasCount) {
    throw new ToolError("invalid_arguments", "Pass either names, or count (with optional prefix and start), not both");
  }
  const capacity = args.capacity ?? 4;
  const wanted = hasNames
    ? args.names.map((name) => name.trim())
    : Array.from({ length: args.count }, (_, index) => `${args.prefix ?? "Table "}${(args.start ?? 1) + index}`.trim());
  const tooLong = wanted.filter((name) => name.length === 0 || [...name].length > 64);
  if (tooLong.length > 0) {
    throw new ToolError("invalid_arguments", "Table names must be 1-64 characters", { details: tooLong });
  }

  const path = businessPath(args.business_id, "/tables");
  const existing = asArray((await ctx.owner.get(path))?.tables);
  const existingNames = new Set(existing.map((table) => normalizeName(table.name)));
  const toCreate = [];
  const skipped = [];
  const seen = new Set();
  for (const name of wanted) {
    const key = normalizeName(name);
    if (existingNames.has(key)) {
      skipped.push({ name, reason: "already exists" });
    } else if (seen.has(key)) {
      skipped.push({ name, reason: "duplicate in request" });
    } else {
      seen.add(key);
      toCreate.push(name);
    }
  }

  if (ctx.dryRun) {
    return {
      dry_run: true,
      request: { method: "POST", path, body_per_table: { name: "<name>", capacity }, count: toCreate.length },
      to_create: toCreate,
      skipped,
      existing_tables: existing.length,
      next: toCreate.length > 0 ? "Call again with dry_run=false to create them." : "Nothing to create.",
    };
  }

  const created = [];
  for (let index = 0; index < toCreate.length; index += 1) {
    try {
      const table = await ctx.owner.post(path, { name: toCreate[index], capacity });
      created.push(projectTable(table, ctx.options.publicUrl));
    } catch (err) {
      const wrapped = new ToolError("bulk_create_stopped", `Stopped after ${created.length} of ${toCreate.length} tables`, {
        details: explainError(err),
        hint: "Created tables are kept. Rerun the same call: existing names are skipped.",
      });
      wrapped.partial = { created, failed: toCreate[index], remaining: toCreate.slice(index + 1), skipped };
      throw wrapped;
    }
  }
  return { dry_run: false, created_count: created.length, created, skipped };
}

// ---------------------------------------------------------------------------
// Handlers: staff

async function listStaff(args, ctx) {
  const response = await ctx.owner.get(businessPath(args.business_id, "/staff"));
  return {
    business_id: args.business_id,
    staff: asArray(response?.staff).map(projectStaff),
    pending_invitations: asArray(response?.pending_invitations).map((invite) => ({
      id: invite.id,
      email: invite.email,
      name: invite.name,
      role: invite.role,
      status: invite.status,
      expires_at: invite.expires_at,
    })),
  };
}

async function inviteStaff(args, ctx) {
  const path = businessPath(args.business_id, "/staff/invite");
  const body = { email: args.email.trim().toLowerCase(), name: args.name.trim(), role: args.role };
  if (ctx.dryRun) {
    const current = await ctx.owner.get(businessPath(args.business_id, "/staff"));
    const conflicts = [];
    if (asArray(current?.staff).some((member) => normalizeName(member.email) === body.email)) {
      conflicts.push("This email already belongs to a staff member of this business (the backend answers 409 STAFF_EMAIL_EXISTS).");
    }
    if (asArray(current?.pending_invitations).some((invite) => normalizeName(invite.email) === body.email)) {
      conflicts.push("An invitation for this email is already pending (the backend answers 409 INVITE_ALREADY_PENDING).");
    }
    return {
      dry_run: true,
      request: { method: "POST", path, body },
      conflicts,
      notes: ["The backend emails the invitation link. With EMAIL_PROVIDER=log no email is sent; the link is written to the backend log only when EMAIL_LOG_CONTENT=true, otherwise copy it from the dashboard staff page."],
      next: conflicts.length > 0 ? "Resolve the conflicts first." : "Call again with dry_run=false to send the invitation.",
    };
  }
  const response = await ctx.owner.post(path, body);
  const result = {
    dry_run: false,
    invitation_id: response?.invitation_id,
    email: body.email,
    role: body.role,
    expires_at: response?.expires_at,
    email_sent: response?.email_sent,
  };
  if (response?.invitation_url) {
    result.invitation_url = args.reveal_invitation_url
      ? response.invitation_url
      : "[redacted: one-time credential; pass reveal_invitation_url=true to include it, or copy it from the dashboard staff page]";
  }
  if (response?.email_sent === false) {
    result.hint = "The invitation email was not sent. Share the link from the dashboard (Staff > pending invitation > copy link), or check EMAIL_PROVIDER.";
  }
  return result;
}

async function changeStaffRole(args, ctx) {
  const path = businessPath(args.business_id, `/staff/${args.staff_id}/role`);
  const body = { new_role: args.new_role, ...(args.reason ? { reason: args.reason } : {}) };
  if (ctx.dryRun) {
    const current = await ctx.owner.get(businessPath(args.business_id, "/staff"));
    const member = asArray(current?.staff).find((row) => Number(row.id) === args.staff_id);
    if (!member) {
      throw new ToolError("not_found", `No staff member with id ${args.staff_id} in business ${args.business_id}`, {
        hint: "List ids with payverge_list_staff.",
      });
    }
    return {
      dry_run: true,
      request: { method: "PUT", path, body },
      staff: projectStaff(member),
      change: { from: member.role, to: args.new_role, changed: member.role !== args.new_role },
      next: "Call again with dry_run=false to apply.",
    };
  }
  const response = await ctx.owner.put(path, body);
  return { dry_run: false, staff_id: args.staff_id, new_role: args.new_role, message: response?.message };
}

// ---------------------------------------------------------------------------
// Handlers: plugins, reservations, hours

async function paymentPluginStatus(args, ctx) {
  const response = await ctx.owner.get(businessPath(args.business_id, "/plugins"));
  const plugins = asArray(response?.plugins).map((plugin) => ({
    plugin_id: plugin.plugin_id,
    name: plugin.name,
    display_name: plugin.display_name || plugin.name,
    category: plugin.category,
    version: plugin.version,
    is_enabled: Boolean(plugin.is_enabled),
    last_status: plugin.last_status || undefined,
    last_error: plugin.last_error ? truncate(String(plugin.last_error), MAX_ERROR_TEXT) : undefined,
    last_error_at: plugin.last_error_at || undefined,
    last_success_at: plugin.last_success_at || undefined,
  }));
  const out = {
    business_id: args.business_id,
    payments_ready: response?.payments_ready,
    enabled_payment_count: response?.enabled_payment_count,
    plugins,
    note: "Credentials are never read by this tool. Configure plugins in the dashboard (Settings > Plugins).",
  };
  if (args.include_catalog) {
    const catalog = await ctx.owner.get("/inside/plugins");
    out.catalog = asArray(catalog?.plugins ?? catalog).map((plugin) => ({
      plugin_id: plugin.id ?? plugin.plugin_id,
      name: plugin.name,
      display_name: plugin.display_name || plugin.name,
      category: plugin.category,
      version: plugin.version,
      coming_soon: plugin.coming_soon || undefined,
      description: plugin.description ? truncate(String(plugin.description), MAX_ERROR_TEXT) : undefined,
    }));
  }
  return out;
}

async function getReservationSettings(args, ctx) {
  const settings = await ctx.owner.get(businessPath(args.business_id, "/reservations/settings"));
  return { business_id: args.business_id, settings: projectReservationSettings(settings) };
}

async function updateReservationSettings(args, ctx) {
  const path = businessPath(args.business_id, "/reservations/settings");
  const current = await ctx.owner.get(path);
  const body = { ...args.changes };
  const minParty = body.min_party_size ?? current?.min_party_size;
  const maxParty = body.max_party_size ?? current?.max_party_size;
  if (typeof minParty === "number" && typeof maxParty === "number" && maxParty < minParty) {
    throw new ToolError("invalid_arguments", `max_party_size (${maxParty}) must be >= min_party_size (${minParty})`);
  }
  const diff = diffFields(current ?? {}, body);
  if (ctx.dryRun) {
    return { dry_run: true, request: { method: "PUT", path, body }, diff, next: "Call again with dry_run=false to apply." };
  }
  const updated = await ctx.owner.put(path, body);
  return { dry_run: false, diff, settings: projectReservationSettings(updated) };
}

async function getOpeningHours(args, ctx) {
  const rows = asArray(await ctx.owner.get(businessPath(args.business_id, "/operating-hours")));
  return { business_id: args.business_id, schedule: rowsToSchedule(rows), rows: rows.length };
}

async function setOpeningHours(args, ctx) {
  const problems = [];
  for (const [day, spec] of Object.entries(args.schedule)) {
    const periods = spec.periods ?? [];
    if (spec.closed === true && periods.length > 0) problems.push(`${day}: a closed day cannot have periods`);
    if (spec.closed !== true && periods.length === 0) problems.push(`${day}: pass closed=true or at least one period`);
  }
  if (problems.length > 0) {
    throw new ToolError("invalid_arguments", "Invalid schedule", { details: problems });
  }

  const path = businessPath(args.business_id, "/operating-hours");
  const currentRows = asArray(await ctx.owner.get(path));
  const current = rowsToSchedule(currentRows);
  const mode = args.mode ?? "merge";
  const next = {};
  for (const day of WEEKDAYS) {
    if (args.schedule[day]) {
      next[day] = normalizeDaySpec(args.schedule[day]);
    } else if (mode === "merge") {
      next[day] = current[day];
    } else {
      next[day] = { unset: true };
    }
  }
  const body = scheduleToRows(next);
  const diff = WEEKDAYS.map((day) => ({
    day,
    from: current[day],
    to: next[day],
    changed: !sameValue(current[day], next[day]),
  })).filter((entry) => entry.changed);
  // Days that currently have a row (open hours or an explicit closed entry)
  // and that replace mode will drop because the caller omitted them.
  const cleared = WEEKDAYS.filter((day) => !args.schedule[day] && current[day] && !current[day].unset);

  if (ctx.dryRun) {
    return {
      dry_run: true,
      mode,
      request: { method: "PUT", path, body },
      diff,
      ...(mode === "replace" ? { clears: cleared } : {}),
      next: mode === "replace"
        ? "mode=replace clears every weekday not in schedule; to apply, call again with dry_run=false and confirm=true."
        : "Call again with dry_run=false to apply.",
    };
  }
  if (mode === "replace" && args.confirm !== true) {
    throw new ToolError("confirmation_required", "mode=replace clears every weekday not in schedule; pass confirm=true together with dry_run=false", {
      details: { clears: cleared },
      hint: "Preview with dry_run=true to see the days it clears, then call again with dry_run=false and confirm=true.",
    });
  }
  if (diff.length === 0) {
    return { dry_run: false, changed: false, schedule: next, message: "Nothing to change; no request sent." };
  }
  await ctx.owner.put(path, body);
  return { dry_run: false, changed: true, mode, diff, schedule: next };
}

// ---------------------------------------------------------------------------
// Menu helpers

async function readMenu(ctx, businessId) {
  const raw = await ctx.owner.get(businessPath(businessId, "/menu"));
  const categories = Array.isArray(raw?.parsed_categories)
    ? raw.parsed_categories
    : Array.isArray(raw?.categories)
      ? raw.categories
      : [];
  return {
    exists: Number(raw?.id) > 0,
    version: typeof raw?.version === "number" ? raw.version : 0,
    categories,
  };
}

async function withMenuVersionRetry(ctx, businessId, version, write) {
  try {
    return await write(version);
  } catch (err) {
    if (err?.status === 409 && err?.code === "MENU_VERSION_CONFLICT") {
      // Someone saved the menu between our read and write: re-read once.
      const fresh = await readMenu(ctx, businessId);
      return write(fresh.version, fresh);
    }
    throw err;
  }
}

// The backend answers a menu write it would have to trim with HTTP 200 and
// `requires_confirmation: true`, without saving. Surface that as a tool error so
// a client that branches on error.code never mistakes it for a saved write.
function throwIfSanitizationReview(response, hint = "Nothing was saved. Fix the reported values, or call again with confirm_sanitization=true to save without them.") {
  if (response?.requires_confirmation) {
    throw new ToolError("sanitization_review_required", "The backend would drop some fields and did not save", {
      details: response.sanitization,
      hint,
    });
  }
}

function menuWriteResult(response, extra = {}) {
  throwIfSanitizationReview(response);
  const sanitization = response?.sanitization;
  return {
    dry_run: false,
    saved: true,
    menu_version: typeof response?.version === "number" ? response.version : undefined,
    ...stripUndefined(extra),
    ...(sanitization && hasDrops(sanitization) ? { sanitization } : {}),
  };
}

function hasDrops(report) {
  return Number(report?.dropped_allergens ?? 0) + Number(report?.dropped_dietary_tags ?? 0) + Number(report?.dropped_items ?? 0) > 0;
}

function toMenuItemPayload(input) {
  return stripUndefined({
    name: input.name.trim(),
    description: input.description ?? "",
    price: input.price,
    cogs: input.cogs,
    image: input.image,
    images: input.image ? [input.image] : undefined,
    options: (input.options ?? []).map(toOptionPayload),
    allergens: input.allergens ?? [],
    dietary_tags: input.dietary_tags ?? [],
    // The backend's zero value is "unavailable"; new items are on sale unless
    // the caller says otherwise.
    is_available: input.is_available ?? true,
  });
}

function toOptionPayload(option) {
  return stripUndefined({
    id: option.id,
    name: option.name,
    price_change: option.price_change ?? 0,
    is_required: option.is_required ?? false,
  });
}

// GET /menu reports the effective (inventory-aware) availability in
// is_available and the stored flag in manual_available. Writes must send the
// stored flag, or an out-of-stock item would be saved as manually disabled.
function storedMenuItem(item) {
  const { inventory_status: _inventory, manual_available: manual, ...rest } = item ?? {};
  return { ...rest, is_available: typeof manual === "boolean" ? manual : item?.is_available !== false };
}

function projectMenuItem(item) {
  const stored = typeof item?.manual_available === "boolean" ? item.manual_available : item?.is_available !== false;
  return stripUndefined({
    id: item.id,
    name: item.name,
    description: item.description || undefined,
    price: item.price,
    cogs: item.cogs || undefined,
    image: item.image || undefined,
    allergens: item.allergens?.length ? item.allergens : undefined,
    dietary_tags: item.dietary_tags?.length ? item.dietary_tags : undefined,
    options: item.options?.length
      ? item.options.map((option) => ({ id: option.id, name: option.name, price_change: option.price_change, is_required: option.is_required }))
      : undefined,
    is_available: stored,
    sellable_now: item.is_available !== false,
    inventory_status: item.inventory_status || undefined,
    sort_order: item.sort_order,
  });
}

function projectCategory(category) {
  return {
    id: category.id,
    name: category.name,
    description: category.description || undefined,
    item_count: Array.isArray(category.items) ? category.items.length : 0,
  };
}

function findCategoriesByName(categories, name) {
  const key = normalizeName(name);
  return (categories ?? []).filter((category) => normalizeName(category.name) === key);
}

function findItemsByName(items, name) {
  const key = normalizeName(name);
  return (items ?? []).filter((item) => normalizeName(item.name) === key);
}

function resolveCategory(categories, args) {
  if (args.category_id) {
    const match = (categories ?? []).find((category) => category.id === args.category_id);
    if (!match) {
      throw new ToolError("not_found", `No category with id ${args.category_id}`, { hint: "List categories with payverge_get_menu." });
    }
    return match;
  }
  if (!args.category_name) {
    throw new ToolError("invalid_arguments", "Pass category_id or category_name");
  }
  const matches = findCategoriesByName(categories, args.category_name);
  if (matches.length === 0) {
    throw new ToolError("not_found", `No category named "${args.category_name}"`, {
      details: { available: (categories ?? []).map((category) => category.name) },
      hint: "Create it with payverge_add_menu_category, or check the spelling.",
    });
  }
  if (matches.length > 1) {
    throw new ToolError("ambiguous_category", `Several categories are named "${args.category_name}"`, {
      details: matches.map((category) => ({ id: category.id, name: category.name })),
      hint: "Pass category_id instead.",
    });
  }
  return matches[0];
}

function resolveItem(categories, args) {
  if (args.item_id) {
    const scope = args.category_id || args.category_name ? [resolveCategory(categories, args)] : categories ?? [];
    for (const category of scope) {
      const item = (category.items ?? []).find((row) => row.id === args.item_id);
      if (item) return { category, item };
    }
    throw new ToolError("not_found", `No menu item with id ${args.item_id}`, { hint: "List items with payverge_get_menu." });
  }
  if (!args.item_name) throw new ToolError("invalid_arguments", "Pass item_id or item_name");
  const scope = args.category_id || args.category_name ? [resolveCategory(categories, args)] : categories ?? [];
  const matches = [];
  for (const category of scope) {
    for (const item of findItemsByName(category.items, args.item_name)) matches.push({ category, item });
  }
  if (matches.length === 0) {
    throw new ToolError("not_found", `No menu item named "${args.item_name}"`, { hint: "List items with payverge_get_menu." });
  }
  if (matches.length > 1) {
    throw new ToolError("ambiguous_item", `Several items are named "${args.item_name}"`, {
      details: matches.map(({ category, item }) => ({ category_id: category.id, category: category.name, item_id: item.id })),
      hint: "Pass item_id (and category_id) instead.",
    });
  }
  return matches[0];
}

// ---------------------------------------------------------------------------
// Hours helpers

function rowsToSchedule(rows) {
  const schedule = {};
  for (let dayIndex = 0; dayIndex < WEEKDAYS.length; dayIndex += 1) {
    const dayRows = rows.filter((row) => Number(row.day_of_week) === dayIndex);
    if (dayRows.length === 0) {
      schedule[WEEKDAYS[dayIndex]] = { unset: true };
    } else if (dayRows.some((row) => row.is_closed)) {
      schedule[WEEKDAYS[dayIndex]] = { closed: true };
    } else {
      schedule[WEEKDAYS[dayIndex]] = {
        closed: false,
        periods: dayRows
          .map((row) => stripUndefined({ open: row.open_time, close: row.close_time, kitchen_close: row.kitchen_close_time || undefined }))
          .sort((a, b) => String(a.open).localeCompare(String(b.open))),
      };
    }
  }
  return schedule;
}

function normalizeDaySpec(spec) {
  if (spec.closed === true) return { closed: true };
  return {
    closed: false,
    periods: spec.periods
      .map((period) => stripUndefined({ open: period.open, close: period.close, kitchen_close: period.kitchen_close }))
      .sort((a, b) => a.open.localeCompare(b.open)),
  };
}

function scheduleToRows(schedule) {
  const rows = [];
  WEEKDAYS.forEach((day, dayIndex) => {
    const spec = schedule[day];
    if (!spec || spec.unset) return;
    if (spec.closed) {
      // Same placeholder times the dashboard editor writes for a closed day.
      rows.push({ day_of_week: dayIndex, open_time: "09:00", close_time: "17:00", kitchen_close_time: null, is_closed: true });
      return;
    }
    for (const period of spec.periods) {
      rows.push({
        day_of_week: dayIndex,
        open_time: period.open,
        close_time: period.close,
        kitchen_close_time: period.kitchen_close ?? null,
        is_closed: false,
      });
    }
  });
  return rows;
}

// ---------------------------------------------------------------------------
// Projections

function projectBusiness(business) {
  if (!business || typeof business !== "object") return business;
  const out = {};
  for (const field of BUSINESS_PROFILE_FIELDS) {
    if (business[field] !== undefined) out[field] = business[field];
  }
  out.ai_settings = projectAiSettings(business.ai_settings);
  out.payout_wallets = {
    settlement_configured: Boolean(business.settlement_address),
    tipping_configured: Boolean(business.tipping_address),
  };
  return out;
}

function projectAiSettings(settings) {
  const value = settings ?? {};
  return {
    enabled: Boolean(value.ai_enabled),
    business_page_enabled: Boolean(value.business_page_ai_enabled),
    name: value.ai_name ?? "",
    priority: value.ai_priority || "balanced",
    special_instructions: value.special_instructions ?? "",
  };
}

function aiServerState(instance) {
  if (instance?.ok && instance.body?.features) {
    return {
      llm_provider_configured: Boolean(instance.body.features.ai),
      source: "GET /api/v1/instance",
    };
  }
  return { llm_provider_configured: "unknown", source: "GET /api/v1/instance unavailable" };
}

function projectTable(table, publicUrl) {
  const code = table?.table_code;
  const qrPath = table?.qr_url || (code ? `/t/${code}` : undefined);
  return stripUndefined({
    id: table.id,
    name: table.name,
    capacity: table.capacity,
    is_active: table.is_active,
    table_code: code,
    qr_path: qrPath,
    guest_url: publicUrl && qrPath ? `${publicUrl}${qrPath}` : undefined,
  });
}

function projectStaff(member) {
  return stripUndefined({
    id: member.id,
    email: member.email,
    name: member.name,
    role: member.role,
    is_active: member.is_active,
    last_login_at: member.last_login_at || undefined,
  });
}

function projectReservationSettings(settings) {
  if (!settings || typeof settings !== "object") return settings;
  const out = {};
  for (const field of Object.keys(reservationSettingsProperties)) {
    if (settings[field] !== undefined) out[field] = settings[field];
  }
  return out;
}

// ---------------------------------------------------------------------------
// Errors and results

export function explainError(err) {
  if (err instanceof ToolError) {
    return stripUndefined({ code: err.code, message: err.message, details: err.details, hint: err.hint });
  }
  if (err?.name === "OwnerSessionError") {
    return stripUndefined({ code: err.code, message: err.message, status: err.status, hint: err.hint, request_id: err.requestId });
  }
  if (err?.name === "PayvergeAdminClientError") {
    const body = err.body && typeof err.body === "object" ? err.body : {};
    const backendMessage = typeof body.error === "string" ? body.error : typeof body.message === "string" ? body.message : undefined;
    const backendCode = typeof body.code === "string" ? body.code : err.code;
    const base = {
      status: err.status,
      backend_code: backendCode,
      message: backendMessage ?? err.message,
      request: err.method && err.path ? `${err.method} ${err.path}` : undefined,
      request_id: err.requestId,
      control: typeof body.control === "string" ? body.control : undefined,
    };
    const { code, hint } = classifyHttpError(err.status, backendCode, backendMessage);
    return stripUndefined({ code, ...base, message: base.message, hint });
  }
  return { code: "internal_error", message: err?.message ?? String(err) };
}

function classifyHttpError(status, backendCode, backendMessage = "") {
  if (status === undefined) {
    return {
      code: "backend_unreachable",
      hint: "Check PAYVERGE_API_BASE_URL (it ends in /api/v1) and that the backend answers GET /api/v1/health/live.",
    };
  }
  if (status === 400) {
    return { code: "invalid_request", hint: "The backend rejected the input; the message names the field." };
  }
  if (status === 401) {
    return {
      code: "unauthorized",
      hint: "The backend rejected the owner session even after a fresh sign-in. Check that the account still exists and can sign in to the dashboard.",
    };
  }
  if (status === 402) {
    if (backendCode === "ai_budget_exceeded") {
      return {
        code: "ai_budget_exceeded",
        hint: "This business reached its AI spending budget. Raise the budget in the AI settings or wait for the next period.",
      };
    }
    return { code: "payment_required", hint: "The backend refused the request for a budget limit; the message says which." };
  }
  if (status === 403) {
    if (backendCode === "business_suspended" || backendCode === "business_closed") {
      return {
        code: backendCode,
        hint: "A server administrator suspended or closed this business, so it is read-only. Reactivate it from /admin before configuring it.",
      };
    }
    return {
      code: "forbidden",
      hint: "The signed-in account is not the owner (or a manager) of this business, or its role lacks the permission. Use the business owner's account.",
    };
  }
  if (status === 404) {
    return {
      code: "not_found",
      hint: "Check business_id with payverge_list_businesses. If the route itself is missing, the backend is older than this MCP server.",
    };
  }
  if (status === 409) {
    if (backendCode === "MENU_VERSION_CONFLICT") {
      return { code: "conflict", hint: "The menu changed twice while writing. Re-read it with payverge_get_menu and retry." };
    }
    if (backendCode === "STAFF_EMAIL_EXISTS" || backendCode === "INVITE_ALREADY_PENDING") {
      return { code: "conflict", hint: "That person is already on the staff list or has a pending invitation; see payverge_list_staff." };
    }
    return { code: "conflict", hint: "The resource changed or already exists; re-read it and retry." };
  }
  if (status === 429) {
    return { code: "rate_limited", hint: "The backend rate limit was hit. Wait a minute and retry." };
  }
  if (status === 503) {
    if (backendCode === "ai_not_configured") {
      return {
        code: "ai_not_configured",
        hint: "No LLM provider is configured. Set OPENROUTER_API_KEY, or LLM_BASE_URL (+ LLM_API_KEY) for a self-hosted model, then restart the backend. See docs/self-hosting/ai.md.",
      };
    }
    if (backendCode === "RUNTIME_CONTROL_DISABLED") {
      return {
        code: "runtime_control_disabled",
        hint: "A platform launch control (named in `control`, e.g. maintenance_mode, read_only_mode, payments_enabled or ai_enabled) blocks this write. A platform admin changes it through /api/v1/admin/runtime-controls; see docs/runbooks/runtime-launch-controls.md.",
      };
    }
    return { code: "service_unavailable", hint: "The backend is not ready. Check GET /api/v1/health/ready and the backend logs." };
  }
  if (status >= 500) {
    return {
      code: "backend_error",
      hint: "Find the request_id in the backend logs: docker compose logs backend | grep <request_id>.",
    };
  }
  return { code: "http_error", hint: undefined };
}

function toolResult(structuredContent) {
  return {
    content: [{ type: "text", text: JSON.stringify(structuredContent, null, 2) }],
    structuredContent,
  };
}

function errorResult(err) {
  const structuredContent = { error: explainError(err) };
  if (err?.partial) structuredContent.partial = err.partial;
  return {
    isError: true,
    content: [{ type: "text", text: JSON.stringify(structuredContent, null, 2) }],
    structuredContent,
  };
}

// ---------------------------------------------------------------------------
// Minimal JSON Schema validator (the subset the tool schemas use).

function validateArgs(schema, value, path = "arguments") {
  const problems = [];
  check(schema, value, path, problems);
  return problems;
}

function check(schema, value, path, problems) {
  if (!schema) return;
  if (schema.type) {
    const types = Array.isArray(schema.type) ? schema.type : [schema.type];
    if (!types.some((type) => matchesType(type, value))) {
      problems.push(`${path} must be ${types.join(" or ")}`);
      return;
    }
  }
  if (schema.enum && !schema.enum.includes(value)) {
    problems.push(`${path} must be one of: ${schema.enum.join(", ")}`);
  }
  if (typeof value === "number") {
    if (schema.minimum !== undefined && value < schema.minimum) problems.push(`${path} must be >= ${schema.minimum}`);
    if (schema.maximum !== undefined && value > schema.maximum) problems.push(`${path} must be <= ${schema.maximum}`);
    if (schema.exclusiveMinimum !== undefined && value <= schema.exclusiveMinimum) problems.push(`${path} must be > ${schema.exclusiveMinimum}`);
    if (schema.exclusiveMaximum !== undefined && value >= schema.exclusiveMaximum) problems.push(`${path} must be < ${schema.exclusiveMaximum}`);
  }
  if (typeof value === "string") {
    const length = [...value].length;
    if (schema.minLength !== undefined && value.trim().length < schema.minLength) {
      problems.push(`${path} must not be empty`);
    }
    if (schema.maxLength !== undefined && length > schema.maxLength) problems.push(`${path} must be at most ${schema.maxLength} characters`);
    if (schema.pattern && !new RegExp(schema.pattern).test(value)) problems.push(`${path} has an invalid format (expected ${schema.pattern})`);
  }
  if (Array.isArray(value)) {
    if (schema.minItems !== undefined && value.length < schema.minItems) problems.push(`${path} needs at least ${schema.minItems} entries`);
    if (schema.maxItems !== undefined && value.length > schema.maxItems) problems.push(`${path} allows at most ${schema.maxItems} entries`);
    if (schema.items) value.forEach((entry, index) => check(schema.items, entry, `${path}[${index}]`, problems));
  }
  if (isPlainObject(value)) {
    for (const key of schema.required ?? []) {
      if (value[key] === undefined) problems.push(`${path}.${key} is required`);
    }
    const properties = schema.properties ?? {};
    const keys = Object.keys(value).filter((key) => value[key] !== undefined);
    if (schema.minProperties !== undefined && keys.length < schema.minProperties) {
      problems.push(`${path} needs at least ${schema.minProperties} field(s)`);
    }
    for (const key of keys) {
      if (properties[key]) {
        check(properties[key], value[key], `${path}.${key}`, problems);
      } else if (schema.additionalProperties === false) {
        problems.push(`${path}.${key} is not a known field`);
      }
    }
  }
}

function matchesType(type, value) {
  switch (type) {
    case "integer":
      return Number.isInteger(value);
    case "number":
      return typeof value === "number" && Number.isFinite(value);
    case "string":
      return typeof value === "string";
    case "boolean":
      return typeof value === "boolean";
    case "array":
      return Array.isArray(value);
    case "object":
      return isPlainObject(value);
    case "null":
      return value === null;
    default:
      return true;
  }
}

// ---------------------------------------------------------------------------
// Small utilities

function assertBusinessAllowed(options, businessId) {
  const allow = options.businessAllowList;
  if (allow && !allow.includes(Number(businessId))) {
    throw new ToolError("business_not_allowed", `Business ${businessId} is not in PAYVERGE_MCP_BUSINESS_IDS`, {
      hint: `This MCP server is limited to businesses ${allow.join(", ")}.`,
    });
  }
}

function businessPath(businessId, suffix = "") {
  return `/inside/businesses/${Number(businessId)}${suffix}`;
}

function diffFields(current, changes) {
  return Object.entries(changes).map(([field, to]) => ({
    field,
    from: current?.[field],
    to,
    changed: !sameValue(current?.[field], to),
  }));
}

function emptyAddress() {
  return { street: "", city: "", state: "", postal_code: "", country: "" };
}

function sameValue(a, b) {
  return canonicalJson(a ?? null) === canonicalJson(b ?? null);
}

function canonicalJson(value) {
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`;
  if (isPlainObject(value)) {
    return `{${Object.keys(value)
      .filter((key) => value[key] !== undefined)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${canonicalJson(value[key])}`)
      .join(",")}}`;
  }
  return JSON.stringify(value);
}

function sha256(text) {
  return createHash("sha256").update(text).digest("hex");
}

async function probe(fn) {
  try {
    return { ok: true, status: 200, body: await fn() };
  } catch (err) {
    return {
      ok: false,
      status: err?.status,
      body: err?.body && typeof err.body === "object" ? err.body : undefined,
      error: err?.message ?? String(err),
    };
  }
}

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function isPlainObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function normalizeName(value) {
  return String(value ?? "").trim().toLowerCase().replace(/\s+/g, " ");
}

function stripUndefined(object) {
  return Object.fromEntries(Object.entries(object).filter(([, value]) => value !== undefined));
}

function trimSlash(value) {
  return String(value).replace(/\/+$/, "");
}

function truncate(text, max) {
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}
