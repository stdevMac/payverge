/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import InstallAppPrompt from "./InstallAppPrompt";

const defaultProps = {
  title: "Open Payverge like an app",
  body: "Faster access from your home screen. No app store needed.",
  installLabel: "Install Payverge",
  dismissLabel: "Not now",
  staffBottomOffset: true,
  isPending: false,
  onInstall: jest.fn(),
  onDismiss: jest.fn(),
};

beforeEach(() => {
  jest.clearAllMocks();
});

test("stays bottom-centered through tablet widths and moves bottom-right on desktop", () => {
  render(<InstallAppPrompt {...defaultProps} />);

  const region = screen.getByRole("region", {
    name: "Open Payverge like an app",
  });
  expect(region).toHaveClass(
    "z-50",
    "max-w-sm",
    "bottom-[calc(4.75rem+env(safe-area-inset-bottom))]",
    "xl:bottom-[calc(1rem+env(safe-area-inset-bottom))]",
    "inset-x-3",
    "mx-auto",
    "xl:left-auto",
    "xl:right-4",
    "xl:mx-0",
    "xl:w-96",
    "xl:max-w-[calc(100vw-2rem)]",
  );
  expect(region.className).not.toMatch(/(?:^|\s)md:(?:left|right|mx|w|max-w|bottom)-/);
  expect(region.className).not.toContain("money-moment");
  expect(region.className).not.toContain("animate-");
  expect(region).not.toHaveAttribute("aria-modal");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("uses the lower safe-area offset for non-staff operators", () => {
  render(<InstallAppPrompt {...defaultProps} staffBottomOffset={false} />);

  expect(screen.getByRole("region", { name: defaultProps.title })).toHaveClass(
    "bottom-[calc(1rem+env(safe-area-inset-bottom))]",
  );
});

test("provides labeled touch targets and routes both actions", () => {
  render(<InstallAppPrompt {...defaultProps} />);

  const dismiss = screen.getByRole("button", { name: "Not now" });
  const install = screen.getByRole("button", { name: "Install Payverge" });

  expect(dismiss).toHaveClass("h-11", "w-11", "focus-visible:ring-2");
  expect(install).toHaveClass("min-h-11", "focus-visible:ring-2");

  fireEvent.click(install);
  fireEvent.click(dismiss);

  expect(defaultProps.onInstall).toHaveBeenCalledTimes(1);
  expect(defaultProps.onDismiss).toHaveBeenCalledTimes(1);
});

test("marks the invitation busy and disables both actions while installing", () => {
  render(<InstallAppPrompt {...defaultProps} isPending />);

  const region = screen.getByRole("region", { name: defaultProps.title });
  const dismiss = screen.getByRole("button", { name: "Not now" });
  const install = screen.getByRole("button", { name: "Install Payverge" });

  expect(region).toHaveAttribute("aria-busy", "true");
  expect(install).toHaveAttribute("aria-busy", "true");
  expect(install).toBeDisabled();
  expect(dismiss).toBeDisabled();

  fireEvent.click(install);
  fireEvent.click(dismiss);
  expect(defaultProps.onInstall).not.toHaveBeenCalled();
  expect(defaultProps.onDismiss).not.toHaveBeenCalled();
});

test("uses the active light-theme tokens without dead dark variants", () => {
  render(<InstallAppPrompt {...defaultProps} />);

  const region = screen.getByRole("region", { name: defaultProps.title });
  expect(region).toHaveClass("border-warm-200", "bg-white");
  expect(screen.getByRole("heading", { name: defaultProps.title })).toHaveClass(
    "text-ink-950",
  );
  expect(screen.getByText(defaultProps.body)).toHaveClass("text-ink-500");
  expect(region.innerHTML).not.toContain("dark:");
});
