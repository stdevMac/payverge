import { getPublicConfig } from "@/config/publicConfig";
import { axiosInstance } from "./tools/instance";

type DirectorPriority = "high" | "medium" | "low";
export type DirectorFeedbackVote = "up" | "down";

export interface DirectorAction {
  title: string;
  description: string;
  deep_link: string;
  priority: DirectorPriority;
}

export interface DirectorResponse {
  summary: string;
  diagnosis: string;
  evidence: string[];
  actions: DirectorAction[];
  expected_impact: string;
  follow_ups: string[];
}

export interface DirectorThread {
  id: number;
  business_id?: number;
  title: string;
  locale?: string;
  created_at: string;
  updated_at: string;
  last_message_at?: string;
  /** Pinned threads sort to the top of the sidebar. Serialized by the DTO so
   *  the actions menu can render Pin/Unpin correctly (was previously missing). */
  pinned?: boolean;
  /** Set only on archived threads (the Archived section). Absent on active. */
  archived_at?: string | null;
}

// ---------------------------------------------------------------------------
// Proposed actions (Director Console v2 write path, Phase 5)
//
// Server-truth DTOs assembled from the persisted proposal row — the client
// never sends params back; apply/undo take only the proposal public id.
// ---------------------------------------------------------------------------

type DirectorProposalKind =
  | "menu.adjust_prices"
  | "menu.set_availability"
  | "menu.edit_content";

interface DirectorProposalPreviewExample {
  name: string;
  before: unknown;
  after: unknown;
}

interface DirectorProposalPreview {
  affected_count: number;
  examples: DirectorProposalPreviewExample[] | null;
  summary: string;
}

export interface DirectorProposedAction {
  id: string;
  kind: DirectorProposalKind;
  title: string;
  description: string;
  preview: DirectorProposalPreview;
  warnings: string[] | null;
  menu_version: number;
  requires_reconfirm: boolean;
  expires_at: string;
}

export interface DirectorActionApplyResult {
  new_menu_version: number;
  proposal_id: string;
  audit_id: number;
  kind: string;
}

export interface DirectorThreadMessage {
  id: number;
  thread_id: number;
  business_id: number;
  role: "user" | "assistant" | "system";
  locale: string;
  content: string;
  structured_response?: DirectorResponse;
  model_name?: string;
  latency_ms?: number;
  feedback_vote?: DirectorFeedbackVote;
  feedback_at?: string;
  created_at: string;
}

/**
 * Build the absolute URL for the Director Console SSE stream endpoint.
 *
 * `useDirectorStream` fetches this URL directly (not via the shared
 * `axiosInstance`), so we have to prepend the API base ourselves — otherwise
 * the request would hit the Next.js origin instead of the backend. The base
 * URL already includes `/api/v1` in every environment (see `.env.example`).
 */
export function askDirectorStreamURL(businessId: string | number): string {
  const base = getPublicConfig().apiUrl;
  return `${base}/inside/businesses/${businessId}/ai/director/ask/stream`;
}

export const listDirectorThreads = async (
  businessId: string | number,
  opts?: { archived?: boolean },
): Promise<{ threads: DirectorThread[] }> => {
  // `?archived=1` returns the archived (soft-deleted) threads for the sidebar's
  // Archived section; the default (no param) returns the active list — legacy
  // shape, BE-first.
  const suffix = opts?.archived ? "?archived=1" : "";
  const response = await axiosInstance.get<{ threads: DirectorThread[] }>(
    `/inside/businesses/${businessId}/ai/director/threads${suffix}`,
  );
  return response.data;
};

export const getDirectorThreadMessages = async (
  businessId: string | number,
  threadId: number,
  limit = 200,
): Promise<{ messages: DirectorThreadMessage[] }> => {
  const params = new URLSearchParams({ limit: String(limit) });
  const response = await axiosInstance.get<{ messages: DirectorThreadMessage[] }>(
    `/inside/businesses/${businessId}/ai/director/threads/${threadId}/messages?${params.toString()}`,
  );
  return response.data;
};

export const patchDirectorThread = async (
  businessId: string | number,
  threadId: number,
  payload: { title: string },
): Promise<void> => {
  await axiosInstance.patch(
    `/inside/businesses/${businessId}/ai/director/threads/${threadId}`,
    payload,
  );
};

/**
 * Soft-archive a thread (reversible). This is the default "Archive" action —
 * it hides the thread from the active sidebar but keeps its transcript, so a
 * mistaken archive is one click of Restore away. The permanent, irreversible
 * erase is {@link deleteDirectorThread}, gated behind a confirmation.
 */
export const archiveDirectorThread = async (
  businessId: string | number,
  threadId: number,
): Promise<void> => {
  await axiosInstance.patch(
    `/inside/businesses/${businessId}/ai/director/threads/${threadId}/archive`,
  );
};

/** Restore a previously archived thread back into the active list. */
export const restoreDirectorThread = async (
  businessId: string | number,
  threadId: number,
): Promise<void> => {
  await axiosInstance.post(
    `/inside/businesses/${businessId}/ai/director/threads/${threadId}/restore`,
  );
};

/**
 * Permanently and irreversibly erase a thread and all its child rows. This is
 * NOT the Archive action — the menu wires Archive to {@link archiveDirectorThread}.
 * Callers MUST gate this behind an explicit confirmation.
 */
export const deleteDirectorThread = async (
  businessId: string | number,
  threadId: number,
): Promise<void> => {
  await axiosInstance.delete(
    `/inside/businesses/${businessId}/ai/director/threads/${threadId}`,
  );
};

export const pinDirectorThread = async (
  businessId: string | number,
  threadId: number,
  pinned: boolean,
): Promise<void> => {
  await axiosInstance.post(
    `/inside/businesses/${businessId}/ai/director/threads/${threadId}/${pinned ? "pin" : "unpin"}`,
  );
};

/**
 * Build the absolute URL for the Director Console thread export endpoint.
 *
 * Like {@link askDirectorStreamURL}, this is fetched directly (via
 * `window.open`) rather than through `axiosInstance`, so we must prepend the
 * API base ourselves.
 */
export function exportDirectorThreadURL(
  businessId: string | number,
  threadId: number,
): string {
  const base = getPublicConfig().apiUrl;
  return `${base}/inside/businesses/${businessId}/ai/director/threads/${threadId}/export?format=md`;
}

export const submitDirectorFeedback = async (
  businessId: string | number,
  messageId: number,
  vote: DirectorFeedbackVote,
): Promise<{ message: DirectorThreadMessage }> => {
  const response = await axiosInstance.post<{ message: DirectorThreadMessage }>(
    `/inside/businesses/${businessId}/ai/director/messages/${messageId}/feedback`,
    { vote },
  );
  return response.data;
};

/**
 * Commit a previously-previewed director proposal. RBAC: director:write AND
 * menu:write. Error statuses the card reacts to: 409 (`menu_changed`),
 * 428 (`reconfirm_required`), 410 (expired/already handled), 404.
 */
export const applyDirectorAction = async (
  businessId: string | number,
  proposalId: string,
  reconfirm = false,
): Promise<{ applied: boolean; result: DirectorActionApplyResult }> => {
  const response = await axiosInstance.post<{
    applied: boolean;
    result: DirectorActionApplyResult;
  }>(`/inside/businesses/${businessId}/ai/director/actions/apply`, {
    proposal_id: proposalId,
    reconfirm,
  });
  return response.data;
};

/**
 * Reverse an applied action within the undo window. Error statuses: 409
 * (`menu_changed_since_apply` | `already_undone`), 410 (window elapsed), 404.
 */
export const undoDirectorAction = async (
  businessId: string | number,
  proposalId: string,
): Promise<{ undone: boolean; result: DirectorActionApplyResult }> => {
  const response = await axiosInstance.post<{
    undone: boolean;
    result: DirectorActionApplyResult;
  }>(`/inside/businesses/${businessId}/ai/director/actions/undo`, {
    proposal_id: proposalId,
  });
  return response.data;
};

/**
 * One row of the "Applied changes" history — a director proposal that was
 * committed to the menu. `can_undo` is the server's truth about whether the
 * 24h undo window is still open AND the menu hasn't advanced past the apply;
 * the client renders an Undo button only when it's true. Survives refresh
 * (backed by the director_action_audits table, not React state) — this is the
 * fix for the stranded-undo defect.
 */
export interface DirectorAppliedAction {
  proposal_id: string;
  kind: string;
  title: string;
  applied_at: string;
  undone_at: string | null;
  can_undo: boolean;
}

export interface DirectorAppliedActionsResponse {
  actions: DirectorAppliedAction[];
}

/**
 * List the most recent applied director actions for a business (newest first,
 * bounded server-side). Powers the "Applied changes" drawer so undo/audit
 * survive a refresh instead of living only in ephemeral React state.
 */
export const listDirectorAppliedActions = async (
  businessId: string | number,
): Promise<DirectorAppliedActionsResponse> => {
  const response = await axiosInstance.get<DirectorAppliedActionsResponse>(
    `/inside/businesses/${businessId}/ai/director/actions/applied`,
  );
  return response.data;
};

export interface DirectorActionErrorInfo {
  status?: number;
  code?: string;
}

/**
 * Pull the HTTP status and the backend's machine-readable error code out of an
 * axios-shaped rejection so the proposal card can branch on 409/428/410
 * without importing axios types.
 */
export function directorActionErrorInfo(err: unknown): DirectorActionErrorInfo {
  const response = (
    err as { response?: { status?: number; data?: { code?: string } } }
  )?.response;
  return { status: response?.status, code: response?.data?.code };
}

interface ProactiveInsightCTA {
  tab: string;
}

export interface ProactiveInsightDTO {
  id: string;
  type: string;
  params: Record<string, unknown>;
  cta: ProactiveInsightCTA;
}

export interface ProactiveInsightsResponse {
  insights: ProactiveInsightDTO[];
}

export async function getDirectorProactiveInsights(
  businessId: number,
): Promise<ProactiveInsightsResponse> {
  const { data } = await axiosInstance.get<ProactiveInsightsResponse>(
    `/inside/businesses/${businessId}/director-console/proactive-insights`,
  );
  return data;
}

// ---------------------------------------------------------------------------
// Sage Briefing front door (Director Console "always-on" GM brief)
//
// The briefing endpoint assembles, in one owner-gated round-trip, the live read
// of the restaurant (`pulse`), the exception insights (`insights`, the existing
// proactive-insights shape, ≤3), the single best forward-looking move (`play`,
// 0 or 1), and a week-over-week win (`win`, only when there's no play). The DTO
// emits numbers + identifiers only — the GM voice and all es / es-AR copy live
// in the frontend i18n templates.
//
// Money fields are float64 dollars (the repo wire contract); the frontend
// formats them with `formatCurrency` + `business.default_currency`. Nullable Go
// pointer fields (`*float64`) map to `number | null` here — honor the null on
// the render path (e.g. the pace clause renders ONLY when `pace_pct != null`).
// ---------------------------------------------------------------------------

export interface BriefingPulse {
  revenue: number;
  /** projected full-day revenue, dollars; null when there is no baseline */
  projected: number | null;
  /** trailing same-weekday average, dollars; null when there is no history */
  typical_day: number | null;
  /** projected vs typical, percent (e.g. 12.0); null when no baseline */
  pace_pct: number | null;
  orders: number;
  avg_ticket: number;
  /** 0..1 fraction; null when there is no recipe data */
  food_cost_pct: number | null;
  /** 0..1 fraction; null when there is no payroll data */
  labor_cost_pct: number | null;
  open_bills: number;
  /** Remaining due on live open/partial checks — not today's collected sales. */
  remaining?: number;
}

type BriefingPlayKind = "reprice_up" | "promote";

export interface BriefingPlay {
  kind: BriefingPlayKind;
  /** destination dashboard tab (always "menu" for v1) */
  tab: string;
  item_name: string;
  /** dollars; reprice_up only (null for promote) */
  current_price: number | null;
  /** dollars; reprice_up only (null for promote) */
  suggested_price: number | null;
  /** estimated dollars/month */
  monthly_impact: number;
}

interface BriefingWin {
  kind: "revenue_up_wow";
  /** already a percent number (e.g. 9.0) */
  pct: number;
}

/**
 * S3-Loop closed-loop panel: mark_posted activity this period + CTA back to
 * Marketing Library. Absent/null when count is zero. Never schedule fields.
 */
interface BriefingMarketing {
  posts_this_period: number;
  period_days: number;
  recent_titles: string[];
  channels: string[];
  /** dashboard tab CTA — always "marketing" */
  tab: string;
}

export interface BriefingResponse {
  state: "active" | "learning";
  pulse: BriefingPulse;
  // Nullable on the wire: a nil insight slice serializes to JSON `null`.
  insights: ProactiveInsightDTO[] | null;
  play: BriefingPlay | null;
  win: BriefingWin | null;
  /** S3-Loop: present only when posts_this_period >= 1 */
  marketing?: BriefingMarketing | null;
}

export async function getDirectorBriefing(
  businessId: number,
): Promise<BriefingResponse> {
  const { data } = await axiosInstance.get<BriefingResponse>(
    `/inside/businesses/${businessId}/director-console/briefing`,
  );
  return data;
}
