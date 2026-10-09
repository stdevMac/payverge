import {
  adaptLegacyResponse,
  parseAssistantResponse,
  type AssistantResponse,
  type LegacyAssistantResponse,
} from "./assistant";

const responseFixture = (
  overrides: Partial<AssistantResponse> = {},
): AssistantResponse => ({
  version: 2,
  response_id: "response-1",
  answer: { format: "markdown", content: "Here is your answer." },
  sections: [],
  steps: [],
  actions: [],
  sources: [],
  entities: [],
  follow_ups: [],
  workflow: null,
  notices: [],
  status: "complete",
  ...overrides,
});

const sourceFixture = () => ({
  id: "source-1",
  type: "menu_item" as const,
  title: "Soup",
  href: "/menu/soup",
  origin: "menu",
  retrieved_at: "2026-08-08T12:00:00Z",
});

const actionFixture = () => ({
  id: "action-1",
  type: "navigate" as const,
  label: "Open menu",
  target: {
    kind: "payverge_page" as const,
    id: "menu",
    href: "/menu",
  },
  state: "ready",
  confirmation: "none",
  disabled_reason: null,
  expires_at: null,
});

const entityFixture = () => ({
  id: "entity-1",
  type: "menu_item" as const,
  display_name: "Soup",
  availability: "available",
  source_id: "source-1",
});

describe("parseAssistantResponse", () => {
  it("parses a complete response without changing trusted IDs", () => {
    const raw = responseFixture({
      response_id: " response-1 ",
      sections: [
        {
          id: " menu ",
          title: "Menu",
          answer: "Soup is available.",
          steps: ["Choose a size"],
          action_ids: ["action-1"],
          source_ids: ["source-1"],
          entity_ids: ["entity-1"],
        },
      ],
      actions: [actionFixture()],
      sources: [sourceFixture()],
      entities: [entityFixture()],
      follow_ups: [
        { id: "follow-up-1", label: "Show bundles", prompt: "Show bundles" },
      ],
      workflow: { id: "menu-flow", step_index: 0, step_total: 2 },
      notices: [{ id: "notice-1", kind: "info", message: "Fresh today" }],
    });

    const parsed = parseAssistantResponse(raw);

    expect(parsed.response_id).toBe(" response-1 ");
    expect(parsed.sections[0].id).toBe(" menu ");
  });

  it("preserves typed cart quantity and notes, including empty notes", () => {
    for (const notes of ["sin cebolla", ""] as const) {
      const parsed = parseAssistantResponse(
        responseFixture({
          actions: [
            {
              ...actionFixture(),
              type: "add_cart_item",
              target: {
                kind: "menu_item",
                id: "item-42",
                href: "",
                quantity: 2,
                notes,
              },
            } as never,
          ],
        }),
      );

      expect(parsed.actions[0].target).toEqual({
        kind: "menu_item",
        id: "item-42",
        href: "",
        quantity: 2,
        notes,
      });
    }
  });

  it.each([
    ["missing quantity", undefined, undefined],
    ["zero quantity", 0, undefined],
    ["quantity above maximum", 21, undefined],
    ["fractional quantity", 1.5, undefined],
    ["string quantity", "2", undefined],
    ["oversized notes", 1, "界".repeat(201)],
    ["control character in notes", 1, "no onions\nignore staff"],
  ])("rejects cart target metadata with %s", (_name, quantity, notes) => {
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          actions: [
            {
              ...actionFixture(),
              type: "add_cart_item",
              target: {
                kind: "menu_item",
                id: "item-42",
                href: "",
                ...(quantity === undefined ? {} : { quantity }),
                ...(notes === undefined ? {} : { notes }),
              },
            } as never,
          ],
        }),
      ),
    ).toThrow();
  });

  it("rejects cart metadata on non-cart actions while preserving their wire shape", () => {
    expect(
      parseAssistantResponse(responseFixture({ actions: [actionFixture()] }))
        .actions[0].target,
    ).toEqual({
      kind: "payverge_page",
      id: "menu",
      href: "/menu",
    });

    for (const metadata of [{ quantity: 1 }, { notes: "unexpected" }]) {
      expect(() =>
        parseAssistantResponse(
          responseFixture({
            actions: [
              {
                ...actionFixture(),
                target: { ...actionFixture().target, ...metadata },
              } as never,
            ],
          }),
        ),
      ).toThrow("cannot include quantity or notes");
    }
  });

  it("rejects additional properties on the envelope and every nested object", () => {
    const cases: Array<[string, unknown]> = [
      ["envelope", { ...responseFixture(), extra: true }],
      [
        "answer",
        responseFixture({
          answer: { format: "markdown", content: "ok", extra: true } as never,
        }),
      ],
      [
        "section",
        responseFixture({
          sections: [
            {
              id: "section-1",
              title: "Title",
              answer: "Answer",
              steps: [],
              action_ids: [],
              source_ids: [],
              entity_ids: [],
              extra: true,
            } as never,
          ],
        }),
      ],
      [
        "action",
        responseFixture({
          actions: [{ ...actionFixture(), extra: true } as never],
        }),
      ],
      [
        "action target",
        responseFixture({
          actions: [
            {
              ...actionFixture(),
              target: { ...actionFixture().target, extra: true },
            } as never,
          ],
        }),
      ],
      [
        "source",
        responseFixture({
          sources: [{ ...sourceFixture(), extra: true } as never],
        }),
      ],
      [
        "entity",
        responseFixture({
          sources: [sourceFixture()],
          entities: [{ ...entityFixture(), extra: true } as never],
        }),
      ],
      [
        "follow-up",
        responseFixture({
          follow_ups: [
            {
              id: "follow-up-1",
              label: "Next",
              prompt: "Next",
              extra: true,
            } as never,
          ],
        }),
      ],
      [
        "workflow",
        responseFixture({
          workflow: {
            id: "flow",
            step_index: 0,
            step_total: 1,
            extra: true,
          } as never,
        }),
      ],
      [
        "notice",
        responseFixture({
          notices: [
            {
              id: "notice-1",
              kind: "info",
              message: "Heads up",
              extra: true,
            } as never,
          ],
        }),
      ],
    ];

    for (const [, raw] of cases) {
      expect(() => parseAssistantResponse(raw)).toThrow();
    }
  });

  it("accepts only the exact contract enums", () => {
    const invalidResponses = [
      responseFixture({ version: 3 as never }),
      responseFixture({ answer: { format: "html" as never, content: "ok" } }),
      responseFixture({ status: "pending" as never }),
      responseFixture({
        actions: [{ ...actionFixture(), type: "delete" as never }],
      }),
      responseFixture({
        actions: [
          {
            ...actionFixture(),
            target: { ...actionFixture().target, kind: "raw_url" as never },
          },
        ],
      }),
      responseFixture({
        sources: [{ ...sourceFixture(), type: "web" as never }],
      }),
      responseFixture({
        sources: [sourceFixture()],
        entities: [{ ...entityFixture(), type: "restaurant" as never }],
      }),
    ];

    for (const raw of invalidResponses) {
      expect(() => parseAssistantResponse(raw)).toThrow();
    }
  });

  it("requires arrays and nullable fields instead of accepting missing values or null arrays", () => {
    for (const key of [
      "sections",
      "steps",
      "actions",
      "sources",
      "entities",
      "follow_ups",
      "notices",
    ] as const) {
      const missing = { ...responseFixture() } as Record<string, unknown>;
      delete missing[key];
      expect(() => parseAssistantResponse(missing)).toThrow();
      expect(() =>
        parseAssistantResponse({ ...responseFixture(), [key]: null }),
      ).toThrow();
    }

    const section = {
      id: "section-1",
      title: "Title",
      answer: "Answer",
      steps: [],
      action_ids: [],
      source_ids: [],
      entity_ids: [],
    };
    for (const key of [
      "steps",
      "action_ids",
      "source_ids",
      "entity_ids",
    ] as const) {
      expect(() =>
        parseAssistantResponse(
          responseFixture({ sections: [{ ...section, [key]: null } as never] }),
        ),
      ).toThrow();
    }

    expect(() =>
      parseAssistantResponse(responseFixture({ workflow: undefined as never })),
    ).toThrow();
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          actions: [
            { ...actionFixture(), disabled_reason: undefined as never },
          ],
        }),
      ),
    ).toThrow();
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          actions: [{ ...actionFixture(), expires_at: undefined as never }],
        }),
      ),
    ).toThrow();
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          sources: [{ ...sourceFixture(), href: undefined as never }],
        }),
      ),
    ).toThrow();
  });

  it("enforces collection, string, and workflow bounds", () => {
    const section = {
      id: "section",
      title: "Title",
      answer: "Answer",
      steps: [],
      action_ids: [],
      source_ids: [],
      entity_ids: [],
    };
    const tooManyCases: Array<[string, unknown]> = [
      [
        "sections",
        responseFixture({
          sections: Array.from({ length: 6 }, (_, i) => ({
            ...section,
            id: `s-${i}`,
          })),
        }),
      ],
      ["steps", responseFixture({ steps: Array(9).fill("step") })],
      [
        "section steps",
        responseFixture({
          sections: [{ ...section, steps: Array(9).fill("step") }],
        }),
      ],
      [
        "actions",
        responseFixture({
          actions: Array.from({ length: 9 }, (_, i) => ({
            ...actionFixture(),
            id: `a-${i}`,
          })),
        }),
      ],
      [
        "sources",
        responseFixture({
          sources: Array.from({ length: 13 }, (_, i) => ({
            ...sourceFixture(),
            id: `s-${i}`,
          })),
        }),
      ],
      [
        "entities",
        responseFixture({
          sources: [sourceFixture()],
          entities: Array.from({ length: 21 }, (_, i) => ({
            ...entityFixture(),
            id: `e-${i}`,
          })),
        }),
      ],
      [
        "follow-ups",
        responseFixture({
          follow_ups: Array.from({ length: 6 }, (_, i) => ({
            id: `f-${i}`,
            label: "Next",
            prompt: "Next",
          })),
        }),
      ],
      [
        "notices",
        responseFixture({
          notices: Array.from({ length: 9 }, (_, i) => ({
            id: `n-${i}`,
            kind: "info",
            message: "Notice",
          })),
        }),
      ],
      [
        "answer",
        responseFixture({
          answer: { format: "plain_text", content: "a".repeat(12_001) },
        }),
      ],
      [
        "unicode answer",
        responseFixture({
          answer: { format: "plain_text", content: "😀".repeat(12_001) },
        }),
      ],
      [
        "workflow negative index",
        responseFixture({
          workflow: { id: "flow", step_index: -1, step_total: 1 },
        }),
      ],
      [
        "workflow index at total",
        responseFixture({
          workflow: { id: "flow", step_index: 1, step_total: 1 },
        }),
      ],
      [
        "workflow zero total",
        responseFixture({
          workflow: { id: "flow", step_index: 0, step_total: 0 },
        }),
      ],
    ];

    for (const [, raw] of tooManyCases) {
      expect(() => parseAssistantResponse(raw)).toThrow();
    }

    expect(() =>
      parseAssistantResponse(
        responseFixture({
          answer: { format: "plain_text", content: "😀".repeat(12_000) },
        }),
      ),
    ).not.toThrow();
  });

  it.each([
    [
      "section",
      {
        sections: [
          {
            id: "same",
            title: "A",
            answer: "",
            steps: [],
            action_ids: [],
            source_ids: [],
            entity_ids: [],
          },
          {
            id: "same",
            title: "B",
            answer: "",
            steps: [],
            action_ids: [],
            source_ids: [],
            entity_ids: [],
          },
        ],
      },
    ],
    [
      "action",
      {
        actions: [
          { ...actionFixture(), id: "same" },
          { ...actionFixture(), id: "same" },
        ],
      },
    ],
    [
      "source",
      {
        sources: [
          { ...sourceFixture(), id: "same" },
          { ...sourceFixture(), id: "same" },
        ],
      },
    ],
    [
      "entity",
      {
        sources: [sourceFixture()],
        entities: [
          { ...entityFixture(), id: "same" },
          { ...entityFixture(), id: "same" },
        ],
      },
    ],
    [
      "follow-up",
      {
        follow_ups: [
          { id: "same", label: "A", prompt: "A" },
          { id: "same", label: "B", prompt: "B" },
        ],
      },
    ],
    [
      "notice",
      {
        notices: [
          { id: "same", kind: "info", message: "A" },
          { id: "same", kind: "warning", message: "B" },
        ],
      },
    ],
  ] as const)("rejects duplicate %s IDs", (kind, overrides) => {
    expect(() =>
      parseAssistantResponse(
        responseFixture(overrides as unknown as Partial<AssistantResponse>),
      ),
    ).toThrow(`duplicate ${kind} id same`);
  });

  it("rejects unresolved section references", () => {
    const raw = responseFixture({
      sections: [
        {
          id: "menu",
          title: "Menu",
          answer: "",
          steps: [],
          action_ids: ["missing"],
          source_ids: [],
          entity_ids: [],
        },
      ],
    });
    expect(() => parseAssistantResponse(raw)).toThrow("unknown action missing");
  });

  it("rejects unresolved section source and entity references", () => {
    const section = {
      id: "menu",
      title: "Menu",
      answer: "",
      steps: [],
      action_ids: [],
      source_ids: ["missing-source"],
      entity_ids: [],
    };
    expect(() =>
      parseAssistantResponse(responseFixture({ sections: [section] })),
    ).toThrow("unknown source missing-source");
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          sections: [
            { ...section, source_ids: [], entity_ids: ["missing-entity"] },
          ],
        }),
      ),
    ).toThrow("unknown entity missing-entity");
  });

  it("rejects unresolved entity source references", () => {
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          entities: [{ ...entityFixture(), source_id: "missing" }],
        }),
      ),
    ).toThrow("entity entity-1 references unknown source missing");
  });

  it.each([
    ["navigate", "external_url"],
    ["external_link", "payverge_page"],
    ["director_handoff", "menu_item"],
    ["add_cart_item", "director"],
    ["capture_lead", "support_request"],
    ["create_support_request", "lead_form"],
  ] as const)("rejects %s actions targeting %s", (type, kind) => {
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          actions: [
            {
              ...actionFixture(),
              type,
              target: { kind, id: "target", href: "" },
            },
          ],
        }),
      ),
    ).toThrow(`type ${type} cannot target kind ${kind}`);
  });

  it("rejects unsafe action and source hrefs", () => {
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          actions: [
            {
              ...actionFixture(),
              target: {
                ...actionFixture().target,
                href: "//evil.example/menu",
              },
            },
          ],
        }),
      ),
    ).toThrow("safe internal href");
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          actions: [
            {
              ...actionFixture(),
              type: "external_link",
              target: {
                kind: "external_url",
                id: "docs",
                href: "https://user:pass@example.com",
              },
            },
          ],
        }),
      ),
    ).toThrow("safe external href");
    expect(() =>
      parseAssistantResponse(
        responseFixture({
          sources: [{ ...sourceFixture(), href: "javascript:alert(1)" }],
        }),
      ),
    ).toThrow("unsafe href");
  });
});

describe("adaptLegacyResponse", () => {
  it("preserves readable V1 content, steps, follow-ups, workflow, and non-nil arrays", () => {
    const legacy: LegacyAssistantResponse = {
      answer: "Plain legacy answer",
      steps: ["Open settings"],
      actions: [],
      follow_ups: ["What next?"],
      workflow: { id: "setup", step_index: 1, step_total: 3 },
    };

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.answer).toEqual({
      format: "markdown",
      content: "Plain legacy answer",
    });
    expect(adapted.steps).toEqual(["Open settings"]);
    expect(adapted.follow_ups).toEqual([
      { id: "follow-up-1", label: "What next?", prompt: "What next?" },
    ]);
    expect(adapted.workflow).toEqual({
      id: "setup",
      step_index: 1,
      step_total: 3,
    });
    expect(adapted).toMatchObject({
      sections: [],
      actions: [],
      sources: [],
      entities: [],
      notices: [],
      status: "complete",
    });
  });

  it("keeps transport errors from looking complete", () => {
    const adapted = adaptLegacyResponse("response-error", {
      answer: "problemas para conectar con la cocina",
      steps: [],
      actions: [],
      follow_ups: [],
      status: "degraded",
    });
    expect(adapted.status).toBe("degraded");
  });

  it("maps safe legacy actions with stable IDs and ready or disabled state", () => {
    const legacy: LegacyAssistantResponse = {
      answer: "Choose an action.",
      steps: [],
      actions: [
        { label: "Menu", href: "/menu", kind: "navigate", disabled: false },
        {
          label: "Docs",
          href: "https://docs.payverge.io/guide",
          kind: "external",
          disabled: true,
          disabled_reason: "Owner only",
        },
        {
          label: "Ask director",
          href: "/director",
          kind: "handoff",
          disabled: false,
        },
      ],
      follow_ups: [],
      workflow: null,
    };

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.actions).toEqual([
      expect.objectContaining({
        id: "action-1",
        type: "navigate",
        state: "ready",
        disabled_reason: null,
        confirmation: "none",
        target: { kind: "payverge_page", id: "legacy-target-1", href: "/menu" },
      }),
      expect.objectContaining({
        id: "action-2",
        type: "external_link",
        state: "disabled",
        disabled_reason: "Owner only",
        target: {
          kind: "external_url",
          id: "legacy-target-2",
          href: "https://docs.payverge.io/guide",
        },
      }),
      expect.objectContaining({
        id: "action-3",
        type: "director_handoff",
        state: "ready",
        target: { kind: "director", id: "legacy-target-3", href: "/director" },
      }),
    ]);
  });

  it("drops unknown and unsafe executable legacy actions while retaining the answer", () => {
    const legacy = {
      answer: "You can still read this answer.",
      steps: [],
      actions: [
        { label: "Delete", href: "/danger", kind: "delete", disabled: false },
        {
          label: "Bad nav",
          href: "//evil.example",
          kind: "navigate",
          disabled: false,
        },
        { label: "Good nav", href: "/safe", kind: "navigate", disabled: false },
        {
          label: "Bad external",
          href: "javascript:alert(1)",
          kind: "external",
          disabled: false,
        },
        {
          label: "Credentials",
          href: "https://user:pass@example.com",
          kind: "external",
          disabled: false,
        },
        {
          label: "Bad handoff",
          href: "https://evil.example",
          kind: "handoff",
          disabled: false,
        },
      ],
      follow_ups: [],
      workflow: null,
    } as unknown as LegacyAssistantResponse;

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.answer.content).toBe("You can still read this answer.");
    expect(adapted.actions).toHaveLength(1);
    expect(adapted.actions[0].id).toBe("action-3");
    expect(adapted.actions[0].target.href).toBe("/safe");
  });

  it("uses the legacy target fallback without making arbitrary URLs executable", () => {
    const legacy = {
      answer: "Fallback target",
      steps: [],
      actions: [
        {
          label: "Safe",
          href: "",
          target: "/settings",
          kind: "navigate",
          disabled: false,
        },
        {
          label: "Unsafe",
          href: "",
          target: "https://evil.example/settings",
          kind: "navigate",
          disabled: false,
        },
      ],
      follow_ups: [],
      workflow: null,
    } as LegacyAssistantResponse;

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.actions).toHaveLength(1);
    expect(adapted.actions[0].target.href).toBe("/settings");
  });

  it("keeps an overlong V1 answer readable with a Unicode-safe bound", () => {
    const answer = "😀".repeat(12_001);
    const legacy: LegacyAssistantResponse = {
      answer,
      steps: [],
      actions: [],
      follow_ups: [],
      workflow: null,
    };

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.answer.format).toBe("markdown");
    expect(adapted.answer.content).toBe("😀".repeat(12_000));
    expect(Array.from(adapted.answer.content)).toHaveLength(12_000);
  });

  it("bounds legacy steps while preserving their readable prefixes", () => {
    const legacy: LegacyAssistantResponse = {
      answer: "Steps remain readable.",
      steps: [
        "😀".repeat(1_001),
        ...Array.from({ length: 8 }, (_, i) => `step-${i + 2}`),
      ],
      actions: [],
      follow_ups: [],
      workflow: null,
    };

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.steps).toHaveLength(8);
    expect(adapted.steps[0]).toBe("😀".repeat(1_000));
    expect(adapted.steps[7]).toBe("step-8");
  });

  it("collects at most eight safe actions after filtering and keeps original sequence IDs", () => {
    const safeActions = Array.from({ length: 9 }, (_, index) => ({
      label: `Safe ${index + 4}`,
      href: `/safe-${index + 4}`,
      kind: "navigate" as const,
      disabled: false,
    }));
    const legacy = {
      answer: "Safe actions remain available.",
      steps: [],
      actions: [
        {
          label: "Unsafe",
          href: "//evil.example",
          kind: "navigate",
          disabled: false,
        },
        {
          label: "😀".repeat(301),
          href: "/safe-2",
          kind: "navigate",
          disabled: true,
          disabled_reason: "😀".repeat(2_001),
        },
        {
          label: "Overlong URL",
          href: `/${"a".repeat(2_000)}`,
          kind: "navigate",
          disabled: false,
        },
        ...safeActions,
      ],
      follow_ups: [],
      workflow: null,
    } as LegacyAssistantResponse;

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.actions).toHaveLength(8);
    expect(adapted.actions.map(({ id }) => id)).toEqual([
      "action-2",
      "action-4",
      "action-5",
      "action-6",
      "action-7",
      "action-8",
      "action-9",
      "action-10",
    ]);
    expect(adapted.actions[0].label).toBe("😀".repeat(300));
    expect(adapted.actions[0].disabled_reason).toBe("😀".repeat(2_000));
    expect(adapted.actions.some(({ id }) => id === "action-3")).toBe(false);
  });

  it("bounds legacy follow-ups while preserving prompt and label text", () => {
    const legacy: LegacyAssistantResponse = {
      answer: "Choose a follow-up.",
      steps: [],
      actions: [],
      follow_ups: [
        "😀".repeat(1_001),
        "Second",
        "Third",
        "Fourth",
        "Fifth",
        "Sixth",
      ],
      workflow: null,
    };

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.follow_ups).toHaveLength(5);
    expect(adapted.follow_ups[0]).toEqual({
      id: "follow-up-1",
      label: "😀".repeat(300),
      prompt: "😀".repeat(1_000),
    });
    expect(adapted.follow_ups[4].id).toBe("follow-up-5");
  });

  it.each([
    ["blank ID", { id: "  ", step_index: 0, step_total: 1 }],
    ["overlong ID", { id: "w".repeat(201), step_index: 0, step_total: 1 }],
    ["negative index", { id: "flow", step_index: -1, step_total: 1 }],
    ["zero total", { id: "flow", step_index: 0, step_total: 0 }],
    ["index at total", { id: "flow", step_index: 1, step_total: 1 }],
  ] as const)("drops legacy workflow with %s", (_name, workflow) => {
    const legacy: LegacyAssistantResponse = {
      answer: "Workflow answer remains readable.",
      steps: [],
      actions: [],
      follow_ups: [],
      workflow,
    };

    const adapted = adaptLegacyResponse("response-legacy", legacy);

    expect(adapted.workflow).toBeNull();
    expect(adapted.answer.content).toBe("Workflow answer remains readable.");
  });
});
