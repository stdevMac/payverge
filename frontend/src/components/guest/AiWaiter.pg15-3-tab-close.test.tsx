/**
 * PG-15.3 — AiWaiter closes on hashchange (the event changeTab now dispatches
 * after pushState). Full changeTab→close co-mount is covered by
 * ConciergeWidget.pg15-3-tab-close.test.tsx + useHashTabs "dispatches hashchange".
 *
 * @jest-environment jsdom
 */

import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../api";

jest.mock("../../api", () => ({
  axiosInstance: {
    get: jest.fn().mockResolvedValue({ data: [] }),
    post: jest.fn(),
  },
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("./aiWaiterCopy", () => ({
  getStarterQuestions: jest.fn().mockResolvedValue([]),
}));
jest.mock("@/api/aiWaiter", () => ({
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
    const { initial, animate, exit, transition, whileHover, whileTap, ...d } = p;
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
  const passthru = (tag = "div") =>
    ({ children, ...p }: any) =>
      R.createElement(tag, p, children);
  return {
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
      ({ placeholder, value, onValueChange, onKeyDown, endContent }: any, ref: any) => (
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
    Button: ({ children, onPress, isDisabled, title, "aria-label": al }: any) => (
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

describe("PG-15.3 AiWaiter closes on hash-tab navigation event", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
  });

  it("closes the open dialog when changeTab-style pushState + hashchange fires", async () => {
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
    expect(await screen.findByRole("dialog")).toBeInTheDocument();

    // Exact sequence performed by fixed useHashTabs.changeTab (see useHashTabs.ts)
    act(() => {
      window.history.pushState(null, "", "#menu");
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  it("does NOT close on pushState alone (documents the bug changeTab must fix)", async () => {
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
    expect(await screen.findByRole("dialog")).toBeInTheDocument();

    act(() => {
      window.history.pushState(null, "", "#delivery");
      // deliberately no hashchange — pre-fix changeTab behaved like this
    });

    // Still open: pushState alone is silent
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
