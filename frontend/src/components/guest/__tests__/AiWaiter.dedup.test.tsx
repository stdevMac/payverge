/** @jest-environment jsdom */
import React from "react";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../../api";

jest.mock("../../../api", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("../aiWaiterCopy", () => ({
  getStarterQuestions: jest.fn().mockResolvedValue([]),
}));
jest.mock("@/api/aiWaiter", () => ({
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
    Button: ({ children, onPress, isDisabled, "aria-label": al }: any) => (
      <button
        type="button"
        disabled={isDisabled}
        onClick={onPress}
        aria-label={al}
      >
        {children}
      </button>
    ),
  };
});

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

describe("AiWaiter optimistic dedup", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
  });

  it("does not duplicate user bubbles when the same text is sent twice and a poll echoes server copies", async () => {
    mockedAxios.get.mockResolvedValue({ data: [] });
    mockedAxios.post.mockResolvedValue({ data: { parts: [{ text: "ok" }] } });

    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T1"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByPlaceholderText(
      /Ask me anything about the menu/i,
    );

    fireEvent.change(input, { target: { value: "same text" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockedAxios.post).toHaveBeenCalledTimes(1));
    fireEvent.change(input, { target: { value: "same text" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockedAxios.post).toHaveBeenCalledTimes(2));

    mockedAxios.get.mockResolvedValueOnce({
      data: [
        { id: 11, role: "user", content: "same text", createdAt: 100 },
        { id: 12, role: "user", content: "same text", createdAt: 101 },
      ],
    });

    await waitFor(() => {
      const bubbles = within(screen.getByRole("log")).getAllByText("same text");
      expect(bubbles.length).toBe(2);
    });
  });
});
