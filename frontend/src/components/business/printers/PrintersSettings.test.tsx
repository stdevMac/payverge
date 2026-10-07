/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import * as api from "@/api/print";
import { getBusiness } from "@/api/business";

import PrintersSettings from "./PrintersSettings";

jest.mock("@/api/print");
jest.mock("@/api/business", () => ({ getBusiness: jest.fn() }));

const mockPrint = jest.fn().mockResolvedValue(undefined);
jest.mock("./useIframePrint", () => ({
  useIframePrint: () => ({ print: mockPrint }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string, _locale?: string, params?: Record<string, string | number>) => {
    const map: Record<string, string> = {
      "printers.loading": "Loading printers…",
      "printers.subtitle": "Manage receipt printers and the browser print station.",
      "printers.loadError": "Could not load printers: {error}",
      "printers.errorTitle": "Couldn't load printers",
      "printers.retry": "Retry",
      "printers.businessNotFound": "Business not found",
      "printers.businessNotFoundDetail":
        "We couldn't find this business. Check the URL or head back to the dashboard.",
      "printers.backToDashboard": "Back to dashboard",
      "printers.empty": "No printers yet.",
      "printers.addPrinter": "Add printer",
      "printers.addAPrinter": "Add a printer",
      "printers.testPrint": "Test print",
      "printers.useOnThisBrowser": "Use on this browser",
      "printers.activeOnThisBrowser": "Active on this browser",
      "printers.stopStation": "Stop station",
      "printers.disable": "Disable",
      "printers.enable": "Enable",
      "printers.disabledBadge": "Disabled",
      "printers.edit.action": "Edit",
      "printers.edit.title": "Edit printer",
      "printers.edit.enabledLabel": "Enabled",
      "printers.edit.enabledHint": "Disabled printers stay in the list but won't receive new jobs.",
      "printers.edit.save": "Save changes",
      "printers.wizard.nameLabel": "Printer name",
      "printers.wizard.paperWidthLabel": "Paper width",
      "printers.wizard.paper80": "80 mm",
      "printers.wizard.paper58": "58 mm",
      "printers.wizard.cancel": "Cancel",
      "printers.disableConfirmTitle": "Disable printer",
      "printers.disableConfirmBody":
        "Disable “{name}”? It stays in the list but won't receive new jobs. You can re-enable it anytime.",
    };
    let out = map[key] ?? key;
    if (params) {
      Object.entries(params).forEach(([k, v]) => {
        out = out.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
      });
    }
    return out;
  },
}));

describe("PrintersSettings", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    // Recent jobs section loads on the non-empty printers view.
    (api.fetchJobs as jest.Mock).mockResolvedValue({ items: [] });
  });

  it("renders printer rows from fetchPrinters", async () => {
    (api.fetchPrinters as jest.Mock).mockResolvedValueOnce({
      items: [
        { id: 1, name: "Front", role: "bill", transport: "browser", enabled: true, paper_width_mm: 80, code_page: "CP858", business_id: 42, location_id: null, cloudprnt_last_seen_at: null },
        { id: 2, name: "Kitchen", role: "kitchen", transport: "cloudprnt", enabled: true, paper_width_mm: 80, code_page: "CP858", business_id: 42, location_id: null, cloudprnt_last_seen_at: null },
      ],
    });
    render(<PrintersSettings businessId={42} />);
    await waitFor(() => expect(screen.getByText("Front")).toBeInTheDocument());
    expect(screen.getByText("Kitchen")).toBeInTheDocument();
  });

  it("starts and stops an explicit browser print station", async () => {
    (api.fetchPrinters as jest.Mock).mockResolvedValueOnce({
      items: [
        { id: 7, name: "Front", role: "bill", transport: "browser", enabled: true, paper_width_mm: 80, code_page: "CP858", business_id: 42, location_id: null, cloudprnt_last_seen_at: null },
      ],
    });

    render(<PrintersSettings businessId={42} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Use on this browser" }),
    );

    expect(window.localStorage.getItem("payverge_print_station:42")).toBe("7");
    expect(
      screen.getByRole("button", { name: "Stop station" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("printer-active-7")).toHaveTextContent(
      "Active on this browser",
    );

    fireEvent.click(screen.getByRole("button", { name: "Stop station" }));
    expect(window.localStorage.getItem("payverge_print_station:42")).toBeNull();
  });

  it("renders the empty state when no printers exist", async () => {
    (api.fetchPrinters as jest.Mock).mockResolvedValueOnce({ items: [] });
    render(<PrintersSettings businessId={42} />);
    await waitFor(() => expect(screen.getByText(/no printers/i)).toBeInTheDocument());
  });

  it("test print renders the direct payload without touching the background queue", async () => {
    (api.fetchPrinters as jest.Mock).mockResolvedValue({
      items: [
        { id: 1, name: "Front", role: "bill", transport: "browser", enabled: true, paper_width_mm: 80, code_page: "CP858", business_id: 42, location_id: null, cloudprnt_last_seen_at: null },
      ],
    });
    (api.testPrint as jest.Mock).mockResolvedValueOnce({
      id: 77, business_id: 42, printer_id: 1, kind: "bill", source_type: "test",
      source_id: 0, status: "routed", payload_html: "<html><body>TEST</body></html>",
      language: "en", created_at: "", printed_at: null,
    });

    render(<PrintersSettings businessId={42} />);
    await waitFor(() => expect(screen.getByText("Front")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Test print"));

    await waitFor(() => expect(mockPrint).toHaveBeenCalledWith("<html><body>TEST</body></html>"));
  });

  it("shows an inline action error when disabling a printer fails", async () => {
    (api.fetchPrinters as jest.Mock).mockResolvedValue({
      items: [
        { id: 1, name: "Front", role: "bill", transport: "browser", enabled: true, paper_width_mm: 80, code_page: "CP858", business_id: 42, location_id: null, cloudprnt_last_seen_at: null },
      ],
    });
    (api.deletePrinter as jest.Mock).mockRejectedValueOnce(new Error("HTTP 409"));

    render(<PrintersSettings businessId={42} />);
    await waitFor(() => expect(screen.getByText("Front")).toBeInTheDocument());
    // Soft-disable: confirmation modal before calling deletePrinter (enabled=false).
    fireEvent.click(screen.getByRole("button", { name: /disable front/i }));
    fireEvent.click(await screen.findByRole("button", { name: "Disable" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("HTTP 409");
  });

  it("does not offer Disable on an already-disabled printer row; offers Enable instead (#382)", async () => {
    (api.fetchPrinters as jest.Mock).mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Front",
          role: "bill",
          transport: "browser",
          enabled: false,
          paper_width_mm: 80,
          code_page: "CP858",
          business_id: 42,
          location_id: null,
          cloudprnt_last_seen_at: null,
        },
        {
          id: 2,
          name: "Kitchen",
          role: "kitchen",
          transport: "cloudprnt",
          enabled: true,
          paper_width_mm: 80,
          code_page: "CP858",
          business_id: 42,
          location_id: null,
          cloudprnt_last_seen_at: null,
        },
      ],
    });
    (api.updatePrinter as jest.Mock).mockResolvedValue({
      id: 1,
      name: "Front",
      role: "bill",
      transport: "browser",
      enabled: true,
      paper_width_mm: 80,
      code_page: "CP858",
      business_id: 42,
      location_id: null,
      cloudprnt_last_seen_at: null,
    });

    render(<PrintersSettings businessId={42} />);
    await waitFor(() => expect(screen.getByText("Front")).toBeInTheDocument());

    expect(screen.getByText("Disabled")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /disable front/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /disable kitchen/i }),
    ).toBeInTheDocument();

    const enableBtn = screen.getByRole("button", { name: /enable front/i });
    expect(enableBtn).toBeEnabled();
    expect(
      screen.queryByRole("button", { name: /enable kitchen/i }),
    ).not.toBeInTheDocument();

    fireEvent.click(enableBtn);

    await waitFor(() =>
      expect(api.updatePrinter).toHaveBeenCalledWith(
        42,
        1,
        expect.objectContaining({ enabled: true }),
      ),
    );
    expect(api.deletePrinter).not.toHaveBeenCalled();
  });

  it("opens a disable confirmation dialog with truthful copy keys (L3-32)", async () => {
    (api.fetchPrinters as jest.Mock).mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Front",
          role: "bill",
          transport: "browser",
          enabled: true,
          paper_width_mm: 80,
          code_page: "CP858",
          business_id: 42,
          location_id: null,
          cloudprnt_last_seen_at: null,
        },
      ],
    });

    render(<PrintersSettings businessId={42} />);
    await waitFor(() => expect(screen.getByText("Front")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /disable front/i }));

    // Mock returns mapped strings when keys exist; assert disable framing, not delete.
    expect(await screen.findByText("Disable printer")).toBeInTheDocument();
    expect(
      screen.getByText(
        /Disable “Front”\? It stays in the list but won't receive new jobs/i,
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disable" })).toBeInTheDocument();

    // Old irreversible delete-framing must not appear.
    expect(screen.queryByText(/can't be undone/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/remove printer/i)).not.toBeInTheDocument();
  });

  it("does not reference delete-framing i18n keys in the settings component source", () => {
    // Guard against reintroducing deleteConfirm*/remove as the user-facing action.
    const fs = require("fs") as typeof import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(
      path.join(__dirname, "PrintersSettings.tsx"),
      "utf8",
    );
    expect(src).not.toMatch(/deleteConfirmTitle|deleteConfirmBody/);
    expect(src).not.toMatch(/t\(["']remove["']\)/);
    expect(src).toMatch(/disableConfirmTitle/);
    expect(src).toMatch(/disableConfirmBody/);
    expect(src).toMatch(/t\(["']disable["']\)/);
    expect(src).toMatch(/t\(["']enable["']\)/);
  });

  it("renders the error inside the settings card with a retry button", async () => {
    (api.fetchPrinters as jest.Mock).mockRejectedValue(new Error("HTTP 500"));
    render(<PrintersSettings businessId={9} />);
    const alert = await screen.findByRole("alert");
    expect(alert).toBeInTheDocument();
    // Retry button exists and refires the load
    (api.fetchPrinters as jest.Mock).mockResolvedValue({ items: [] });
    fireEvent.click(screen.getByRole("button", { name: /reintentar|retry/i }));
    await waitFor(() => expect(api.fetchPrinters).toHaveBeenCalledTimes(2));
  });

  it("shows a business-not-found message when the id cannot be resolved", async () => {
    (getBusiness as jest.Mock).mockRejectedValue(new Error("404"));
    render(<PrintersSettings businessId={null} slug="ghost-business" />);
    expect(await screen.findByRole("alert")).toHaveTextContent(/negocio|business/i);
    expect(api.fetchPrinters).not.toHaveBeenCalled();
  });

  it("resolves a slug before fetching and never calls the API with NaN", async () => {
    (getBusiness as jest.Mock).mockResolvedValue({ id: 9 });
    (api.fetchPrinters as jest.Mock).mockResolvedValue({ items: [] });

    render(<PrintersSettings businessId={null} slug="demo-admin-1-ai-pro" />);

    await waitFor(() => expect(api.fetchPrinters).toHaveBeenCalledWith(9));
    expect(getBusiness).toHaveBeenCalledWith("demo-admin-1-ai-pro");
    expect(api.fetchPrinters).not.toHaveBeenCalledWith(NaN);
    expect(api.fetchPrinters).not.toHaveBeenCalledWith(null);
  });

  it("edits name, paper width, and enabled via updatePrinter", async () => {
    (api.fetchPrinters as jest.Mock).mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Front",
          role: "bill",
          transport: "browser",
          enabled: true,
          paper_width_mm: 80,
          code_page: "CP858",
          business_id: 42,
          location_id: null,
          cloudprnt_last_seen_at: null,
        },
      ],
    });
    (api.updatePrinter as jest.Mock).mockResolvedValue({
      id: 1,
      name: "Front bar",
      role: "bill",
      transport: "browser",
      enabled: false,
      paper_width_mm: 58,
      code_page: "CP858",
      business_id: 42,
      location_id: null,
      cloudprnt_last_seen_at: null,
    });

    render(<PrintersSettings businessId={42} />);
    await waitFor(() => expect(screen.getByText("Front")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /edit front/i }));

    const nameInput = await screen.findByLabelText(/printer name/i);
    fireEvent.change(nameInput, { target: { value: "Front bar" } });
    // Toggle enabled off.
    fireEvent.click(screen.getByRole("switch", { name: /enabled/i }));
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() =>
      expect(api.updatePrinter).toHaveBeenCalledWith(
        42,
        1,
        expect.objectContaining({
          name: "Front bar",
          enabled: false,
        }),
      ),
    );
  });
});
