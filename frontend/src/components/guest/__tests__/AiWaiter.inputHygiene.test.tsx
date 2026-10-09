/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../../api";

jest.mock("../../../api", () => ({ axiosInstance: { get: jest.fn(), post: jest.fn() } }));
jest.mock("react-hot-toast", () => ({ toast: { success: jest.fn(), error: jest.fn() } }));
jest.mock("../aiWaiterCopy", () => ({ getStarterQuestions: jest.fn().mockResolvedValue([]) }));
jest.mock("@/api/aiWaiter", () => ({
  createAiWaiterSession: jest.fn().mockResolvedValue({ session_token: "t", greeting: "hi", expires_at: "2999-01-01T00:00:00Z" }),
  isSessionUnknownError: () => false,
  classifyChatError: (e: any) => {
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
    Input: R.forwardRef(({ placeholder, value, onValueChange, onKeyDown, endContent, maxLength }: any, ref: any) => (
      <label><input ref={ref} placeholder={placeholder} value={value} maxLength={maxLength} onChange={(e) => onValueChange?.(e.target.value)} onKeyDown={onKeyDown} />{endContent}</label>
    )),
    Button: ({ children, onPress, isDisabled, title, "aria-label": al }: any) => <button type="button" disabled={isDisabled} onClick={onPress} title={title} aria-label={al}>{children}</button>,
  };
});

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

async function open() {
  render(<AiWaiter businessId={1} businessName="Cafe" menuData={[]} onAddToCart={jest.fn()} tableCode="T1" />);
  fireEvent.click(screen.getAllByRole("button")[0]);
  return await screen.findByPlaceholderText(/Ask me anything about the menu/i);
}

describe("AiWaiter input hygiene + error UX", () => {
  beforeEach(() => { jest.clearAllMocks(); localStorage.clear(); mockedAxios.get.mockResolvedValue({ data: [] }); });

  it("caps the input at maxLength 500", async () => {
    const input = await open();
    expect(input).toHaveAttribute("maxLength", "500");
  });

  it("shows a character counter once past 400 chars", async () => {
    const input = await open();
    fireEvent.change(input, { target: { value: "x".repeat(420) } });
    expect(await screen.findByText("420/500")).toBeInTheDocument();
  });

  it("surfaces a friendly rate-limit message on 429", async () => {
    const input = await open();
    mockedAxios.post.mockRejectedValueOnce({ response: { status: 429 } });
    fireEvent.change(input, { target: { value: "hi" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(await screen.findByText(/sending messages a little fast/i)).toBeInTheDocument();
  });

  it("surfaces a too-long message on 413", async () => {
    const input = await open();
    mockedAxios.post.mockRejectedValueOnce({ response: { status: 413 } });
    fireEvent.change(input, { target: { value: "hi" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(await screen.findByText(/message is too long/i)).toBeInTheDocument();
  });
});
