/** @jest-environment jsdom */
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import toast from "react-hot-toast";
import CustomersTab from "../CustomersTab";

// Minimal stub — the test reads summary / tier labels / toasts by visible text.
const I18N_STUB: Record<string, string> = {
  "businessDashboard.crm.summary.totalCustomers": "Total customers",
  "businessDashboard.crm.summary.activeThisMonth": "Active this month",
  "businessDashboard.crm.summary.avgLifetimeSpend": "Avg lifetime spend",
  "businessDashboard.crm.summary.topTierCustomers": "Top-tier customers",
  "businessDashboard.crm.tiers.all": "All",
  "businessDashboard.crm.tiers.legendPrefix": "Tiers by lifetime spend:",
  "businessDashboard.crm.tiers.thresholdHint": "{tier} · spent ≥ {amount}",
  "businessDashboard.crm.tiers.fallbackRule":
    "Tiers follow lifetime spend from the Loyalty program.",
  "businessDashboard.crm.points.rule":
    "Points: {rate} per {amount} spent. Balance can be adjusted and does not set the guest's tier.",
  "businessDashboard.crm.points.fallbackRule":
    "Points come from the Loyalty earn rate and optional comps — they do not set the guest's tier.",
  "businessDashboard.crm.points.independentHint":
    "Point balance is independent of lifetime spend and does not set the guest's tier.",
  "businessDashboard.crm.adjustPoints": "Adjust points",
  "businessDashboard.crm.toasts.pointsAdjusted": "Loyalty points updated",
  "businessDashboard.crm.toasts.pointsAdjustError":
    "Couldn't update loyalty points. Please try again.",
  "businessDashboard.crm.remove.action": "Remove",
  "businessDashboard.crm.remove.confirmTitle": "Remove from this business?",
  "businessDashboard.crm.remove.confirmDescription":
    "{name} will be removed from this business only.",
  "businessDashboard.crm.remove.confirmAction": "Remove from business",
  "businessDashboard.crm.remove.success": "Customer removed from this business",
  "businessDashboard.crm.remove.error": "Couldn't remove customer. Please try again.",
  "businessDashboard.crm.customerList": "Customers",
  "businessDashboard.crm.filterByTierAria": "Filter by loyalty tier",
  "businessDashboard.crm.toasts.notesSaved": "Notes saved",
  "businessDashboard.crm.toasts.notesError": "Couldn't save notes. Please try again.",
  "businessDashboard.crm.toasts.loadError": "Couldn't load customers. Please try again.",
  "businessDashboard.crm.toasts.addCustomerSuccess": "Customer added",
  "businessDashboard.crm.toasts.customerLinked": "Customer linked to this business",
  "businessDashboard.crm.toasts.addCustomerError": "Couldn't add customer. Please try again.",
  "businessDashboard.crm.toasts.tagsSaved": "Tags saved",
  "businessDashboard.crm.searchPlaceholder": "Search customers",
  "businessDashboard.crm.addCustomer": "Add Customer",
  "businessDashboard.crm.customerNamePlaceholder": "Full name",
  "businessDashboard.crm.phonePlaceholder": "+1 555 123 4567",
  "businessDashboard.crm.name": "Name",
  "businessDashboard.crm.email": "Email",
  "businessDashboard.crm.phone": "Phone",
  "businessDashboard.crm.validation.emailInvalid": "Enter a valid email address.",
  "businessDashboard.crm.validation.phoneInvalid":
    "Enter a valid phone number (digits only, at least 7).",
  "businessDashboard.crm.validation.nameRequired": "Name is required.",
  "businessDashboard.crm.validation.emailRequired": "Email is required.",
  "businessDashboard.crm.validation.nameEmailRequired":
    "Name and email are required to save.",
  "businessDashboard.crm.empty.customers.title": "No customers yet",
  "businessDashboard.crm.empty.customers.subtitle":
    "Customer profiles are created when guests identify themselves at checkout",
  "businessDashboard.crm.empty.customers.captureCta": "Set up table QR codes",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale?: string,
    params?: Record<string, string | number>,
  ) => {
    let value = I18N_STUB[key] ?? key;
    if (params) {
      for (const [name, next] of Object.entries(params)) {
        value = value.replaceAll(`{${name}}`, String(next));
      }
    }
    return value;
  },
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "USD" })),
}));

jest.mock("@/api/loyalty", () => ({
  getLoyalty: jest.fn(() =>
    Promise.resolve({
      program: {
        id: 1,
        business_id: 1,
        enabled: true,
        points_per_dollar: 1,
        redemption_points_per_dollar: 100,
      },
      tiers: [
        { name: "Bronze", min_lifetime_spent: 0, sort_order: 0 },
        { name: "Silver", min_lifetime_spent: 250, sort_order: 1 },
        { name: "Gold", min_lifetime_spent: 1000, sort_order: 2 },
      ],
    }),
  ),
}));

// Mock CurrencyPrice to just render its amount.
jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>${amount}</span>,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
const toastMock = toast as unknown as {
  success: jest.Mock;
  error: jest.Mock;
};

const GOLD = {
  id: 1,
  name: "AI Guest 16",
  email: "g16@x.test",
  loyalty_points: 628,
  loyalty_tier: "Gold",
  total_spent: 1740,
  visits: 8,
};
const SILVER = {
  id: 2,
  name: "AI Guest 8",
  email: "g08@x.test",
  loyalty_points: 364,
  loyalty_tier: "Silver",
  total_spent: 980,
  visits: 9,
};

const SUMMARY = {
  total_customers: 500,
  active_this_month: 120,
  avg_lifetime_spend: 1234,
  top_tier_count: 75,
};

jest.mock("@/api/crm", () => ({
  getCustomers: () => Promise.resolve([]),
  businessCRMAPI: {
    getCustomers: jest.fn(),
    getCustomerDetails: jest.fn(),
    updateCustomerNotes: jest.fn(),
    updateCustomerTags: jest.fn(() => Promise.resolve({})),
    addCustomer: jest.fn(() => Promise.resolve({})),
    adjustCustomerLoyaltyPoints: jest.fn(() =>
      Promise.resolve({ message: "ok", loyalty_points: 50 }),
    ),
    unlinkCustomer: jest.fn(() => Promise.resolve({ unlinked: true })),
    exportCustomers: jest.fn(() => Promise.resolve(new Blob())),
    getCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
    toggleCRM: jest.fn(() => Promise.resolve({ enabled: true })),
  },
}));

// Pull the typed mocks out of the mocked module (avoids jest.mock hoisting traps).
import { businessCRMAPI } from "@/api/crm";
const getCustomersMock = businessCRMAPI.getCustomers as jest.Mock;
const updateCustomerNotesMock = businessCRMAPI.updateCustomerNotes as jest.Mock;
const getCustomerDetailsMock = businessCRMAPI.getCustomerDetails as jest.Mock;
const addCustomerMock = businessCRMAPI.addCustomer as jest.Mock;
const adjustPointsMock =
  businessCRMAPI.adjustCustomerLoyaltyPoints as jest.Mock;
const unlinkCustomerMock = businessCRMAPI.unlinkCustomer as jest.Mock;

beforeEach(() => {
  jest.clearAllMocks();
  getCustomerDetailsMock.mockResolvedValue({
    customer: { ...GOLD, customer: { name: GOLD.name } },
  });
  // Default: tier param drives which rows come back (server-side filter).
  getCustomersMock.mockImplementation((_b, _p, _ps, _s, tier: string) => {
    const all = [GOLD, SILVER];
    const rows = tier && tier !== "All" ? all.filter((c) => c.loyalty_tier === tier) : all;
    return Promise.resolve({
      customers: rows,
      total: rows.length,
      total_pages: 1,
      summary: SUMMARY,
    });
  });
});

describe("CustomersTab", () => {
  it("renders summary cards from the server meta (whole-base, not the page)", async () => {
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/Total customers/i)).toBeInTheDocument();
    // total_customers = 500 even though only 2 rows are on the page.
    expect(screen.getByText("500")).toBeInTheDocument();
    expect(screen.getByText("75")).toBeInTheDocument();
  });

  it("empty state offers a CTA to the guest capture path (tables/QR)", async () => {
    getCustomersMock.mockResolvedValue({
      customers: [],
      summary: {
        total_customers: 0,
        active_this_month: 0,
        avg_lifetime_spend: 0,
        top_tier_count: 0,
      },
    });
    const onNavigateToTab = jest.fn();
    render(
      <CustomersTab businessId={1} onNavigateToTab={onNavigateToTab} />,
    );
    const cta = await screen.findByRole("button", {
      name: "Set up table QR codes",
    });
    fireEvent.click(cta);
    expect(onNavigateToTab).toHaveBeenCalledWith("tables");
  });

  it("filters by tier server-side (refetches with the tier param)", async () => {
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();
    expect(screen.getByText(/AI Guest 8/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Gold" }));

    await waitFor(() =>
      expect(getCustomersMock).toHaveBeenCalledWith(1, 1, 20, "", "Gold", {
        sortBy: "last_visit_at",
        sortDir: "desc",
      }),
    );
    await waitFor(() =>
      expect(screen.queryByText(/AI Guest 8/)).not.toBeInTheDocument(),
    );
    expect(screen.getByText(/AI Guest 16/)).toBeInTheDocument();
  });

  it("debounces search — one fetch per settled term, not per keystroke", async () => {
    render(<CustomersTab businessId={1} />);
    // Wait for the search input to render (after the initial load resolves).
    const input = await screen.findByPlaceholderText("Search customers");
    getCustomersMock.mockClear();

    fireEvent.change(input, { target: { value: "a" } });
    fireEvent.change(input, { target: { value: "ab" } });
    fireEvent.change(input, { target: { value: "abc" } });

    // The debounce keys the network call on the settled term, so the three
    // keystrokes collapse to a single fetch with the final value.
    await waitFor(() =>
      expect(getCustomersMock).toHaveBeenCalledWith(1, 1, 20, "abc", "All", {
        sortBy: "last_visit_at",
        sortDir: "desc",
      }),
    );
    const abcCalls = getCustomersMock.mock.calls.filter(
      (call) => call[3] === "abc",
    );
    expect(abcCalls).toHaveLength(1);
    // No intermediate "a" / "ab" fetches fired.
    expect(
      getCustomersMock.mock.calls.some((call) => call[3] === "a" || call[3] === "ab"),
    ).toBe(false);
  });

  it("keeps the notes modal open and toasts on save failure", async () => {
    updateCustomerNotesMock.mockRejectedValue(new Error("boom"));
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();

    // Open details -> Edit Notes (stub returns the full key as the label).
    // Exact name: the row also has a remove action (L5-9) whose aria-label
    // ends with the customer name.
    fireEvent.click(screen.getByRole("button", { name: "AI Guest 16" }));
    fireEvent.click(
      await screen.findByText("businessDashboard.crm.editNotes"),
    );
    // Save (the notes modal "save" button)
    const saveButtons = await screen.findAllByText("businessDashboard.crm.save");
    fireEvent.click(saveButtons[saveButtons.length - 1]);

    await waitFor(() =>
      expect(toastMock.error).toHaveBeenCalledWith(
        "Couldn't save notes. Please try again.",
      ),
    );
    // Modal stays open: the notes textarea label is still present.
    expect(
      screen.getByText("businessDashboard.crm.notes"),
    ).toBeInTheDocument();
  });

  it("surfaces an error toast when the customer load fails (F4)", async () => {
    getCustomersMock.mockReset();
    getCustomersMock.mockRejectedValue(new Error("network down"));
    render(<CustomersTab businessId={1} />);

    await waitFor(() =>
      expect(toastMock.error).toHaveBeenCalledWith(
        "Couldn't load customers. Please try again.",
      ),
    );
  });

  it("keeps the add-customer modal open and toasts on add failure (F5)", async () => {
    addCustomerMock.mockRejectedValue(new Error("boom"));
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();

    // Open the Add Customer modal (button label resolves to "Add Customer").
    fireEvent.click(screen.getByRole("button", { name: /Add Customer/i }));

    // Fill the required name + email so the save button enables.
    const nameInput = await screen.findByPlaceholderText("Full name");
    fireEvent.change(nameInput, { target: { value: "New Guest" } });
    const emailInput = screen.getByPlaceholderText("email@example.com");
    fireEvent.change(emailInput, { target: { value: "new@x.test" } });

    // Save (the add modal "save" button is the last one rendered).
    const saveButtons = await screen.findAllByText("businessDashboard.crm.save");
    fireEvent.click(saveButtons[saveButtons.length - 1]);

    await waitFor(() =>
      expect(toastMock.error).toHaveBeenCalledWith(
        "Couldn't add customer. Please try again.",
      ),
    );
    // Modal stays open: the name input is still present with its value intact.
    expect(screen.getByPlaceholderText("Full name")).toHaveValue("New Guest");
  });
});

describe("L5-3 CRM add-customer localized validation", () => {
  it("email is type=text (no native English type=email bubble)", async () => {
    render(<CustomersTab businessId={1} />);
    fireEvent.click(await screen.findByRole("button", { name: /Add Customer/i }));
    const email = await screen.findByTestId("crm-add-email");
    expect(email).toHaveAttribute("type", "text");
    expect(email).toHaveAttribute("inputmode", "email");
  });

  it("rejects garbage phone with localized message and does not call API", async () => {
    render(<CustomersTab businessId={1} />);
    fireEvent.click(await screen.findByRole("button", { name: /Add Customer/i }));
    fireEvent.change(await screen.findByTestId("crm-add-name"), {
      target: { value: "Sam" },
    });
    fireEvent.change(screen.getByTestId("crm-add-email"), {
      target: { value: "sam@example.com" },
    });
    fireEvent.change(screen.getByTestId("crm-add-phone"), {
      target: { value: "abc" },
    });
    const saveButtons = screen.getAllByText("businessDashboard.crm.save");
    fireEvent.click(saveButtons[saveButtons.length - 1]);
    await waitFor(() => {
      expect(screen.getByTestId("crm-add-field-error")).toHaveTextContent(
        /valid phone/i,
      );
    });
    expect(addCustomerMock).not.toHaveBeenCalled();
  });

  it("explains missing name/email instead of silent disabled Guardar", async () => {
    render(<CustomersTab businessId={1} />);
    fireEvent.click(await screen.findByRole("button", { name: /Add Customer/i }));
    expect(await screen.findByTestId("crm-add-required-hint")).toBeInTheDocument();
    // Save stays enabled so press can surface field errors.
    const saveButtons = screen.getAllByText("businessDashboard.crm.save");
    const saveBtn = saveButtons[saveButtons.length - 1].closest("button");
    expect(saveBtn).not.toBeDisabled();
  });
});

describe("L5-1 CRM search stays mounted on refetch", () => {
  it("does not unmount the search input while a debounced search refetch is in flight", async () => {
    let resolveSecond: (value: unknown) => void = () => {};
    let calls = 0;
    getCustomersMock.mockImplementation(() => {
      calls += 1;
      if (calls === 1) {
        return Promise.resolve({
          customers: [GOLD, SILVER],
          total: 2,
          total_pages: 1,
          summary: SUMMARY,
        });
      }
      return new Promise((resolve) => {
        resolveSecond = resolve;
      });
    });

    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();

    const search = screen.getByPlaceholderText("Search customers");
    fireEvent.change(search, { target: { value: "guest" } });

    await waitFor(() => expect(calls).toBeGreaterThanOrEqual(2), {
      timeout: 2000,
    });

    // Input still mounted with the typed value — not swapped for skeleton.
    const searchAfter = screen.getByPlaceholderText("Search customers");
    expect(searchAfter).toHaveValue("guest");
    expect(searchAfter).toHaveAttribute("aria-busy", "true");

    resolveSecond({
      customers: [GOLD],
      total: 1,
      total_pages: 1,
      summary: SUMMARY,
    });
  });
});

describe("L5-4 add-customer created vs linked toast", () => {
  it("toasts link copy when API returns created:false", async () => {
    addCustomerMock.mockResolvedValue({ created: false, connection: {} });
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Add Customer/i }));
    fireEvent.change(await screen.findByPlaceholderText("Full name"), {
      target: { value: "Existing Guest" },
    });
    fireEvent.change(screen.getByPlaceholderText("email@example.com"), {
      target: { value: "exist@x.test" },
    });
    const saveButtons = await screen.findAllByText("businessDashboard.crm.save");
    fireEvent.click(saveButtons[saveButtons.length - 1]);
    await waitFor(() =>
      expect(toastMock.success).toHaveBeenCalledWith(
        "Customer linked to this business",
      ),
    );
  });

  it("toasts added copy when API returns created:true", async () => {
    addCustomerMock.mockResolvedValue({ created: true, connection: {} });
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Add Customer/i }));
    fireEvent.change(await screen.findByPlaceholderText("Full name"), {
      target: { value: "New Guest" },
    });
    fireEvent.change(screen.getByPlaceholderText("email@example.com"), {
      target: { value: "new@x.test" },
    });
    const saveButtons = await screen.findAllByText("businessDashboard.crm.save");
    fireEvent.click(saveButtons[saveButtons.length - 1]);
    await waitFor(() =>
      expect(toastMock.success).toHaveBeenCalledWith("Customer added"),
    );
  });
});

describe("L5-10 notes save returns to details", () => {
  it("reopens Detalles after a successful notes save", async () => {
    updateCustomerNotesMock.mockResolvedValue({});
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "AI Guest 16" }));
    fireEvent.click(
      await screen.findByText("businessDashboard.crm.editNotes"),
    );
    const saveButtons = await screen.findAllByText("businessDashboard.crm.save");
    fireEvent.click(saveButtons[saveButtons.length - 1]);
    await waitFor(() =>
      expect(toastMock.success).toHaveBeenCalledWith("Notes saved"),
    );
    // Details surface is back (customer name heading / detail chrome).
    expect(
      await screen.findByText("businessDashboard.crm.editNotes"),
    ).toBeInTheDocument();
  });
});

describe("CRM list chrome, loyalty rules, and row actions", () => {
  it("keeps Add Customer out of the search + tier-chip filter row", async () => {
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();

    const filters = screen.getByTestId("crm-customer-filters");
    expect(
      within(filters).queryByRole("button", { name: /Add Customer/i }),
    ).not.toBeInTheDocument();
    expect(within(filters).getByPlaceholderText("Search customers")).toBeInTheDocument();
    expect(within(filters).getByRole("button", { name: "Gold" })).toBeInTheDocument();
  });

  it("shows spend-based tier thresholds from the loyalty program", async () => {
    render(<CustomersTab businessId={1} />);
    const legend = await screen.findByTestId("crm-tier-legend");
    expect(legend).toHaveTextContent("Tiers by lifetime spend:");
    expect(legend).toHaveTextContent(/Bronze/);
    expect(legend).toHaveTextContent(/Silver/);
    expect(legend).toHaveTextContent(/250/);
    expect(legend).toHaveTextContent(/Gold/);
    expect(legend).toHaveTextContent(/1,000|1000/);
  });

  it("shows the points earn rule so balance is not mistaken for spend", async () => {
    render(<CustomersTab businessId={1} />);
    const rule = await screen.findByTestId("crm-points-rule");
    expect(rule).toHaveTextContent(/Points:/);
    expect(rule).toHaveTextContent(/1/);
    expect(rule).toHaveTextContent(/does not set the guest's tier/);
  });

  it("confirms before unlinking a customer", async () => {
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: /Remove AI Guest 16/i }),
    );
    expect(
      await screen.findByText("Remove from this business?"),
    ).toBeInTheDocument();
    expect(unlinkCustomerMock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByText("Remove from business"));
    await waitFor(() => expect(unlinkCustomerMock).toHaveBeenCalledWith(1, 1));
  });

  it("adjusts points through the existing loyalty-points API", async () => {
    adjustPointsMock.mockResolvedValue({
      message: "Loyalty points updated",
      loyalty_points: 50,
    });
    render(<CustomersTab businessId={1} />);
    expect(await screen.findByText(/AI Guest 16/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "AI Guest 16" }));
    fireEvent.click(await screen.findByTestId("crm-adjust-points"));
    const input = await screen.findByTestId("crm-points-input");
    fireEvent.change(input, { target: { value: "50" } });
    fireEvent.click(screen.getByTestId("crm-points-save"));

    await waitFor(() =>
      expect(adjustPointsMock).toHaveBeenCalledWith(1, 1, 50, ""),
    );
    expect(toastMock.success).toHaveBeenCalledWith("Loyalty points updated");
  });
});
