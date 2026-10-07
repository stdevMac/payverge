/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";

jest.mock("../../../api", () => ({ axiosInstance: { get: jest.fn().mockResolvedValue({ data: [] }), post: jest.fn() } }));
jest.mock("@/api/aiWaiter", () => ({
  ...jest.requireActual("@/api/aiWaiter"),
  createAiWaiterSession: jest.fn().mockResolvedValue({
    session_token: "test-session",
    greeting: "Hi",
    expires_at: "2999-01-01T00:00:00Z",
  }),
}));
jest.mock("@/hooks/useSSEMessages", () => ({ useSSEMessages: jest.fn() }));
jest.mock("react-hot-toast", () => ({ toast: { success: jest.fn(), error: jest.fn() } }));
jest.mock("../aiWaiterCopy", () => ({ getStarterQuestions: jest.fn().mockResolvedValue(["¿Qué hay de bueno hoy?"]) }));
jest.mock("../../../i18n/GuestTranslationProvider", () => ({ useGuestTranslation: () => ({ t: (key: string) => key }) }));
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

describe("AiWaiter i18n", () => {
  beforeEach(() => { localStorage.clear(); });
  it("renders Spanish placeholder from the guest bundle, not an inline dict", async () => {
    render(<AiWaiter businessId={1} businessName="Cafe" menuData={[]} onAddToCart={jest.fn()} tableCode="T1" language="es" />);
    fireEvent.click(screen.getAllByRole("button")[0]);
    expect(await screen.findByPlaceholderText("Pregúntame lo que quieras sobre el menú")).toBeInTheDocument();
  });
});
