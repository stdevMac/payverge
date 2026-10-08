/**
 * @jest-environment jsdom
 */

import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import OpsAssistantWidget from "./OpsAssistantWidget";
import { askOpsAssistant, submitOpsFeedback } from "@/api/opsAssistant";
import { axiosInstance } from "@/api/tools/instance";
import { adaptLegacyResponse, type AssistantResponse } from "@/types/assistant";

// Jest only lets a jest.mock() factory reference out-of-scope vars whose names
// begin with "mock" (case-insensitive), so the shared push spy is named
// accordingly.
const mockPushSpy = jest.fn();
let mockLocale = "test";
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPushSpy }),
}));

jest.mock("@/api/opsAssistant", () => ({
  askOpsAssistant: jest.fn(),
  writeDirectorHandoff: jest.fn(),
  listOpsThreadMessages: jest.fn().mockResolvedValue({ messages: [] }),
  submitOpsFeedback: jest.fn().mockResolvedValue({}),
  readOpsSession: jest.fn().mockReturnValue(null),
  writeOpsSession: jest.fn(),
  newClientRequestId: jest.fn(() => "req-test-1"),
  OpsHistoryContractMismatchError: class extends Error {},
  OpsResponseContractMismatchError: class extends Error {},
}));

jest.mock("@/utils/analytics", () => ({
  trackEvent: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale }),
  getTranslation: (key: string, locale: string) => {
    if (key === "opsAssistant.suggestions") return [];
    if (locale === "test") return key.split(".").at(-1) ?? key;
    const messages = jest.requireActual(
      `@/i18n/messages/${locale}/opsAssistant.json`,
    ) as Record<string, unknown>;
    return key
      .split(".")
      .slice(1)
      .reduce<unknown>(
        (value, segment) =>
          typeof value === "object" && value !== null
            ? (value as Record<string, unknown>)[segment]
            : undefined,
        messages,
      );
  },
}));

jest.mock("framer-motion", () => {
  // Preserve the real module (NextUI's <Button> ripple relies on `m.span` and
  // LazyMotion) while stubbing `motion.*` to plain elements and flattening
  // LazyMotion so the ripple's async loader stays inside React's act() boundary.
  const actual = jest.requireActual("framer-motion") as Record<string, unknown>;
  const ReactMock = jest.requireActual("react") as typeof React;
  const Motion = new Proxy(
    {},
    {
      get: (_target, tag: string) =>
        ReactMock.forwardRef<HTMLElement, any>(
          (
            {
              animate: _animate,
              exit: _exit,
              initial: _initial,
              transition: _transition,
              whileHover: _whileHover,
              whileTap: _whileTap,
              ...props
            },
            ref,
          ) => ReactMock.createElement(tag, { ...props, ref }),
        ),
    },
  );

  return {
    ...actual,
    LazyMotion: ({ children }: { children: React.ReactNode }) => (
      <>{children}</>
    ),
    AnimatePresence: ({ children }: { children: React.ReactNode }) => (
      <>{children}</>
    ),
    motion: Motion,
    useReducedMotion: () => true,
  };
});

const mockAskOpsAssistant = jest.mocked(askOpsAssistant);
const mockSubmitOpsFeedback = jest.mocked(submitOpsFeedback);


const opsTopics = [
  {
    id: "menu-add-item",
    title: "Agregar un producto al menú",
    answer: "Gestioná los productos desde Menú.",
    step: "Elegí una categoría y completá los datos del producto.",
    label: "Abrir Menú",
    tab: "menu",
  },
  {
    id: "tables-create-qr",
    title: "Crear una mesa y su QR",
    answer: "Creá la mesa desde Mesas.",
    step: "Guardá la mesa y descargá su código QR.",
    label: "Abrir Mesas",
    tab: "tables",
  },
  {
    id: "plugins-connect",
    title: "Conectar una integración de pagos",
    answer: "Revisá las integraciones disponibles.",
    step: "Conectá el proveedor y comprobá su estado.",
    label: "Abrir Integraciones",
    tab: "plugins",
  },
  {
    id: "inventory-stock",
    title: "Revisar el inventario",
    answer: "Consultá las existencias desde Inventario.",
    step: "Revisá el stock actual antes de registrar movimientos.",
    label: "Abrir Inventario",
    tab: "inventory",
  },
  {
    id: "ai-waiter-configure",
    title: "Configurar Camarero IA",
    answer: "Configurá el asistente desde Camarero IA.",
    step: "Revisá el idioma y las opciones de atención.",
    label: "Abrir Camarero IA",
    tab: "ai-waiter",
  },
] as const;

const fiveSectionResponse: AssistantResponse = {
  version: 2,
  response_id: "ops-response-five",
  answer: {
    format: "markdown",
    content: "Encontré cinco guías operativas verificadas.",
  },
  sections: opsTopics.map((topic) => ({
    id: topic.id,
    title: topic.title,
    answer: topic.answer,
    steps: [topic.step],
    action_ids: [`navigate:${topic.id}`],
    source_ids: [`guide:${topic.id}`],
    entity_ids: [],
  })),
  steps: [],
  actions: opsTopics.map((topic) => ({
    id: `navigate:${topic.id}`,
    type: "navigate" as const,
    label: topic.label,
    target: {
      kind: "dashboard_area" as const,
      id: topic.tab,
      href: `/business/7/dashboard?tab=${topic.tab}`,
    },
    state: "ready",
    confirmation: "none",
    disabled_reason: null,
    expires_at: null,
  })),
  sources: opsTopics.map((topic) => ({
    id: `guide:${topic.id}`,
    type: "dashboard_guide" as const,
    title: topic.title,
    href: null,
    origin: "ops_guide_catalog",
    retrieved_at: "2026-08-08T12:00:00Z",
  })),
  entities: [],
  follow_ups: [
    {
      id: "continuar-configuracion",
      label: "Ver la siguiente guía",
      prompt: "Mostrame la siguiente guía de configuración.",
    },
  ],
  workflow: null,
  notices: [],
  status: "complete",
};

const legacyV2 = (
  responseID: string,
  response: Parameters<typeof adaptLegacyResponse>[1],
) => adaptLegacyResponse(responseID, response);

function sourceOnlyResponse(count: number): AssistantResponse {
  return {
    ...fiveSectionResponse,
    response_id: `ops-source-count-${count}`,
    sections: [],
    actions: [],
    sources: fiveSectionResponse.sources.slice(0, count),
    follow_ups: [],
  };
}

function renderWidget() {
  return render(
    <OpsAssistantWidget
      businessId={7}
      activeTab="overview"
      dock="content"
    />,
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function openAndSend(text: string) {
  fireEvent.click(
    screen.getByRole("button", { name: /^(fabLabel|Help|Ayuda)$/ }),
  );
  fireEvent.change(
    screen.getByPlaceholderText(/^(placeholder|How do I…\?|¿Cómo puedo…\?)$/),
    {
      target: { value: text },
    },
  );
  fireEvent.click(screen.getByRole("button", { name: /^(send|Send|Enviar)$/ }));
}

beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn();
});

beforeEach(() => {
  mockLocale = "test";
  mockAskOpsAssistant.mockReset();
  const api = jest.requireMock("@/api/opsAssistant") as {
    readOpsSession: jest.Mock;
    listOpsThreadMessages: jest.Mock;
  };
  api.readOpsSession.mockReturnValue(null);
  api.listOpsThreadMessages.mockResolvedValue({ messages: [] });
  mockAskOpsAssistant.mockResolvedValue({
    contract_version: "v2",
    thread: { id: 1, title: "Thread" },
    assistant_message: { id: 1, content: "Here you go." },
    response: {
      answer: "Here you go.",
      steps: [],
      actions: [],
      follow_ups: [],
    },
    response_v2: legacyV2("ops-test-default", {
      answer: "Here you go.",
      steps: [],
      actions: [],
      follow_ups: [],
    }),
    usage: { model: "test", latency_ms: 1 },
  });
});

afterEach(() => {
  jest.clearAllMocks();
});

describe("OpsAssistantWidget", () => {
  it("emits one bounded render fallback per malformed direct V2 attempt across retry", async () => {
    mockLocale = "en";
    const api = jest.requireMock("@/api/opsAssistant") as {
      OpsResponseContractMismatchError: new () => Error;
    };
    const analytics = jest.requireMock("@/utils/analytics") as {
      trackEvent: jest.Mock;
    };
    mockAskOpsAssistant.mockRejectedValueOnce(
      new api.OpsResponseContractMismatchError(),
    );

    const view = renderWidget();
    openAndSend("sentinel private direct prompt");
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(
      analytics.trackEvent.mock.calls.filter(
        ([name]) => name === "assistant_render_fallback",
      ),
    ).toEqual([
      [
        "assistant_render_fallback",
        { surface: "ops", locale: "en", contract_version: "v2" },
      ],
    ]);
    expect(JSON.stringify(analytics.trackEvent.mock.calls)).not.toContain(
      "sentinel private direct prompt",
    );

    view.rerender(
      <OpsAssistantWidget
        businessId={7}
        activeTab="overview"
        dock="content"
      />,
    );
    expect(
      analytics.trackEvent.mock.calls.filter(
        ([name]) => name === "assistant_render_fallback",
      ),
    ).toHaveLength(1);
    mockAskOpsAssistant.mockRejectedValueOnce(
      new api.OpsResponseContractMismatchError(),
    );
    fireEvent.click(screen.getByRole("button", { name: /try again/i }));
    await waitFor(() => expect(mockAskOpsAssistant).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        analytics.trackEvent.mock.calls.filter(
          ([name]) => name === "assistant_render_fallback",
        ),
      ).toHaveLength(2),
    );
    expect(
      analytics.trackEvent.mock.calls.filter(
        ([name]) => name === "assistant_retry_requested",
      ),
    ).toHaveLength(1);
  });

  it("does not emit render fallback for a direct network error", async () => {
    mockLocale = "en";
    const analytics = jest.requireMock("@/utils/analytics") as {
      trackEvent: jest.Mock;
    };
    mockAskOpsAssistant.mockRejectedValueOnce(new Error("sentinel network"));

    renderWidget();
    openAndSend("sentinel private network prompt");
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());

    expect(
      analytics.trackEvent.mock.calls.filter(
        ([name]) => name === "assistant_render_fallback",
      ),
    ).toHaveLength(0);
    expect(JSON.stringify(analytics.trackEvent.mock.calls)).not.toMatch(
      /sentinel|network prompt/,
    );
  });

  it("preserves history mismatch and emits one render fallback for malformed restored V2", async () => {
    mockLocale = "en";
    const api = jest.requireMock("@/api/opsAssistant") as {
      readOpsSession: jest.Mock;
      listOpsThreadMessages: jest.Mock;
      OpsHistoryContractMismatchError: new () => Error;
    };
    const analytics = jest.requireMock("@/utils/analytics") as {
      trackEvent: jest.Mock;
    };
    api.readOpsSession.mockReturnValue({ threadId: 55 });
    api.listOpsThreadMessages.mockRejectedValueOnce(
      new api.OpsHistoryContractMismatchError(),
    );

    renderWidget();
    await waitFor(() => expect(api.listOpsThreadMessages).toHaveBeenCalled());
    await waitFor(() =>
      expect(
        analytics.trackEvent.mock.calls.filter(([name]) =>
          [
            "assistant_history_contract_mismatch",
            "assistant_render_fallback",
          ].includes(name),
        ),
      ).toHaveLength(2),
    );
    expect(
      analytics.trackEvent.mock.calls.filter(
        ([name]) => name === "assistant_history_contract_mismatch",
      ),
    ).toHaveLength(1);
    expect(
      analytics.trackEvent.mock.calls.filter(
        ([name]) => name === "assistant_render_fallback",
      ),
    ).toEqual([
      [
        "assistant_render_fallback",
        { surface: "ops", locale: "en", contract_version: "v2" },
      ],
    ]);
  });

  it("renders five localized V2 sections and preserves them after action close and reopen", async () => {
    mockAskOpsAssistant.mockResolvedValueOnce({
      contract_version: "v2",
      thread: { id: 31, title: "Guías operativas" },
      assistant_message: {
        id: 77,
        content: fiveSectionResponse.answer.content,
      },
      response: {
        answer: fiveSectionResponse.answer.content,
        steps: [],
        actions: [],
        follow_ups: [],
      },
      response_v2: fiveSectionResponse,
      usage: { model: "deterministic-guide", latency_ms: 1 },
    });

    renderWidget();
    openAndSend("Necesito ayuda con cinco áreas");

    for (const topic of opsTopics) {
      expect(
        await screen.findByRole("heading", { name: topic.title }),
      ).toBeInTheDocument();
      expect(screen.getByText(topic.step)).toBeInTheDocument();
    }
    expect(screen.queryByText("Open Menu")).not.toBeInTheDocument();
    expect(screen.queryByText("Choose a category")).not.toBeInTheDocument();
    expect(screen.queryByText("menu-add-item")).not.toBeInTheDocument();
    expect(screen.queryByText("guide:menu-add-item")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Abrir Inventario" }));
    expect(mockPushSpy).toHaveBeenCalledWith(
      "/business/7/dashboard?tab=inventory",
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole("button", { name: "fabLabel" }));
    for (const topic of opsTopics) {
      expect(
        screen.getByRole("heading", { name: topic.title }),
      ).toBeInTheDocument();
    }
  });

  it("restores the same V2 structure from history and preserves its assistant feedback id", async () => {
    const api = jest.requireMock("@/api/opsAssistant") as {
      readOpsSession: jest.Mock;
      listOpsThreadMessages: jest.Mock;
    };
    api.readOpsSession.mockReturnValue({ threadId: 55 });
    api.listOpsThreadMessages.mockResolvedValue({
      messages: [
        { id: 70, role: "user", content: "Necesito cinco guías" },
        {
          id: 88,
          role: "assistant",
          content: fiveSectionResponse.answer.content,
          response_v2: fiveSectionResponse,
        },
      ],
    });

    renderWidget();
    fireEvent.click(screen.getByRole("button", { name: "fabLabel" }));

    for (const topic of opsTopics) {
      expect(
        await screen.findByRole("heading", { name: topic.title }),
      ).toBeInTheDocument();
    }
    fireEvent.click(screen.getByRole("button", { name: "feedbackUp" }));
    await waitFor(() =>
      expect(mockSubmitOpsFeedback).toHaveBeenCalledWith(7, 88, "up"),
    );
  });

  it("rejects a present invalid response_v2 instead of adapting V1", async () => {
    const actualAPI = jest.requireActual(
      "@/api/opsAssistant",
    ) as typeof import("@/api/opsAssistant");
    const post = jest.spyOn(axiosInstance, "post").mockResolvedValueOnce({
      data: {
        thread: { id: 1, title: "Thread" },
        assistant_message: { id: 2, content: "Legacy answer" },
        response: {
          answer: "Legacy answer",
          steps: [],
          actions: [],
          follow_ups: [],
        },
        response_v2: { version: 2, response_id: "broken" },
        usage: { model: "test", latency_ms: 1 },
      },
    });

    await expect(
      actualAPI.askOpsAssistant(7, { message: "hello" }),
    ).rejects.toThrow();
    post.mockRestore();
  });

  it("rejects malformed present V2 history instead of falling back to readable legacy content", async () => {
    const actualAPI = jest.requireActual(
      "@/api/opsAssistant",
    ) as typeof import("@/api/opsAssistant");
    const get = jest.spyOn(axiosInstance, "get").mockResolvedValueOnce({
      data: {
        messages: [
          {
            id: 2,
            role: "assistant",
            content: "Legacy content must not mask corrupt structured history.",
            structured_response: {
              answer:
                "Legacy content must not mask corrupt structured history.",
              steps: [],
              actions: [],
              follow_ups: [],
            },
            response_v2: { version: 2, response_id: "broken-history" },
          },
        ],
      },
    });

    await expect(actualAPI.listOpsThreadMessages(7, 4)).rejects.toThrow();
    get.mockRestore();
  });

  it.each([
    ["en", 1, "1 source used", "1 verified source"],
    ["en", 2, "2 sources used", "2 verified sources"],
    ["es", 1, "1 fuente utilizada", "1 fuente verificada"],
    ["es", 2, "2 fuentes utilizadas", "2 fuentes verificadas"],
  ])(
    "uses grammatical %s source labels for count %i",
    async (locale, count, disclosure, region) => {
      mockLocale = locale;
      const responseV2 = sourceOnlyResponse(count);
      mockAskOpsAssistant.mockResolvedValueOnce({
        contract_version: "v2",
        thread: { id: 31, title: "Sources" },
        assistant_message: { id: 77, content: responseV2.answer.content },
        response: {
          answer: responseV2.answer.content,
          steps: [],
          actions: [],
          follow_ups: [],
        },
        response_v2: responseV2,
        usage: { model: "deterministic-guide", latency_ms: 1 },
      });

      renderWidget();
      openAndSend("Sources");

      const sourceButton = await screen.findByRole("button", {
        name: disclosure,
      });
      fireEvent.click(sourceButton);
      expect(screen.getByRole("region", { name: region })).toBeInTheDocument();
    },
  );

  it("does not navigate a native action outside the current business canonical Ops routes", async () => {
    const foreignResponse: AssistantResponse = {
      ...fiveSectionResponse,
      response_id: "ops-response-foreign-action",
      sections: [fiveSectionResponse.sections[0]],
      actions: [
        {
          ...fiveSectionResponse.actions[0],
          target: {
            ...fiveSectionResponse.actions[0].target,
            href: "/business/999/dashboard?tab=menu",
          },
        },
      ],
      sources: [fiveSectionResponse.sources[0]],
    };
    mockAskOpsAssistant.mockResolvedValueOnce({
      contract_version: "v2",
      thread: { id: 31, title: "Guía" },
      assistant_message: { id: 77, content: foreignResponse.answer.content },
      response: {
        answer: foreignResponse.answer.content,
        steps: [],
        actions: [],
        follow_ups: [],
      },
      response_v2: foreignResponse,
      usage: { model: "deterministic-guide", latency_ms: 1 },
    });

    renderWidget();
    openAndSend("Abrí el menú");

    fireEvent.click(await screen.findByRole("button", { name: "Abrir Menú" }));
    expect(mockPushSpy).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("ignores stale history after the business changes", async () => {
    const api = jest.requireMock("@/api/opsAssistant") as {
      readOpsSession: jest.Mock;
      listOpsThreadMessages: jest.Mock;
    };
    const firstHistory = deferred<{
      messages: Array<{ id: number; role: string; content: string }>;
    }>();
    api.readOpsSession.mockImplementation((businessId: number) => ({
      threadId: businessId,
    }));
    api.listOpsThreadMessages.mockImplementation((businessId: number) => {
      if (businessId === 7) return firstHistory.promise;
      return Promise.resolve({
        messages: [
          { id: 8, role: "user", content: "Consulta del negocio ocho" },
        ],
      });
    });

    const view = renderWidget();
    view.rerender(
      <OpsAssistantWidget
        businessId={8}
        activeTab="overview"
        dock="content"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "fabLabel" }));
    expect(
      await screen.findByText("Consulta del negocio ocho"),
    ).toBeInTheDocument();

    await act(async () => {
      firstHistory.resolve({
        messages: [
          {
            id: 7,
            role: "user",
            content: "Consulta obsoleta del negocio siete",
          },
        ],
      });
      await firstHistory.promise;
    });
    expect(
      screen.queryByText("Consulta obsoleta del negocio siete"),
    ).not.toBeInTheDocument();
  });

  it("drops an in-flight ask and clears its transient state when the business changes", async () => {
    const ask = deferred<Awaited<ReturnType<typeof askOpsAssistant>>>();
    mockAskOpsAssistant.mockReturnValueOnce(ask.promise);

    const view = renderWidget();
    openAndSend("Consulta obsoleta del negocio siete");
    expect(screen.getAllByText("thinking").length).toBeGreaterThan(0);

    view.rerender(
      <OpsAssistantWidget
        businessId={8}
        activeTab="overview"
        dock="content"
      />,
    );
    await waitFor(() =>
      expect(screen.queryAllByText("thinking")).toHaveLength(0),
    );
    expect(screen.getByPlaceholderText("placeholder")).toHaveValue("");

    await act(async () => {
      ask.resolve({
        contract_version: "v2",
        thread: { id: 31, title: "Old business" },
        assistant_message: {
          id: 77,
          content: fiveSectionResponse.answer.content,
        },
        response: {
          answer: fiveSectionResponse.answer.content,
          steps: [],
          actions: [],
          follow_ups: [],
        },
        response_v2: fiveSectionResponse,
        usage: { model: "deterministic-guide", latency_ms: 1 },
      });
      await ask.promise;
    });

    expect(
      screen.queryByText(fiveSectionResponse.answer.content),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Abrir Menú" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("does not offer retry for an in-flight ask that fails after the business changes", async () => {
    let rejectAsk!: (error: Error) => void;
    const ask = new Promise<Awaited<ReturnType<typeof askOpsAssistant>>>(
      (_, reject) => {
        rejectAsk = reject;
      },
    );
    mockAskOpsAssistant.mockReturnValueOnce(ask);

    const view = renderWidget();
    openAndSend("Consulta que no debe reintentarse");
    view.rerender(
      <OpsAssistantWidget
        businessId={8}
        activeTab="overview"
        dock="content"
      />,
    );
    await act(async () => {
      rejectAsk(new Error("old_business_failed"));
      await expect(ask).rejects.toThrow("old_business_failed");
    });

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "retry" }),
    ).not.toBeInTheDocument();
  });

  it("shows a visible Help label on the closed FAB pill (#61)", () => {
    renderWidget();
    const fab = screen.getByRole("button", { name: "fabLabel" });
    expect(fab).toHaveAttribute("data-testid", "ops-assistant-fab");
    expect(fab).toHaveTextContent("fabLabel");
    expect(fab.className).toMatch(/rounded-full/);
    expect(fab.className).toMatch(/px-/);
  });

  it("hides the floating Help FAB when hideFab is set (#61)", () => {
    render(
      <OpsAssistantWidget
        businessId={7}
        activeTab="overview"
        dock="content"
        hideFab
      />,
    );
    expect(screen.queryByTestId("ops-assistant-fab")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "fabLabel" }),
    ).not.toBeInTheDocument();
  });

  it("still opens from openSignal when the FAB is hidden", async () => {
    render(
      <OpsAssistantWidget
        businessId={7}
        activeTab="overview"
        openSignal={1}
        dock="content"
        hideFab
      />,
    );
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.queryByTestId("ops-assistant-fab")).not.toBeInTheDocument();
  });

  it("renders a labeled modal dialog when opened", () => {
    renderWidget();
    fireEvent.click(screen.getByRole("button", { name: "fabLabel" }));

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveAttribute("aria-label", "title");
  });

  it("consumes each nonzero open signal once across close and locale rerenders", async () => {
    const analytics = jest.requireMock("@/utils/analytics") as {
      trackEvent: jest.Mock;
    };
    const view = render(
      <OpsAssistantWidget
        businessId={7}
        activeTab="overview"
        openSignal={1}
        dock="content"
      />,
    );

    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "close" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );

    mockLocale = "en";
    view.rerender(
      <OpsAssistantWidget
        businessId={7}
        activeTab="overview"
        openSignal={1}
        dock="content"
      />,
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(
      analytics.trackEvent.mock.calls.filter(
        ([event]) => event === "assistant_opened",
      ),
    ).toHaveLength(1);
  });

  it("does not refetch history on locale-only rerenders and uses the new locale for future analytics", async () => {
    const api = jest.requireMock("@/api/opsAssistant") as {
      readOpsSession: jest.Mock;
      listOpsThreadMessages: jest.Mock;
    };
    const analytics = jest.requireMock("@/utils/analytics") as {
      trackEvent: jest.Mock;
    };
    api.readOpsSession.mockReturnValue({ threadId: 55 });
    api.listOpsThreadMessages.mockResolvedValue({ messages: [] });

    const view = renderWidget();
    await waitFor(() =>
      expect(api.listOpsThreadMessages).toHaveBeenCalledTimes(1),
    );

    mockLocale = "en";
    view.rerender(
      <OpsAssistantWidget
        businessId={7}
        activeTab="overview"
        dock="content"
      />,
    );
    await act(async () => {
      await Promise.resolve();
    });

    expect(api.listOpsThreadMessages).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Help" }));
    expect(analytics.trackEvent).toHaveBeenCalledWith("assistant_opened", {
      surface: "ops",
      locale: "en",
    });
  });

  it("closes on Escape", async () => {
    renderWidget();
    fireEvent.click(screen.getByRole("button", { name: "fabLabel" }));

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });

    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("offers a retry that resends the last message after a failure", async () => {
    mockAskOpsAssistant.mockRejectedValueOnce(new Error("ask_failed"));

    renderWidget();
    openAndSend("How do I add a menu item?");

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(mockAskOpsAssistant).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "retry" }));

    await waitFor(() => expect(mockAskOpsAssistant).toHaveBeenCalledTimes(2));
    expect(mockAskOpsAssistant).toHaveBeenLastCalledWith(
      7,
      expect.objectContaining({ message: "How do I add a menu item?" }),
    );
    await waitFor(() =>
      expect(screen.getByText("Here you go.")).toBeInTheDocument(),
    );
  });

  it("pushes the route and collapses the panel when a navigate action is clicked", async () => {
    mockAskOpsAssistant.mockResolvedValueOnce({
      contract_version: "v2",
      thread: { id: 1, title: "Thread" },
      assistant_message: { id: 1, content: "Open the menu." },
      response: {
        answer: "Open the menu.",
        steps: [],
        actions: [
          {
            label: "Open Menu",
            href: "/business/7/dashboard?tab=menu",
            kind: "navigate",
            disabled: false,
          },
        ],
        follow_ups: [],
      },
      response_v2: legacyV2("ops-test-menu", {
        answer: "Open the menu.",
        steps: [],
        actions: [
          {
            label: "Open Menu",
            href: "/business/7/dashboard?tab=menu",
            kind: "navigate",
            disabled: false,
          },
        ],
        follow_ups: [],
      }),
      usage: { model: "test", latency_ms: 1 },
    });

    renderWidget();
    openAndSend("How do I edit the menu?");

    const actionBtn = await screen.findByRole("button", { name: /Open Menu/ });
    fireEvent.click(actionBtn);

    expect(mockPushSpy).toHaveBeenCalledWith("/business/7/dashboard?tab=menu");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("renders structured steps and sends a clicked follow-up", async () => {
    mockAskOpsAssistant.mockResolvedValueOnce({
      contract_version: "v2",
      thread: { id: 1, title: "Thread" },
      assistant_message: { id: 1, content: "Open the menu builder." },
      response: {
        answer: "Open the menu builder.",
        steps: ["Choose a category, then select Add item."],
        actions: [],
        follow_ups: ["How do I add modifiers?"],
      },
      response_v2: legacyV2("ops-test-steps", {
        answer: "Open the menu builder.",
        steps: ["Choose a category, then select Add item."],
        actions: [],
        follow_ups: ["How do I add modifiers?"],
      }),
      usage: { model: "test", latency_ms: 1 },
    });

    renderWidget();
    openAndSend("How do I add a menu item?");

    expect(
      await screen.findByText("Choose a category, then select Add item."),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "How do I add modifiers?" }),
    );

    await waitFor(() => expect(mockAskOpsAssistant).toHaveBeenCalledTimes(2));
    expect(mockAskOpsAssistant).toHaveBeenLastCalledWith(
      7,
      expect.objectContaining({ message: "How do I add modifiers?" }),
    );
  });

  it("sends a stable client_request_id and reuses it on retry", async () => {
    const { newClientRequestId } = jest.requireMock("@/api/opsAssistant") as {
      newClientRequestId: jest.Mock;
    };
    newClientRequestId.mockReturnValue("req-stable-42");
    mockAskOpsAssistant.mockRejectedValueOnce(new Error("ask_failed"));

    renderWidget();
    openAndSend("How do I add a menu item?");

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(mockAskOpsAssistant).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        message: "How do I add a menu item?",
        client_request_id: "req-stable-42",
      }),
    );

    mockAskOpsAssistant.mockResolvedValueOnce({
      contract_version: "v2",
      thread: { id: 1, title: "Thread" },
      assistant_message: { id: 9, content: "Here you go." },
      response: {
        answer: "Here you go.",
        steps: [],
        actions: [],
        follow_ups: [],
      },
      response_v2: legacyV2("ops-test-retry", {
        answer: "Here you go.",
        steps: [],
        actions: [],
        follow_ups: [],
      }),
      usage: { model: "test", latency_ms: 1 },
    });
    fireEvent.click(screen.getByRole("button", { name: "retry" }));

    await waitFor(() => expect(mockAskOpsAssistant).toHaveBeenCalledTimes(2));
    expect(mockAskOpsAssistant).toHaveBeenLastCalledWith(
      7,
      expect.objectContaining({
        message: "How do I add a menu item?",
        client_request_id: "req-stable-42",
      }),
    );
  });

  it("restores an active thread from sessionStorage on mount", async () => {
    const api = jest.requireMock("@/api/opsAssistant") as {
      readOpsSession: jest.Mock;
      listOpsThreadMessages: jest.Mock;
    };
    api.readOpsSession.mockReturnValue({ threadId: 55 });
    api.listOpsThreadMessages.mockResolvedValue({
      messages: [
        { id: 1, role: "user", content: "Where is the menu?" },
        { id: 2, role: "assistant", content: "Open Menu → Categories." },
      ],
    });

    renderWidget();
    fireEvent.click(screen.getByRole("button", { name: "fabLabel" }));

    expect(
      await screen.findByText("Open Menu → Categories."),
    ).toBeInTheDocument();
    expect(api.listOpsThreadMessages).toHaveBeenCalledWith(7, 55);
  });

  it("starts a new conversation and clears the session", async () => {
    const api = jest.requireMock("@/api/opsAssistant") as {
      writeOpsSession: jest.Mock;
    };
    mockAskOpsAssistant.mockResolvedValueOnce({
      contract_version: "v2",
      thread: { id: 3, title: "Thread" },
      assistant_message: { id: 1, content: "Answer." },
      response: { answer: "Answer.", steps: [], actions: [], follow_ups: [] },
      response_v2: legacyV2("ops-test-new-conversation", {
        answer: "Answer.",
        steps: [],
        actions: [],
        follow_ups: [],
      }),
      usage: { model: "test", latency_ms: 1 },
    });

    renderWidget();
    openAndSend("Hello");
    await waitFor(() =>
      expect(screen.getByText("Answer.")).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole("button", { name: "newConversation" }));
    expect(api.writeOpsSession).toHaveBeenCalledWith(7, undefined);
    expect(screen.queryByText("Answer.")).not.toBeInTheDocument();
  });

  it("submits feedback for the latest assistant message when thumbs up is clicked", async () => {
    mockAskOpsAssistant.mockResolvedValueOnce({
      contract_version: "v2",
      thread: { id: 3, title: "Thread" },
      assistant_message: { id: 42, content: "Answer." },
      response: { answer: "Answer.", steps: [], actions: [], follow_ups: [] },
      response_v2: legacyV2("ops-test-feedback", {
        answer: "Answer.",
        steps: [],
        actions: [],
        follow_ups: [],
      }),
      usage: { model: "test", latency_ms: 1 },
    });

    renderWidget();
    openAndSend("Hello");
    await waitFor(() =>
      expect(screen.getByText("Answer.")).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole("button", { name: "feedbackUp" }));

    await waitFor(() =>
      expect(mockSubmitOpsFeedback).toHaveBeenCalledWith(7, 42, "up"),
    );
  });
});
