/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

// jsdom lacks scrollTo on elements
Element.prototype.scrollTo = jest.fn() as unknown as typeof Element.prototype.scrollTo;

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string, _locale?: string, params?: Record<string, string | number>) => {
    if (key === "aiMenuOnboarding.wizard.charCount" && params) return `${params.count}/${params.max}`;
    return key;
  },
}));
jest.mock("@/api/business", () => ({
  startWizardSession: jest.fn().mockResolvedValue({ session_id: 1, response: { message: "Hi! Tell me about your restaurant.", suggested_options: [] } }),
  sendWizardMessage: jest.fn(),
  generateMenuFromWizard: jest.fn(),
}));
jest.mock("@/utils/apiError", () => ({ errMessage: (e: any) => String(e) }));
jest.mock("framer-motion", () => {
  const R = require("react");
  const M = R.forwardRef(({ children, ...p }: any, ref: any) => {
    const { initial, animate, exit, transition, whileHover, whileTap, layout, ...d } = p;
    return R.createElement("div", { ...d, ref }, children);
  });
  return { AnimatePresence: ({ children }: any) => R.createElement(R.Fragment, null, children), motion: new Proxy({}, { get: () => M }) };
});
jest.mock("@nextui-org/react", () => {
  const R = require("react");
  return {
    Card: ({ children, ...p }: any) => <div {...p}>{children}</div>,
    CardBody: ({ children, ...p }: any) => <div {...p}>{children}</div>,
    Chip: ({ children, ...p }: any) => <span {...p}>{children}</span>,
    Progress: () => <div data-testid="progress" />,
    Avatar: () => <div data-testid="avatar" />,
    Input: R.forwardRef(({ value, onChange, placeholder, onKeyDown, maxLength, disabled }: any, ref: any) => (
      <input ref={ref} value={value} placeholder={placeholder} maxLength={maxLength} disabled={disabled} onChange={onChange} onKeyDown={onKeyDown} />
    )),
    Button: ({ children, onClick, onPress, isDisabled, isLoading }: any) => (
      <button type="button" disabled={isDisabled || isLoading} onClick={onPress || onClick}>{children}</button>
    ),
  };
});

import AIWizard from "../AIWizard";

const baseProps = { businessId: 1, isPro: true, onMenuGenerated: jest.fn(), onUpgradeToPro: jest.fn() };

async function openChat() {
  render(<AIWizard {...baseProps} />);
  fireEvent.click(screen.getByText("aiMenuOnboarding.wizard.startConversation"));
  return await screen.findByPlaceholderText("aiMenuOnboarding.wizard.inputPlaceholder");
}

describe("AIWizard input hygiene", () => {
  beforeEach(() => jest.clearAllMocks());

  it("caps the wizard chat input at maxLength 500", async () => {
    const input = await openChat();
    expect(input).toHaveAttribute("maxLength", "500");
  });

  it("shows a character counter once the input passes 400 chars", async () => {
    const input = await openChat();
    fireEvent.change(input, { target: { value: "x".repeat(420) } });
    expect(await screen.findByText("420/500")).toBeInTheDocument();
  });

  it("shows no counter under 400 chars", async () => {
    const input = await openChat();
    fireEvent.change(input, { target: { value: "x".repeat(120) } });
    await waitFor(() => expect(screen.queryByText("120/500")).not.toBeInTheDocument());
  });
});
