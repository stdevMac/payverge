/** @jest-environment jsdom */

import React, { useState } from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ChatMessage } from "@/components/chat/types";
import type { AssistantMessageLabels } from "@/components/assistant/AssistantMessage";
import {
  parseAssistantResponse,
  type AssistantResponse,
} from "@/types/assistant";
import ChatShell from "../ChatShell";

jest.mock("framer-motion", () => {
  const React = require("react");
  const components = new Map<string, React.ComponentType<any>>();
  const motion = new Proxy(
    {},
    {
      get: (_target, tag: string) => {
        if (!components.has(tag)) {
          components.set(tag, ({ children, ...props }: any) => {
            const { initial, animate, exit, transition, layout, ...rest } =
              props;
            return React.createElement(tag, rest, children);
          });
        }
        return components.get(tag);
      },
    },
  );
  return {
    motion,
    AnimatePresence: ({ children }: any) => <>{children}</>,
    useReducedMotion: () => true,
  };
});

jest.mock("@nextui-org/react", () => {
  const React = require("react");
  return {
    Button: ({
      children,
      onPress,
      isDisabled,
      isLoading,
      startContent: _startContent,
      ...props
    }: any) => (
      <button
        type="button"
        disabled={isDisabled || isLoading}
        onClick={() => onPress?.()}
        {...props}
      >
        {children}
      </button>
    ),
    Input: ({ onValueChange, ...props }: any) => (
      <input
        {...props}
        onChange={(event) => onValueChange?.(event.target.value)}
      />
    ),
    ScrollShadow: ({ children, ...props }: any) => (
      <div {...props}>{children}</div>
    ),
  };
});

jest.mock("next/link", () => {
  return function MockLink({ children, href, ...props }: any) {
    return (
      <a href={href} {...props}>
        {children}
      </a>
    );
  };
});

type MutableMetrics = {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
};

class ResizeObserverMock {
  static instances: ResizeObserverMock[] = [];

  readonly observe = jest.fn();
  readonly unobserve = jest.fn();
  readonly disconnect = jest.fn();

  constructor(private readonly callback: ResizeObserverCallback) {
    ResizeObserverMock.instances.push(this);
  }

  fire(target: Element): void {
    this.callback(
      [{ target } as ResizeObserverEntry],
      this as unknown as ResizeObserver,
    );
  }
}

function installMetrics(
  element: HTMLElement,
  initial: Partial<MutableMetrics>,
): MutableMetrics {
  const metrics: MutableMetrics = {
    scrollTop: initial.scrollTop ?? 0,
    scrollHeight: initial.scrollHeight ?? 600,
    clientHeight: initial.clientHeight ?? 300,
  };
  Object.defineProperties(element, {
    scrollTop: {
      configurable: true,
      get: () => metrics.scrollTop,
      set: (value: number) => {
        metrics.scrollTop = value;
      },
    },
    scrollHeight: {
      configurable: true,
      get: () => metrics.scrollHeight,
    },
    clientHeight: {
      configurable: true,
      get: () => metrics.clientHeight,
    },
  });
  return metrics;
}

function setRect(element: HTMLElement, top: number, height: number): void {
  jest.spyOn(element, "getBoundingClientRect").mockReturnValue({
    top,
    bottom: top + height,
    height,
    left: 0,
    right: 300,
    width: 300,
    x: 0,
    y: top,
    toJSON: () => ({}),
  });
}

const labels: AssistantMessageLabels = {
  usedSources: (count) => `Fuentes usadas: ${count}`,
  sourcesRegion: (count) => `Detalle de ${count} fuentes`,
  sourceOrigin: (origin) => `Origen: ${origin}`,
  externalSource: (hostname) => `Fuente externa: ${hostname}`,
  actions: "Acciones",
  steps: "Pasos",
  entities: "Elementos relacionados",
  entityAvailability: (availability) => `Disponibilidad: ${availability}`,
  followUps: "Preguntas sugeridas",
  notices: "Avisos",
  noticeKind: (kind) => `Aviso: ${kind}`,
  status: (status) => `Estado: ${status}`,
  workflowProgress: ({ id, current, total }) => `${id}: ${current}/${total}`,
  disabledActionReason: (reason) => reason ?? "No disponible",
  renderError: "No se pudo mostrar la respuesta.",
};

function response(
  overrides: Partial<AssistantResponse> = {},
): AssistantResponse {
  return parseAssistantResponse({
    version: 2,
    response_id: "reply-1",
    answer: {
      format: "markdown",
      content: "Read the [plans](/privacy-policy).",
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
    ...overrides,
  });
}

const baseProps = {
  title: "Ayuda",
  messages: [] as ChatMessage[],
  input: "",
  onInputChange: jest.fn(),
  onSend: jest.fn(),
  onClose: jest.fn(),
  placeholder: "Escribí tu pregunta",
  sendLabel: "Enviar",
  loadingLabel: "Pensando",
  dialogLabel: "Asistente",
  closeLabel: "Cerrar",
  portal: false,
  conversationLabel: "Conversación",
  jumpToLatestLabel: "Ir a la respuesta más reciente",
  completionAnnouncement: "Respuesta lista",
  assistantLabels: labels,
};

const originalResizeObserver = globalThis.ResizeObserver;
const originalMatchMedia = window.matchMedia;
const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  ResizeObserverMock.instances = [];
  globalThis.ResizeObserver =
    ResizeObserverMock as unknown as typeof ResizeObserver;
  window.matchMedia = jest.fn().mockReturnValue({
    matches: true,
    media: "(prefers-reduced-motion: reduce)",
    onchange: null,
    addListener: jest.fn(),
    removeListener: jest.fn(),
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    dispatchEvent: jest.fn(),
  });
});

afterEach(() => {
  if (originalResizeObserver) {
    globalThis.ResizeObserver = originalResizeObserver;
  } else {
    Reflect.deleteProperty(globalThis, "ResizeObserver");
  }
  window.matchMedia = originalMatchMedia;
  if (originalScrollIntoView) {
    HTMLElement.prototype.scrollIntoView = originalScrollIntoView;
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  }
  jest.restoreAllMocks();
});

describe("ChatShell shared conversation primitives", () => {
  it("prefers response V2 rich text and only adapts legacy fields when response is absent", () => {
    const v2 = response();
    const legacyAction = {
      label: "Open contact",
      href: "/terms-and-conditions",
      kind: "navigate" as const,
    };
    const { rerender } = render(
      <ChatShell
        {...baseProps}
        messages={[
          {
            role: "assistant",
            content: "Legacy answer that must not render",
            actions: [legacyAction],
            steps: ["Legacy step"],
            followUps: ["Legacy follow-up"],
            response: v2,
          },
        ]}
      />,
    );

    expect(screen.getByRole("link", { name: "plans" })).toHaveAttribute(
      "href",
      "/privacy-policy",
    );
    expect(screen.getByText("Estado: complete")).toBeVisible();
    expect(
      screen.queryByText("Legacy answer that must not render"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Legacy step")).not.toBeInTheDocument();

    rerender(
      <ChatShell
        {...baseProps}
        messages={[
          {
            role: "assistant",
            content: "Legacy **plain** answer",
            actions: [legacyAction],
            steps: ["Legacy step"],
            followUps: ["Legacy follow-up"],
          },
        ]}
      />,
    );

    // Legacy answers route through AssistantRichText markdown (NEW-7).
    expect(screen.getByText("plain").tagName).toBe("STRONG");
    expect(screen.queryByText("Legacy **plain** answer")).not.toBeInTheDocument();
    expect(screen.getByText("Legacy step")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open contact" })).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Legacy follow-up" }),
    ).toBeVisible();
  });

  it("renders legacy assistant markdown **bold** without raw asterisks (NEW-7)", () => {
    render(
      <ChatShell
        {...baseProps}
        assistantLabels={undefined}
        messages={[
          {
            role: "assistant",
            content:
              "Payverge offers **Operations** at **USD 99/mes** with a **14-day free trial**.",
          },
        ]}
      />,
    );

    expect(screen.getByText("Operations").tagName).toBe("STRONG");
    expect(screen.getByText("USD 99/mes").tagName).toBe("STRONG");
    expect(screen.getByText("14-day free trial").tagName).toBe("STRONG");
    expect(screen.queryByText(/\*\*Operations\*\*/)).not.toBeInTheDocument();
    // Source/section labels must not repeat the brand shell title.
    expect(screen.queryByText(/^Ayuda:/)).not.toBeInTheDocument();
    // Status chip is a friendly label, not the brand title (dedupe).
    expect(screen.getByText("Complete")).toBeVisible();
    expect(screen.getAllByText("Ayuda")).toHaveLength(1); // header title only
  });

  it("renders native and adapted responses through AssistantMessage without leaking internal enums", async () => {
    const user = userEvent.setup();
    const nativeResponse = response({
      response_id: "native-metadata",
      sections: [
        {
          id: "localized-details",
          title: "Detalle localizado",
          answer: "Contenido de la sección",
          steps: [],
          action_ids: [],
          source_ids: [],
          entity_ids: ["localized-plan"],
        },
      ],
      steps: ["Review the localized plan"],
      actions: [
        {
          id: "open-pricing",
          type: "navigate",
          label: "Abrir precios",
          target: {
            kind: "payverge_page",
            id: "pricing",
            href: "/privacy-policy",
          },
          state: "ready",
          confirmation: "none",
          disabled_reason: null,
          expires_at: null,
        },
      ],
      sources: [
        {
          id: "pricing-source",
          type: "pricing_registry",
          title: "Guía de precios",
          href: "/privacy-policy",
          origin: "pricing_registry",
          retrieved_at: "2026-08-08T12:00:00Z",
        },
      ],
      entities: [
        {
          id: "localized-plan",
          type: "offer",
          display_name: "Plan localizado",
          availability: "internal_availability_state",
          source_id: "pricing-source",
        },
      ],
      workflow: {
        id: "internal_workflow_id",
        step_index: 0,
        step_total: 2,
      },
      notices: [
        {
          id: "availability-note",
          kind: "internal_availability_warning",
          message: "Disponibilidad sujeta a confirmación",
        },
      ],
      status: "blocked",
    });
    const onActionClick = jest.fn();

    const { container } = render(
      <ChatShell
        {...baseProps}
        assistantLabels={undefined}
        messages={[
          {
            role: "assistant",
            content: "native fallback must not replace V2 metadata",
            response: nativeResponse,
          },
          {
            role: "assistant",
            content: "Legacy answer",
            steps: ["Legacy preserved step"],
          },
        ]}
        onActionClick={onActionClick}
      />,
    );

    expect(container.querySelectorAll("article")).toHaveLength(2);
    expect(screen.getByText("Detalle localizado")).toBeVisible();
    expect(screen.getByText("Plan localizado")).toBeVisible();
    expect(screen.getByText("Review the localized plan")).toBeVisible();
    expect(screen.getByText("Legacy preserved step")).toBeVisible();
    expect(
      screen.getByText("Disponibilidad sujeta a confirmación"),
    ).toBeVisible();
    expect(screen.queryByText("blocked")).not.toBeInTheDocument();
    expect(
      screen.queryByText("internal_availability_warning"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("internal_availability_state"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("internal_workflow_id")).not.toBeInTheDocument();
    expect(screen.getByText("1/2")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Abrir precios" }));
    expect(onActionClick).toHaveBeenCalledWith({
      label: "Abrir precios",
      href: "/privacy-policy",
      kind: "navigate",
      disabled: false,
      disabled_reason: undefined,
    });
    await user.click(screen.getByRole("button", { name: /Guía de precios/ }));
    expect(
      screen.getByRole("link", { name: "Guía de precios" }),
    ).toHaveAttribute("href", "/privacy-policy");
  });

  it("shows follow-ups only on the latest assistant response while preserving historical records", () => {
    const { container } = render(
      <ChatShell
        {...baseProps}
        messages={[
          {
            role: "assistant",
            content: "Historical answer",
            steps: ["Historical preserved step"],
            actions: [
              {
                label: "Historical preserved action",
                href: "/terms-and-conditions",
                kind: "navigate",
              },
            ],
            followUps: ["Historical follow-up"],
          },
          {
            role: "assistant",
            content: "Latest answer",
            followUps: ["Latest follow-up"],
          },
        ]}
      />,
    );

    expect(container.querySelectorAll("article")).toHaveLength(2);
    expect(screen.getByText("Historical preserved step")).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Historical preserved action" }),
    ).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Historical follow-up" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Latest follow-up" }),
    ).toBeVisible();
  });

  it("keeps shared action, follow-up, feedback, welcome, retry, help, and restart callbacks", async () => {
    const user = userEvent.setup();
    const onActionClick = jest.fn();
    const onSuggestionClick = jest.fn();
    const onVote = jest.fn();
    const onRetry = jest.fn();
    const onNewConversation = jest.fn();
    const action = {
      id: "action-pricing",
      type: "navigate" as const,
      label: "Abrir precios",
      target: {
        kind: "payverge_page" as const,
        id: "pricing",
        href: "/privacy-policy",
      },
      state: "ready",
      confirmation: "none",
      disabled_reason: null,
      expires_at: null,
    };

    const { rerender } = render(
      <ChatShell
        {...baseProps}
        messages={[
          {
            role: "assistant",
            content: "legacy",
            response: response({
              actions: [action],
              follow_ups: [
                { id: "follow-1", label: "Contame más", prompt: "Más" },
              ],
            }),
          },
        ]}
        onActionClick={onActionClick}
        onSuggestionClick={onSuggestionClick}
        feedback={{ onVote, upLabel: "Sirvió", downLabel: "No sirvió" }}
        onNewConversation={onNewConversation}
        newConversationLabel="Nueva conversación"
      />,
    );

    await user.click(screen.getByRole("button", { name: "Abrir precios" }));
    expect(onActionClick).toHaveBeenCalledWith({
      label: "Abrir precios",
      href: "/privacy-policy",
      kind: "navigate",
      disabled: false,
      disabled_reason: undefined,
    });
    await user.click(screen.getByRole("button", { name: "Contame más" }));
    expect(onSuggestionClick).toHaveBeenCalledWith("Más");
    await user.click(screen.getByRole("button", { name: "Sirvió" }));
    expect(onVote).toHaveBeenCalledWith("up");
    await user.click(
      screen.getByRole("button", { name: "Nueva conversación" }),
    );
    expect(onNewConversation).toHaveBeenCalledTimes(1);

    rerender(
      <ChatShell
        {...baseProps}
        messages={[]}
        welcomeMessage="Bienvenido"
        welcomeHint="Elegí una opción"
        suggestions={["Ver planes"]}
        onSuggestionClick={onSuggestionClick}
        errorMessage={null}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Ver planes" }));
    expect(onSuggestionClick).toHaveBeenCalledWith("Ver planes");

    rerender(
      <ChatShell
        {...baseProps}
        messages={[]}
        errorMessage="No se pudo responder"
        onRetry={onRetry}
        retryLabel="Reintentar"
        errorHelp={<a href="/terms-and-conditions">Contacto</a>}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("link", { name: "Contacto" })).toHaveAttribute(
      "href",
      "/terms-and-conditions",
    );
  });

  it("preserves V2-only actions for a capable handler and never coerces them to legacy navigation", async () => {
    const user = userEvent.setup();
    const onAssistantAction = jest.fn();
    const onActionClick = jest.fn();
    const addCartAction = {
      id: "add-bowl",
      type: "add_cart_item" as const,
      label: "Agregar bowl",
      target: {
        kind: "menu_item" as const,
        id: "bowl-1",
        href: "",
        quantity: 1,
      },
      state: "ready",
      confirmation: "none",
      disabled_reason: null,
      expires_at: null,
    };
    const messages: ChatMessage[] = [
      {
        role: "assistant",
        content: "legacy",
        response: response({ actions: [addCartAction] }),
      },
    ];
    const { rerender } = render(
      <ChatShell
        {...baseProps}
        messages={messages}
        onAssistantAction={onAssistantAction}
        onActionClick={onActionClick}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Agregar bowl" }));
    expect(onAssistantAction).toHaveBeenCalledWith(addCartAction);
    expect(onActionClick).not.toHaveBeenCalled();

    onAssistantAction.mockClear();
    rerender(
      <ChatShell
        {...baseProps}
        messages={messages}
        onActionClick={onActionClick}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Agregar bowl" }));
    expect(onAssistantAction).not.toHaveBeenCalled();
    expect(onActionClick).not.toHaveBeenCalled();
  });

  it("keeps confirmation-required actions enabled without dispatching them through the lossy legacy callback", async () => {
    const user = userEvent.setup();
    const onAssistantAction = jest.fn();
    const onActionClick = jest.fn();
    const confirmationAction = {
      id: "confirm-pricing",
      type: "navigate" as const,
      label: "Confirmar precios",
      target: {
        kind: "payverge_page" as const,
        id: "pricing",
        href: "/privacy-policy",
      },
      state: "ready",
      confirmation: "required",
      disabled_reason: null,
      expires_at: null,
    };
    const messages: ChatMessage[] = [
      {
        role: "assistant",
        content: "legacy",
        response: response({ actions: [confirmationAction] }),
      },
    ];
    const { rerender } = render(
      <ChatShell
        {...baseProps}
        messages={messages}
        onActionClick={onActionClick}
      />,
    );

    const actionButton = screen.getByRole("button", {
      name: "Confirmar precios",
    });
    expect(actionButton).toBeEnabled();
    await user.click(actionButton);
    expect(onActionClick).not.toHaveBeenCalled();

    rerender(
      <ChatShell
        {...baseProps}
        messages={messages}
        onAssistantAction={onAssistantAction}
        onActionClick={onActionClick}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Confirmar precios" }),
    ).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Confirmar precios" }));
    expect(onAssistantAction).toHaveBeenCalledWith(confirmationAction);
    expect(onActionClick).not.toHaveBeenCalled();
  });

  it("marks a live user turn terminal when loading ends in an error without an assistant response", () => {
    const messages: ChatMessage[] = [
      { role: "user", content: "Question that fails" },
    ];
    const { rerender } = render(
      <ChatShell {...baseProps} messages={messages} loading />,
    );

    expect(screen.getByRole("log", { name: "Conversación" })).toHaveAttribute(
      "aria-busy",
      "true",
    );

    rerender(
      <ChatShell
        {...baseProps}
        messages={messages}
        loading={false}
        errorMessage="No se pudo responder"
      />,
    );
    expect(screen.getByRole("log", { name: "Conversación" })).toHaveAttribute(
      "aria-busy",
      "false",
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryByText("Respuesta lista")).not.toBeInTheDocument();
  });

  it("uses a controlled multiline composer for desktop, Shift+Enter, IME, and mobile", async () => {
    const sent: string[] = [];

    function Harness({ mobile = false }: { mobile?: boolean }) {
      const [value, setValue] = useState("");
      return (
        <ChatShell
          {...baseProps}
          input={value}
          onInputChange={setValue}
          onSend={() => sent.push(value)}
          mobileComposer={mobile}
        />
      );
    }

    const { rerender } = render(<Harness />);
    const textarea = screen.getByRole("textbox", {
      name: "Escribí tu pregunta",
    });
    expect(textarea.tagName).toBe("TEXTAREA");
    expect(textarea).toHaveAttribute("rows", "1");

    fireEvent.change(textarea, { target: { value: "Primera\nSegunda" } });
    expect(textarea).toHaveValue("Primera\nSegunda");
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true });
    expect(sent).toEqual([]);

    fireEvent.compositionStart(textarea);
    fireEvent.keyDown(textarea, { key: "Enter", keyCode: 229 });
    fireEvent.compositionEnd(textarea);
    expect(sent).toEqual([]);

    await act(async () => {
      fireEvent.keyDown(textarea, { key: "Enter" });
      await Promise.resolve();
    });
    expect(sent).toEqual(["Primera\nSegunda"]);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Enviar" })).toBeVisible(),
    );

    rerender(<Harness mobile />);
    const mobileTextarea = screen.getByRole("textbox", {
      name: "Escribí tu pregunta",
    });
    fireEvent.change(mobileTextarea, { target: { value: "Móvil" } });
    fireEvent.keyDown(mobileTextarea, { key: "Enter" });
    expect(sent).toEqual(["Primera\nSegunda"]);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Enviar" }));
      await Promise.resolve();
    });
    expect(sent).toEqual(["Primera\nSegunda", "Móvil"]);
  });

  it("aligns a live long answer at its start and never scrolls the document", () => {
    const windowScroll = jest.spyOn(window, "scrollTo").mockImplementation();
    const sentinelScroll = jest.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: sentinelScroll,
    });
    const messages: ChatMessage[] = [
      { role: "user", content: "Explain everything" },
      { role: "assistant", content: "legacy", response: response() },
    ];
    const { rerender } = render(
      <ChatShell {...baseProps} messages={messages} loading />,
    );
    const scroller = screen.getByRole("log", { name: "Conversación" });
    const responseStart = screen.getByTestId("assistant-response-start");
    const metrics = installMetrics(scroller, {
      scrollTop: 500,
      scrollHeight: 1800,
      clientHeight: 400,
    });
    setRect(scroller, 100, 400);
    setRect(responseStart, 620, 760);

    rerender(<ChatShell {...baseProps} messages={messages} loading={false} />);

    expect(metrics.scrollTop).toBe(1008);
    expect(
      screen.getByRole("button", {
        name: "Ir a la respuesta más reciente",
      }),
    ).toBeVisible();
    expect(windowScroll).not.toHaveBeenCalled();
    expect(sentinelScroll).not.toHaveBeenCalled();
  });

  it("preserves a reader's scroll-up position through completion and late growth", () => {
    const messages: ChatMessage[] = [
      { role: "user", content: "Question" },
      { role: "assistant", content: "legacy", response: response() },
    ];
    const { rerender } = render(
      <ChatShell {...baseProps} messages={messages} loading />,
    );
    const scroller = screen.getByRole("log", { name: "Conversación" });
    const responseStart = screen.getByTestId("assistant-response-start");
    const metrics = installMetrics(scroller, {
      scrollTop: 300,
      scrollHeight: 1000,
      clientHeight: 400,
    });
    setRect(scroller, 0, 400);
    setRect(responseStart, 240, 120);

    fireEvent.wheel(scroller);
    fireEvent.scroll(scroller);
    rerender(<ChatShell {...baseProps} messages={messages} loading={false} />);
    expect(metrics.scrollTop).toBe(300);
    expect(
      screen.getByRole("button", {
        name: "Ir a la respuesta más reciente",
      }),
    ).toBeVisible();

    metrics.scrollHeight = 1400;
    act(() => ResizeObserverMock.instances[0]?.fire(responseStart));
    expect(metrics.scrollTop).toBe(300);
  });
});
