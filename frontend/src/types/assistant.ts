import { z } from "zod";

const MAX_ID = 200;
const MAX_TYPE = 100;
const MAX_TITLE = 300;
const MAX_SECTION_ANSWER = 4_000;
const MAX_STEP = 1_000;
const MAX_LABEL = 300;
const MAX_HREF = 2_000;
const MAX_NOTICE = 2_000;
const MAX_TIMESTAMP = 100;
const MAX_SOURCE_TITLE = 500;
const MAX_ENTITY_NAME = 300;
const MAX_FOLLOW_UP_PROMPT = 1_000;
const MAX_CART_NOTES = 200;

const actionTypes = [
  "navigate",
  "external_link",
  "director_handoff",
  "add_cart_item",
  "capture_lead",
  "create_support_request",
] as const;

const actionTargetKinds = [
  "bundle",
  "dashboard_area",
  "director",
  "external_url",
  "lead_form",
  "menu_item",
  "payverge_page",
  "support_request",
] as const;

const sourceTypes = [
  "payverge_page",
  "product_registry",
  "pricing_registry",
  "knowledge_entry",
  "dashboard_guide",
  "business_state",
  "menu_item",
  "bundle",
  "offer",
  "order_state",
  "support_record",
] as const;

const entityTypes = ["menu_item", "bundle", "offer"] as const;

const boundedString = (limit: number, field: string) =>
  z.string().refine((value) => Array.from(value).length <= limit, {
    message: `${field} exceeds ${limit} characters`,
  });

const truncateCodePoints = (value: string, limit: number): string =>
  Array.from(value).slice(0, limit).join("");

const boundedID = (kind: string) =>
  boundedString(MAX_ID, `${kind} id`).refine(
    (value) => value.trim().length > 0,
    {
      message: `${kind} id is required`,
    },
  );

const answerSchema = z
  .object({
    format: z.enum(["markdown", "plain_text"]),
    content: boundedString(12_000, "answer content"),
  })
  .strict();

const sectionSchema = z
  .object({
    id: boundedID("section"),
    title: boundedString(MAX_TITLE, "section title"),
    answer: boundedString(MAX_SECTION_ANSWER, "section answer"),
    steps: z.array(boundedString(MAX_STEP, "section step")).max(8),
    action_ids: z.array(boundedString(MAX_ID, "section action id")).max(8),
    source_ids: z.array(boundedString(MAX_ID, "section source id")).max(12),
    entity_ids: z.array(boundedString(MAX_ID, "section entity id")).max(20),
  })
  .strict();

const actionTargetSchema = z
  .object({
    kind: z.enum(actionTargetKinds),
    id: boundedID("action target"),
    href: boundedString(MAX_HREF, "action href"),
    quantity: z.number().int().min(1).max(20).optional(),
    notes: boundedString(MAX_CART_NOTES, "action notes")
      .refine(
        (value) =>
          !Array.from(value).some((character) => {
            const code = character.codePointAt(0) ?? 0;
            return code <= 0x1f || code === 0x7f;
          }),
        { message: "action notes contains control characters" },
      )
      .optional(),
  })
  .strict();

const actionSchema = z
  .object({
    id: boundedID("action"),
    type: z.enum(actionTypes),
    label: boundedString(MAX_LABEL, "action label"),
    target: actionTargetSchema,
    state: boundedString(MAX_TYPE, "action state"),
    confirmation: boundedString(MAX_TYPE, "action confirmation"),
    disabled_reason: boundedString(
      MAX_NOTICE,
      "action disabled_reason",
    ).nullable(),
    expires_at: boundedString(MAX_TIMESTAMP, "action expires_at").nullable(),
  })
  .strict();

const sourceSchema = z
  .object({
    id: boundedID("source"),
    type: z.enum(sourceTypes),
    title: boundedString(MAX_SOURCE_TITLE, "source title"),
    href: boundedString(MAX_HREF, "source href").nullable(),
    origin: boundedString(MAX_TYPE, "source origin"),
    retrieved_at: boundedString(MAX_TIMESTAMP, "source retrieved_at"),
  })
  .strict();

const entitySchema = z
  .object({
    id: boundedID("entity"),
    type: z.enum(entityTypes),
    display_name: boundedString(MAX_ENTITY_NAME, "entity display_name"),
    availability: boundedString(MAX_TYPE, "entity availability"),
    source_id: boundedID("entity source"),
  })
  .strict();

const followUpSchema = z
  .object({
    id: boundedID("follow-up"),
    label: boundedString(MAX_LABEL, "follow-up label"),
    prompt: boundedString(MAX_FOLLOW_UP_PROMPT, "follow-up prompt"),
  })
  .strict();

const workflowSchema = z
  .object({
    id: boundedID("workflow"),
    step_index: z.number().int(),
    step_total: z.number().int(),
  })
  .strict()
  .refine(
    ({ step_index, step_total }) =>
      step_total > 0 && step_index >= 0 && step_index < step_total,
    {
      message:
        "workflow requires 0 <= step_index < step_total with positive step_total",
    },
  );

const noticeSchema = z
  .object({
    id: boundedID("notice"),
    kind: boundedString(MAX_TYPE, "notice kind"),
    message: boundedString(MAX_NOTICE, "notice message"),
  })
  .strict();

const assistantResponseSchema = z
  .object({
    version: z.literal(2),
    response_id: boundedID("response"),
    answer: answerSchema,
    sections: z.array(sectionSchema).max(5),
    steps: z.array(boundedString(MAX_STEP, "step")).max(8),
    actions: z.array(actionSchema).max(8),
    sources: z.array(sourceSchema).max(12),
    entities: z.array(entitySchema).max(20),
    follow_ups: z.array(followUpSchema).max(5),
    workflow: workflowSchema.nullable(),
    notices: z.array(noticeSchema).max(8),
    status: z.enum(["complete", "needs_clarification", "blocked", "degraded"]),
  })
  .strict();
type AssistantActionTarget = z.infer<typeof actionTargetSchema>;
export type AssistantAction = z.infer<typeof actionSchema>;
export type AssistantSource = z.infer<typeof sourceSchema>;
export type AssistantEntity = z.infer<typeof entitySchema>;
export type AssistantFollowUp = z.infer<typeof followUpSchema>;
type AssistantWorkflow = z.infer<typeof workflowSchema>;
export type AssistantNotice = z.infer<typeof noticeSchema>;
export type AssistantResponse = z.infer<typeof assistantResponseSchema>;

const allowedTargetKinds: Record<
  AssistantAction["type"],
  ReadonlySet<AssistantActionTarget["kind"]>
> = {
  navigate: new Set(["dashboard_area", "payverge_page"]),
  external_link: new Set(["external_url"]),
  director_handoff: new Set(["director"]),
  add_cart_item: new Set(["menu_item", "bundle"]),
  capture_lead: new Set(["lead_form"]),
  create_support_request: new Set(["support_request"]),
};

const isSafeInternalHref = (href: string): boolean => {
  if (!href.startsWith("/") || href.startsWith("//") || href.includes("\\")) {
    return false;
  }
  return !Array.from(href).some((character) => {
    const code = character.codePointAt(0) ?? 0;
    return code <= 0x1f || code === 0x7f;
  });
};

const isSafeExternalHref = (href: string): boolean => {
  if (href.includes("\\")) return false;
  try {
    const parsed = new URL(href.trim());
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.host.length > 0 &&
      parsed.username === "" &&
      parsed.password === ""
    );
  } catch {
    return false;
  }
};

const identitySet = (kind: string, ids: readonly string[]): Set<string> => {
  const result = new Set<string>();
  for (const id of ids) {
    if (result.has(id)) {
      throw new Error(`duplicate ${kind} id ${id}`);
    }
    result.add(id);
  }
  return result;
};

const assertActionTarget = (action: AssistantAction): void => {
  if (!allowedTargetKinds[action.type].has(action.target.kind)) {
    throw new Error(
      `action ${action.id} type ${action.type} cannot target kind ${action.target.kind}`,
    );
  }

  if (action.type === "add_cart_item") {
    if (action.target.quantity === undefined) {
      throw new Error(`add_cart_item action ${action.id} quantity is required`);
    }
  } else if (
    action.target.quantity !== undefined ||
    action.target.notes !== undefined
  ) {
    throw new Error(
      `action ${action.id} type ${action.type} cannot include quantity or notes`,
    );
  }

  if (action.type === "navigate" && !isSafeInternalHref(action.target.href)) {
    throw new Error(
      `navigate action ${action.id} requires a safe internal href`,
    );
  }
  if (
    action.type === "external_link" &&
    !isSafeExternalHref(action.target.href)
  ) {
    throw new Error(
      `external_link action ${action.id} requires a safe external href`,
    );
  }
  if (
    action.type !== "navigate" &&
    action.type !== "external_link" &&
    action.target.href !== "" &&
    !isSafeInternalHref(action.target.href)
  ) {
    throw new Error(`action ${action.id} cannot use an external href`);
  }
};

export const parseAssistantResponse = (raw: unknown): AssistantResponse => {
  const response = assistantResponseSchema.parse(raw);
  identitySet(
    "section",
    response.sections.map(({ id }) => id),
  );
  const actionIDs = identitySet(
    "action",
    response.actions.map(({ id }) => id),
  );
  const sourceIDs = identitySet(
    "source",
    response.sources.map(({ id }) => id),
  );
  const entityIDs = identitySet(
    "entity",
    response.entities.map(({ id }) => id),
  );
  identitySet(
    "follow-up",
    response.follow_ups.map(({ id }) => id),
  );
  identitySet(
    "notice",
    response.notices.map(({ id }) => id),
  );

  for (const section of response.sections) {
    for (const id of section.action_ids) {
      if (!actionIDs.has(id)) {
        throw new Error(
          `section ${section.id} references unknown action ${id}`,
        );
      }
    }
    for (const id of section.source_ids) {
      if (!sourceIDs.has(id)) {
        throw new Error(
          `section ${section.id} references unknown source ${id}`,
        );
      }
    }
    for (const id of section.entity_ids) {
      if (!entityIDs.has(id)) {
        throw new Error(
          `section ${section.id} references unknown entity ${id}`,
        );
      }
    }
  }

  for (const action of response.actions) {
    assertActionTarget(action);
  }
  for (const source of response.sources) {
    if (
      source.href !== null &&
      !isSafeInternalHref(source.href) &&
      !isSafeExternalHref(source.href)
    ) {
      throw new Error(`source ${source.id} has unsafe href`);
    }
  }
  for (const entity of response.entities) {
    if (!sourceIDs.has(entity.source_id)) {
      throw new Error(
        `entity ${entity.id} references unknown source ${entity.source_id}`,
      );
    }
  }

  return response;
};

interface LegacyAssistantAction {
  label: string;
  href: string;
  target?: string;
  kind: "navigate" | "external" | "handoff";
  disabled?: boolean;
  disabled_reason?: string;
}

interface LegacyAssistantWorkflow {
  id: string;
  step_index: number;
  step_total: number;
}

export interface LegacyAssistantResponse {
  answer: string;
  steps: string[];
  actions: LegacyAssistantAction[];
  follow_ups: string[];
  workflow?: LegacyAssistantWorkflow | null;
  status?: AssistantResponse["status"];
}

const adaptLegacyAction = (
  action: LegacyAssistantAction,
  index: number,
): AssistantAction | null => {
  const href = action.href || action.target || "";
  if (Array.from(href).length > MAX_HREF) return null;

  let type: AssistantAction["type"];
  let targetKind: AssistantActionTarget["kind"];

  switch (action.kind) {
    case "navigate":
      if (!isSafeInternalHref(href)) return null;
      type = "navigate";
      targetKind = "payverge_page";
      break;
    case "external":
      if (!isSafeExternalHref(href)) return null;
      type = "external_link";
      targetKind = "external_url";
      break;
    case "handoff":
      if (href !== "" && !isSafeInternalHref(href)) return null;
      type = "director_handoff";
      targetKind = "director";
      break;
    default:
      return null;
  }

  return {
    id: `action-${index}`,
    type,
    label: truncateCodePoints(action.label, MAX_LABEL),
    target: {
      kind: targetKind,
      id: `legacy-target-${index}`,
      href,
    },
    state: action.disabled ? "disabled" : "ready",
    confirmation: "none",
    disabled_reason:
      action.disabled_reason === undefined
        ? null
        : truncateCodePoints(action.disabled_reason, MAX_NOTICE),
    expires_at: null,
  };
};

const adaptLegacyWorkflow = (
  workflow: LegacyAssistantWorkflow | null | undefined,
): AssistantWorkflow | null => {
  if (
    workflow === null ||
    workflow === undefined ||
    workflow.id.trim().length === 0 ||
    Array.from(workflow.id).length > MAX_ID ||
    !Number.isInteger(workflow.step_index) ||
    !Number.isInteger(workflow.step_total) ||
    workflow.step_total <= 0 ||
    workflow.step_index < 0 ||
    workflow.step_index >= workflow.step_total
  ) {
    return null;
  }
  return {
    id: workflow.id,
    step_index: workflow.step_index,
    step_total: workflow.step_total,
  };
};

export const adaptLegacyResponse = (
  responseID: string,
  legacy: LegacyAssistantResponse,
): AssistantResponse => {
  const actions = (legacy.actions ?? [])
    .flatMap((action, index) => {
      const adapted = adaptLegacyAction(action, index + 1);
      return adapted === null ? [] : [adapted];
    })
    .slice(0, 8);

  return parseAssistantResponse({
    version: 2,
    response_id: responseID,
    answer: {
      // Legacy model replies often include **bold**/lists — render via markdown.
      format: "markdown",
      content: truncateCodePoints(legacy.answer, 12_000),
    },
    sections: [],
    steps: (legacy.steps ?? [])
      .slice(0, 8)
      .map((step) => truncateCodePoints(step, MAX_STEP)),
    actions,
    sources: [],
    entities: [],
    follow_ups: (legacy.follow_ups ?? []).slice(0, 5).map((text, index) => ({
      id: `follow-up-${index + 1}`,
      label: truncateCodePoints(text, MAX_LABEL),
      prompt: truncateCodePoints(text, MAX_FOLLOW_UP_PROMPT),
    })),
    workflow: adaptLegacyWorkflow(legacy.workflow),
    notices: [],
    status: legacy.status ?? "complete",
  });
};
