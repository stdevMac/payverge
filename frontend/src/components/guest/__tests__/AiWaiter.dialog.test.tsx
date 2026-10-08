/** @jest-environment jsdom */
import React from "react";
import fs from "fs";
import path from "path";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../../api";

jest.mock("../../../api", () => ({
  axiosInstance: {
    get: jest.fn().mockResolvedValue({ data: [] }),
    post: jest.fn(),
  },
}));
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
    useReducedMotion: () => true,
  };
});
jest.mock("@nextui-org/react", () => {
  const R = require("react");
  const passthru =
    (tag = "div") =>
    ({ children, ...p }: any) =>
      R.createElement(tag, p, children);
  return {
    // Modal is open-driven by isOpen; render children only when open, expose role=dialog.
    Modal: ({ children, isOpen, "aria-label": ariaLabel }: any) =>
      isOpen
        ? R.createElement(
            "div",
            { role: "dialog", "aria-modal": "true", "aria-label": ariaLabel },
            children,
          )
        : null,
    ModalContent: passthru(),
    ModalHeader: passthru(),
    ModalBody: passthru(),
    ModalFooter: passthru(),
    Card: passthru(),
    CardBody: passthru(),
    ScrollShadow: R.forwardRef(({ children }: any, ref: any) => (
      <div ref={ref}>{children}</div>
    )),
    Image: ({ alt }: any) => (
      <div data-testid="rendered-image" aria-label={alt} />
    ),
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
const menu = [
  {
    items: [
      {
        id: "m1",
        name: "Burger",
        price: 9,
        image: "https://cdn.payverge.io/biz/1/burger.jpg",
      },
    ],
  },
];

describe("AiWaiter dialog a11y + brand re-skin", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
  });

  it("renders the chat panel as a named role=dialog (NextUI Modal) when opened", async () => {
    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={menu}
        onAddToCart={jest.fn()}
        tableCode="T1"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toBeInTheDocument();
    // WCAG 4.1.2: the dialog must carry an accessible name (aria-label=aiName),
    // since the header is a custom <div> and NextUI can't auto-wire labelledby.
    expect(dialog.getAttribute("aria-label")).toBeTruthy();
  });

  it("keeps a legacy Markdown image as inert plain text without rendering media", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        parts: [{ text: "![Daily special](https://evil.example.com/x.gif)" }],
      },
    });
    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        menuData={menu}
        onAddToCart={jest.fn()}
        tableCode="T1"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    const input = await screen.findByPlaceholderText(/Burger/i);
    fireEvent.change(input, { target: { value: "show me" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockedAxios.post).toHaveBeenCalled());
    // plain_text path strips lightweight markdown image syntax to alt text
    // (no media element) so hostile external URLs never render as <img>.
    expect(await screen.findByText("Daily special")).toBeInTheDocument();
    expect(
      screen.queryByText("![Daily special](https://evil.example.com/x.gif)"),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId("rendered-image")).not.toBeInTheDocument();
    expect(document.querySelector("img")).not.toBeInTheDocument();
  });

  it("source has no off-brand black chrome, green status dots, emoji cart toast, or hex #333 toast", () => {
    const src = fs.readFileSync(
      path.resolve(__dirname, "../AiWaiter.tsx"),
      "utf8",
    );
    expect(src).not.toMatch(/bg-black/);
    expect(src).not.toMatch(/bg-green-500/);
    expect(src).not.toMatch(/icon:\s*"🛒"/);
    expect(src).not.toMatch(/background:\s*'#333'/);
    expect(src).not.toMatch(/from-brand to-brand\b/);
  });
});
