/** @jest-environment jsdom */

import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PwaInstallContextValue } from "@/providers/PwaInstallProvider";
import PwaInstallSurface from "./PwaInstallSurface";

const mockRequestInstall = jest.fn<Promise<void>, []>(() => Promise.resolve());
const mockDismissForSevenDays = jest.fn();
const mockCloseHelp = jest.fn();
const mockConfirmManualInstall = jest.fn();
const mockRecordDashboardVisit = jest.fn();
const mockGetRememberedDashboardPath = jest.fn<string | null, []>(() => null);
const mockReleaseFloatingPrompt = jest.fn();
const mockClaimFloatingPrompt = jest.fn<() => void, []>(
  () => mockReleaseFloatingPrompt,
);

let mockPwa: PwaInstallContextValue = {
  state: "manual-install",
  identity: { key: "staff:7:42", roleType: "staff" },
  shouldShowFloatingPrompt: true,
  helpMode: null,
  claimFloatingPrompt: mockClaimFloatingPrompt,
  requestInstall: mockRequestInstall,
  dismissForSevenDays: mockDismissForSevenDays,
  closeHelp: mockCloseHelp,
  confirmManualInstall: mockConfirmManualInstall,
  recordDashboardVisit: mockRecordDashboardVisit,
  getRememberedDashboardPath: mockGetRememberedDashboardPath,
};

const initialMockPwa = { ...mockPwa };

let mockReducedMotion = true;

jest.mock("framer-motion", () => ({
  ...jest.requireActual("framer-motion"),
  useReducedMotion: () => mockReducedMotion,
}));

jest.mock("@/providers/PwaInstallProvider", () => ({
  usePwaInstall: () => mockPwa,
}));

jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

jest.mock("@/i18n/operatorChromeCatalog", () => ({
  getChromeTranslation: (key: string) =>
    ({
      "pwa.card.title": "Open Payverge like an app",
      "pwa.card.body":
        "Faster access from your home screen. No app store needed.",
      "pwa.actions.install": "Install Payverge",
      "pwa.actions.dismiss": "Not now",
      "pwa.manual.title": "Add Payverge to your Home Screen",
      "pwa.manual.stepShare": "Tap Share in Safari.",
      "pwa.manual.stepAdd": "Choose Add to Home Screen.",
      "pwa.manual.stepConfirm": "Tap Add.",
      "pwa.manual.done": "I've added it",
      "pwa.unsupported.title": "Installation unavailable",
      "pwa.unsupported.body":
        "Open Payverge in Safari or a compatible browser.",
      "pwa.actions.close": "Close",
    })[key] ?? key,
}));

beforeEach(() => {
  mockRequestInstall.mockReset().mockResolvedValue(undefined);
  mockDismissForSevenDays.mockReset();
  mockCloseHelp.mockReset();
  mockConfirmManualInstall.mockReset();
  mockRecordDashboardVisit.mockReset();
  mockGetRememberedDashboardPath.mockReset().mockReturnValue(null);
  mockReleaseFloatingPrompt.mockReset();
  mockClaimFloatingPrompt
    .mockReset()
    .mockReturnValue(mockReleaseFloatingPrompt);
  mockPwa = { ...initialMockPwa };
  mockReducedMotion = true;
});

test("shows the floating card and routes its actions through the controller", async () => {
  render(<PwaInstallSurface allowFloatingPrompt />);

  expect(mockClaimFloatingPrompt).toHaveBeenCalledTimes(1);

  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Install Payverge" }));
  });
  fireEvent.click(screen.getByRole("button", { name: "Not now" }));

  expect(mockRequestInstall).toHaveBeenCalledTimes(1);
  expect(mockDismissForSevenDays).toHaveBeenCalledTimes(1);
});

test("guards a pending install request against synchronous re-entry", async () => {
  let installButton: HTMLButtonElement;
  let resolveInstall: () => void = () => undefined;
  const pendingInstall = new Promise<void>((resolve) => {
    resolveInstall = resolve;
  });
  mockRequestInstall.mockImplementation(() => {
    if (mockRequestInstall.mock.calls.length === 1) {
      fireEvent.click(installButton);
    }
    return pendingInstall;
  });

  render(<PwaInstallSurface allowFloatingPrompt />);
  installButton = screen.getByRole("button", {
    name: "Install Payverge",
  });
  const dismissButton = screen.getByRole("button", { name: "Not now" });

  fireEvent.click(installButton);

  expect(mockRequestInstall).toHaveBeenCalledTimes(1);
  expect(installButton).toBeDisabled();
  expect(dismissButton).toBeDisabled();
  expect(
    screen.getByRole("region", { name: "Open Payverge like an app" }),
  ).toHaveAttribute("aria-busy", "true");

  fireEvent.click(installButton);
  fireEvent.keyDown(installButton, { key: "Enter" });
  fireEvent.click(dismissButton);
  expect(mockRequestInstall).toHaveBeenCalledTimes(1);
  expect(mockDismissForSevenDays).not.toHaveBeenCalled();

  await act(async () => {
    resolveInstall();
    await pendingInstall;
  });

  expect(installButton).toBeEnabled();
  expect(dismissButton).toBeEnabled();
});

test("hides the invitation when the controller says it is not eligible", () => {
  mockPwa = { ...mockPwa, shouldShowFloatingPrompt: false };

  render(<PwaInstallSurface />);

  expect(screen.queryByRole("region")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Install Payverge" }),
  ).not.toBeInTheDocument();
});

test("hides an eligible floating card when the route gate disallows it", () => {
  render(<PwaInstallSurface allowFloatingPrompt={false} />);

  expect(mockClaimFloatingPrompt).not.toHaveBeenCalled();
  expect(screen.queryByRole("region")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Install Payverge" }),
  ).not.toBeInTheDocument();
});

test("releases a floating-prompt claim and does not double-claim route rerenders", () => {
  const view = render(<PwaInstallSurface allowFloatingPrompt />);

  view.rerender(<PwaInstallSurface allowFloatingPrompt />);
  expect(mockClaimFloatingPrompt).toHaveBeenCalledTimes(1);

  view.rerender(<PwaInstallSurface allowFloatingPrompt={false} />);
  expect(mockReleaseFloatingPrompt).toHaveBeenCalledTimes(1);
});

test("claims once when a route transition becomes eligible", () => {
  const view = render(<PwaInstallSurface allowFloatingPrompt={false} />);
  expect(mockClaimFloatingPrompt).not.toHaveBeenCalled();

  view.rerender(<PwaInstallSurface allowFloatingPrompt />);
  expect(mockClaimFloatingPrompt).toHaveBeenCalledTimes(1);

  view.rerender(<PwaInstallSurface allowFloatingPrompt />);
  expect(mockClaimFloatingPrompt).toHaveBeenCalledTimes(1);
});

test.each(["manual-install", "unsupported"] as const)(
  "keeps %s help globally available when the floating route gate is closed",
  (helpMode) => {
    mockPwa = {
      ...mockPwa,
      shouldShowFloatingPrompt: true,
      helpMode,
    };

    render(<PwaInstallSurface allowFloatingPrompt={false} />);

    expect(screen.queryByRole("region")).not.toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  },
);

test("shows ordered manual instructions with safe-area footer padding", () => {
  mockPwa = {
    ...mockPwa,
    shouldShowFloatingPrompt: false,
    helpMode: "manual-install",
  };

  render(<PwaInstallSurface />);

  const dialog = screen.getByRole("dialog", {
    name: "Add Payverge to your Home Screen",
  });
  const steps = within(dialog).getAllByRole("listitem");
  expect(steps).toHaveLength(3);
  expect(steps[0]).toHaveTextContent("1Tap Share in Safari.");
  expect(steps[1]).toHaveTextContent("2Choose Add to Home Screen.");
  expect(steps[2]).toHaveTextContent("3Tap Add.");

  expect(within(dialog).getByRole("contentinfo")).toHaveClass(
    "pb-[max(1rem,env(safe-area-inset-bottom))]",
  );
});

test("closes manual help without also confirming completion", async () => {
  mockPwa = {
    ...mockPwa,
    shouldShowFloatingPrompt: false,
    helpMode: "manual-install",
  };
  const view = render(<PwaInstallSurface />);
  mockCloseHelp.mockImplementation(() => {
    mockPwa = { ...mockPwa, helpMode: null };
    view.rerender(<PwaInstallSurface />);
  });

  fireEvent.click(screen.getByRole("button", { name: "Close" }));

  expect(mockCloseHelp).toHaveBeenCalledTimes(1);
  expect(mockConfirmManualInstall).not.toHaveBeenCalled();
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
});

test("confirms manual completion without also calling close", async () => {
  mockPwa = {
    ...mockPwa,
    shouldShowFloatingPrompt: false,
    helpMode: "manual-install",
  };
  const view = render(<PwaInstallSurface />);
  mockConfirmManualInstall.mockImplementation(() => {
    mockPwa = { ...mockPwa, helpMode: null };
    view.rerender(<PwaInstallSurface />);
  });

  fireEvent.click(screen.getByRole("button", { name: "I've added it" }));

  expect(mockConfirmManualInstall).toHaveBeenCalledTimes(1);
  expect(mockCloseHelp).not.toHaveBeenCalled();
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
});

test("moves initial focus inside manual help and contains Tab navigation", async () => {
  const user = userEvent.setup();
  mockPwa = {
    ...mockPwa,
    shouldShowFloatingPrompt: false,
    helpMode: "manual-install",
  };

  render(<PwaInstallSurface />);

  const dialog = screen.getByRole("dialog", {
    name: "Add Payverge to your Home Screen",
  });
  const close = within(dialog).getByRole("button", { name: "Close" });
  const done = within(dialog).getByRole("button", { name: "I've added it" });
  await waitFor(() =>
    expect(dialog.contains(document.activeElement)).toBe(true),
  );

  act(() => close.focus());
  await user.tab();
  expect(done).toHaveFocus();
  await user.tab();
  expect(close).toHaveFocus();
});

test("closes on Escape and restores focus to the invoking control", async () => {
  mockPwa = {
    ...mockPwa,
    shouldShowFloatingPrompt: false,
    helpMode: null,
  };
  const view = render(
    <>
      <button type="button">Account install action</button>
      <PwaInstallSurface />
    </>,
  );
  const trigger = screen.getByRole("button", {
    name: "Account install action",
  });
  trigger.focus();

  mockPwa = { ...mockPwa, helpMode: "manual-install" };
  view.rerender(
    <>
      <button type="button">Account install action</button>
      <PwaInstallSurface />
    </>,
  );
  const dialog = screen.getByRole("dialog", {
    name: "Add Payverge to your Home Screen",
  });
  await waitFor(() =>
    expect(dialog.contains(document.activeElement)).toBe(true),
  );

  mockCloseHelp.mockImplementation(() => {
    mockPwa = { ...mockPwa, helpMode: null };
    view.rerender(
      <>
        <button type="button">Account install action</button>
        <PwaInstallSurface />
      </>,
    );
  });
  fireEvent.keyDown(dialog, { key: "Escape" });

  expect(mockCloseHelp).toHaveBeenCalledTimes(1);
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  await waitFor(() => expect(trigger).toHaveFocus());
});

test("removes NextUI modal motion when reduced motion is requested", () => {
  mockReducedMotion = true;
  mockPwa = {
    ...mockPwa,
    shouldShowFloatingPrompt: false,
    helpMode: "unsupported",
  };

  render(<PwaInstallSurface />);

  const wrapper = screen.getByRole("dialog").parentElement;
  expect(wrapper).toHaveAttribute("data-slot", "wrapper");
  expect(wrapper?.style.transform).toBe("");
  expect(wrapper?.style.opacity).toBe("");
});

test("shows unsupported help with a close action only", () => {
  mockPwa = {
    ...mockPwa,
    shouldShowFloatingPrompt: false,
    helpMode: "unsupported",
  };

  render(<PwaInstallSurface />);

  const dialog = screen.getByRole("dialog", {
    name: "Installation unavailable",
  });
  expect(dialog).toHaveTextContent(
    "Open Payverge in Safari or a compatible browser.",
  );
  expect(within(dialog).queryByRole("list")).not.toBeInTheDocument();
  expect(
    within(dialog).queryByRole("button", { name: "I've added it" }),
  ).not.toBeInTheDocument();

  fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
  expect(mockCloseHelp).toHaveBeenCalledTimes(1);
});

test("does not render controller identity or browser implementation details", () => {
  mockPwa = {
    ...mockPwa,
    shouldShowFloatingPrompt: false,
    helpMode: "manual-install",
  };

  render(<PwaInstallSurface />);

  expect(document.body).not.toHaveTextContent("staff:7:42");
  expect(document.body).not.toHaveTextContent("manual-install");
  expect(document.body).not.toHaveTextContent("beforeinstallprompt");
});
