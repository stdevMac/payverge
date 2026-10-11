import { parseAssistantResponse } from "@/types/assistant";
import {
  forDinerTranscript,
  stripDuplicatedDishList,
} from "./aiWaiterDinerTranscript";

const PHOTO = "https://payverge.io/media/biz/86/bowl.jpg";

const menu = [
  {
    items: [
      {
        id: "bowl",
        name: "Harvest Bowl",
        image: PHOTO,
        is_available: true,
      },
      {
        id: "tacos",
        name: "Market Tacos",
        image: "https://payverge.io/media/biz/86/tacos.jpg",
        is_available: true,
      },
      {
        id: "tea",
        name: "Iced Tea",
        image: "https://payverge.io/media/biz/86/tea.jpg",
        is_available: true,
      },
      {
        id: "hidden",
        name: "Off Menu Steak",
        image: "https://payverge.io/media/biz/86/steak.jpg",
        is_available: false,
        orderability_state: "manual_disabled",
      },
    ],
  },
];

const source = (id: string) => ({
  id,
  type: "menu_item" as const,
  title: "Menu",
  href: null,
  origin: "menu_snapshot",
  retrieved_at: "2026-08-21T00:00:00Z",
});

const entity = (id: string, display_name: string, availability = "available") => ({
  id: `menu_item:${id}`,
  type: "menu_item" as const,
  display_name,
  availability,
  source_id: "src-1",
});

const groundedAsk = (entities: ReturnType<typeof entity>[], content: string) =>
  parseAssistantResponse({
    version: 2,
    response_id: "diner-reshape-1",
    answer: { format: "markdown", content },
    sections: [],
    steps: [],
    actions: [],
    sources: [source("src-1"), source("src-2"), source("src-3")],
    entities,
    follow_ups: [],
    workflow: null,
    notices: [],
    status: "complete",
  });

describe("forDinerTranscript (#723)", () => {
  it("paints dish cards while the venue is closed for ordering", () => {
    const closedMenu = [
      {
        items: menu[0].items.map((item) => ({
          ...item,
          is_available: false,
          isAvailable: false,
          orderability_state: "business_closed",
        })),
      },
    ];
    const response = groundedAsk(
      [
        entity("bowl", "Harvest Bowl"),
        entity("tacos", "Market Tacos"),
        entity("tea", "Iced Tea"),
      ],
      [
        "Here are some available options from the menu:",
        "- **Harvest Bowl** — available",
        "- **Market Tacos** — available",
        "- **Iced Tea** — available",
      ].join("\n"),
    );

    const diner = forDinerTranscript(response, closedMenu, {
      bowl: { state: "business_closed", orderable: false },
      tacos: { state: "business_closed", orderable: false },
      tea: { state: "business_closed", orderable: false },
    });

    expect(Array.from(diner.media.keys()).map((item) => item.display_name)).toEqual(
      ["Harvest Bowl", "Market Tacos", "Iced Tea"],
    );
    expect(diner.media.get(diner.response.entities[0])?.[0]).toBe(PHOTO);
    expect(diner.response.sources).toEqual([]);
    expect(diner.response.answer.content).toBe(
      "Here are some available options from the menu:",
    );
    expect(diner.response.answer.content).not.toMatch(/— available/);
  });

  it("still paints cards when only the orderability map says the kitchen is closed", () => {
    const response = groundedAsk(
      [entity("bowl", "Harvest Bowl")],
      "- **Harvest Bowl** — available",
    );

    const diner = forDinerTranscript(response, menu, {
      bowl: { state: "ordering_disabled", orderable: false },
    });

    expect(diner.response.entities).toHaveLength(1);
    expect(diner.media.size).toBe(1);
    expect(diner.response.answer.content).toBe("");
  });

  it("omits a true 86 and strips its unavailable protocol bullet", () => {
    const response = groundedAsk(
      [
        entity("bowl", "Harvest Bowl"),
        entity("hidden", "Off Menu Steak", "unavailable"),
      ],
      [
        "- **Harvest Bowl** — available",
        "- **Off Menu Steak** — unavailable",
      ].join("\n"),
    );

    const diner = forDinerTranscript(response, menu);

    expect(diner.response.entities.map((item) => item.display_name)).toEqual([
      "Harvest Bowl",
    ]);
    expect(diner.response.sources).toEqual([]);
    expect(diner.response.answer.content).not.toContain("Harvest Bowl");
    expect(diner.response.answer.content).not.toContain("Off Menu Steak");
    expect(diner.response.answer.content).not.toMatch(/— unavailable/);
  });

  it("does not paint a card for inventory_out when closed hours hid it under business_closed", () => {
    // Live guest payload: hours remap inventory_out → business_closed, and
    // the catalog row has no orderability_state / no planted map 86. The
    // surviving signal is catalog inventory_status (out_of_stock).
    const closedEightySix = [
      {
        items: [
          {
            id: "steak",
            name: "Steak Plate",
            image: "https://payverge.io/media/biz/86/steak.jpg",
            is_available: true,
            inventory_status: "out_of_stock",
          },
        ],
      },
    ];
    const response = groundedAsk(
      [entity("steak", "Steak Plate")],
      [
        "Here are some available options from the menu:",
        "- **Steak Plate** — available",
        "- **Steak Plate** — unavailable",
      ].join("\n"),
    );

    const diner = forDinerTranscript(response, closedEightySix, {
      steak: { state: "business_closed", orderable: false },
    });

    expect(diner.media.size).toBe(0);
    expect(diner.response.entities).toEqual([]);
    expect(diner.response.sources).toEqual([]);
    expect(diner.response.answer.content).toBe(
      "Here are some available options from the menu:",
    );
    expect(diner.response.answer.content).not.toMatch(/— available/);
    expect(diner.response.answer.content).not.toMatch(/— unavailable/);
    expect(diner.response.answer.content).not.toContain("Steak Plate");
  });

  it("still cards a closed-hours dish that is not 86'd on the live catalog", () => {
    const liveClosed = [
      {
        items: [
          {
            id: "bowl",
            name: "Harvest Bowl",
            image: PHOTO,
            is_available: true,
          },
        ],
      },
    ];
    const response = groundedAsk(
      [entity("bowl", "Harvest Bowl")],
      [
        "Here are some available options from the menu:",
        "- **Harvest Bowl** — available",
      ].join("\n"),
    );

    const diner = forDinerTranscript(response, liveClosed, {
      bowl: { state: "business_closed", orderable: false },
    });

    expect(Array.from(diner.media.keys()).map((item) => item.display_name)).toEqual(
      ["Harvest Bowl"],
    );
    expect(diner.response.answer.content).not.toMatch(/— available/);
  });

  it("omits open-venue inventory_out even without a catalog orderability_state", () => {
    const liveOpenEightySix = [
      {
        items: [
          {
            id: "steak",
            name: "Steak Plate",
            image: "https://payverge.io/media/biz/86/steak.jpg",
            is_available: true,
            inventory_status: "out_of_stock",
          },
        ],
      },
    ];
    const response = groundedAsk(
      [entity("steak", "Steak Plate")],
      "- **Steak Plate** — available",
    );

    const diner = forDinerTranscript(response, liveOpenEightySix, {
      steak: { state: "inventory_out", orderable: false },
    });

    expect(diner.media.size).toBe(0);
    expect(diner.response.answer.content).not.toMatch(/— available/);
  });

  it("drops the protocol bullet when no dish has a trusted photo", () => {
    const response = groundedAsk(
      [entity("bowl", "Harvest Bowl")],
      "- **Harvest Bowl** — available",
    );

    const diner = forDinerTranscript(
      response,
      [{ items: [{ id: "bowl", name: "Harvest Bowl", is_available: true }] }],
    );

    expect(diner.media.size).toBe(0);
    expect(diner.response.entities).toEqual([]);
    expect(diner.response.answer.content).toBe("");
  });
});

describe("stripDuplicatedDishList", () => {
  it("drops only the bullets that name a shown dish", () => {
    const content = [
      "Happy to help — here are my picks.",
      "- **Harvest Bowl** — available",
      "- **Mystery Pie** — available",
    ].join("\n");

    expect(stripDuplicatedDishList(content, new Set(["Harvest Bowl"]))).toBe(
      ["Happy to help — here are my picks.", "- **Mystery Pie** — available"].join(
        "\n",
      ),
    );
  });

  it("does not eat a waiter sentence that merely mentions a dish", () => {
    expect(
      stripDuplicatedDishList(
        "The Harvest Bowl is the one I would start with.",
        new Set(["Harvest Bowl"]),
      ),
    ).toBe("The Harvest Bowl is the one I would start with.");
  });
});
