import { axiosInstance } from "@/api/tools/instance";

// Engagement (Slice 9) — the single domain client for the staff-facing
// engagement surfaces: checklists, documents, recognition (shout-outs) and
// polls. One file, four cohesive sub-objects so call sites read like separate
// clients without a 4-file sprawl. Every shape mirrors the SHIPPED backend
// (envelope { success, data }; author/from ids are taken from the staff session
// server-side, so they are never sent on create). NO money on any of these
// wires — engagement carries operational context, never dollars.

const base = (businessId: string) => "/inside/businesses/" + businessId;

// ---- Checklists ----
//
// NOTE on the staff read path: `GET /checklists/runs` returns the caller's
// assigned runs ONLY (run rows — id/status/dates). `GET /checklists/runs/:runId`
// (checklist:complete) returns the run-detail — the per-item completion state —
// so the runs overview drills into a per-item ticker. `tickItem` flips one item;
// the server recomputes the run status from its items.

export type ChecklistRunStatus = "pending" | "in_progress" | "complete";
export type ChecklistKind = "onboarding" | "opening" | "closing" | "custom";

export interface ChecklistRun {
  id: number;
  business_id: number;
  template_id: number;
  assigned_staff_id: number | null;
  shift_id: number | null;
  for_date: string;
  status: ChecklistRunStatus;
  completed_at: string | null;
}

// The operator run-status view (?scope=business): a run enriched with the
// template + assignee names so the manager list reads at a glance. Mirrors the
// backend ChecklistRunView (a ChecklistRun plus the two joined names).
export interface ChecklistRunView extends ChecklistRun {
  template_name: string;
  assigned_staff_name: string;
}

export interface ChecklistTemplate {
  id: number;
  business_id: number;
  name: string;
  kind: ChecklistKind;
  position_id: number | null;
  is_active: boolean;
}

export interface ChecklistTemplateItemInput {
  label: string;
  sort_order: number;
  is_required: boolean;
}

// One item on a run with its per-run completion state, in sort order (mirrors the
// backend ChecklistRunItem). `completed_at` is null until the item is ticked done.
interface ChecklistRunItem {
  item_id: number;
  label: string;
  is_required: boolean;
  sort_order: number;
  done: boolean;
  note: string;
  completed_at: string | null;
}

export interface ChecklistRunDetail {
  run: ChecklistRun;
  items: ChecklistRunItem[];
}

export const checklistsApi = {
  listTemplates: async (businessId: string): Promise<ChecklistTemplate[]> => {
    const res = await axiosInstance.get(base(businessId) + "/checklists/templates");
    return res.data.data;
  },
  createTemplate: async (
    businessId: string,
    input: { name: string; kind: ChecklistKind; position_id?: number; items: ChecklistTemplateItemInput[] },
  ): Promise<ChecklistTemplate> => {
    const res = await axiosInstance.post(base(businessId) + "/checklists/templates", input);
    return res.data.data;
  },
  createRun: async (
    businessId: string,
    input: { template_id: number; assigned_staff_id?: number; shift_id?: number; for_date?: string },
  ): Promise<ChecklistRun> => {
    const res = await axiosInstance.post(base(businessId) + "/checklists/runs", input);
    return res.data.data;
  },
  listRuns: async (businessId: string): Promise<ChecklistRun[]> => {
    const res = await axiosInstance.get(base(businessId) + "/checklists/runs");
    return res.data.data;
  },
  // Operator run-status list (manager/owner): every run in the business with the
  // template + assignee names joined. A non-manager caller is quietly served
  // their own runs server-side (no escalation).
  listBusinessRuns: async (businessId: string): Promise<ChecklistRunView[]> => {
    const res = await axiosInstance.get(base(businessId) + "/checklists/runs", {
      params: { scope: "business" },
    });
    return res.data.data;
  },
  // The run-detail: the run + its items with per-run completion state. Non-managers
  // may only read a run assigned to them (own-run scope enforced server-side).
  getRunDetail: async (businessId: string, runId: number): Promise<ChecklistRunDetail> => {
    const res = await axiosInstance.get(base(businessId) + "/checklists/runs/" + runId);
    return res.data.data;
  },
  tickItem: async (
    businessId: string,
    runId: number,
    itemId: number,
    done: boolean,
    note = "",
  ): Promise<ChecklistRun> => {
    const res = await axiosInstance.post(
      base(businessId) + "/checklists/runs/" + runId + "/items/" + itemId,
      { done, note },
    );
    return res.data.data;
  },
};

// ---- Documents (versioned policies/handbooks + ack) ----

export interface DocumentRow {
  id: number;
  business_id: number;
  title: string;
  url: string;
  content: string;
  version: number;
  require_ack: boolean;
  audience_filter: string;
  created_at: string;
}

/**
 * The documents list plus the caller's per-document ack state. `acked[id]` is true
 * only for documents this caller has acknowledged at the CURRENT version; a missing
 * id means not-yet-acked (a new version re-opens ack, so an old ack stops counting).
 * Lets the surface hide the Acknowledge button across reloads. Mirrors AnnouncementFeed.
 */
export interface DocumentFeed {
  documents: DocumentRow[];
  acked: Record<string, boolean>;
}

export const documentsApi = {
  list: async (businessId: string): Promise<DocumentFeed> => {
    const res = await axiosInstance.get(base(businessId) + "/documents");
    return { documents: res.data.data, acked: res.data.acked ?? {} };
  },
  create: async (
    businessId: string,
    input: { title: string; content?: string; url?: string; require_ack: boolean; audience_filter?: string },
  ): Promise<DocumentRow> => {
    const res = await axiosInstance.post(base(businessId) + "/documents", input);
    return res.data.data;
  },
  // Edit a document and publish a NEW version (the backend bumps `version`, which
  // re-opens acknowledgement for everyone). doc:manage — manager/owner.
  update: async (
    businessId: string,
    docId: number,
    input: { title: string; content?: string; url?: string; require_ack: boolean; audience_filter?: string },
  ): Promise<DocumentRow> => {
    const res = await axiosInstance.put(base(businessId) + "/documents/" + docId, input);
    return res.data.data;
  },
  ack: async (businessId: string, docId: number): Promise<void> => {
    await axiosInstance.post(base(businessId) + "/documents/" + docId + "/ack", {});
  },
};

// ---- Recognition (shout-outs) ----

export type ShoutoutVisibility = "team" | "private";

export interface Shoutout {
  id: number;
  business_id: number;
  from_staff_id: number;
  to_staff_id: number;
  message: string;
  emoji: string;
  visibility: ShoutoutVisibility;
  created_at: string;
}

export const recognitionApi = {
  list: async (businessId: string): Promise<Shoutout[]> => {
    const res = await axiosInstance.get(base(businessId) + "/shoutouts");
    return res.data.data;
  },
  send: async (
    businessId: string,
    input: { to_staff_id: number; message: string; emoji?: string; visibility?: ShoutoutVisibility },
  ): Promise<Shoutout> => {
    const res = await axiosInstance.post(base(businessId) + "/shoutouts", input);
    return res.data.data;
  },
};

// ---- Polls ----
//
// The list returns each poll with its options (label only) plus the caller's
// own vote (`my_vote`); vote tallies live on the separate results endpoint
// (which the Vote handler also returns inline). Anonymous polls never carry a
// voter list — results expose `votes` only.

type PollStatus = "open" | "closed";

interface PollOptionRow {
  id: number;
  poll_id: number;
  label: string;
  sort_order: number;
}

export interface Poll {
  id: number;
  business_id: number;
  author_staff_id: number;
  question: string;
  is_anonymous: boolean;
  audience_filter: string;
  status: PollStatus;
  closes_at: string | null;
  options: PollOptionRow[];
  my_vote: number | null;
}

interface PollOptionResult {
  option_id: number;
  label: string;
  votes: number;
  voters?: number[];
}

export interface PollResultView {
  poll_id: number;
  question: string;
  is_anonymous: boolean;
  status: PollStatus;
  options: PollOptionResult[];
}

export const pollsApi = {
  list: async (businessId: string): Promise<Poll[]> => {
    const res = await axiosInstance.get(base(businessId) + "/polls");
    return res.data.data;
  },
  create: async (
    businessId: string,
    input: {
      question: string;
      is_anonymous: boolean;
      closes_at?: string;
      audience_filter?: string;
      options: { label: string; sort_order: number }[];
    },
  ): Promise<Poll> => {
    const res = await axiosInstance.post(base(businessId) + "/polls", input);
    return res.data.data;
  },
  results: async (businessId: string, pollId: number): Promise<PollResultView> => {
    const res = await axiosInstance.get(base(businessId) + "/polls/" + pollId + "/results");
    return res.data.data;
  },
  // The Vote handler returns the (post-vote) results inline so the UI can render
  // the tally without a second round-trip.
  vote: async (businessId: string, pollId: number, optionId: number): Promise<PollResultView> => {
    const res = await axiosInstance.post(base(businessId) + "/polls/" + pollId + "/vote", {
      option_id: optionId,
    });
    return res.data.data;
  },
  close: async (businessId: string, pollId: number): Promise<void> => {
    await axiosInstance.post(base(businessId) + "/polls/" + pollId + "/close", {});
  },
};
