/** @jest-environment jsdom */
import React from "react";
import { ReadableStream } from "stream/web";
import { TextEncoder } from "util";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../api";
import { toast } from "react-hot-toast";
import { trackEvent } from "@/utils/analytics";

jest.mock("../../api", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

jest.mock("./aiWaiterCopy", () => ({
  getStarterQuestions: jest.fn().mockResolvedValue([]),
}));

jest.mock("@/api/aiWaiter", () => ({
  ...jest.requireActual("@/api/aiWaiter"),
  createAiWaiterSession: jest.fn().mockResolvedValue({
    session_token: "tst",
    greeting: "hi",
    expires_at: "2999-01-01T00:00:00Z",
  }),
  isSessionUnknownError: () => false,
  classifyChatError: () => "generic",
}));

jest.mock("react-hot-toast", () => ({
  toast: {
    success: jest.fn(),
    error: jest.fn(),
  },
}));

jest.mock("@/utils/analytics", () => ({
  trackEvent: jest.fn(),
}));

jest.mock("framer-motion", () => {
  const ReactActual = require("react");
  const MotionComponent = ReactActual.forwardRef(
    ({ children, ...props }: any, ref: any) => {
      const {
        initial: _initial,
        animate: _animate,
        exit: _exit,
        transition: _transition,
        whileHover: _whileHover,
        whileTap: _whileTap,
        ...domProps
      } = props;
      return ReactActual.createElement("div", { ...domProps, ref }, children);
    },
  );

  return {
    AnimatePresence: ({ children }: any) =>
      ReactActual.createElement(ReactActual.Fragment, null, children),
    useReducedMotion: () => false,
    motion: new Proxy(
      {},
      {
        get: () => MotionComponent,
      },
    ),
  };
});

jest.mock("@nextui-org/react", () => {
  const ReactActual = require("react");
  return {
    Modal: ({ children, isOpen }: any) =>
      isOpen
        ? ReactActual.createElement(
            "div",
            { role: "dialog", "aria-modal": "true" },
            children,
          )
        : null,
    ModalContent: ({ children, ...p }: any) =>
      ReactActual.createElement("div", p, children),
    ModalBody: ({ children, ...p }: any) =>
      ReactActual.createElement("div", p, children),
    ModalFooter: ({ children, ...p }: any) =>
      ReactActual.createElement("div", p, children),
    Card: ({ children, ...props }: { children: React.ReactNode }) => (
      <div {...props}>{children}</div>
    ),
    CardBody: ({ children, ...props }: { children: React.ReactNode }) => (
      <div {...props}>{children}</div>
    ),
    ScrollShadow: ReactActual.forwardRef(({ children }: any, ref: any) => (
      <div ref={ref}>{children}</div>
    )),
    Image: ({ alt }: { alt: string }) => <div aria-label={alt} />,
    Input: ReactActual.forwardRef(
      (
        { placeholder, value, onValueChange, onKeyDown, endContent }: any,
        ref: any,
      ) => (
        <label>
          <input
            ref={ref}
            placeholder={placeholder}
            value={value}
            onChange={(event) => onValueChange?.(event.target.value)}
            onKeyDown={onKeyDown}
          />
          {endContent}
        </label>
      ),
    ),
    Button: ({
      children,
      onPress,
      isDisabled,
      title,
      "aria-label": al,
    }: {
      children?: React.ReactNode;
      onPress?: () => void;
      isDisabled?: boolean;
      title?: string;
      "aria-label"?: string;
    }) => (
      <button
        type="button"
        disabled={isDisabled}
        onClick={onPress}
        title={title}
        aria-label={al}
      >
        {children}
      </button>
    ),
  };
});

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;
const mockedToast = toast as jest.Mocked<typeof toast>;
const mockedTrackEvent = jest.mocked(trackEvent);
const originalFetch = global.fetch;
let activeStreamController: ReadableStreamDefaultController<Uint8Array>;

beforeEach(() => {
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      activeStreamController = controller;
    },
  });
  global.fetch = jest.fn().mockResolvedValue({
    ok: true,
    status: 200,
    body,
  }) as jest.MockedFunction<typeof fetch>;
});

afterAll(() => {
  global.fetch = originalFetch;
});

describe("AiWaiter", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
  });

  it("does not fuzzy match bundle tool calls", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        parts: [
          {
            functionCall: {
              name: "add_to_cart",
              args: { item_name: "Family", quantity: 1, item_type: "bundle" },
            },
          },
        ],
      },
    });
    const onAddToCart = jest.fn();

    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[]}
        bundles={[{ id: 10, name: "Family Combo", price: 45, is_active: true }]}
        onAddToCart={onAddToCart}
        tableCode="TABLE123"
        isOrderingEnabled={true}
      />,
    );

    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByPlaceholderText(
      /Ask me anything about the menu/i,
    );
    fireEvent.change(input, { target: { value: "Add the family bundle" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      expect(mockedAxios.post).toHaveBeenCalled();
    });
    expect(onAddToCart).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/not sure which item you meant/i),
    ).toBeInTheDocument();
  });

  const v2CartResponse = (
    itemId: string,
    kind: "menu_item" | "bundle" = "menu_item",
  ) => ({
    version: 2,
    response_id: "waiter-response-route",
    answer: { format: "plain_text", content: "I'll help you continue." },
    sections: [],
    steps: [],
    actions: [
      {
        id: "add-cart-route",
        type: "add_cart_item",
        label: "Add Bowl",
        target: { kind, id: itemId, href: "", quantity: 1 },
        state: "ready",
        confirmation: "none",
        disabled_reason: null,
        expires_at: null,
      },
    ],
    sources: [],
    entities: [],
    follow_ups: [],
    workflow: null,
    notices: [],
    status: "complete",
  });

  it("uses the authoritative V2 stable ID once when translated names collide", async () => {
    const chosenId = "55555555-5555-4555-8555-555555555555";
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [
          {
            text: "legacy",
            functionCall: { name: "add_to_cart", args: { item_name: "Bowl" } },
          },
        ],
        response_v2: v2CartResponse(chosenId),
      },
    });
    const onAddToCart = jest.fn().mockReturnValue("applied");

    const menuData = [
      {
        items: [
          {
            id: "66666666-6666-4666-8666-666666666666",
            name: "Bowl",
            price: 10,
            is_available: true,
          },
          { id: chosenId, name: "Bowl", price: 12, is_available: true },
        ],
      },
    ];
    const waiter = (language = "en") => (
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={menuData}
        onAddToCart={onAddToCart}
        tableCode="TABLE123"
        language={language}
      />
    );
    const view = render(waiter());
    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByPlaceholderText(
      /what goes well with Bowl/i,
    );
    fireEvent.change(input, { target: { value: "Add the second Bowl" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => expect(onAddToCart).toHaveBeenCalledTimes(1));
    expect(onAddToCart).toHaveBeenCalledWith("Bowl", 12, 1, "", {
      itemType: "menu_item",
      menuItemId: chosenId,
    });
    expect(mockedToast.success).toHaveBeenCalledTimes(1);
    expect(mockedTrackEvent).toHaveBeenCalledWith(
      "assistant_action_completed",
      {
        surface: "waiter",
        contract_version: "v2",
        locale: "en",
        action_kind: "add_cart_item",
      },
    );
    expect(
      screen.queryByRole("button", { name: "Add Bowl" }),
    ).not.toBeInTheDocument();

    act(() => {
      const streamResponse = {
        ...v2CartResponse(chosenId),
        response_id: "stream-cart-echo",
        answer: { format: "plain_text", content: "Stream cart echo" },
      };
      activeStreamController.enqueue(
        new TextEncoder().encode(
          `event: message.created\ndata: ${JSON.stringify({
            id: 91,
            role: "assistant",
            content: "legacy",
            createdAt: Math.floor(Date.now() / 1000),
            response_v2: streamResponse,
          })}\n\n`,
        ),
      );
    });
    expect(await screen.findByText("Stream cart echo")).toBeInTheDocument();
    expect(onAddToCart).toHaveBeenCalledTimes(1);
    expect(
      mockedTrackEvent.mock.calls.filter(
        ([name]) => name === "assistant_action_completed",
      ),
    ).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Add Bowl" }),
    ).not.toBeInTheDocument();

    mockedAxios.get.mockResolvedValue({
      data: [
        {
          id: 92,
          role: "assistant",
          content: "legacy",
          createdAt: Math.floor(Date.now() / 1000) + 1,
          response_v2: {
            ...v2CartResponse(chosenId),
            response_id: "history-cart-echo",
            answer: { format: "plain_text", content: "History cart echo" },
          },
        },
      ],
    });
    view.rerender(waiter("es"));
    expect(await screen.findByText("History cart echo")).toBeInTheDocument();
    expect(onAddToCart).toHaveBeenCalledTimes(1);
    expect(
      mockedTrackEvent.mock.calls.filter(
        ([name]) => name === "assistant_action_completed",
      ),
    ).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Add Bowl" }),
    ).not.toBeInTheDocument();
  });

  it("does not downgrade a malformed present V2 to a valid legacy cart call", async () => {
    const malformed = {
      role: "model",
      parts: [
        {
          functionCall: {
            name: "add_to_cart",
            args: { item_name: "Burger" },
          },
        },
      ],
      response_v2: {
        ...v2CartResponse("77777777-7777-4777-8777-777777777777"),
        version: 1,
      },
    };
    mockedAxios.post
      .mockResolvedValueOnce({ data: malformed })
      .mockResolvedValueOnce({ data: malformed });
    const onAddToCart = jest.fn().mockReturnValue("applied");
    const waiter = (
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[
          {
            items: [
              {
                id: "77777777-7777-4777-8777-777777777777",
                name: "Burger",
                price: 12,
              },
            ],
          },
        ]}
        onAddToCart={onAddToCart}
        tableCode="TABLE123"
      />
    );
    const view = render(waiter);
    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByPlaceholderText(
      /what goes well with Burger/i,
    );
    fireEvent.change(input, { target: { value: "Add a Burger" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockedAxios.post).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.getByText(/trouble connecting/i)).toBeInTheDocument(),
    );
    expect(onAddToCart).not.toHaveBeenCalled();
    expect(mockedToast.success).not.toHaveBeenCalled();
    expect(
      mockedTrackEvent.mock.calls.filter(
        ([name]) => name === "assistant_render_fallback",
      ),
    ).toEqual([
      [
        "assistant_render_fallback",
        { surface: "waiter", locale: "en", contract_version: "v2" },
      ],
    ]);

    view.rerender(waiter);
    expect(
      mockedTrackEvent.mock.calls.filter(
        ([name]) => name === "assistant_render_fallback",
      ),
    ).toHaveLength(1);
    fireEvent.click(await screen.findByRole("button", { name: /retry/i }));
    await waitFor(() => expect(mockedAxios.post).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        mockedTrackEvent.mock.calls.filter(
          ([name]) => name === "assistant_render_fallback",
        ),
      ).toHaveLength(2),
    );
    expect(
      mockedTrackEvent.mock.calls.filter(
        ([name]) => name === "assistant_retry_requested",
      ),
    ).toHaveLength(1);
    expect(onAddToCart).not.toHaveBeenCalled();
  });

  it.each([
    [true, 1],
    [false, 0],
  ])(
    "honors live bundle active=%s for V2 stable-ID actions",
    async (isActive, expectedCalls) => {
      mockedAxios.post.mockResolvedValueOnce({
        data: {
          role: "model",
          parts: [{ text: "legacy" }],
          response_v2: v2CartResponse("10", "bundle"),
        },
      });
      const onAddToCart = jest.fn().mockReturnValue("deferred");
      render(
        <AiWaiter
          businessId={1}
          businessName="Cafe"
          menuData={[]}
          bundles={[
            { id: 10, name: "Family Combo", price: 45, is_active: isActive },
          ]}
          onAddToCart={onAddToCart}
          tableCode="TABLE123"
        />,
      );
      fireEvent.click(screen.getAllByRole("button")[0]);
      const input = await screen.findByPlaceholderText(/ask me anything/i);
      fireEvent.change(input, { target: { value: "Add the combo" } });
      fireEvent.keyDown(input, { key: "Enter" });
      await waitFor(() => expect(mockedAxios.post).toHaveBeenCalled());
      await waitFor(() =>
        expect(onAddToCart).toHaveBeenCalledTimes(expectedCalls),
      );
    },
  );

  it("adds a bundle only on exact normalized name match", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        parts: [
          {
            functionCall: {
              name: "add_to_cart",
              args: {
                item_name: "family combo",
                quantity: 1,
                item_type: "bundle",
              },
            },
          },
        ],
      },
    });
    const onAddToCart = jest.fn().mockReturnValue("applied");

    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[]}
        bundles={[{ id: 10, name: "Family Combo", price: 45, is_active: true }]}
        onAddToCart={onAddToCart}
        tableCode="TABLE123"
        isOrderingEnabled={true}
      />,
    );

    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByPlaceholderText(
      /Ask me anything about the menu/i,
    );
    fireEvent.change(input, { target: { value: "Add the family combo" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      expect(onAddToCart).toHaveBeenCalledWith("Family Combo", 45, 1, "", {
        itemType: "bundle",
        bundleId: 10,
      });
    });
    expect(mockedToast.success).toHaveBeenCalledTimes(1);
    expect(
      await screen.findByText(/Added 1x Family Combo to your cart/i),
    ).toBeInTheDocument();
  });

  it("uses continuation copy without claiming success when the landing defers", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        parts: [
          {
            functionCall: {
              name: "add_to_cart",
              args: {
                item_name: "Burger",
                quantity: 1,
                item_type: "menu_item",
              },
            },
          },
        ],
      },
    });
    const onAddToCart = jest.fn().mockReturnValue("deferred");

    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[
          {
            items: [
              {
                id: "f95b2ef7-0989-4389-a096-607e2b6f427c",
                name: "Burger",
                price: 12,
              },
            ],
          },
        ]}
        onAddToCart={onAddToCart}
        tableCode="TABLE123"
        isOrderingEnabled={true}
      />,
    );

    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByPlaceholderText(
      /what goes well with Burger/i,
    );
    fireEvent.change(input, { target: { value: "Add a burger" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(
      await screen.findByText(/opening the menu so you can continue/i),
    ).toBeInTheDocument();
    expect(mockedToast.success).not.toHaveBeenCalled();
    expect(
      screen.queryByText(/Added 1x Burger to your cart/i),
    ).not.toBeInTheDocument();
    expect(
      mockedTrackEvent.mock.calls.some(
        ([name]) => name === "assistant_action_completed",
      ),
    ).toBe(false);
  });

  it.each([
    ["rejected", jest.fn().mockReturnValue("rejected")],
    [
      "thrown",
      jest.fn(() => {
        throw new Error("route unavailable");
      }),
    ],
  ])(
    "fails closed when the cart callback is %s",
    async (_name, onAddToCart) => {
      mockedAxios.post.mockResolvedValueOnce({
        data: {
          parts: [
            {
              functionCall: {
                name: "add_to_cart",
                args: {
                  item_name: "Burger",
                  quantity: 1,
                  item_type: "menu_item",
                },
              },
            },
          ],
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
                  id: "f95b2ef7-0989-4389-a096-607e2b6f427c",
                  name: "Burger",
                  price: 12,
                },
              ],
            },
          ]}
          onAddToCart={onAddToCart}
          tableCode="TABLE123"
          isOrderingEnabled={true}
        />,
      );

      fireEvent.click(screen.getAllByRole("button")[0]);
      const input = await screen.findByPlaceholderText(
        /what goes well with Burger/i,
      );
      fireEvent.change(input, { target: { value: "Add a burger" } });
      fireEvent.keyDown(input, { key: "Enter" });

      expect(
        await screen.findByText(/trouble understanding/i),
      ).toBeInTheDocument();
      expect(mockedToast.success).not.toHaveBeenCalled();
      expect(
        mockedTrackEvent.mock.calls.some(
          ([name]) => name === "assistant_action_completed",
        ),
      ).toBe(false);
    },
  );

  it("tracks open, close, send, and retry transitions without content", async () => {
    mockedAxios.post
      .mockRejectedValueOnce(new Error("sentinel private prompt"))
      .mockResolvedValueOnce({
        data: {
          role: "model",
          parts: [{ text: "Recovered response" }],
        },
      });
    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="TABLE123"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /open.*assistant/i }));
    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_opened", {
      surface: "waiter",
      locale: "en",
    });
    const input = await screen.findByPlaceholderText(/ask me anything/i);
    fireEvent.change(input, { target: { value: "sentinel private prompt" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockedAxios.post).toHaveBeenCalledTimes(1));
    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_message_sent", {
      surface: "waiter",
      locale: "en",
      message_size_bucket: "0-80",
    });

    const retry = await screen.findByRole("button", { name: /retry/i });
    fireEvent.click(retry);
    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_retry_requested", {
      surface: "waiter",
      locale: "en",
    });
    expect(await screen.findByText("Recovered response")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /close.*assistant/i }));
    expect(mockedTrackEvent).toHaveBeenCalledWith("assistant_closed", {
      surface: "waiter",
      locale: "en",
    });
    expect(JSON.stringify(mockedTrackEvent.mock.calls)).not.toContain(
      "sentinel private prompt",
    );
  });

  it("reports a malformed restored V2 contract once across reopen without payload data", async () => {
    const malformed = {
      id: 77,
      role: "assistant",
      content: "sentinel unsafe downgrade",
      created_at: 77,
      response_v2: { version: 2, answer: { content: "broken" } },
    };
    mockedAxios.get.mockResolvedValue({ data: [malformed, malformed] });
    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="TABLE123"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /open.*assistant/i }));
    await waitFor(() =>
      expect(
        mockedTrackEvent.mock.calls.filter(
          ([name]) => name === "assistant_history_contract_mismatch",
        ),
      ).toHaveLength(1),
    );
    fireEvent.click(screen.getByRole("button", { name: /close.*assistant/i }));
    fireEvent.click(screen.getByRole("button", { name: /open.*assistant/i }));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));

    expect(
      mockedTrackEvent.mock.calls.filter(
        ([name]) => name === "assistant_history_contract_mismatch",
      ),
    ).toHaveLength(1);
    expect(
      mockedTrackEvent.mock.calls.filter(
        ([name]) => name === "assistant_render_fallback",
      ),
    ).toEqual([
      [
        "assistant_render_fallback",
        { surface: "waiter", locale: "en", contract_version: "v2" },
      ],
    ]);
    expect(JSON.stringify(mockedTrackEvent.mock.calls)).not.toMatch(
      /sentinel unsafe downgrade|broken|77/,
    );
  });

  it("does not add items when ordering is disabled", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        parts: [
          {
            function_call: {
              name: "add_to_cart",
              args: { item_name: "Burger", quantity: 2 },
            },
          },
        ],
      },
    });
    const onAddToCart = jest.fn();

    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[{ items: [{ id: "m1", name: "Burger", price: 12 }] }]}
        onAddToCart={onAddToCart}
        tableCode="TABLE123"
        isOrderingEnabled={false}
      />,
    );

    fireEvent.click(screen.getAllByRole("button")[0]);
    // Placeholder is now menu-aware: the first menu item (Burger) is substituted
    // into placeholderWithDish. (audit D6)
    const input = await screen.findByPlaceholderText(
      "E.g. what goes well with Burger?",
    );
    fireEvent.change(input, { target: { value: "Add two burgers" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      expect(mockedAxios.post).toHaveBeenCalled();
    });
    expect(onAddToCart).not.toHaveBeenCalled();
    expect(mockedToast.success).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Ordering is not available for this table/i),
    ).toBeInTheDocument();
  });

  it("does not promise cart adds in the footer when ordering is disabled (Closed Mode)", async () => {
    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[{ items: [{ id: "m1", name: "Burger", price: 12 }] }]}
        onAddToCart={jest.fn()}
        tableCode="TABLE123"
        isOrderingEnabled={false}
      />,
    );

    fireEvent.click(screen.getAllByRole("button")[0]);
    await screen.findByPlaceholderText(/what goes well with Burger/i);
    expect(
      screen.queryByText(/add items to your cart/i),
    ).not.toBeInTheDocument();
    // Menu Q&A footer stays honest while closed. The empty-thread greeting
    // bubble (#791) repeats the phrase, so match any occurrence.
    expect(
      screen.getAllByText(/ask me anything about the menu/i).length,
    ).toBeGreaterThan(0);
  });

  it("keeps hostile V2 Markdown inert on the real waiter response path", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy response" }],
        response_v2: {
          version: 2,
          response_id: "waiter-hostile-markdown",
          answer: {
            format: "markdown",
            content: `Safe [pricing](/privacy-policy) and [docs](https://docs.example.com/guide).

[Bad](javascript:alert(1))
![Tracking pixel](https://evil.example/pixel.png)
<iframe src="https://evil.example"></iframe>
<a href="/privacy-policy" onclick="alert(2)">raw handler</a>`,
          },
          sections: [],
          steps: [],
          actions: [],
          sources: [],
          entities: [],
          follow_ups: [],
          workflow: null,
          notices: [],
          status: "complete",
        },
      },
    });

    const { container } = render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="TABLE123"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByPlaceholderText(/ask me anything/i);
    fireEvent.change(input, { target: { value: "Show the safe links" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(
      await screen.findByRole("link", { name: "pricing" }),
    ).toHaveAttribute("href", "/privacy-policy");
    expect(screen.getByRole("link", { name: /docs/ })).toHaveAttribute(
      "rel",
      "noopener noreferrer",
    );
    expect(screen.getByText("Bad")).toBeVisible();
    expect(screen.queryByRole("link", { name: "Bad" })).not.toBeInTheDocument();
    expect(screen.getByText("Tracking pixel")).toBeVisible();
    expect(
      container.querySelector("img,iframe,[onclick]"),
    ).not.toBeInTheDocument();
  });
});

describe("AiWaiter history-fetch bootstrap (audit H2)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
    mockedAxios.post.mockResolvedValue({ data: { parts: [] } });
  });

  it("fetches history once per normalized locale and not on same-locale re-renders", async () => {
    const { rerender } = render(
      <AiWaiter
        businessId={2}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="t1"
        mode="ordering"
        language=""
      />,
    );
    // The chat opens via the floating button (isOpen is internal state).
    fireEvent.click(screen.getAllByRole("button")[0]);
    await waitFor(() =>
      expect(
        mockedAxios.get.mock.calls.filter(([url]) =>
          String(url).includes("/messages"),
        ),
      ).toHaveLength(1),
    );
    // Re-render with the guest locale hydrating "" → "es" (the production
    // trigger for a locale-sensitive history refresh. The bearer stays the
    // same, while content is fetched once for each normalized locale.
    rerender(
      <AiWaiter
        businessId={2}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="t1"
        mode="ordering"
        language="es"
      />,
    );
    rerender(
      <AiWaiter
        businessId={2}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="t1"
        mode="ordering"
        language="es"
      />,
    );

    await waitFor(() =>
      expect(
        mockedAxios.get.mock.calls.filter(([url]) =>
          String(url).includes("/messages"),
        ),
      ).toHaveLength(2),
    );
    const historyCalls = mockedAxios.get.mock.calls.filter(([url]) =>
      String(url).includes("/messages"),
    );
    expect(
      historyCalls.map(
        ([, config]) =>
          (config?.params as Record<string, unknown> | undefined)?.language,
      ),
    ).toEqual(["en", "es"]);
  });

  it("never sends an empty language param", async () => {
    render(
      <AiWaiter
        businessId={2}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="t1"
        mode="ordering"
        language={""}
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);

    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());
    for (const [, config] of mockedAxios.get.mock.calls) {
      const params = config?.params as Record<string, unknown> | undefined;
      if (params && "language" in params) {
        expect(params.language).not.toBe("");
      }
    }
  });
});

describe("AiWaiter SSE streaming", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
    mockedAxios.post.mockResolvedValue({ data: { parts: [] } });
  });

  it("appends a streamed staff-takeover reply to the transcript", async () => {
    render(
      <AiWaiter
        businessId={5}
        businessName="Testaurant"
        menuData={[]}
        tableCode="A1"
        onAddToCart={jest.fn()}
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    await waitFor(() => expect(global.fetch).toHaveBeenCalled());

    act(() => {
      const responseV2 = {
        version: 2,
        response_id: "staff-stream-7",
        answer: {
          format: "plain_text",
          content: "A waiter is on the way!",
        },
        sections: [],
        steps: [],
        actions: [],
        sources: [],
        entities: [],
        follow_ups: [],
        workflow: null,
        notices: [],
        status: "complete",
      };
      const message = {
        id: 7,
        role: "assistant",
        content: "legacy text must not replace the structured reply",
        createdAt: Math.floor(Date.now() / 1000),
        response_v2: responseV2,
      };
      activeStreamController.enqueue(
        new TextEncoder().encode(
          `event: message.created\ndata: ${JSON.stringify(message)}\n\n`,
        ),
      );
    });

    expect(
      await screen.findByText(/A waiter is on the way!/),
    ).toBeInTheDocument();
  });
});
