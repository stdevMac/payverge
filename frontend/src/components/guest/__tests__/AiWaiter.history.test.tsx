/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../../api";
import { createAiWaiterSession } from "@/api/aiWaiter";

jest.mock("../../../api", () => ({ axiosInstance: { get: jest.fn(), post: jest.fn() } }));
jest.mock("react-hot-toast", () => ({ toast: { success: jest.fn(), error: jest.fn() } }));
jest.mock("../aiWaiterCopy", () => ({ getStarterQuestions: jest.fn().mockResolvedValue([]) }));
jest.mock("@/api/aiWaiter", () => ({
  createAiWaiterSession: jest.fn(),
  isSessionUnknownError: (e: any) => e?.response?.status === 404 && e?.response?.data?.code === "session_unknown",
  classifyChatError: (e: any) => {
    if (e?.response?.status === 404 && e?.response?.data?.code === "session_unknown") return "session_unknown";
    if (e?.response?.status === 429) return "rate_limited";
    if (e?.response?.status === 413) return "too_long";
    if (e?.response?.data?.code === "over_budget") return "over_budget";
    return "generic";
  },
}));
jest.mock("framer-motion", () => {
  const R = require("react");
  const M = R.forwardRef(({ children, ...p }: any, ref: any) => { const { initial, animate, exit, transition, whileHover, whileTap, ...d } = p; return R.createElement("div", { ...d, ref }, children); });
  return { AnimatePresence: ({ children }: any) => R.createElement(R.Fragment, null, children), motion: new Proxy({}, { get: () => M }), useReducedMotion: () => false };
});
jest.mock("@nextui-org/react", () => {
  const R = require("react");
  return {
    Modal: ({ children, isOpen }: any) => (isOpen ? R.createElement("div", { role: "dialog", "aria-modal": "true" }, children) : null),
    ModalContent: ({ children, ...p }: any) => R.createElement("div", p, children),
    ModalBody: ({ children, ...p }: any) => R.createElement("div", p, children),
    ModalFooter: ({ children, ...p }: any) => R.createElement("div", p, children),
    Card: ({ children, ...p }: any) => <div {...p}>{children}</div>,
    CardBody: ({ children, ...p }: any) => <div {...p}>{children}</div>,
    ScrollShadow: R.forwardRef(({ children }: any, ref: any) => <div ref={ref}>{children}</div>),
    Image: ({ alt }: any) => <div aria-label={alt} />,
    Input: R.forwardRef(({ placeholder, value, onValueChange, onKeyDown, endContent }: any, ref: any) => (
      <label><input ref={ref} placeholder={placeholder} value={value} onChange={(e) => onValueChange?.(e.target.value)} onKeyDown={onKeyDown} />{endContent}</label>
    )),
    Button: ({ children, onPress, isDisabled, title, "aria-label": al }: any) => <button type="button" disabled={isDisabled} onClick={onPress} title={title} aria-label={al}>{children}</button>,
  };
});

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;
const mockedCreate = createAiWaiterSession as jest.Mock;

// Regression guard for the stale-history bug: handleSendMessage read
// `messagesRef.current` synchronously, but the ref is only written inside the
// batched setMessages updater (flushed AFTER the synchronous handler runs), so
// the guest's just-typed message was EXCLUDED from the POST. The model then got
// a conversation with no question and deflected or emitted a blank, and the
// backend saved zero user rows. The history sent to the backend MUST end with
// the message the guest just sent.
describe("AiWaiter outgoing history", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
    mockedAxios.post.mockResolvedValue({ data: { role: "model", parts: [{ text: "Here are our vegetarian dishes." }] } });
    mockedCreate.mockResolvedValue({ session_token: "srv-token-1", greeting: "Hi", expires_at: "2999-01-01T00:00:00Z" });
  });

  it("includes the just-sent user message in the history POSTed to the backend", async () => {
    // Pre-seed a session token so ensureSession() short-circuits and the send
    // path runs synchronously with no intervening await — exactly the condition
    // that exposed the stale-ref bug (the common case after the first message).
    localStorage.setItem(
      "ai_session_3_ordering_T9",
      JSON.stringify({ token: "srv-token-1", expires_at: "2999-01-01T00:00:00Z" }),
    );

    render(<AiWaiter businessId={3} businessName="Cafe" menuData={[]} onAddToCart={jest.fn()} tableCode="T9" />);
    fireEvent.click(screen.getAllByRole("button")[0]); // open panel

    const input = await screen.findByRole("textbox");
    const question = "what vegetarian options do you have?";
    fireEvent.change(input, { target: { value: question } });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      const call = mockedAxios.post.mock.calls.find((c) => String(c[0]).includes("/ai-waiter/3"));
      expect(call).toBeTruthy();
    });

    const call = mockedAxios.post.mock.calls.find((c) => String(c[0]).includes("/ai-waiter/3"))!;
    const sentHistory = (call[1] as any).history as Array<{ role: string; content: string }>;
    expect(Array.isArray(sentHistory)).toBe(true);
    expect(sentHistory.length).toBeGreaterThan(0);
    // The tail of the outgoing history must be the message the guest just typed.
    expect(sentHistory[sentHistory.length - 1]).toMatchObject({ role: "user", content: question });
  });
});
