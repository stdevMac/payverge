/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import PositionsManager, {
  type PositionsManagerLabels,
} from "./PositionsManager";
import { positionsApi } from "@/api/positions";

jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/api/positions", () => ({
  positionsApi: { list: jest.fn(), create: jest.fn(), remove: jest.fn() },
}));

const mocked = positionsApi as unknown as {
  list: jest.Mock;
  create: jest.Mock;
  remove: jest.Mock;
};

const labels: PositionsManagerLabels = {
  title: "Positions & roles",
  subtitle: "The roles you schedule shifts around — every shift needs one.",
  addButton: "Add position",
  namePlaceholder: "Position name",
  departmentPlaceholder: "Department (optional)",
  departmentHint:
    "Optional — group positions into departments like Front of house (FOH). Each department gets its own team chat channel.",
  suggestionsLabel: "Common roles — tap to add:",
  suggestionNames: "Server, Host, Kitchen, Bartender, Busser",
  empty: "No positions yet",
  retire: "Retire",
  retireConfirmTitle: "Archive this position?",
  retireConfirmDescription:
    "Server will be archived and hidden from new shifts. You can reactivate it later — this is not permanent destroy.",
  retireConfirmAction: "Archive position",
  saved: "Saved",
  saveError: "Save failed",
  duplicateName: "A position with this name already exists.",
  retired: "Retired",
  retireError: "Retire failed",
  loadError: "Load failed",
  retry: "Retry",
  loading: "Loading",
};

const serverPosition = {
  id: 3,
  business_id: 42,
  name: "Server",
  color_hex: "#1a6b6a",
  department: "FOH",
  is_active: true,
  sort_order: 0,
};

function renderManager(
  ui: React.ReactElement,
  options?: { invalidateQueries?: jest.Mock },
) {
  const invalidateQueries =
    options?.invalidateQueries ??
    jest.fn().mockResolvedValue(undefined);
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // Spy so create/retire success can assert the shared ["positions", id] key.
  jest.spyOn(qc, "invalidateQueries").mockImplementation(invalidateQueries as any);
  return {
    qc,
    invalidateQueries,
    ...render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>),
  };
}

beforeEach(() => {
  jest.clearAllMocks();
});

describe("PositionsManager", () => {
  it("offers quick-add chips on an empty list and creates on tap", async () => {
    mocked.list.mockResolvedValue([]);
    mocked.create.mockResolvedValue(serverPosition);

    renderManager(<PositionsManager businessId="42" labels={labels} />);
    // findBy* rides its own retry budget so the on-mount list fetch resolving
    // late under full-suite worker contention doesn't trip the 5s default.
    expect(
      await screen.findByText("Common roles — tap to add:", undefined, {
        timeout: 10_000,
      }),
    ).toBeInTheDocument();
    // All five suggested roles render as tappable chips.
    for (const role of ["Server", "Host", "Kitchen", "Bartender", "Busser"]) {
      expect(screen.getByRole("button", { name: role })).toBeInTheDocument();
    }

    await userEvent.click(screen.getByRole("button", { name: "Server" }));
    await waitFor(() =>
      expect(mocked.create).toHaveBeenCalledWith("42", {
        name: "Server",
        department: "",
        sort_order: 0,
      }),
    );
    // The list reloads after a quick-add.
    await waitFor(() => expect(mocked.list).toHaveBeenCalledTimes(2));
  });

  it("hides the quick-add row once a position exists, and explains departments", async () => {
    mocked.list.mockResolvedValue([serverPosition]);

    renderManager(<PositionsManager businessId="42" labels={labels} />);
    expect(
      await screen.findByText("Server", undefined, { timeout: 10_000 }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Common roles — tap to add:")).toBeNull();
    // The department helper always renders under the form.
    expect(
      screen.getByText(/Each department gets its own team chat channel/),
    ).toBeInTheDocument();
  });

  it("invalidates the shared positions query after create (L5-20)", async () => {
    mocked.list.mockResolvedValue([]);
    mocked.create.mockResolvedValue(serverPosition);
    const invalidateQueries = jest.fn().mockResolvedValue(undefined);

    renderManager(<PositionsManager businessId="42" labels={labels} />, {
      invalidateQueries,
    });
    expect(
      await screen.findByText("Common roles — tap to add:", undefined, {
        timeout: 10_000,
      }),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Server" }));
    await waitFor(() => expect(mocked.create).toHaveBeenCalled());
    await waitFor(() =>
      expect(invalidateQueries).toHaveBeenCalledWith({
        queryKey: ["positions", "42"],
      }),
    );
  });

  // L5-19: retire must not fire on a single ungated click; confirm first.
  it("does not call remove until the retire ConfirmationModal is confirmed", async () => {
    const user = userEvent.setup();
    mocked.list.mockResolvedValue([serverPosition]);
    mocked.remove.mockResolvedValue(undefined);

    renderManager(<PositionsManager businessId="42" labels={labels} />);
    expect(
      await screen.findByText("Server", undefined, { timeout: 10_000 }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Retire Server" }));

    // Modal opens with soft-archive copy; API not called yet.
    expect(
      await screen.findByText("Archive this position?"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/archived and hidden from new shifts/i),
    ).toBeInTheDocument();
    expect(mocked.remove).not.toHaveBeenCalled();

    await user.click(
      screen.getByRole("button", { name: "Archive position" }),
    );
    await waitFor(() =>
      expect(mocked.remove).toHaveBeenCalledWith("42", 3),
    );
  });

  // L5-18: case-insensitive duplicate names rejected on form + chips.
  it("rejects a case-insensitive duplicate position name without calling create (L5-18)", async () => {
    mocked.list.mockResolvedValue([serverPosition]);
    mocked.create.mockResolvedValue(serverPosition);
    renderManager(<PositionsManager businessId="42" labels={labels} />);
    expect(
      await screen.findByText("Server", undefined, { timeout: 10_000 }),
    ).toBeInTheDocument();

    const input = screen.getByTestId("position-name-input");
    await userEvent.type(input, "server");
    await userEvent.click(screen.getByRole("button", { name: "Add position" }));

    expect(mocked.create).not.toHaveBeenCalled();
    expect(
      await screen.findByText("A position with this name already exists."),
    ).toBeInTheDocument();
  });

  it("invalidates the shared positions query after confirmed retire (L5-20)", async () => {
    const user = userEvent.setup();
    mocked.list.mockResolvedValue([serverPosition]);
    mocked.remove.mockResolvedValue(undefined);
    const invalidateQueries = jest.fn().mockResolvedValue(undefined);

    renderManager(<PositionsManager businessId="42" labels={labels} />, {
      invalidateQueries,
    });
    expect(
      await screen.findByText("Server", undefined, { timeout: 10_000 }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Retire Server" }));
    await user.click(
      await screen.findByRole("button", { name: "Archive position" }),
    );
    await waitFor(() =>
      expect(mocked.remove).toHaveBeenCalledWith("42", 3),
    );
    await waitFor(() =>
      expect(invalidateQueries).toHaveBeenCalledWith({
        queryKey: ["positions", "42"],
      }),
    );
  });
});
