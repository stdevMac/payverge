import { axiosInstance } from "@/api/tools/instance";

/** Ops-assistant support handoffs (admin /admin/escalations). */
export interface Escalation {
  id: number;
  source: "ops" | string;
  business_id?: number | null;
  session_ref: string;
  issue: string;
  transcript_summary: string;
  contact_email: string;
  status: string;
  admin_notes: string;
  created_at: string;
  updated_at?: string;
}

export interface EscalationsPage {
  escalations: Escalation[];
  total: number;
}

/** The ops-assistant thread an escalation came from. */
export interface OpsTranscriptTarget {
  businessId: number;
  threadId: number;
}

export async function getAdminEscalations(
  page = 1,
  limit = 50,
): Promise<EscalationsPage> {
  const response = await axiosInstance.get<EscalationsPage>(
    "/admin/escalations",
    { params: { page, limit }, _useCache: false },
  );
  return {
    escalations: response.data.escalations || [],
    total: response.data.total || 0,
  };
}

export async function patchEscalation(
  id: number,
  body: { status?: string; admin_notes?: string },
): Promise<Escalation> {
  const response = await axiosInstance.patch<{ escalation: Escalation }>(
    `/admin/escalations/${id}`,
    body,
  );
  return response.data.escalation;
}

export interface OpsMessage {
  id: number;
  thread_id: number;
  business_id: number;
  role: string;
  content: string;
  created_at: string;
}

export async function getOpsTranscript(
  businessId: number,
  threadId: number,
): Promise<{ messages: OpsMessage[]; business_id: number; thread_id: number }> {
  const response = await axiosInstance.get<{
    messages: OpsMessage[];
    business_id: number;
    thread_id: number;
  }>(`/admin/ops-threads/${businessId}/${threadId}/messages`, {
    _useCache: false,
  });
  return {
    messages: response.data.messages || [],
    business_id: response.data.business_id,
    thread_id: response.data.thread_id,
  };
}

/**
 * Ops escalations reference their thread as `thread:<id>`. Anything else
 * (for example a row left by the removed marketing concierge) has no
 * transcript to show.
 */
export function parseEscalationTranscriptTarget(
  sessionRef: string,
  businessId?: number | null,
): OpsTranscriptTarget | null {
  const ref = sessionRef?.trim() || "";
  if (!businessId || !ref.startsWith("thread:")) return null;
  const threadId = Number.parseInt(ref.slice("thread:".length), 10);
  return Number.isFinite(threadId) ? { businessId, threadId } : null;
}

export const ESCALATION_STATUSES = ["open", "in_progress", "resolved", "closed"] as const;
