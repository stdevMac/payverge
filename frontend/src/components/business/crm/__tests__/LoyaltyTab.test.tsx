/** @jest-environment jsdom */
import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
import LoyaltyTab from "../LoyaltyTab";

const okLoyalty = () =>
  Promise.resolve({
    program: {
      id: 1,
      business_id: 1,
      enabled: true,
      points_per_dollar: 1,
      redemption_points_per_dollar: 100,
    },
    tiers: [
      { id: 1, name: "Bronze", min_lifetime_spent: 0, sort_order: 0 },
      { id: 2, name: "Silver", min_lifetime_spent: 200, sort_order: 1 },
    ],
  });

type LoyaltyPayload = {
  enabled?: boolean;
  points_per_dollar?: number;
  redemption_points_per_dollar?: number;
  tiers: unknown[];
};

const mockPutLoyalty = jest.fn<
  Promise<{ ok: boolean }>,
  [number, LoyaltyPayload]
>(() => Promise.resolve({ ok: true }));
const mockPreviewLoyalty = jest.fn<
  Promise<{
    total_customers: number;
    tier_distribution: Record<string, number>;
  }>,
  [number, LoyaltyPayload]
>(() =>
  Promise.resolve({
    total_customers: 10,
    tier_distribution: { Bronze: 6, Silver: 4 },
  }),
);
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();

// Swappable per-test so we can simulate a load failure without resetModules
// (which would create a duplicate-React context and break useSimpleLocale).
let mockGetLoyaltyImpl: (businessId: number) => Promise<any> = () =>
  okLoyalty();

jest.mock("@/api/loyalty", () => ({
  getLoyalty: (businessId: number) => mockGetLoyaltyImpl(businessId),
  putLoyalty: (...args: [number, LoyaltyPayload]) => mockPutLoyalty(...args),
  previewLoyalty: (...args: [number, LoyaltyPayload]) =>
    mockPreviewLoyalty(...args),
}));

// LoyaltyTab now pulls `default_currency` from getBusiness so the AED
// label hardcoding is gone. Tests don't care about the currency name,
// just that the call doesn't fail and the tab still renders.
jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() =>
    Promise.resolve({ id: 1, default_currency: "USD" }),
  ),
}));

// Toast calls fire on save success/failure. Tests don't assert on toasts,
// but the component pulls useToast from context so we provide a stub.
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  }),
}));

beforeEach(() => {
  mockGetLoyaltyImpl = okLoyalty;
  mockPutLoyalty.mockClear();
  mockPreviewLoyalty.mockClear();
  mockShowSuccess.mockClear();
  mockShowError.mockClear();
});

describe("LoyaltyTab L5-12 outside-label spacing", () => {
  it("wraps the points-rate field in flex+gap+pt-1 so the label clears the Switch", async () => {
    render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByTestId("loyalty-rate-field-wrap")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("loyalty-rate-field-wrap")).toHaveClass(
      "flex",
      "flex-col",
      "gap-2",
      "pt-1",
    );
  });
});

describe("LoyaltyTab independent earn/redeem rates", () => {
  it("saves distinct earn and redeem rates (not 100% cashback by default)", async () => {
    render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByTestId("loyalty-redeem-rate")).toBeInTheDocument(),
    );

    expect(screen.getByTestId("loyalty-earn-explanation").textContent).toMatch(
      /1/,
    );
    expect(
      screen.getByTestId("loyalty-redeem-explanation").textContent,
    ).toMatch(/100/);

    fireEvent.change(screen.getByTestId("loyalty-points-rate"), {
      target: { value: "1.25" },
    });
    fireEvent.change(screen.getByTestId("loyalty-redeem-rate"), {
      target: { value: "100" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() =>
      expect(mockPutLoyalty).toHaveBeenCalledWith(
        1,
        expect.objectContaining({
          points_per_dollar: 1.25,
          redemption_points_per_dollar: 100,
        }),
      ),
    );
  });
});

describe("LoyaltyTab L5-13 locale points-rate", () => {
  it('saves points_per_dollar 2.5 for comma-decimal "2,5" (not 25)', async () => {
    render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByTestId("loyalty-points-rate")).toBeInTheDocument(),
    );

    const rateInput = screen.getByTestId("loyalty-points-rate");
    fireEvent.change(rateInput, { target: { value: "2,5" } });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() =>
      expect(mockPutLoyalty).toHaveBeenCalledWith(
        1,
        expect.objectContaining({
          points_per_dollar: 2.5,
          redemption_points_per_dollar: 100,
        }),
      ),
    );
    const payload = mockPutLoyalty.mock.calls[0][1];
    expect(payload.points_per_dollar).not.toBe(25);
    expect(payload.points_per_dollar).not.toBe(2);
  });

  it("uses type=text inputMode=decimal so the browser cannot strip commas", async () => {
    render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByTestId("loyalty-points-rate")).toBeInTheDocument(),
    );
    const rateInput = screen.getByTestId("loyalty-points-rate");
    expect(rateInput).toHaveAttribute("type", "text");
    expect(rateInput).toHaveAttribute("inputmode", "decimal");
  });
});

describe("LoyaltyTab", () => {
  it("loads program and renders editable fields", async () => {
    render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("1")).toBeInTheDocument(),
    );
    expect(screen.getByDisplayValue("Silver")).toBeInTheDocument();
  });

  it("does not claim autosave while the explicit-save bar is clean (#380)", async () => {
    render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("1")).toBeInTheDocument(),
    );
    expect(screen.queryByText(/saved automatically/i)).not.toBeInTheDocument();
    expect(screen.getByText(/No unsaved changes/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save changes/i })).toBeDisabled();
  });

  it("adds a new tier", async () => {
    render(<LoyaltyTab businessId={1} />);
    await waitFor(() => screen.getByDisplayValue("Silver"));
    fireEvent.click(screen.getByRole("button", { name: /add tier/i }));
    expect(
      screen.getAllByPlaceholderText(/tier name/i).length,
    ).toBeGreaterThanOrEqual(3);
  });

  it("uses the business's default_currency in the rate label (locale money)", async () => {
    render(<LoyaltyTab businessId={1} />);
    // S-4: label embeds formatMoneyAmount (e.g. "$1.00"), not bare "USD 1.00".
    await waitFor(() =>
      expect(
        screen.getByText(/Points earned per\s+\$1\.00\s+spent/i),
      ).toBeInTheDocument(),
    );
  });
});

describe("LoyaltyTab invalid loaded ladder", () => {
  it("honors a server-invalid ladder and does not preview or silently save it", async () => {
    mockGetLoyaltyImpl = () =>
      Promise.resolve({
        program: {
          id: 1,
          business_id: 1,
          enabled: true,
          points_per_dollar: 1,
          redemption_points_per_dollar: 100,
        },
        valid: false,
        // The backend is authoritative. Keep this ladder structurally valid so
        // the test proves the UI honors the server validity bit instead of
        // re-deriving only the currently-known client checks.
        tiers: [
          { id: 1, name: "Bronze", min_lifetime_spent: 0, sort_order: 0 },
          { id: 2, name: "Silver", min_lifetime_spent: 250, sort_order: 1 },
          { id: 3, name: "Gold", min_lifetime_spent: 750, sort_order: 2 },
        ],
      });

    render(<LoyaltyTab businessId={1} />);

    await waitFor(() =>
      expect(
        screen.getByText(/This tier ladder needs repair/i),
      ).toBeInTheDocument(),
    );

    // Change an unrelated field so Save becomes dirty. The invalid loaded
    // ladder must still be refused at the commit boundary.
    fireEvent.change(screen.getByDisplayValue("1"), {
      target: { value: "2" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(mockPutLoyalty).not.toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 450));
    expect(mockPreviewLoyalty).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent(
      /This tier ladder needs repair/i,
    );
  });
});

describe("LoyaltyTab stale business responses", () => {
  it("ignores a previous business response after the active business is edited", async () => {
    let resolveOldLoad!: (value: any) => void;
    let resolveCurrentLoad!: (value: any) => void;
    const oldLoad = new Promise((resolve) => {
      resolveOldLoad = resolve;
    });
    const currentLoad = new Promise((resolve) => {
      resolveCurrentLoad = resolve;
    });
    mockGetLoyaltyImpl = (id) => (id === 1 ? oldLoad : currentLoad);

    const { rerender } = render(<LoyaltyTab businessId={1} />);
    rerender(<LoyaltyTab businessId={2} />);

    resolveCurrentLoad({
      program: {
        id: 2,
        business_id: 2,
        enabled: true,
        points_per_dollar: 2,
        redemption_points_per_dollar: 100,
      },
      tiers: [
        {
          id: 20,
          name: "Current Bronze",
          min_lifetime_spent: 0,
          sort_order: 0,
        },
        {
          id: 21,
          name: "Current Silver",
          min_lifetime_spent: 100,
          sort_order: 1,
        },
      ],
    });
    await waitFor(() =>
      expect(screen.getByDisplayValue("Current Silver")).toBeInTheDocument(),
    );

    fireEvent.change(screen.getByDisplayValue("2"), {
      target: { value: "3" },
    });
    resolveOldLoad({
      program: {
        id: 1,
        business_id: 1,
        enabled: true,
        points_per_dollar: 9,
        redemption_points_per_dollar: 100,
      },
      valid: false,
      tiers: [
        {
          id: 10,
          name: "Old Bronze",
          min_lifetime_spent: 0,
          sort_order: 0,
        },
        {
          id: 11,
          name: "Old Silver",
          min_lifetime_spent: 200,
          sort_order: 1,
        },
      ],
    });

    await waitFor(() =>
      expect(screen.getByDisplayValue("3")).toBeInTheDocument(),
    );
    expect(screen.getByDisplayValue("Current Silver")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("Old Silver")).not.toBeInTheDocument();
    expect(
      screen.queryByText(/This tier ladder needs repair/i),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() =>
      expect(mockPutLoyalty).toHaveBeenCalledWith(
        2,
        expect.objectContaining({ points_per_dollar: 3 }),
      ),
    );
  });
});

describe("LoyaltyTab preview failures", () => {
  it("clears the previous preview when a later preview request fails", async () => {
    render(<LoyaltyTab businessId={1} />);

    await waitFor(() =>
      expect(screen.getByText("Preview")).toBeInTheDocument(),
    );
    mockPreviewLoyalty.mockClear();
    mockPreviewLoyalty.mockRejectedValueOnce(new Error("preview failed"));

    fireEvent.change(screen.getByDisplayValue("1"), {
      target: { value: "2" },
    });

    await waitFor(() => expect(mockPreviewLoyalty).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.queryByText("Preview")).not.toBeInTheDocument(),
    );
  });
});

describe("LoyaltyTab stale preview responses", () => {
  it("keeps the newer preview when an older request resolves afterward", async () => {
    jest.useFakeTimers();

    try {
      render(<LoyaltyTab businessId={1} />);

      // Let the initial load and its preview settle before replacing the
      // preview mock with two deferred requests.
      await act(async () => {
        await Promise.resolve();
      });
      act(() => {
        jest.advanceTimersByTime(400);
      });
      await act(async () => {
        await Promise.resolve();
      });

      type PreviewResult = {
        total_customers: number;
        tier_distribution: Record<string, number>;
      };
      const pendingPreviews: Array<{
        resolve: (result: PreviewResult) => void;
        reject: (error: Error) => void;
      }> = [];
      mockPreviewLoyalty.mockClear();
      mockPreviewLoyalty.mockImplementation(
        () =>
          new Promise<PreviewResult>((resolve, reject) => {
            pendingPreviews.push({ resolve, reject });
          }),
      );

      fireEvent.change(screen.getByDisplayValue("1"), {
        target: { value: "2" },
      });
      act(() => {
        jest.advanceTimersByTime(400);
      });
      expect(pendingPreviews).toHaveLength(1);

      fireEvent.change(screen.getByDisplayValue("2"), {
        target: { value: "3" },
      });
      act(() => {
        jest.advanceTimersByTime(400);
      });
      expect(pendingPreviews).toHaveLength(2);

      await act(async () => {
        pendingPreviews[1].resolve({
          total_customers: 20,
          tier_distribution: { Bronze: 17, Silver: 3 },
        });
        await Promise.resolve();
      });
      expect(
        screen.getByText("17 Bronze · 3 Silver (of 20 customers)"),
      ).toBeInTheDocument();

      await act(async () => {
        pendingPreviews[0].resolve({
          total_customers: 99,
          tier_distribution: { Bronze: 99 },
        });
        await Promise.resolve();
      });

      expect(
        screen.getByText("17 Bronze · 3 Silver (of 20 customers)"),
      ).toBeInTheDocument();
      expect(
        screen.queryByText("99 Bronze (of 99 customers)"),
      ).not.toBeInTheDocument();
    } finally {
      jest.useRealTimers();
    }
  });
});

describe("LoyaltyTab stale save responses", () => {
  it("ignores a previous business save success after switching businesses", async () => {
    let resolveSave!: (value: { ok: boolean }) => void;
    const pendingSave = new Promise<{ ok: boolean }>((resolve) => {
      resolveSave = resolve;
    });
    mockPutLoyalty.mockImplementationOnce(() => pendingSave);

    const { rerender } = render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("1")).toBeInTheDocument(),
    );
    fireEvent.change(screen.getByDisplayValue("1"), {
      target: { value: "2" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() =>
      expect(mockPutLoyalty).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ points_per_dollar: 2 }),
      ),
    );

    mockGetLoyaltyImpl = (id) =>
      Promise.resolve(
        id === 2
          ? {
              program: {
                id: 2,
                business_id: 2,
                enabled: true,
                points_per_dollar: 4,
                redemption_points_per_dollar: 100,
              },
              tiers: [
                {
                  id: 20,
                  name: "Current Bronze",
                  min_lifetime_spent: 0,
                  sort_order: 0,
                },
                {
                  id: 21,
                  name: "Current Silver",
                  min_lifetime_spent: 100,
                  sort_order: 1,
                },
              ],
            }
          : okLoyalty(),
      );
    rerender(<LoyaltyTab businessId={2} />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("Current Silver")).toBeInTheDocument(),
    );

    resolveSave({ ok: true });
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(mockShowSuccess).not.toHaveBeenCalled();
    expect(mockShowError).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: /save changes/i }),
    ).toBeDisabled();
  });

  it("ignores a previous business save failure after reloading", async () => {
    let rejectSave!: (error: Error) => void;
    const pendingSave = new Promise<{ ok: boolean }>((_, reject) => {
      rejectSave = reject;
    });
    mockPutLoyalty.mockImplementationOnce(() => pendingSave);

    const { unmount } = render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("1")).toBeInTheDocument(),
    );
    fireEvent.change(screen.getByDisplayValue("1"), {
      target: { value: "2" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(mockPutLoyalty).toHaveBeenCalledTimes(1));

    unmount();
    render(<LoyaltyTab businessId={1} />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("1")).toBeInTheDocument(),
    );
    rejectSave(new Error("stale save failed"));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(mockShowSuccess).not.toHaveBeenCalled();
    expect(mockShowError).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: /save changes/i }),
    ).toBeDisabled();
  });
});

// R3-ML-2: a failed load must NOT render destructive editable defaults
// (enabled/rate/empty-tiers) that arm a Save which would wipe the program.
describe("LoyaltyTab load failure", () => {
  it("shows an error/retry state and no Save button when the program fails to load", async () => {
    mockGetLoyaltyImpl = () => Promise.reject(new Error("boom"));
    const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});

    try {
      render(<LoyaltyTab businessId={1} />);

      await waitFor(() =>
        expect(screen.getByRole("alert")).toBeInTheDocument(),
      );
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Couldn't load your loyalty program. Nothing was changed.",
      );
      // The destructive editor/Save is not rendered on load failure.
      expect(
        screen.queryByRole("button", { name: /save/i }),
      ).not.toBeInTheDocument();
      // A localized retry affordance is offered instead.
      expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
      expect(warnSpy).not.toHaveBeenCalledWith(
        expect.stringContaining("[i18n] Missing translation"),
      );
    } finally {
      warnSpy.mockRestore();
    }

    mockGetLoyaltyImpl = okLoyalty;
  });
});
