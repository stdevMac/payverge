/** @jest-environment jsdom */
import React from "react";
import { renderToString } from "react-dom/server";
import { hydrateRoot, type Root } from "react-dom/client";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import {
  aiWaiterTranscriptKey,
  readStoredTranscript,
} from "@/components/guest/aiWaiterTranscriptStore";
import { axiosInstance } from "../../../api";

/**
 * First-render snapshots of the real transport hook. A lazy
 * `useState(() => readStoredTranscript())` restores during that first client
 * call whenever jsdom has localStorage — the production SSR bug never would.
 * Layout-effect restore must therefore show up on a *later* paint.
 */
const sagePaints = (): string[] => {
  const holder = globalThis as { __sagePaints?: string[] };
  if (!holder.__sagePaints) holder.__sagePaints = [];
  return holder.__sagePaints;
};

jest.mock("@/components/guest/useAiWaiterTransport", () => {
  const actual = jest.requireActual(
    "@/components/guest/useAiWaiterTransport",
  ) as typeof import("@/components/guest/useAiWaiterTransport");
  return {
    ...actual,
    useAiWaiterTransport: (
      opts: Parameters<typeof actual.useAiWaiterTransport>[0],
    ) => {
      const result = actual.useAiWaiterTransport(opts);
      const paints = (globalThis as { __sagePaints?: string[] }).__sagePaints;
      paints?.push(result.messages.map((message) => message.content).join("||"));
      return result;
    },
  };
});

jest.mock("next/image", () => ({
  __esModule: true,
  default: ({ alt, src }: { alt: string; src: string }) => (
    // eslint-disable-next-line @next/next/no-img-element -- Next Image test double.
    <img alt={alt} data-testid="rendered-image" src={src} />
  ),
}));
jest.mock("../../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: (key: string) => key }),
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
// Persist only. Panel geometry is asserted against real NextUI slots /
// a real Modal in aiWaiterPanel.layout.test.ts and
// AiWaiter.panelViewport.test.tsx — not this hand-rolled dialog.
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

const GREETING = "Hi! I'm Sage — ask me anything about the menu.";
const HOUR_MS = 60 * 60 * 1000;
/**
 * Shipped English copy for `aiWaiter.resumedTranscriptNotice`. The panel reads
 * the real bundle for its own language and falls back to en.json, so asserting
 * the literal proves the key exists and is wired to the restore.
 */
const RESUMED_NOTICE =
  "Sage started a new conversation and won't remember the messages above.";

/** Every new session persists its own hello, so history always opens with one. */
const greetingRow = () => ({
  id: 1,
  role: "assistant",
  content: GREETING,
  created_at: 1_754_000_000,
});

const answerPayload = (answer: string) => ({
  version: 2,
  response_id: `resume-${answer.length}`,
  answer: { format: "markdown", content: answer },
  sections: [],
  steps: [],
  actions: [],
  sources: [],
  entities: [],
  follow_ups: [],
  workflow: null,
  notices: [],
  status: "complete",
});

const menu = [{ items: [] as Array<Record<string, unknown>> }];

function mountPanel() {
  const view = render(
    <AiWaiter
      businessId={1}
      businessName="Cafe"
      menuData={menu}
      bundles={[] as any}
      onAddToCart={jest.fn()}
      tableCode="UJYDMVCJ5D"
    />,
  );
  fireEvent.click(screen.getAllByRole("button")[0]);
  return view;
}

/** Opens Sage the way a diner does and waits for the thread to settle. */
async function openPanel() {
  const view = mountPanel();
  await screen.findByRole("textbox", { name: "Message the AI assistant" });
  await screen.findByText(GREETING);
  return view;
}

async function ask(question: string, answer: string) {
  mockedAxios.post.mockResolvedValueOnce({
    data: {
      role: "model",
      parts: [{ text: answer }],
      response_v2: answerPayload(answer),
    },
  });
  const input = screen.getByRole("textbox", {
    name: "Message the AI assistant",
  });
  fireEvent.change(input, { target: { value: question } });
  fireEvent.keyDown(input, { key: "Enter" });
  await screen.findByText(answer);
}

const SCOPE = "1\u0000UJYDMVCJ5D\u0000ordering\u0000en";

const withNoWindowStorage = <T,>(run: () => T): T => {
  const descriptor = Object.getOwnPropertyDescriptor(window, "localStorage");
  if (!descriptor) throw new Error("jsdom localStorage descriptor missing");
  const error = console.error;
  jest.spyOn(console, "error").mockImplementation((...args) => {
    if (
      String(args[0] ?? "").includes(
        "useLayoutEffect does nothing on the server",
      )
    ) {
      return;
    }
    error.apply(console, args);
  });
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    get() {
      throw new Error("ssr-no-window");
    },
  });
  try {
    return run();
  } finally {
    Object.defineProperty(window, "localStorage", descriptor);
    (console.error as jest.Mock).mockRestore();
  }
};

const TWO_TURN_CONTENTS = [
  "what is good here?",
  "The burger is excellent.",
  "and a dessert?",
  "The flan is wonderful.",
];

const Panel = () => (
  <AiWaiter
    businessId={1}
    businessName="Cafe"
    menuData={menu}
    bundles={[] as any}
    onAddToCart={jest.fn()}
    tableCode="UJYDMVCJ5D"
  />
);

describe("Guest Sage panel persistence and placement (#724)", () => {
  let clockOffset = 0;

  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    (globalThis as { __sagePaints?: string[] }).__sagePaints = [];
    clockOffset = 0;
    const realNow = Date.now();
    jest.spyOn(Date, "now").mockImplementation(() => realNow + clockOffset);
    mockedAxios.get.mockResolvedValue({ data: [greetingRow()] });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("reopens on the same transcript after the diner refreshes the page", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    first.unmount();

    // A refresh: the bearer token was memory-only, so this is a brand-new
    // server session whose history holds nothing but its own greeting.
    await openPanel();

    expect(
      await screen.findByText("The burger is excellent."),
    ).toBeInTheDocument();
    expect(screen.getByText("what is good here?")).toBeInTheDocument();
  });

  it("keeps a two-turn thread after refresh", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    await ask("and a dessert?", "The flan is wonderful.");
    first.unmount();

    await openPanel();

    expect(await screen.findByText("The burger is excellent.")).toBeInTheDocument();
    expect(screen.getByText("what is good here?")).toBeInTheDocument();
    expect(screen.getByText("and a dessert?")).toBeInTheDocument();
    expect(screen.getByText("The flan is wonderful.")).toBeInTheDocument();
  });

  it("restores a real two-turn dinner after SSR empty paint, hydrate, and greeting", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    await ask("and a dessert?", "The flan is wonderful.");
    first.unmount();
    expect(
      readStoredTranscript(SCOPE).messages.map((message) => message.content),
    ).toEqual(expect.arrayContaining(TWO_TURN_CONTENTS));

    let resolveHistory!: (value: { data: unknown }) => void;
    const history = new Promise<{ data: unknown }>((resolve) => {
      resolveHistory = resolve;
    });
    mockedAxios.get.mockReturnValueOnce(history as never);

    (globalThis as { __sagePaints?: string[] }).__sagePaints = [];
    const html = withNoWindowStorage(() => renderToString(<Panel />));
    expect(html).not.toContain("The burger is excellent.");
    expect(html).not.toContain("The flan is wonderful.");
    expect(sagePaints()[0] ?? "").not.toContain("The burger is excellent.");

    const container = document.createElement("div");
    container.innerHTML = html;
    document.body.appendChild(container);
    let root: Root | undefined;
    try {
      (globalThis as { __sagePaints?: string[] }).__sagePaints = [];
      // Hydrate with jsdom localStorage available. A lazy useState
      // initializer that reads here would restore on this first call.
      await act(async () => {
        root = hydrateRoot(container, <Panel />);
      });
      expect(sagePaints()[0] ?? "").not.toContain("The burger is excellent.");
      expect(sagePaints()[0] ?? "").not.toContain("The flan is wonderful.");
      expect(
        sagePaints().some((paint) =>
          paint.includes("The burger is excellent."),
        ),
      ).toBe(true);

      const open = container.querySelector("button");
      expect(open).not.toBeNull();
      await act(async () => {
        fireEvent.click(open as HTMLButtonElement);
      });
      expect(container.textContent).toContain("The burger is excellent.");
      expect(container.textContent).toContain("The flan is wonderful.");

      resolveHistory({ data: [greetingRow()] });
      await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());
      expect(container.textContent).toContain("The burger is excellent.");
      expect(container.textContent).toContain("and a dessert?");
      expect(
        readStoredTranscript(SCOPE).messages.map((message) => message.content),
      ).toEqual(expect.arrayContaining(TWO_TURN_CONTENTS));
    } finally {
      await act(async () => {
        root?.unmount();
      });
      container.remove();
    }
  });

  it("does not dump Complete/sources/Availability from a raw envelope in the blob", async () => {
    const scope = "1\u0000UJYDMVCJ5D\u0000ordering\u0000en";
    expect(aiWaiterTranscriptKey(scope)).toContain("UJYDMVCJ5D");
    const rawEnvelope = {
      version: 2,
      response_id: "waiter-message-9",
      answer: {
        format: "markdown",
        content: [
          "Here are some available options from the menu:",
          "- **Harvest Bowl** — available",
        ].join("\n"),
      },
      sections: [],
      steps: [],
      actions: [],
      sources: [
        {
          id: "src-1",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
        {
          id: "src-2",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
        {
          id: "src-3",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
      ],
      entities: [
        {
          id: "menu_item:bowl",
          type: "menu_item",
          display_name: "Harvest Bowl",
          availability: "available",
          source_id: "src-1",
        },
      ],
      follow_ups: [],
      workflow: null,
      notices: [],
      status: "complete",
    };
    localStorage.setItem(
      aiWaiterTranscriptKey(scope),
      JSON.stringify({
        v: 1,
        savedAt: Date.now(),
        greeting: GREETING,
        messages: [
          {
            role: "assistant",
            content: GREETING,
            response: answerPayload(GREETING),
          },
          {
            role: "user",
            content: "suggest a dish",
          },
          {
            role: "assistant",
            content: rawEnvelope.answer.content,
            response: rawEnvelope,
            contractVersion: "v2",
          },
        ],
      }),
    );

    await openPanel();

    expect(screen.getByText("suggest a dish")).toBeInTheDocument();
    expect(screen.queryByText("Complete")).toBeNull();
    expect(screen.queryByText("Availability: Available")).toBeNull();
    expect(screen.queryByText(/sources? used/i)).toBeNull();
    expect(screen.queryByText(/— available/)).toBeNull();
  });

  it("does not dump protocol chrome when a dish turn is restored", async () => {
    const closedMenu = [
      {
        items: [
          {
            id: "bowl",
            name: "Harvest Bowl",
            image: "https://payverge.io/media/biz/86/bowl.jpg",
            is_available: false,
            orderability_state: "business_closed",
          },
        ],
      },
    ];
    const dishAnswer = {
      ...answerPayload(
        [
          "Here are some available options from the menu:",
          "- **Harvest Bowl** — available",
        ].join("\n"),
      ),
      sources: [
        {
          id: "src-1",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
        {
          id: "src-2",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
        {
          id: "src-3",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
      ],
      entities: [
        {
          id: "menu_item:bowl",
          type: "menu_item",
          display_name: "Harvest Bowl",
          availability: "available",
          source_id: "src-1",
        },
      ],
    };
    const mountClosed = () => {
      const view = render(
        <AiWaiter
          businessId={1}
          businessName="Cafe"
          menuData={closedMenu}
          itemOrderability={{
            bowl: { state: "business_closed", orderable: false },
          }}
          bundles={[] as any}
          onAddToCart={jest.fn()}
          tableCode="UJYDMVCJ5D"
        />,
      );
      fireEvent.click(screen.getAllByRole("button")[0]);
      return view;
    };

    const first = mountClosed();
    await screen.findByText(GREETING);
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        role: "model",
        parts: [{ text: "legacy" }],
        response_v2: dishAnswer,
      },
    });
    const input = screen.getByRole("textbox", {
      name: "Message the AI assistant",
    });
    fireEvent.change(input, { target: { value: "what is a good dish?" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await screen.findByTestId("rendered-image");
    first.unmount();

    mountClosed();
    await screen.findByTestId("rendered-image");
    expect(screen.getByText("Harvest Bowl")).toBeInTheDocument();
    expect(screen.queryByText(/— available/)).toBeNull();
    expect(screen.queryByText("Complete")).toBeNull();
    expect(screen.queryByText("Availability: Available")).toBeNull();
    expect(screen.queryByText(/sources? used/i)).toBeNull();
  });

  it("tells the diner Sage started a new conversation instead of implying it remembers", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    first.unmount();

    await openPanel();

    // The transcript above the notice is a local replay. The server minted a
    // brand-new conversation and buildAuthoritativeWaiterHistory feeds the
    // model only that conversation's rows, so the next turn is a FIRST message.
    // Showing the old turns without saying so is the lie.
    expect(
      await screen.findByText(RESUMED_NOTICE),
    ).toBeInTheDocument();
  });

  it("does not stack the resumed-conversation notice on every refresh", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    first.unmount();

    const second = await openPanel();
    await screen.findByText(RESUMED_NOTICE);
    second.unmount();

    await openPanel();

    await screen.findByText("The burger is excellent.");
    await waitFor(() =>
      expect(screen.getAllByText(RESUMED_NOTICE)).toHaveLength(1),
    );
  });

  it("keeps a fresh server row that reuses a restored turn's id", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    first.unmount();

    // localStorage is untrusted input: a stale blob (or a shared tablet the
    // previous diner wrote to) can carry any id at all. Once it carries an id
    // the NEW session also mints, mergeMessages splices the restored row out
    // and returns WITHOUT pushing the server row, so the real answer is lost
    // and the id is tombstoned for the rest of the session.
    const key = Object.keys(localStorage).find((name) =>
      name.startsWith("pv_ai_waiter_chat_v"),
    );
    expect(key).toBeDefined();
    const blob = JSON.parse(localStorage.getItem(key as string) as string);
    for (const message of blob.messages) {
      if (message.role !== "assistant") continue;
      message.id = 9;
      if (message.response) message.response.response_id = "waiter-message-9";
    }
    localStorage.setItem(key as string, JSON.stringify(blob));

    mockedAxios.get.mockResolvedValue({
      data: [
        greetingRow(),
        {
          id: 9,
          role: "assistant",
          content: "The soup is off tonight.",
          created_at: 1_754_000_100,
        },
      ],
    });

    await openPanel();

    expect(
      await screen.findByText("The soup is off tonight."),
    ).toBeInTheDocument();
  });

  it("does not stack another greeting onto the thread on every refresh", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    first.unmount();

    await openPanel();

    await screen.findByText("The burger is excellent.");
    await waitFor(() => expect(screen.getAllByText(GREETING)).toHaveLength(1));
  });

  it("still resumes the transcript late in a long dinner", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    first.unmount();

    clockOffset = 5 * HOUR_MS + 59 * 60 * 1000;
    await openPanel();

    expect(
      await screen.findByText("The burger is excellent."),
    ).toBeInTheDocument();
  });

  it("forgets the transcript once the table session has aged out", async () => {
    const first = await openPanel();
    await ask("what is good here?", "The burger is excellent.");
    first.unmount();

    clockOffset = 6 * HOUR_MS + 60 * 1000;
    await openPanel();

    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
    expect(screen.queryByText("The burger is excellent.")).toBeNull();
    expect(screen.queryByText("what is good here?")).toBeNull();
  });

  it("never restores a question the diner refreshed away from mid-answer", async () => {
    const first = await openPanel();
    // Sage is still thinking when the diner reloads.
    mockedAxios.post.mockReturnValueOnce(new Promise(() => {}) as never);
    const input = screen.getByRole("textbox", {
      name: "Message the AI assistant",
    });
    fireEvent.change(input, { target: { value: "is the kitchen open?" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await screen.findAllByText("is the kitchen open?");
    first.unmount();

    await openPanel();

    // The greeting is back; a question with no reply and no way to retry is not.
    expect(screen.queryByText("is the kitchen open?")).toBeNull();
  });

});
