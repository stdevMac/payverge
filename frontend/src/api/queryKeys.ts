type QueryKeyValue =
  | string
  | number
  | boolean
  | null
  | QueryKeyValue[]
  | { [key: string]: QueryKeyValue | undefined };

const normalizeQueryKeyValue = (value: unknown): QueryKeyValue | undefined => {
  if (value === undefined) return undefined;
  if (
    value === null ||
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  ) {
    return value;
  }
  if (Array.isArray(value)) {
    return value
      .map((entry) => normalizeQueryKeyValue(entry))
      .filter((entry): entry is QueryKeyValue => entry !== undefined);
  }
  if (typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .map(([key, entry]) => [key, normalizeQueryKeyValue(entry)] as const)
        .filter(([, entry]) => entry !== undefined)
        .sort(([a], [b]) => a.localeCompare(b)),
    );
  }
  return String(value);
};

const stableQueryFilters = (filters?: Record<string, unknown>): string => {
  const normalized = normalizeQueryKeyValue(filters ?? {});
  return JSON.stringify(normalized);
};

export const queryKeys = {
  auth: {
    all: ["auth"] as const,
    profile: (userId: string) => ["auth", "profile", userId] as const,
    staffProfile: () => ["auth", "staff-profile"] as const,
    session: () => ["auth", "session"] as const,
  },
  business: {
    all: ["business"] as const,
    detail: (businessId: string) => ["business", businessId] as const,
    dashboard: (businessId: string) =>
      ["business", businessId, "dashboard"] as const,
    menu: (businessId: string) => ["business", businessId, "menu"] as const,
    analytics: (businessId: string) =>
      ["business", businessId, "analytics"] as const,
    access: (businessId: string) => ["business", businessId, "access"] as const,
    pageData: (businessId: string) =>
      ["business", businessId, "pageData"] as const,
    openStatus: (businessId: string) =>
      ["business", businessId, "openStatus"] as const,
    googleRating: (businessId: string) =>
      ["business", businessId, "googleRating"] as const,
    deliverySettings: (businessId: string) =>
      ["business", businessId, "deliverySettings"] as const,
    reservationSettings: (businessId: string) =>
      ["business", businessId, "reservationSettings"] as const,
  },
  inventory: {
    all: ["inventory"] as const,
    summary: (businessId: string) =>
      ["inventory", businessId, "summary"] as const,
    items: (businessId: string) => ["inventory", businessId, "items"] as const,
    recipes: (businessId: string) =>
      ["inventory", businessId, "recipes"] as const,
    movements: (businessId: string) =>
      ["inventory", businessId, "movements"] as const,
    settings: (businessId: string) =>
      ["inventory", businessId, "settings"] as const,
  },
  orders: {
    all: ["orders"] as const,
    list: (businessId: string, filters?: Record<string, unknown>) =>
      ["orders", businessId, stableQueryFilters(filters)] as const,
  },
  payments: {
    all: ["payments"] as const,
    list: (businessId: string) => ["payments", businessId] as const,
    alternativeGuest: (billNumber: string) =>
      ["payments", "alternative", "guest", billNumber] as const,
    alternativeInside: (billId: string) =>
      ["payments", "alternative", "inside", billId] as const,
  },
  notifications: {
    dashboard: (businessId: string) =>
      ["notifications", businessId, "dashboard"] as const,
  },
  alerts: {
    all: ["alerts"] as const,
    active: (businessId: string, filters?: Record<string, unknown>) =>
      ["alerts", businessId, "active", stableQueryFilters(filters)] as const,
    events: (businessId: string, alertId: string | number) =>
      ["alerts", businessId, "events", String(alertId)] as const,
    settings: (businessId: string) =>
      ["alerts", businessId, "settings"] as const,
    recent: (businessId: string) => ["alerts", businessId, "recent"] as const,
  },
  reservations: {
    all: ["reservations"] as const,
    list: (businessId: string, filters?: Record<string, unknown>) =>
      ["reservations", businessId, stableQueryFilters(filters)] as const,
    detail: (businessId: string, reservationId: string | number) =>
      ["reservations", businessId, String(reservationId)] as const,
    settings: (businessId: string) =>
      ["reservations", businessId, "settings"] as const,
  },
  delivery: {
    all: ["delivery"] as const,
    list: (businessId: string, filters?: Record<string, unknown>) =>
      ["delivery", businessId, stableQueryFilters(filters)] as const,
    settings: (businessId: string) =>
      ["delivery", businessId, "settings"] as const,
    drivers: (businessId: string) =>
      ["delivery", businessId, "drivers"] as const,
    zones: (businessId: string) => ["delivery", businessId, "zones"] as const,
  },
  accounting: {
    all: ["accounting"] as const,
    summary: (businessId: string, start: string, end: string) =>
      ["accounting", businessId, "summary", start, end] as const,
    entries: (businessId: string, filters?: Record<string, unknown>) =>
      [
        "accounting",
        businessId,
        "entries",
        stableQueryFilters(filters),
      ] as const,
  },
  cashRegister: {
    all: (businessId: string) => ["cash-register", businessId] as const,
    current: (businessId: string) =>
      ["cash-register", businessId, "current"] as const,
    sessions: (
      businessId: string,
      params?: { limit?: number; offset?: number },
    ) =>
      [
        "cash-register",
        businessId,
        "sessions",
        stableQueryFilters(params),
      ] as const,
    session: (businessId: string, sessionId: number) =>
      ["cash-register", businessId, "sessions", sessionId] as const,
    unassigned: (businessId: string) =>
      ["cash-register", businessId, "unassigned"] as const,
    unassignedList: (
      businessId: string,
      params?: { limit?: number; offset?: number },
    ) =>
      [
        "cash-register",
        businessId,
        "unassigned",
        "list",
        stableQueryFilters(params),
      ] as const,
  },
  fiscal: {
    all: ["fiscal"] as const,
    settings: (businessId: string) =>
      ["fiscal", businessId, "settings"] as const,
    receipts: (businessId: string, filters?: Record<string, unknown>) =>
      ["fiscal", businessId, "receipts", stableQueryFilters(filters)] as const,
  },
  staff: {
    all: ["staff"] as const,
    list: (businessId: string) => ["staff", businessId, "list"] as const,
    invitations: (businessId: string) =>
      ["staff", businessId, "invitations"] as const,
    permissions: (businessId: string, staffId: string | number) =>
      ["staff", businessId, "permissions", String(staffId)] as const,
  },
  schedule: {
    all: ["schedule"] as const,
    week: (businessId: string, week: string) =>
      ["schedule", businessId, "week", week] as const,
    laborPreview: (
      businessId: string,
      scheduleId: number,
      salesTarget?: number,
    ) =>
      [
        "schedule",
        businessId,
        "laborPreview",
        scheduleId,
        salesTarget ?? null,
      ] as const,
  },
  availability: {
    all: ["availability"] as const,
    // Scoped to the staff principal: a shared device must never serve one
    // staff member's cached "mine" rows to the next.
    mine: (businessId: string, staffId: string | number) =>
      ["availability", businessId, "mine", String(staffId)] as const,
    team: (businessId: string) => ["availability", businessId, "team"] as const,
  },
  timeOff: {
    all: ["timeOff"] as const,
    list: (businessId: string, status?: string) =>
      ["timeOff", businessId, "list", status ?? "all"] as const,
  },
  timesheet: {
    all: ["timesheet"] as const,
    // Scoped to the staff principal: a shared device must never serve one
    // staff member's cached "mine" rows to the next.
    mine: (businessId: string, staffId: string | number) =>
      ["timesheet", businessId, "mine", String(staffId)] as const,
    review: (businessId: string, status?: string) =>
      ["timesheet", businessId, "review", status ?? "pending_review"] as const,
  },
  liveFloor: {
    all: ["liveFloor"] as const,
    day: (businessId: string, date?: string) =>
      ["liveFloor", businessId, date ?? "today"] as const,
  },
  kiosk: {
    all: ["kiosk"] as const,
    roster: (businessId: string) => ["kiosk", businessId, "roster"] as const,
  },
  coverage: {
    all: ["coverage"] as const,
    open: (businessId: string) => ["coverage", businessId, "open"] as const,
    // Scoped to the staff principal: a shared device must never serve one
    // staff member's cached "mine" rows to the next.
    mine: (businessId: string, staffId: string | number) =>
      ["coverage", businessId, "mine", String(staffId)] as const,
    history: (businessId: string) =>
      ["coverage", businessId, "history"] as const,
  },
  logbook: {
    all: ["logbook"] as const,
    list: (businessId: string, date: string) =>
      ["logbook", businessId, date] as const,
  },
  engagement: {
    all: ["engagement"] as const,
    // Staff-nav badge counts (unacked announcements + pending checklists) — polled
    // so the More menu / bottom nav surface buried work (Phase 5 · Slice 5a).
    badges: (businessId: string) =>
      ["engagement", businessId, "badges"] as const,
    checklistRuns: (businessId: string) =>
      ["engagement", businessId, "checklists", "runs"] as const,
    // Operator business-wide run-status list (?scope=business) — distinct from the
    // staff "my runs" key so the manager view and the staff view don't collide.
    checklistBusinessRuns: (businessId: string) =>
      ["engagement", businessId, "checklists", "runs", "business"] as const,
    checklistRunDetail: (businessId: string, runId: number) =>
      ["engagement", businessId, "checklists", "runs", runId] as const,
    checklistTemplates: (businessId: string) =>
      ["engagement", businessId, "checklists", "templates"] as const,
    documents: (businessId: string) =>
      ["engagement", businessId, "documents"] as const,
    shoutouts: (businessId: string) =>
      ["engagement", businessId, "shoutouts"] as const,
    polls: (businessId: string) => ["engagement", businessId, "polls"] as const,
    pollResults: (businessId: string, pollId: number) =>
      ["engagement", businessId, "polls", pollId, "results"] as const,
  },
  chat: {
    all: ["chat"] as const,
    channels: (businessId: string) => ["chat", businessId, "channels"] as const,
    messages: (businessId: string, channelId: number) =>
      ["chat", businessId, "messages", channelId] as const,
    announcements: (businessId: string) =>
      ["chat", businessId, "announcements"] as const,
    acks: (businessId: string, announcementId: number) =>
      ["chat", businessId, "acks", announcementId] as const,
    // Batch ack-summary for the VISIBLE require_ack announcements — one bounded
    // call per feed refresh, replacing the per-announcement AckRoster poll.
    ackSummaries: (businessId: string, ids: number[]) =>
      ["chat", businessId, "ackSummaries", ids.join(",")] as const,
  },
  crm: {
    all: ["crm"] as const,
    customers: (businessId: string, filters?: Record<string, unknown>) =>
      ["crm", businessId, "customers", stableQueryFilters(filters)] as const,
    customer: (businessId: string, customerId: string | number) =>
      ["crm", businessId, "customers", String(customerId)] as const,
  },
  translations: {
    category: (businessId: string, lang: string) =>
      ["translations", businessId, "category", lang] as const,
    item: (businessId: string, lang: string) =>
      ["translations", businessId, "item", lang] as const,
  },
} as const;
