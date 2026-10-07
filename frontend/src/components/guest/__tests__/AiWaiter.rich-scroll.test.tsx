/** @jest-environment jsdom */

import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { AssistantResponse } from "@/types/assistant";
import type { AiWaiterTransportMessage } from "../useAiWaiterTransport";
import { AiWaiter } from "../AiWaiter";

jest.mock("next/image", () => ({
  __esModule: true,
  default: ({ alt, src }: { alt: string; src: string }) => (
    // eslint-disable-next-line @next/next/no-img-element -- Next Image test double.
    <img alt={alt} data-testid="rich-entity-image" src={src} />
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

let mockMessages: AiWaiterTransportMessage[] = [];
let mockIsLoading = false;
const mockSendMessage = jest.fn();
const mockAppendAssistantText = jest.fn();

jest.mock("../useAiWaiterSession", () => ({
  useAiWaiterSession: () => ({
    sessionToken: "session-token",
    ensureSession: jest.fn().mockResolvedValue("session-token"),
    recreateSession: jest.fn().mockResolvedValue("session-token"),
  }),
}));

jest.mock("../useAiWaiterTransport", () => ({
  useAiWaiterTransport: () => ({
    messages: mockMessages,
    isLoading: mockIsLoading,
    isPolling: false,
    lastFailedInput: null,
    sendMessage: mockSendMessage,
    retryFailedMessage: jest.fn(),
    appendAssistantText: mockAppendAssistantText,
    resetConversation: jest.fn(),
  }),
}));

jest.mock("../aiWaiterCopy", () => ({
  getStarterQuestions: jest.fn().mockResolvedValue([]),
}));

jest.mock("react-hot-toast", () => ({
  toast: Object.assign(jest.fn(), {
    success: jest.fn(),
    error: jest.fn(),
  }),
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
    motion: new Proxy({}, { get: () => MotionComponent }),
    useReducedMotion: () => true,
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
    ModalContent: ({ children, ...props }: any) => (
      <div {...props}>{children}</div>
    ),
    ModalBody: ({ children, ...props }: any) => (
      <div {...props}>{children}</div>
    ),
    Card: ({ children, ...props }: any) => <div {...props}>{children}</div>,
    CardBody: ({ children, ...props }: any) => <div {...props}>{children}</div>,
    ScrollShadow: ReactActual.forwardRef(
      ({ children, ...props }: any, ref: any) => (
        <div ref={ref} {...props}>
          {children}
        </div>
      ),
    ),
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
            onChange={(event) => onValueChange(event.target.value)}
            onKeyDown={onKeyDown}
          />
          {endContent}
        </label>
      ),
    ),
    Button: ({ children, onPress, onClick, isDisabled, ...props }: any) => {
      const domProps = { ...props };
      delete domProps["isIcon" + "Only"];
      delete domProps.isLoading;
      delete domProps.radius;
      delete domProps.size;
      delete domProps.variant;
      return (
        <button
          type="button"
          disabled={isDisabled}
          onClick={onPress ?? onClick}
          {...domProps}
        >
          {children}
        </button>
      );
    },
  };
});

function response(
  overrides: Partial<AssistantResponse> = {},
): AssistantResponse {
  return {
    version: 2,
    response_id: "waiter-rich-1",
    answer: {
      format: "markdown",
      content:
        "# Chef picks\n\n- Harvest Bowl\n- Soup\n\n[Menu details](/privacy-policy)",
    },
    sections: [
      {
        id: "section-picks",
        title: "Why these work",
        answer: "Fresh and balanced.",
        steps: [],
        action_ids: [],
        source_ids: ["source-bowl"],
        entity_ids: [],
      },
    ],
    steps: [],
    actions: [],
    sources: [
      {
        id: "source-bowl",
        type: "menu_item",
        title: "Harvest Bowl",
        href: null,
        origin: "menu_snapshot",
        retrieved_at: "2026-08-08T00:00:00Z",
      },
    ],
    entities: [],
    follow_ups: [],
    workflow: null,
    notices: [],
    status: "complete",
    ...overrides,
  };
}

const baseProps = {
  businessId: 7,
  businessName: "Cafe",
  menuData: [
    {
      items: [
        {
          id: "item-1",
          name: "Harvest Bowl",
          description: "",
          price: 12,
          is_available: true,
        },
      ],
    },
  ],
  onAddToCart: jest.fn(),
  tableCode: "T7",
};

function assistantMessage(
  value: AssistantResponse,
  delivery: AiWaiterTransportMessage["delivery"] = "direct",
): AiWaiterTransportMessage {
  return {
    id: 12,
    role: "assistant",
    content: value.answer.content,
    response: value,
    delivery,
  };
}

class TestResizeObserver implements ResizeObserver {
  static instances: TestResizeObserver[] = [];
  readonly observed = new Set<Element>();
  constructor(private readonly callback: ResizeObserverCallback) {
    TestResizeObserver.instances.push(this);
  }
  observe(target: Element) {
    this.observed.add(target);
  }
  unobserve(target: Element) {
    this.observed.delete(target);
  }
  disconnect() {
    this.observed.clear();
  }
  trigger() {
    this.callback([], this);
  }
}

describe("AiWaiter shared rich UI", () => {
  const originalResizeObserver = global.ResizeObserver;

  beforeEach(() => {
    mockMessages = [];
    mockIsLoading = false;
    mockSendMessage.mockReset().mockResolvedValue({ type: "ignored" });
    mockAppendAssistantText.mockReset();
    baseProps.onAddToCart.mockReset();
    TestResizeObserver.instances = [];
    global.ResizeObserver = TestResizeObserver;
    localStorage.clear();
    sessionStorage.clear();
  });

  afterAll(() => {
    global.ResizeObserver = originalResizeObserver;
  });

  it("renders shared Markdown, sections, and safe links without executable actions or a retrieval-source disclosure", async () => {
    mockMessages = [
      assistantMessage(
        response({
          sections: [
            {
              id: "section-picks",
              title: "Why these work",
              answer: "Fresh and balanced.",
              steps: [],
              action_ids: ["already-consumed"],
              source_ids: ["source-bowl"],
              entity_ids: [],
            },
          ],
          actions: [
            {
              id: "already-consumed",
              type: "add_cart_item",
              label: "Add Harvest Bowl",
              target: {
                kind: "menu_item",
                id: "item-1",
                href: "",
                quantity: 1,
              },
              state: "ready",
              confirmation: "none",
              disabled_reason: null,
              expires_at: null,
            },
          ],
        }),
      ),
    ];

    render(<AiWaiter {...baseProps} />);
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));

    expect(
      await screen.findByRole("heading", { level: 3, name: "Chef picks" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("list")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Why these work" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Menu details" })).toHaveAttribute(
      "href",
      "/privacy-policy",
    );
    expect(
      screen.queryByRole("button", { name: "Add Harvest Bowl" }),
    ).not.toBeInTheDocument();

    // A diner reads a waiter's answer, not the retrieval trail (#723).
    expect(
      screen.queryByRole("button", { name: "1 source used" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("region", { name: "1 source detail" }),
    ).not.toBeInTheDocument();
  });

  it("keeps a failed multiline draft and uses the native textarea", async () => {
    mockSendMessage.mockResolvedValueOnce({
      type: "error",
      error: "generic",
    });
    render(<AiWaiter {...baseProps} />);
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));

    const composer = await screen.findByRole("textbox", {
      name: "Message the AI assistant",
    });
    expect(composer.tagName).toBe("TEXTAREA");
    expect(composer).toHaveAttribute("maxLength", "500");
    expect(composer).toHaveAttribute(
      "placeholder",
      "E.g. what goes well with Harvest Bowl?",
    );
    expect(composer).toHaveFocus();
    fireEvent.change(composer, {
      target: { value: "first line\nsecond line" },
    });
    fireEvent.compositionStart(composer);
    fireEvent.keyDown(composer, {
      key: "Enter",
      isComposing: true,
      keyCode: 229,
    });
    expect(mockSendMessage).not.toHaveBeenCalled();
    fireEvent.compositionEnd(composer);
    fireEvent.click(screen.getByRole("button", { name: "Send message" }));

    await waitFor(() =>
      expect(mockSendMessage).toHaveBeenCalledWith("first line\nsecond line"),
    );
    expect(composer).toHaveValue("first line\nsecond line");
  });

  it("uses mobile multiline Enter semantics while keeping explicit send available", async () => {
    const originalWidth = window.innerWidth;
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 390,
    });
    mockSendMessage.mockResolvedValue({ type: "assistant" });
    render(<AiWaiter {...baseProps} />);
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));
    const composer = await screen.findByRole("textbox", {
      name: "Message the AI assistant",
    });
    fireEvent.change(composer, { target: { value: "keep composing" } });
    fireEvent.keyDown(composer, { key: "Enter" });
    expect(mockSendMessage).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Send message" }));
    await waitFor(() =>
      expect(mockSendMessage).toHaveBeenCalledWith("keep composing"),
    );
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: originalWidth,
    });
  });

  it("anchors a live long response, preserves user scroll-up across resize, and jumps to latest", async () => {
    const view = render(<AiWaiter {...baseProps} language="es" />);
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));
    await screen.findByRole("textbox", {
      name: "Escribe al asistente de IA",
    });
    const log = screen.getByRole("log", {
      name: "Conversación con el asistente de IA",
    }) as HTMLDivElement;
    let scrollHeight = 1_000;
    Object.defineProperties(log, {
      clientHeight: { configurable: true, value: 200 },
      scrollHeight: { configurable: true, get: () => scrollHeight },
    });
    log.getBoundingClientRect = jest.fn(
      () =>
        ({
          top: 20,
          bottom: 220,
          height: 200,
          left: 0,
          right: 400,
          width: 400,
          x: 0,
          y: 20,
          toJSON: () => ({}),
        }) as DOMRect,
    );
    log.scrollTo = jest.fn(
      (optionsOrX?: ScrollToOptions | number, y?: number) => {
        const top = typeof optionsOrX === "number" ? y : optionsOrX?.top;
        if (typeof top === "number") log.scrollTop = top;
      },
    ) as typeof log.scrollTo;
    const rectSpy = jest
      .spyOn(HTMLElement.prototype, "getBoundingClientRect")
      .mockImplementation(function (this: HTMLElement) {
        if (this.hasAttribute("data-assistant-response-start")) {
          return {
            top: 120,
            bottom: 720,
            height: 600,
            left: 0,
            right: 360,
            width: 360,
            x: 0,
            y: 120,
            toJSON: () => ({}),
          } as DOMRect;
        }
        return {
          top: 0,
          bottom: 0,
          height: 0,
          left: 0,
          right: 0,
          width: 0,
          x: 0,
          y: 0,
          toJSON: () => ({}),
        } as DOMRect;
      });

    mockMessages = [
      {
        role: "user",
        content: "Dame todos los detalles",
        clientNonce: "turn-long",
        delivery: "optimistic",
      },
    ];
    mockIsLoading = true;
    view.rerender(<AiWaiter {...baseProps} language="es" />);
    expect(log).toHaveAttribute("aria-busy", "true");

    mockMessages = [
      ...mockMessages,
      assistantMessage(response({ response_id: "long-live" })),
    ];
    mockIsLoading = false;
    view.rerender(<AiWaiter {...baseProps} language="es" />);

    await waitFor(() => expect(log.scrollTop).toBe(1_088));
    expect(log).toHaveAttribute("aria-busy", "false");
    expect(screen.getByRole("status")).toHaveTextContent("Respuesta lista");
    const jump = screen.getByRole("button", { name: "Ir a lo más reciente" });

    fireEvent.click(jump);
    expect(log.scrollTop).toBe(1_000);
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Ir a lo más reciente" }),
      ).not.toBeInTheDocument(),
    );

    fireEvent.wheel(log);
    log.scrollTop = 200;
    fireEvent.scroll(log);
    expect(
      await screen.findByRole("button", { name: "Ir a lo más reciente" }),
    ).toBeInTheDocument();
    scrollHeight = 1_200;
    act(() => {
      for (const observer of TestResizeObserver.instances) observer.trigger();
    });
    expect(log.scrollTop).toBe(200);
    rectSpy.mockRestore();
  });

  it("treats restored long history as restored without a completion announcement", async () => {
    mockMessages = [assistantMessage(response(), "history")];
    render(<AiWaiter {...baseProps} />);
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));

    const log = await screen.findByRole("log", {
      name: "AI assistant conversation",
    });
    expect(log).toHaveAttribute("aria-busy", "false");
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
    expect(
      screen.queryByRole("button", { name: "Jump to latest" }),
    ).not.toBeInTheDocument();
  });

  it("treats a delta-polled assistant response as live", async () => {
    const view = render(<AiWaiter {...baseProps} />);
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));
    mockMessages = [assistantMessage(response(), "poll")];
    view.rerender(<AiWaiter {...baseProps} />);

    expect(await screen.findByRole("status")).toHaveTextContent("Answer ready");
  });

  it("renders grounded media on every assistant turn, not only the latest", async () => {
    const entityResponse = (id: string, name: string, responseID: string) =>
      response({
        response_id: responseID,
        answer: { format: "plain_text", content: `${name} details` },
        sources: [
          {
            id: `source-${id}`,
            type: "menu_item",
            title: name,
            href: null,
            origin: "menu_snapshot",
            retrieved_at: "2026-08-08T00:00:00Z",
          },
        ],
        entities: [
          {
            id: `menu_item:${id}`,
            type: "menu_item",
            display_name: name,
            availability: "available",
            source_id: `source-${id}`,
          },
        ],
      });
    mockMessages = [
      assistantMessage(
        entityResponse("old", "Old Dish", "old-response"),
        "history",
      ),
      assistantMessage(
        entityResponse("new", "New Dish", "new-response"),
        "direct",
      ),
    ];
    render(
      <AiWaiter
        {...baseProps}
        menuData={[
          {
            items: [
              {
                id: "old",
                name: "Old Dish",
                price: 10,
                is_available: true,
                image: "https://payverge.io/media/old.jpg",
              },
              {
                id: "new",
                name: "New Dish",
                price: 11,
                is_available: true,
                image: "https://payverge.io/media/new.jpg",
              },
            ],
          },
        ]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));

    expect(await screen.findByText("Old Dish details")).toBeInTheDocument();
    expect(screen.getByText("New Dish details")).toBeInTheDocument();
    expect(screen.getAllByText("Old Dish").length).toBeGreaterThan(0);
    expect(screen.getAllByText("New Dish").length).toBeGreaterThan(0);
    // Scrolled-back turns keep their photo: a card that cannot show one is
    // dropped as a duplicate of the prose (#723), so gating media on the
    // latest turn would leave the history with no dish cards at all.
    const images = screen.getAllByTestId("rich-entity-image");
    expect(images.map((image) => image.getAttribute("src"))).toEqual([
      "https://payverge.io/media/old.jpg",
      "https://payverge.io/media/new.jpg",
    ]);
  });

  it("resets live scroll and announcements when closed and reopened into restored scope", async () => {
    const view = render(<AiWaiter {...baseProps} />);
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));
    mockMessages = [
      assistantMessage(
        response({ response_id: "live-before-close" }),
        "stream",
      ),
    ];
    view.rerender(<AiWaiter {...baseProps} />);
    expect(await screen.findByRole("status")).toHaveTextContent("Answer ready");
    const log = screen.getByRole("log", {
      name: "AI assistant conversation",
    });
    Object.defineProperties(log, {
      clientHeight: { configurable: true, value: 100 },
      scrollHeight: { configurable: true, value: 1_000 },
    });
    log.scrollTop = 100;
    fireEvent.wheel(log);
    fireEvent.scroll(log);
    expect(
      await screen.findByRole("button", { name: "Jump to latest" }),
    ).toBeInTheDocument();
    const observer = TestResizeObserver.instances.at(-1);

    fireEvent.click(screen.getByRole("button", { name: "Close AI assistant" }));
    expect(screen.queryByRole("log")).not.toBeInTheDocument();
    expect(observer?.observed.size).toBe(0);

    mockMessages = [
      assistantMessage(
        response({ response_id: "restored-new-scope" }),
        "history",
      ),
    ];
    view.rerender(<AiWaiter {...baseProps} tableCode="T8" />);
    fireEvent.click(screen.getByRole("button", { name: "Open AI assistant" }));
    expect(await screen.findByRole("status")).toBeEmptyDOMElement();
    expect(
      screen.queryByRole("button", { name: "Jump to latest" }),
    ).not.toBeInTheDocument();
  });
});
