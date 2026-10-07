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
      {
        id: "m2",
        name: "Bowl Verde Fresco",
        price: 12,
        image: "https://payverge.io/media/biz/1/bowl.jpg",
        is_available: false,
      },
    ],
  },
];

const source = (id: string, title: string) => ({
  id,
  type: "menu_item" as const,
  title,
  href: null,
  origin: "menu_snapshot",
  retrieved_at: "2026-08-08T00:00:00Z",
});

const entity = (
  id: string,
  display_name: string,
  availability = "available",
) => ({
  id,
  type: "menu_item" as const,
  display_name,
  availability,
  source_id: "src-1",
});

const suggestionResponse = (entities: ReturnType<typeof entity>[]) => ({
  version: 2,
  response_id: "diner-chrome-1",
  answer: {
    format: "markdown",
    content: "Happy to help — here are my picks.",
  },
  sections: [],
  steps: [],
  actions: [],
  sources: [
    source("src-1", "Menu"),
    source("src-2", "Menu"),
    source("src-3", "Menu"),
  ],
  entities,
  follow_ups: [],
  workflow: null,
  notices: [],
  status: "complete",
});

async function send(response_v2: unknown) {
  mockedAxios.post.mockResolvedValueOnce({
    data: { role: "model", parts: [{ text: "legacy" }], response_v2 },
  });
  const view = render(
    <AiWaiter
      businessId={1}
      businessName="Cafe"
      menuData={menu}
      bundles={[] as any}
      onAddToCart={jest.fn()}
      tableCode="T1"
    />,
  );
  fireEvent.click(screen.getAllByRole("button")[0]);
  const input = await screen.findByRole("textbox", {
    name: "Message the AI assistant",
  });
  fireEvent.change(input, {
    target: { value: "suggest a main, a dessert and a drink" },
  });
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(mockedAxios.post).toHaveBeenCalled());
  return view;
}

describe("AiWaiter diner transcript chrome (#723)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
  });

  it("never prints the Complete status chip to a diner", async () => {
    await send(suggestionResponse([entity("menu_item:m1", "Burger")]));

    await screen.findByTestId("rendered-image");
    expect(screen.queryByText("Complete")).toBeNull();
  });

  it("never prints an availability row on a suggested dish", async () => {
    await send(suggestionResponse([entity("menu_item:m1", "Burger")]));

    await screen.findByTestId("rendered-image");
    expect(screen.queryByText("Availability: Available")).toBeNull();
  });

  it("never prints the retrieval source count to a diner", async () => {
    await send(suggestionResponse([entity("menu_item:m1", "Burger")]));

    await screen.findByTestId("rendered-image");
    expect(screen.queryByText(/sources? used/i)).toBeNull();
  });

  it("drops a suggested-dish card that carries no photo instead of repeating the name", async () => {
    await send(
      suggestionResponse([
        entity("menu_item:m1", "Burger"),
        entity("menu_item:missing", "Limonada Casera"),
      ]),
    );

    await screen.findByTestId("rendered-image");
    expect(screen.queryByText("Limonada Casera")).toBeNull();
    expect(screen.getByText("Burger")).toBeInTheDocument();
  });

  it("hides the related-items heading when no suggested dish has a photo", async () => {
    await send(
      suggestionResponse([entity("menu_item:missing", "Limonada Casera")]),
    );

    await waitFor(() =>
      expect(screen.getByText("Happy to help — here are my picks.")).toBeInTheDocument(),
    );
    expect(screen.queryByText("Related menu items")).toBeNull();
  });

  it("omits an unavailable dish entirely", async () => {
    await send(
      suggestionResponse([
        entity("menu_item:m1", "Burger"),
        entity("menu_item:m2", "Bowl Verde Fresco", "unavailable"),
      ]),
    );

    await screen.findByTestId("rendered-image");
    expect(screen.queryByText("Bowl Verde Fresco")).toBeNull();
    expect(screen.queryByText("Unavailable")).toBeNull();
  });

  it("still shows dish cards when the kitchen is closed for ordering", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: {
          ...suggestionResponse([entity("menu_item:m1", "Burger")]),
          answer: {
            format: "markdown",
            content: [
              "Here are some available options from the menu:",
              "- **Burger** — available",
            ].join("\n"),
          },
        },
      },
    });
    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[
          {
            items: [
              {
                id: "m1",
                name: "Burger",
                price: 9,
                image: "https://payverge.io/media/biz/1/burger.jpg",
                is_available: false,
                isAvailable: false,
                orderability_state: "business_closed",
              },
              {
                id: "m2",
                name: "Bowl Verde Fresco",
                price: 12,
                image: "https://payverge.io/media/biz/1/bowl.jpg",
                is_available: false,
                isAvailable: false,
                orderability_state: "business_closed",
              },
            ],
          },
        ]}
        itemOrderability={{
          m1: { state: "business_closed", orderable: false },
          m2: { state: "business_closed", orderable: false },
        }}
        bundles={[] as any}
        onAddToCart={jest.fn()}
        tableCode="T1"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByRole("textbox", {
      name: "Message the AI assistant",
    });
    fireEvent.change(input, {
      target: { value: "suggest a main, a dessert and a drink" },
    });
    fireEvent.keyDown(input, { key: "Enter" });

    await screen.findByTestId("rendered-image");
    expect(screen.getByText("Burger")).toBeInTheDocument();
    expect(screen.queryByText(/— available/)).toBeNull();
    expect(screen.queryByText("Complete")).toBeNull();
    expect(screen.queryByText("Availability: Available")).toBeNull();
    expect(screen.queryByText(/sources? used/i)).toBeNull();
  });

  it("omits an 86 hidden under closed hours on the live catalog shape", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: {
          ...suggestionResponse([
            entity("menu_item:steak", "Steak Plate"),
            entity("menu_item:m1", "Burger"),
          ]),
          answer: {
            format: "markdown",
            content: [
              "Here are some available options from the menu:",
              "- **Steak Plate** — available",
              "- **Burger** — available",
            ].join("\n"),
          },
        },
      },
    });
    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[
          {
            items: [
              {
                id: "steak",
                name: "Steak Plate",
                price: 42,
                image: "https://payverge.io/media/biz/1/steak.jpg",
                is_available: true,
                inventory_status: "out_of_stock",
              },
              {
                id: "m1",
                name: "Burger",
                price: 9,
                image: "https://payverge.io/media/biz/1/burger.jpg",
                is_available: true,
              },
            ],
          },
        ]}
        itemOrderability={{
          steak: { state: "business_closed", orderable: false },
          m1: { state: "business_closed", orderable: false },
        }}
        bundles={[] as any}
        onAddToCart={jest.fn()}
        tableCode="T1"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByRole("textbox", {
      name: "Message the AI assistant",
    });
    fireEvent.change(input, {
      target: { value: "suggest a main, a dessert and a drink" },
    });
    fireEvent.keyDown(input, { key: "Enter" });

    await screen.findByTestId("rendered-image");
    expect(screen.getByText("Burger")).toBeInTheDocument();
    expect(screen.queryByText("Steak Plate")).toBeNull();
    expect(screen.queryByText(/— available/)).toBeNull();
    expect(screen.queryByText(/— unavailable/)).toBeNull();
    expect(screen.queryByText("Complete")).toBeNull();
  });
});
