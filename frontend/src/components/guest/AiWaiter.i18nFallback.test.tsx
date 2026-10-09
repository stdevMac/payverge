/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../api";

jest.mock("../../api", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));

jest.mock("./aiWaiterCopy", () => ({ getStarterQuestions: jest.fn().mockResolvedValue(["Co dziś polecacie?"]) }));

jest.mock("@/api/aiWaiter", () => ({
  createAiWaiterSession: jest.fn().mockResolvedValue({ session_token: "tst", greeting: "hi", expires_at: "2999-01-01T00:00:00Z" }),
  isSessionUnknownError: () => false,
  classifyChatError: () => "generic",
}));

jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
  }),
}));

jest.mock("framer-motion", () => {
  const ReactActual = require("react");
  const MotionComponent = ReactActual.forwardRef(
    ({ children, ...props }: any, ref: any) => {
      const {
        initial: _i,
        animate: _a,
        exit: _e,
        transition: _t,
        whileHover: _wh,
        whileTap: _wt,
        ...domProps
      } = props;
      return ReactActual.createElement("div", { ...domProps, ref }, children);
    },
  );
  return {
    AnimatePresence: ({ children }: any) =>
      ReactActual.createElement(ReactActual.Fragment, null, children),
    useReducedMotion: () => false,
    motion: new Proxy({}, { get: () => MotionComponent }),
  };
});

jest.mock("@nextui-org/react", () => {
  const ReactActual = require("react");
  return {
    Modal: ({ children, isOpen }: any) => (isOpen ? ReactActual.createElement("div", { role: "dialog", "aria-modal": "true" }, children) : null),
    ModalContent: ({ children, ...p }: any) => ReactActual.createElement("div", p, children),
    ModalBody: ({ children, ...p }: any) => ReactActual.createElement("div", p, children),
    ModalFooter: ({ children, ...p }: any) => ReactActual.createElement("div", p, children),
    Card: ({ children }: any) => <div>{children}</div>,
    CardBody: ({ children }: any) => <div>{children}</div>,
    ScrollShadow: ReactActual.forwardRef(({ children }: any, ref: any) => (
      <div ref={ref}>{children}</div>
    )),
    Image: ({ alt }: { alt: string }) => <div aria-label={alt} />,
    Input: ReactActual.forwardRef(
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
    Button: ({ children, onPress, isDisabled, "aria-label": al }: any) => (
      <button type="button" disabled={isDisabled} onClick={onPress} aria-label={al}>
        {children}
      </button>
    ),
  };
});

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

describe("AiWaiter i18n — guest bundle fallback", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
  });

  it("renders starter chips from getStarterQuestions and uses enGuest fallback for tUi keys", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        parts: [
          {
            function_call: {
              name: "add_to_cart",
              args: { item_name: "Burger", quantity: 1 },
            },
          },
        ],
      },
    });

    render(
      <AiWaiter
        businessId={1}
        businessName="Cafe"
        language="pl"
        menuData={[{ items: [{ id: "m1", name: "Burger", price: 12 }] }]}
        onAddToCart={jest.fn()}
        tableCode="T1"
        isOrderingEnabled
      />,
    );

    // Open the chat; the starter chip renders from getStarterQuestions mock.
    fireEvent.click(screen.getAllByRole("button")[0]);
    expect(await screen.findByText("Co dziś polecacie?")).toBeInTheDocument();

    // The placeholder resolves a real tUi key (active bundle or enGuest fallback)
    // and is menu-aware: the first menu item (Burger) is substituted into
    // placeholderWithDish, so the bare key never leaks. (D6)
    const input = await screen.findByPlaceholderText(
      (content) => typeof content === "string" && content.includes("Burger"),
    );
    expect(input).toBeInTheDocument();
  });
});
