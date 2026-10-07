/** @jest-environment jsdom */
import {
  act,
  render,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";

import { getSetupStatus, completeOnboarding } from "@/api/onboarding";
import OnboardingHub from "./OnboardingHub";

const mockTrackOptionalActivationEvent = jest.fn((..._args: unknown[]) =>
  Promise.resolve("sent"),
);
jest.mock("@/lib/analytics/activationEvents", () => ({
  trackOptionalActivationEvent: (...args: unknown[]) =>
    mockTrackOptionalActivationEvent(...args),
  safeActivationToken: (
    value: string | null | undefined,
    fallback = "unknown",
  ) => value || fallback,
}));

jest.mock("@/api/onboarding", () => ({
  getSetupStatus: jest.fn(),
  completeOnboarding: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

const baseProps = {
  businessId: 42,
  businessName: "Trattoria Bella Vista",
  onboardingCompletedAt: null as string | null,
  refreshKey: 0,
  onNavigateToTab: jest.fn(),
};

beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  (getSetupStatus as jest.Mock).mockReset();
  (completeOnboarding as jest.Mock).mockReset();
  (getSetupStatus as jest.Mock).mockReturnValue(new Promise(() => {})); // never resolves
  (completeOnboarding as jest.Mock).mockResolvedValue({
    completed_at: new Date().toISOString(),
  });
  mockTrackOptionalActivationEvent.mockClear();
});

it("renders an accessible skeleton while setup status is loading", () => {
  render(<OnboardingHub {...baseProps} />);
  const loading = screen.getByRole("status", {
    name: /setupProgress\.recovery\.loading/,
  });
  expect(loading).toHaveAttribute("aria-busy", "true");
});

const incompleteStatus = {
  steps: {
    business_profile: {
      done: true,
      has_name: true,
      has_address: true,
      has_currency: true,
    },
    tables: { done: false, count: 0 },
    menu: { done: false, categories: 0, items: 0 },
    staff: { done: false, count: 0 },
    payment: { done: false, count: 0 },
  },
  completed_count: 1,
  total_count: 5,
  required_done: false,
  all_done: false,
};

it("shows an accessible error and retry when the initial status request fails", async () => {
  (getSetupStatus as jest.Mock).mockRejectedValueOnce(new Error("offline"));
  render(<OnboardingHub {...baseProps} />);

  expect(await screen.findByRole("alert")).toHaveTextContent(
    /setupProgress\.recovery\.loadErrorTitle/,
  );
  expect(
    screen.getByRole("button", { name: /setupProgress\.recovery\.retry/ }),
  ).toBeInTheDocument();
});

it("restores the checklist when an initial status retry succeeds", async () => {
  (getSetupStatus as jest.Mock)
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);

  fireEvent.click(
    await screen.findByRole("button", {
      name: /setupProgress\.recovery\.retry/,
    }),
  );

  expect(
    await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/),
  ).toBeInTheDocument();
  expect(getSetupStatus).toHaveBeenCalledTimes(2);
});

it("keeps last-known progress visible when a refresh fails", async () => {
  (getSetupStatus as jest.Mock)
    .mockResolvedValueOnce(incompleteStatus)
    .mockRejectedValueOnce(new Error("offline"));
  const { rerender } = render(<OnboardingHub {...baseProps} refreshKey={0} />);
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);

  rerender(<OnboardingHub {...baseProps} refreshKey={1} />);

  expect(
    await screen.findByText(/setupProgress\.recovery\.refreshError/),
  ).toBeInTheDocument();
  expect(
    screen.getByText(/setupProgress\.incomplete\.welcomeSubtitle/),
  ).toBeInTheDocument();
});

it("retries with bounded backoff and offers support after repeated failure", async () => {
  jest.useFakeTimers();
  (getSetupStatus as jest.Mock).mockRejectedValue(new Error("offline"));

  try {
    render(<OnboardingHub {...baseProps} />);
    await act(async () => {
      await Promise.resolve();
    });
    expect(getSetupStatus).toHaveBeenCalledTimes(1);

    await act(async () => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });
    expect(getSetupStatus).toHaveBeenCalledTimes(2);

    await act(async () => {
      jest.advanceTimersByTime(1_000);
      await Promise.resolve();
    });
    expect(getSetupStatus).toHaveBeenCalledTimes(3);
    // No SUPPORT_EMAIL in the test env: the support link stays hidden
    // rather than pointing at the removed /contact page.
    expect(
      screen.queryByRole("link", { name: /setupProgress\.recovery\.support/ }),
    ).toBeNull();

    await act(async () => {
      jest.advanceTimersByTime(10_000);
      await Promise.resolve();
    });
    expect(getSetupStatus).toHaveBeenCalledTimes(3);
  } finally {
    jest.useRealTimers();
  }
});

it("hydrates the last-known-good checklist while a new request is pending", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  const first = render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);
  first.unmount();

  (getSetupStatus as jest.Mock).mockReset();
  (getSetupStatus as jest.Mock).mockReturnValue(new Promise(() => {}));
  render(<OnboardingHub {...baseProps} />);

  expect(
    screen.getByText(/setupProgress\.incomplete\.welcomeSubtitle/),
  ).toBeInTheDocument();
});

it("keeps a hide preference after an error and successful recovery", async () => {
  localStorage.setItem(
    "payverge_hub_hidden_until_42",
    String(Date.now() + 60 * 60 * 1000),
  );
  (getSetupStatus as jest.Mock)
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce(incompleteStatus);
  const { container } = render(<OnboardingHub {...baseProps} />);

  fireEvent.click(
    await screen.findByRole("button", {
      name: /setupProgress\.recovery\.retry/,
    }),
  );
  await waitFor(() => expect(getSetupStatus).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(container.firstChild).toBeNull());
});

it("renders State A welcome hero + chips when setup is incomplete", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);
  await waitFor(() =>
    screen.getByText(/setupProgress\.incomplete\.welcomeSubtitle/),
  );
  expect(
    screen.getByText(/setupProgress\.incomplete\.welcomeSubtitle/),
  ).toBeInTheDocument();
  expect(
    screen.getByText(/setupProgress\.incomplete\.progress/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", {
      name: /setupProgress\.incomplete\.continueSetup/,
    }),
  ).toBeInTheDocument();
});

it("navigates to first incomplete tab when Continue setup is clicked", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);
  const cta = await screen.findByRole("button", {
    name: /setupProgress\.incomplete\.continueSetup/,
  });
  fireEvent.click(cta);
  // Continue-setup follows backend FirstMissingRequiredStep ranking (menu first).
  expect(baseProps.onNavigateToTab).toHaveBeenCalledWith("menu");
});

it("tracks the visible next step and its click without using operator data", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);

  await waitFor(() =>
    expect(mockTrackOptionalActivationEvent).toHaveBeenCalledWith(
      "onboarding_step_viewed",
      expect.objectContaining({
        locale: "en",
        onboarding_step: "menu",
      }),
    ),
  );

  fireEvent.click(
    screen.getByRole("button", {
      name: /setupProgress\.incomplete\.continueSetup/,
    }),
  );
  expect(mockTrackOptionalActivationEvent).toHaveBeenCalledWith(
    "onboarding_step_clicked",
    expect.objectContaining({
      locale: "en",
      onboarding_step: "menu",
    }),
  );
  expect(mockTrackOptionalActivationEvent).not.toHaveBeenCalledWith(
    expect.anything(),
    expect.objectContaining({ businessName: expect.anything() }),
  );
});

it("navigates to a step's tab when an incomplete chip is clicked", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);
  const chip = await screen.findByRole("button", {
    name: /setupProgress\.steps\.menu/,
  });
  fireEvent.click(chip);
  expect(baseProps.onNavigateToTab).toHaveBeenCalledWith("menu");
});

it("shows optional layout step only after tables exist and deep-links to Spaces", async () => {
  const withTablesNoLayout = {
    ...incompleteStatus,
    steps: {
      ...incompleteStatus.steps,
      tables: { done: true, count: 2 },
      layout: { done: false, count: 0 },
    },
    completed_count: 2,
  };
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(withTablesNoLayout);
  render(<OnboardingHub {...baseProps} />);
  const layoutChip = await screen.findByTestId("onboarding-layout-step");
  expect(layoutChip).toBeInTheDocument();
  fireEvent.click(layoutChip);
  expect(baseProps.onNavigateToTab).toHaveBeenCalledWith(
    "tables?tablesView=spaces",
  );
});

it("hides layout step when tables are not done yet", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);
  await waitFor(() =>
    screen.getByText(/setupProgress\.incomplete\.welcomeSubtitle/),
  );
  expect(screen.queryByTestId("onboarding-layout-step")).toBeNull();
});

it("payment chip opens the payment chooser instead of navigating away", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);
  const chip = await screen.findByRole("button", {
    name: /setupProgress\.steps\.payment/,
  });
  fireEvent.click(chip);
  // No direct navigation — the chooser renders inline.
  expect(baseProps.onNavigateToTab).not.toHaveBeenCalledWith("plugins");
  expect(
    screen.getByText(/setupProgress\.paymentChooser\.cashTitle/),
  ).toBeInTheDocument();

  // (a) cash today → Bills
  fireEvent.click(
    screen.getByRole("button", {
      name: /setupProgress\.paymentChooser\.cashCta/,
    }),
  );
  expect(baseProps.onNavigateToTab).toHaveBeenCalledWith("bills");

  // (b) Stripe keys → Plugins
  fireEvent.click(
    screen.getByRole("button", {
      name: /setupProgress\.paymentChooser\.stripeCta/,
    }),
  );
  expect(baseProps.onNavigateToTab).toHaveBeenCalledWith("plugins");

  // (c) payout wallet → Settings
  fireEvent.click(
    screen.getByRole("button", {
      name: /setupProgress\.paymentChooser\.walletCta/,
    }),
  );
  expect(baseProps.onNavigateToTab).toHaveBeenCalledWith("settings");
});

it("payment chooser shows the payout wallet as already added when the API says so", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce({
    ...incompleteStatus,
    steps: {
      ...incompleteStatus.steps,
      payment: { done: false, count: 0, has_settlement_address: true },
    },
  });
  render(<OnboardingHub {...baseProps} />);
  fireEvent.click(
    await screen.findByRole("button", {
      name: /setupProgress\.steps\.payment/,
    }),
  );
  expect(
    screen.getByText(/setupProgress\.paymentChooser\.walletDone/),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", {
      name: /setupProgress\.paymentChooser\.walletCta/,
    }),
  ).not.toBeInTheDocument();
});

it("renders done chips as static (not buttons)", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.steps\.business_profile/);
  expect(
    screen.queryByRole("button", {
      name: /setupProgress\.steps\.business_profile/,
    }),
  ).toBeNull();
});

it("renders null when hidden_until is in the future", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  localStorage.setItem(
    "payverge_hub_hidden_until_42",
    String(Date.now() + 60 * 60 * 1000),
  );
  const { container } = render(<OnboardingHub {...baseProps} />);
  await waitFor(() =>
    expect((getSetupStatus as jest.Mock).mock.calls.length).toBe(1),
  );
  expect(container.firstChild).toBeNull();
});

it("renders State A when hidden_until has expired", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  localStorage.setItem(
    "payverge_hub_hidden_until_42",
    String(Date.now() - 60 * 60 * 1000),
  );
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);
});

it("writes hidden_until ≈ now+24h when Hide for today is clicked", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  const { container } = render(<OnboardingHub {...baseProps} />);
  const hideBtn = await screen.findByRole("button", {
    name: /setupProgress\.incomplete\.hideForToday/,
  });
  const before = Date.now();
  fireEvent.click(hideBtn);
  const written = Number(localStorage.getItem("payverge_hub_hidden_until_42"));
  expect(written).toBeGreaterThanOrEqual(before + 24 * 60 * 60 * 1000 - 5_000);
  expect(written).toBeLessThanOrEqual(before + 24 * 60 * 60 * 1000 + 5_000);
  await waitFor(() => expect(container.firstChild).toBeNull());
});

it("renders State A even when hidden if force_show flag is set", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  localStorage.setItem(
    "payverge_hub_hidden_until_42",
    String(Date.now() + 60 * 60 * 1000),
  );
  sessionStorage.setItem("payverge_hub_force_show_42", "1");
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);
});

it("consumes the force_show flag on mount so it is a one-shot override", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  sessionStorage.setItem("payverge_hub_force_show_42", "1");
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);
  expect(sessionStorage.getItem("payverge_hub_force_show_42")).toBeNull();
});

it("Hide for today hides the hub even when it was force-shown via Reopen setup", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  sessionStorage.setItem("payverge_hub_force_show_42", "1");
  const { container } = render(<OnboardingHub {...baseProps} />);
  const hideBtn = await screen.findByRole("button", {
    name: /setupProgress\.incomplete\.hideForToday/,
  });
  fireEvent.click(hideBtn);
  await waitFor(() => expect(container.firstChild).toBeNull());
});

it("ignores a corrupt (non-numeric) hidden_until and renders State A", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  localStorage.setItem("payverge_hub_hidden_until_42", "not-a-number");
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);
});

const doneStatus = {
  steps: {
    business_profile: {
      done: true,
      has_name: true,
      has_address: true,
      has_currency: true,
    },
    tables: { done: true, count: 4 },
    menu: { done: true, categories: 2, items: 12 },
    staff: { done: true, count: 3 },
    payment: { done: true, count: 1 },
  },
  completed_count: 5,
  total_count: 5,
  required_done: true,
  all_done: true,
};

it("renders State B celebration when all_done and completed_at is set within 24h", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  const fiveHoursAgo = new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString();
  render(<OnboardingHub {...baseProps} onboardingCompletedAt={fiveHoursAgo} />);
  await screen.findByText(/setupProgress\.celebration\.title/);
  expect(
    screen.getByText(/setupProgress\.celebration\.subtitle/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: /setupProgress\.celebration\.cta/ }),
  ).toBeInTheDocument();
  expect(completeOnboarding).not.toHaveBeenCalled();
});

it("fires completeOnboarding exactly once when all_done flips true with null completed_at", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  (completeOnboarding as jest.Mock).mockResolvedValueOnce({
    completed_at: new Date().toISOString(),
  });
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.celebration\.title/);
  await waitFor(() => expect(completeOnboarding).toHaveBeenCalledTimes(1));
  expect(completeOnboarding).toHaveBeenCalledWith(42);
});

it("retries a failed completion without issuing duplicate in-flight requests", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  let resolveRetry: ((value: { completed_at: string }) => void) | undefined;
  (completeOnboarding as jest.Mock)
    .mockRejectedValueOnce(new Error("network"))
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRetry = resolve;
        }),
    );
  const errSpy = jest.spyOn(console, "error").mockImplementation(() => {});

  render(<OnboardingHub {...baseProps} />);

  await waitFor(() => expect(completeOnboarding).toHaveBeenCalledTimes(1));
  const retry = await screen.findByRole("button", {
    name: /setupProgress\.recovery\.retryCompletion/,
  });
  fireEvent.click(retry);
  fireEvent.click(retry);
  expect(completeOnboarding).toHaveBeenCalledTimes(2);

  resolveRetry?.({ completed_at: new Date().toISOString() });
  expect(
    await screen.findByText(/setupProgress\.celebration\.title/),
  ).toBeInTheDocument();

  errSpy.mockRestore();
});

it("navigates to tables tab when celebration CTA is clicked", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  const fiveHoursAgo = new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString();
  render(<OnboardingHub {...baseProps} onboardingCompletedAt={fiveHoursAgo} />);
  const cta = await screen.findByRole("button", {
    name: /setupProgress\.celebration\.cta/,
  });
  fireEvent.click(cta);
  expect(baseProps.onNavigateToTab).toHaveBeenCalledWith("tables");
});

it("shows a preview-as-guest link on the celebration card when a first table code is available", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  const fiveHoursAgo = new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString();
  render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={fiveHoursAgo}
      firstTableCode="AI-T01"
    />,
  );
  await screen.findByText(/setupProgress\.celebration\.title/);
  const link = screen.getByRole("link", {
    name: /setupProgress\.celebration\.previewCta/,
  });
  expect(link).toHaveAttribute("href", "/t/AI-T01");
  expect(link).toHaveAttribute("target", "_blank");
  expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
});

it("omits the preview-as-guest link when there is no table yet", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  const fiveHoursAgo = new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString();
  render(<OnboardingHub {...baseProps} onboardingCompletedAt={fiveHoursAgo} />);
  await screen.findByText(/setupProgress\.celebration\.title/);
  expect(
    screen.queryByText(/setupProgress\.celebration\.previewCta/),
  ).not.toBeInTheDocument();
});

it("renders null when celebration window has elapsed (>24h)", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  const thirtyHoursAgo = new Date(
    Date.now() - 30 * 60 * 60 * 1000,
  ).toISOString();
  const { container } = render(
    <OnboardingHub {...baseProps} onboardingCompletedAt={thirtyHoursAgo} />,
  );
  await waitFor(() =>
    expect((getSetupStatus as jest.Mock).mock.calls.length).toBe(1),
  );
  expect(container.firstChild).toBeNull();
  expect(completeOnboarding).not.toHaveBeenCalled();
});

// Required steps (profile + tables + menu) done, optional steps (staff,
// payment) skipped: the operator is "live" and should be celebrated.
const requiredDoneStatus = {
  steps: {
    business_profile: {
      done: true,
      has_name: true,
      has_address: true,
      has_currency: true,
    },
    tables: { done: true, count: 4 },
    menu: { done: true, categories: 2, items: 12 },
    staff: { done: false, count: 0 },
    payment: { done: false, count: 0 },
  },
  completed_count: 3,
  total_count: 5,
  required_done: true,
  all_done: false,
};

it("celebrates when required steps are done even if optional steps are skipped", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(requiredDoneStatus);
  const fiveHoursAgo = new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString();
  render(<OnboardingHub {...baseProps} onboardingCompletedAt={fiveHoursAgo} />);
  await screen.findByText(/setupProgress\.celebration\.title/);
  expect(completeOnboarding).not.toHaveBeenCalled();
});

it("fires completeOnboarding when required_done flips true with null completed_at", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(requiredDoneStatus);
  (completeOnboarding as jest.Mock).mockResolvedValueOnce({
    completed_at: new Date().toISOString(),
  });
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.celebration\.title/);
  await waitFor(() => expect(completeOnboarding).toHaveBeenCalledTimes(1));
});

it("calls onVisibilityChange(true) when rendering State A", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  const onVisibilityChange = jest.fn();
  render(
    <OnboardingHub {...baseProps} onVisibilityChange={onVisibilityChange} />,
  );
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);
  expect(onVisibilityChange).toHaveBeenLastCalledWith(true);
});

it("calls onVisibilityChange(false) when rendering null (dormant)", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  const thirtyHoursAgo = new Date(
    Date.now() - 30 * 60 * 60 * 1000,
  ).toISOString();
  const onVisibilityChange = jest.fn();
  render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={thirtyHoursAgo}
      onVisibilityChange={onVisibilityChange}
    />,
  );
  await waitFor(() =>
    expect(onVisibilityChange).toHaveBeenLastCalledWith(false),
  );
});

it("calls onVisibilityChange(true) when rendering celebration", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(doneStatus);
  const fiveHoursAgo = new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString();
  const onVisibilityChange = jest.fn();
  render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={fiveHoursAgo}
      onVisibilityChange={onVisibilityChange}
    />,
  );
  await screen.findByText(/setupProgress\.celebration\.title/);
  expect(onVisibilityChange).toHaveBeenLastCalledWith(true);
});

it("re-fetches setup status when refreshKey changes", async () => {
  (getSetupStatus as jest.Mock)
    .mockResolvedValueOnce(incompleteStatus)
    .mockResolvedValueOnce({ ...incompleteStatus, completed_count: 3 });
  const { rerender } = render(<OnboardingHub {...baseProps} refreshKey={0} />);
  await waitFor(() =>
    expect((getSetupStatus as jest.Mock).mock.calls.length).toBe(1),
  );
  rerender(<OnboardingHub {...baseProps} refreshKey={1} />);
  await waitFor(() =>
    expect((getSetupStatus as jest.Mock).mock.calls.length).toBe(2),
  );
});

const completeStatus = {
  steps: {
    business_profile: {
      done: true,
      has_name: true,
      has_address: true,
      has_currency: true,
    },
    tables: { done: true, count: 3 },
    menu: { done: true, categories: 2, items: 10 },
    staff: { done: false, count: 0 },
    payment: { done: false, count: 0 },
  },
  completed_count: 3,
  total_count: 5,
  required_done: true,
  all_done: false,
};

it("chip display order is unchanged (profile chip still renders first)", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);
  const chips = screen.getAllByText(/setupProgress\.steps\./);
  expect(chips[0]).toHaveTextContent("setupProgress.steps.business_profile");
});

it("celebration: print CTA sets the one-shot flag and navigates to Tables", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(completeStatus);
  render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={new Date().toISOString()}
    />,
  );
  const printCta = await screen.findByRole("button", {
    name: /setupProgress\.celebration\.printCta/,
  });
  fireEvent.click(printCta);
  expect(window.sessionStorage.getItem("payverge_print_qr_on_open_42")).toBe(
    "1",
  );
  expect(baseProps.onNavigateToTab).toHaveBeenCalledWith("tables");
});

it("celebration renders the test-order guidance line", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(completeStatus);
  render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={new Date().toISOString()}
    />,
  );
  expect(
    await screen.findByText(/setupProgress\.celebration\.testOrderHint/),
  ).toBeInTheDocument();
});

it("celebration renders a dismiss (X) button", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(completeStatus);
  render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={new Date().toISOString()}
    />,
  );
  const dismiss = await screen.findByRole("button", {
    name: /setupProgress\.celebration\.dismiss/,
  });
  expect(dismiss).toBeInTheDocument();
});

it("dismissing the celebration hides the card and writes the dismissed flag", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(completeStatus);
  const { container } = render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={new Date().toISOString()}
    />,
  );
  const dismiss = await screen.findByRole("button", {
    name: /setupProgress\.celebration\.dismiss/,
  });
  fireEvent.click(dismiss);
  expect(localStorage.getItem("payverge_hub_celebration_dismissed_42")).toBe(
    "1",
  );
  await waitFor(() => expect(container.firstChild).toBeNull());
});

it("does not show the celebration when the dismissed flag is already set", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(completeStatus);
  localStorage.setItem("payverge_hub_celebration_dismissed_42", "1");
  const { container } = render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={new Date().toISOString()}
    />,
  );
  await waitFor(() =>
    expect((getSetupStatus as jest.Mock).mock.calls.length).toBe(1),
  );
  expect(screen.queryByText(/setupProgress\.celebration\.title/)).toBeNull();
  expect(container.firstChild).toBeNull();
});

it("reports onVisibilityChange(false) after the celebration is dismissed", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(completeStatus);
  const onVisibilityChange = jest.fn();
  render(
    <OnboardingHub
      {...baseProps}
      onboardingCompletedAt={new Date().toISOString()}
      onVisibilityChange={onVisibilityChange}
    />,
  );
  const dismiss = await screen.findByRole("button", {
    name: /setupProgress\.celebration\.dismiss/,
  });
  fireEvent.click(dismiss);
  await waitFor(() =>
    expect(onVisibilityChange).toHaveBeenLastCalledWith(false),
  );
});

it("celebration dismissal does not affect the incomplete-checklist mode", async () => {
  (getSetupStatus as jest.Mock).mockResolvedValueOnce(incompleteStatus);
  localStorage.setItem("payverge_hub_celebration_dismissed_42", "1");
  render(<OnboardingHub {...baseProps} />);
  await screen.findByText(/setupProgress\.incomplete\.welcomeSubtitle/);
});
