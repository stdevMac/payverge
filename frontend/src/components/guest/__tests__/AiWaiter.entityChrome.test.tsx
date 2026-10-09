/** @jest-environment jsdom */
/**
 * Issue 791 — Sage related-items chrome. Suggested dishes must render as a
 * compact 2-up grid with a price on every card, tapping a card adds that dish
 * to the cart, closed-hours cards keep the price but are not tappable, and an
 * empty thread greets the guest with a real chat bubble instead of a void.
 * Menu prices are backend float64 dollars — formatted, never re-divided.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../../api";
import { toast } from "react-hot-toast";

jest.mock("next/image", () => {
  const R = require("react");
  return {
    __esModule: true,
    default: R.forwardRef(
      ({ alt, src }: { alt: string; src: string }, ref: any) => (
        // eslint-disable-next-line @next/next/no-img-element -- Next Image test double.
        <img ref={ref} alt={alt} data-testid="rendered-image" src={src} />
      ),
    ),
  };
});
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
jest.mock("react-hot-toast", () => {
  const callable = Object.assign(jest.fn(), {
    success: jest.fn(),
    error: jest.fn(),
  });
  return { __esModule: true, toast: callable };
});
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
const mockedToast = toast as jest.Mocked<typeof toast> &
  jest.Mock;

const menu = [
  {
    items: [
      {
        id: "m1",
        name: "Harvest Bowl",
        price: 18.5,
        image: "https://payverge.io/media/biz/1/bowl.jpg",
        is_available: true,
      },
      {
        id: "m2",
        name: "Iced Tea",
        price: 3.5,
        image: "https://payverge.io/media/biz/1/tea.jpg",
        is_available: true,
      },
    ],
  },
];

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
  response_id: "entity-chrome-1",
  answer: { format: "markdown", content: "Here are my picks." },
  sections: [],
  steps: [],
  actions: [],
  sources: [
    {
      id: "src-1",
      type: "menu_item" as const,
      title: "Menu",
      href: null,
      origin: "menu_snapshot",
      retrieved_at: "2026-08-08T00:00:00Z",
    },
  ],
  entities,
  follow_ups: [],
  workflow: null,
  notices: [],
  status: "complete",
});

async function openAndAsk(view: ReturnType<typeof render>) {
  fireEvent.click(screen.getAllByRole("button")[0]);
  const input = await screen.findByRole("textbox", {
    name: "Message the AI assistant",
  });
  fireEvent.change(input, { target: { value: "what do you recommend?" } });
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(mockedAxios.post).toHaveBeenCalled());
  return view;
}

describe("AiWaiter related-items chrome (issue 791)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
  });

  it("renders each suggested dish with its menu price", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: suggestionResponse([
          entity("menu_item:m1", "Harvest Bowl"),
          entity("menu_item:m2", "Iced Tea"),
        ]),
      },
    });
    await openAndAsk(
      render(
        <AiWaiter
          businessId={1}
          businessName="Cafe"
          menuData={menu}
          bundles={[] as any}
          onAddToCart={jest.fn()}
          tableCode="T1"
        />,
      ),
    );

    await screen.findAllByTestId("rendered-image");
    expect(
      screen.getByText((content) => content.includes("$18.50")),
    ).toBeInTheDocument();
    expect(
      screen.getByText((content) => content.includes("$3.50")),
    ).toBeInTheDocument();
  });

  it("lays suggested dishes out as a compact grid, not a full-width stack", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: suggestionResponse([
          entity("menu_item:m1", "Harvest Bowl"),
          entity("menu_item:m2", "Iced Tea"),
        ]),
      },
    });
    await openAndAsk(
      render(
        <AiWaiter
          businessId={1}
          businessName="Cafe"
          menuData={menu}
          bundles={[] as any}
          onAddToCart={jest.fn()}
          tableCode="T1"
        />,
      ),
    );

    await screen.findAllByTestId("rendered-image");
    expect(
      document.querySelector('[data-entity-layout="grid"]'),
    ).not.toBeNull();
  });

  it("adds the dish to the cart when its card is tapped", async () => {
    const onAddToCart = jest.fn().mockReturnValue("applied");
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: suggestionResponse([entity("menu_item:m1", "Harvest Bowl")]),
      },
    });
    await openAndAsk(
      render(
        <AiWaiter
          businessId={1}
          businessName="Cafe"
          menuData={menu}
          bundles={[] as any}
          onAddToCart={onAddToCart}
          tableCode="T1"
        />,
      ),
    );

    const addButton = await screen.findByRole("button", {
      name: "Add Harvest Bowl to your order",
    });
    fireEvent.click(addButton);

    expect(onAddToCart).toHaveBeenCalledWith("Harvest Bowl", 18.5, 1, "", {
      itemType: "menu_item",
      menuItemId: "m1",
    });
    expect(mockedToast.success).toHaveBeenCalled();
  });

  it("does not stack a generic toast on top of the host's own rejection toast", async () => {
    // Issue 791 follow-up (GM2): the menu page's addToCart always surfaces a
    // reason-specific toast before returning false → "rejected". Sage must not
    // pile a contradictory generic "unavailable" toast on top of it.
    const onAddToCart = jest.fn().mockReturnValue("rejected");
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: suggestionResponse([entity("menu_item:m1", "Harvest Bowl")]),
      },
    });
    await openAndAsk(
      render(
        <AiWaiter
          businessId={1}
          businessName="Cafe"
          menuData={menu}
          bundles={[] as any}
          onAddToCart={onAddToCart}
          tableCode="T1"
        />,
      ),
    );

    const addButton = await screen.findByRole("button", {
      name: "Add Harvest Bowl to your order",
    });
    fireEvent.click(addButton);

    expect(onAddToCart).toHaveBeenCalledTimes(1);
    expect(mockedToast.error).not.toHaveBeenCalled();
    expect(mockedToast.success).not.toHaveBeenCalled();
  });

  it("still surfaces its own rejection toast when the host add throws silently", async () => {
    // GM2 guard: a throw never reaches the host's toast path, so Sage's
    // generic fallback is the only feedback the guest gets — keep it.
    const onAddToCart = jest.fn(() => {
      throw new Error("cart adapter exploded");
    });
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: suggestionResponse([entity("menu_item:m1", "Harvest Bowl")]),
      },
    });
    await openAndAsk(
      render(
        <AiWaiter
          businessId={1}
          businessName="Cafe"
          menuData={menu}
          bundles={[] as any}
          onAddToCart={onAddToCart}
          tableCode="T1"
        />,
      ),
    );

    const addButton = await screen.findByRole("button", {
      name: "Add Harvest Bowl to your order",
    });
    fireEvent.click(addButton);

    expect(onAddToCart).toHaveBeenCalledTimes(1);
    expect(mockedToast.error).toHaveBeenCalledTimes(1);
  });

  it("keeps the price but stays untappable when the venue is closed for ordering", async () => {
    const onAddToCart = jest.fn();
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: suggestionResponse([entity("menu_item:c1", "Harvest Bowl")]),
      },
    });
    await openAndAsk(
      render(
        <AiWaiter
          businessId={1}
          businessName="Cafe"
          menuData={[
            {
              items: [
                {
                  id: "c1",
                  name: "Harvest Bowl",
                  price: 18.5,
                  image: "https://payverge.io/media/biz/1/bowl.jpg",
                  is_available: false,
                  isAvailable: false,
                  orderability_state: "business_closed",
                },
              ],
            },
          ]}
          itemOrderability={{
            c1: { state: "business_closed", orderable: false },
          }}
          bundles={[] as any}
          onAddToCart={onAddToCart}
          tableCode="T1"
        />,
      ),
    );

    await screen.findByTestId("rendered-image");
    expect(
      screen.getByText((content) => content.includes("$18.50")),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Add Harvest Bowl to your order" }),
    ).toBeNull();
    expect(onAddToCart).not.toHaveBeenCalled();
  });

  it("greets an empty thread with a chat bubble instead of a bare void", async () => {
    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        aiName="Sage"
        menuData={menu}
        bundles={[] as any}
        onAddToCart={jest.fn()}
        tableCode="T1"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    await screen.findByRole("textbox", { name: "Message the AI assistant" });

    expect(
      screen.getByText("Hi, I'm Sage! Ask me anything about the menu."),
    ).toBeInTheDocument();
  });
});
