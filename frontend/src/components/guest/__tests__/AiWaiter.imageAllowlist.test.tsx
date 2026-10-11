/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../../api";

jest.mock("next/image", () => ({
  __esModule: true,
  default: ({ alt, src }: { alt: string; src: string }) => (
    // eslint-disable-next-line @next/next/no-img-element -- Next Image test double.
    <img alt={alt} data-testid="rendered-image" src={src} />
  ),
}));
jest.mock("../../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, vars?: Record<string, unknown>) =>
      key === "menu.itemImageAlt"
        ? `${String(vars?.name)} image ${String(vars?.index)}`
        : key,
  }),
}));

jest.mock("../../../api", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("@/hooks/useSSEMessages", () => ({ useSSEMessages: jest.fn() }));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("../aiWaiterCopy", () => ({
  getStarterQuestions: jest.fn().mockResolvedValue([]),
}));
jest.mock("@/api/aiWaiter", () => ({
  ...jest.requireActual("@/api/aiWaiter"),
  createAiWaiterSession: jest.fn().mockResolvedValue({
    session_token: "t",
    greeting: "hi",
    expires_at: "2999-01-01T00:00:00Z",
  }),
  isSessionUnknownError: () => false,
  classifyChatError: () => "generic",
}));
jest.mock("framer-motion", () => {
  const R = require("react");
  const M = R.forwardRef(({ children, ...p }: any, ref: any) => {
    const { initial, animate, exit, transition, whileHover, whileTap, ...d } =
      p;
    return R.createElement("div", { ...d, ref }, children);
  });
  return {
    AnimatePresence: ({ children }: any) =>
      R.createElement(R.Fragment, null, children),
    motion: new Proxy({}, { get: () => M }),
    useReducedMotion: () => false,
  };
});
jest.mock("@nextui-org/react", () => {
  const R = require("react");
  return {
    Modal: ({ children, isOpen }: any) =>
      isOpen
        ? R.createElement(
            "div",
            { role: "dialog", "aria-modal": "true" },
            children,
          )
        : null,
    ModalContent: ({ children, ...p }: any) =>
      R.createElement("div", p, children),
    ModalBody: ({ children, ...p }: any) => R.createElement("div", p, children),
    ModalFooter: ({ children, ...p }: any) =>
      R.createElement("div", p, children),
    Card: ({ children, ...p }: any) => <div {...p}>{children}</div>,
    CardBody: ({ children, ...p }: any) => <div {...p}>{children}</div>,
    ScrollShadow: R.forwardRef(({ children }: any, ref: any) => (
      <div ref={ref}>{children}</div>
    )),
    Image: ({ alt }: { alt: string }) => <div aria-label={alt} />,
    Input: R.forwardRef(
      (
        { placeholder, value, onValueChange, onKeyDown, endContent }: any,
        ref: any,
      ) => (
        <label>
          <input
            ref={ref}
            placeholder={placeholder}
            value={value}
            onChange={(e) => onValueChange?.(e.target.value)}
            onKeyDown={onKeyDown}
          />
          {endContent}
        </label>
      ),
    ),
    Button: ({
      children,
      onPress,
      onClick,
      isDisabled,
      title,
      className,
      "aria-label": al,
    }: any) => (
      <button
        type="button"
        disabled={isDisabled}
        onClick={onPress ?? onClick}
        title={title}
        className={className}
        aria-label={al}
      >
        {children}
      </button>
    ),
  };
});

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;
type FixtureMenuCategory = { items: Array<Record<string, unknown>> };
const menu: FixtureMenuCategory[] = [
  {
    items: [
      {
        id: "m1",
        name: "Burger",
        price: 9,
        image: "https://payverge.io/media/biz/1/burger.jpg",
        is_available: true,
      },
    ],
  },
];

const v2Response = (
  entityId: string,
  content = "Here it is",
  entityType: "menu_item" | "bundle" | "offer" = "menu_item",
) => ({
  version: 2,
  response_id: `image-${entityId}`,
  answer: { format: "markdown", content },
  sections: [],
  steps: [],
  actions: [],
  sources: [
    {
      id: "menu-source",
      type: entityType,
      title: "Burger",
      href: null,
      origin: "menu_snapshot",
      retrieved_at: "2026-08-08T00:00:00Z",
    },
  ],
  entities: [
    {
      id: entityId,
      type: entityType,
      display_name: "Burger",
      availability: "available",
      source_id: "menu-source",
    },
  ],
  follow_ups: [],
  workflow: null,
  notices: [],
  status: "complete",
});

describe("AiWaiter image allowlist", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
  });

  async function send(
    response_v2: ReturnType<typeof v2Response>,
    menuData: FixtureMenuCategory[] = menu,
    bundles: Array<Record<string, unknown>> = [],
  ) {
    mockedAxios.post.mockResolvedValueOnce({
      data: { role: "model", parts: [{ text: "legacy" }], response_v2 },
    });
    const view = render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={menuData}
        bundles={bundles as any}
        onAddToCart={jest.fn()}
        tableCode="T1"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByRole("textbox", {
      name: "Message the AI assistant",
    });
    fireEvent.change(input, { target: { value: "show me a photo" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockedAxios.post).toHaveBeenCalled());
    return view;
  }

  it("renders a uniquely matched structured menu-item entity through ImageCarousel", async () => {
    await send(v2Response("menu_item:m1"));
    const image = await screen.findByTestId("rendered-image");
    expect(image).toHaveAttribute(
      "src",
      "https://payverge.io/media/biz/1/burger.jpg",
    );
    expect(image).toHaveAccessibleName("Burger image 1");
  });

  it("matches an arbitrary raw stable ID after removing exactly one entity namespace", async () => {
    await send(v2Response("menu_item:menu_item:m1"), [
      {
        items: [
          {
            ...menu[0].items[0],
            id: "menu_item:m1",
          },
        ],
      },
    ]);

    expect(await screen.findByTestId("rendered-image")).toHaveAttribute(
      "src",
      "https://payverge.io/media/biz/1/burger.jpg",
    );
  });

  it("never treats Markdown images as entity media, even on a menu host", async () => {
    await send(
      v2Response(
        "menu_item:unknown",
        "![Burger](https://payverge.io/media/biz/1/burger.jpg)",
      ),
    );
    await waitFor(() =>
      expect(screen.queryByTestId("rendered-image")).not.toBeInTheDocument(),
    );
    expect(screen.queryByText(/images\.payverge\.io/)).not.toBeInTheDocument();
  });

  it("omits media when the same namespaced stable ID is duplicated in the live catalog", async () => {
    await send(v2Response("menu_item:m1"), [
      ...menu,
      {
        items: [
          {
            id: "m1",
            name: "Another Burger",
            price: 10,
            image: "https://payverge.io/media/biz/1/other.jpg",
            is_available: true,
          },
        ],
      },
    ]);

    await waitFor(() =>
      expect(screen.queryByTestId("rendered-image")).not.toBeInTheDocument(),
    );
  });

  it.each([
    ["raw unnamespaced entity ID", v2Response("m1"), menu, []],
    [
      "wrong entity namespace",
      v2Response("menu_item:m1", "Here", "bundle"),
      menu,
      [],
    ],
    [
      "namespaced suffix without an exact live stable-ID match",
      v2Response("menu_item:menu_item:m1"),
      menu,
      [],
    ],
    [
      "unavailable live item",
      v2Response("menu_item:m1"),
      [{ items: [{ ...menu[0].items[0], is_available: false }] }],
      [],
    ],
    [
      "hidden live item",
      v2Response("menu_item:m1"),
      [
        {
          items: [
            {
              ...menu[0].items[0],
              is_available: true,
              orderability_state: "manual_disabled",
            },
          ],
        },
      ],
      [],
    ],
    [
      "bundle entity despite a live bundle image",
      v2Response("bundle:10", "Here", "bundle"),
      menu,
      [
        {
          id: 10,
          name: "Combo",
          price: 20,
          image: "https://payverge.io/media/biz/1/combo.jpg",
          is_active: true,
        },
      ],
    ],
    [
      "inactive bundle entity",
      v2Response("bundle:10", "Here", "bundle"),
      menu,
      [
        {
          id: 10,
          name: "Combo",
          price: 20,
          image: "https://payverge.io/media/biz/1/combo.jpg",
          is_active: false,
        },
      ],
    ],
    [
      "offer entity with no live source",
      v2Response("offer:9", "Here", "offer"),
      menu,
      [],
    ],
  ])("omits media for %s", async (_name, structured, menuData, bundles) => {
    await send(structured, menuData, bundles);
    await waitFor(() =>
      expect(screen.queryByTestId("rendered-image")).not.toBeInTheDocument(),
    );
  });

  it("filters empty, duplicate, and unsafe configured images", async () => {
    const allowed = "https://payverge.io/media/biz/1/burger.jpg";
    await send(v2Response("menu_item:m1"), [
      {
        items: [
          {
            ...menu[0].items[0],
            image: allowed,
            images: ["", allowed, allowed, "javascript:alert(1)"],
          },
        ],
      },
    ]);

    const images = await screen.findAllByTestId("rendered-image");
    expect(images).toHaveLength(1);
    expect(images[0]).toHaveAttribute("src", allowed);
  });

  it("positions structured-media carousel controls with logical RTL utilities", async () => {
    const view = await send(v2Response("menu_item:m1"), [
      {
        items: [
          {
            ...menu[0].items[0],
            images: [
              "https://payverge.io/media/biz/1/burger-side.jpg",
              "https://payverge.io/media/biz/1/burger-top.jpg",
            ],
          },
        ],
      },
    ]);

    const previous = await screen.findByRole("button", {
      name: "accessibility.previousImage",
    });
    expect(previous).toHaveClass("start-2");
    expect(
      screen.getByRole("button", { name: "accessibility.nextImage" }),
    ).toHaveClass("end-2");
    expect(previous.querySelector("svg")).toHaveClass("rtl:rotate-180");
    expect(
      screen
        .getByRole("button", { name: "accessibility.nextImage" })
        .querySelector("svg"),
    ).toHaveClass("rtl:rotate-180");
    expect(screen.getByText("1/3")).toHaveClass("end-2");
    expect(screen.getByText("menu.swipeToBrowse")).toHaveClass("start-2");
    const dots = view.container.querySelector(".left-1\\/2");
    expect(dots).toHaveClass("-translate-x-1/2");
    fireEvent.click(
      screen.getByRole("button", { name: "accessibility.nextImage" }),
    );
    expect(screen.getByText("2/3")).toBeInTheDocument();
  });

  it("caps a response at three entities and three trusted images per entity, dropping the uncapped card", async () => {
    const sources = Array.from({ length: 4 }, (_, index) => ({
      id: `source-${index + 1}`,
      type: "menu_item" as const,
      title: `Dish ${index + 1}`,
      href: null,
      origin: "menu_snapshot",
      retrieved_at: "2026-08-08T00:00:00Z",
    }));
    const entities = sources.map((source, index) => ({
      id: `menu_item:m${index + 1}`,
      type: "menu_item" as const,
      display_name: `Dish ${index + 1}`,
      availability: "available" as const,
      source_id: source.id,
    }));
    const structured = {
      ...v2Response("menu_item:m1"),
      response_id: "capped-media",
      sources,
      entities,
    };
    const menuData = [
      {
        items: entities.map((entity, index) => ({
          id: `m${index + 1}`,
          name: entity.display_name,
          price: 10 + index,
          is_available: true,
          image: `https://payverge.io/media/biz/1/m${index + 1}-1.jpg`,
          images: [2, 3, 4].map(
            (image) =>
              `https://payverge.io/media/biz/1/m${index + 1}-${image}.jpg`,
          ),
        })),
      },
    ];

    await send(structured, menuData);

    const images = await screen.findAllByTestId("rendered-image");
    expect(images).toHaveLength(9);
    expect(
      images.some((image) => image.getAttribute("src")?.includes("-4.jpg")),
    ).toBe(false);
    expect(
      images.some((image) => image.getAttribute("src")?.includes("m4-")),
    ).toBe(false);
    for (const entity of entities.slice(0, 3)) {
      expect(screen.getByText(entity.display_name)).toBeInTheDocument();
    }
    // The capped-out dish loses its card rather than keeping a photo-less one
    // that just repeats the prose (#723).
    expect(screen.queryByText("Dish 4")).not.toBeInTheDocument();
  });

  it("rejects a foreign HTTPS image on the exact live entity row", async () => {
    await send(v2Response("menu_item:m1"), [
      {
        items: [
          {
            ...menu[0].items[0],
            image: "https://evil.example/track.gif",
          },
        ],
      },
    ]);

    await waitFor(() =>
      expect(screen.queryByTestId("rendered-image")).not.toBeInTheDocument(),
    );
  });
});
