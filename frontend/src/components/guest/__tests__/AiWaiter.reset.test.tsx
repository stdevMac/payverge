/** @jest-environment jsdom */
import React from "react";
import { ReadableStream } from "stream/web";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../../api";
import { createAiWaiterSession } from "@/api/aiWaiter";
import type { AssistantResponse } from "@/types/assistant";

jest.mock("../../../api", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("../aiWaiterCopy", () => ({
  getStarterQuestions: jest.fn().mockResolvedValue(["What's popular tonight?"]),
}));
jest.mock("@/api/aiWaiter", () => ({
  ...jest.requireActual("@/api/aiWaiter"),
  createAiWaiterSession: jest.fn(),
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
    Image: ({ alt }: any) => <div aria-label={alt} />,
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
      isDisabled,
      title,
      "aria-label": al,
    }: any) => (
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
const mockedCreate = createAiWaiterSession as jest.Mock;
const originalFetch = global.fetch;

const v2 = (responseId: string, answer: string): AssistantResponse => ({
  version: 2,
  response_id: responseId,
  answer: { format: "plain_text", content: answer },
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

function greetingRow(id: number, text: string) {
  return {
    id,
    role: "assistant",
    content: text,
    created_at: id,
    response_v2: v2(`greet-${id}`, text),
  };
}

beforeEach(() => {
  const body = new ReadableStream<Uint8Array>({
    start() {
      // Keep the authenticated stream open until the component aborts it.
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

describe("AiWaiter reset flow", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedCreate
      .mockResolvedValueOnce({
        session_token: "srv-token-1",
        greeting: "Hi, AI here",
        expires_at: "2999-01-01T00:00:00Z",
      })
      .mockResolvedValue({
        session_token: "srv-token-2",
        greeting: "Hello again",
        expires_at: "2999-01-01T00:00:00Z",
      });
    mockedAxios.get.mockImplementation(() =>
      Promise.resolve({
        data: [greetingRow(mockedAxios.get.mock.calls.length, "Hi, AI here")],
      }),
    );
    mockedAxios.post.mockResolvedValue({
      data: {
        id: 80,
        role: "model",
        parts: [],
        response_v2: v2("direct-80", "Sure thing"),
      },
    });
  });

  it("after reset shows notice then greeting and restores starter chips", async () => {
    mockedAxios.get
      .mockResolvedValueOnce({ data: [greetingRow(1, "Hi, AI here")] })
      .mockResolvedValue({ data: [greetingRow(2, "Hello again")] });

    render(
      <AiWaiter
        businessId={3}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T9"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Hi, AI here")).toBeInTheDocument();
    expect(
      await within(dialog).findByRole("button", {
        name: "What's popular tonight?",
      }),
    ).toBeInTheDocument();

    const input = await screen.findByPlaceholderText(
      /Ask me anything about the menu/i,
    );
    fireEvent.change(input, { target: { value: "hi" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(await within(dialog).findByText("Sure thing")).toBeInTheDocument();
    expect(
      within(dialog).queryByRole("button", {
        name: "What's popular tonight?",
      }),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /reset chat/i }));

    await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(2));
    const transcript = await waitFor(() => {
      const notice = within(dialog).getByText(/Starting a new conversation/i);
      const greeting = within(dialog).getByText("Hello again");
      return { notice, greeting };
    });
    expect(
      transcript.notice.compareDocumentPosition(transcript.greeting) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      await within(dialog).findByRole("button", {
        name: "What's popular tonight?",
      }),
    ).toBeInTheDocument();
  });

  it("shows rate-limit copy on a 429 reset and does not auto-retry session create", async () => {
    mockedCreate.mockReset();
    mockedCreate
      .mockResolvedValueOnce({
        session_token: "srv-token-1",
        greeting: "Hi, AI here",
        expires_at: "2999-01-01T00:00:00Z",
      })
      .mockRejectedValue({
        response: { status: 429, data: { code: "RATE_LIMITED" } },
      });
    mockedAxios.get.mockResolvedValue({
      data: [greetingRow(1, "Hi, AI here")],
    });

    render(
      <AiWaiter
        businessId={3}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T9"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    await screen.findByText("Hi, AI here");

    fireEvent.click(screen.getByRole("button", { name: /reset chat/i }));
    expect(
      await screen.findByText(/sending messages a little fast/i),
    ).toBeInTheDocument();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockedCreate).toHaveBeenCalledTimes(2);
  });
});
