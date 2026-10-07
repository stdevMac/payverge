/** @jest-environment jsdom */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import * as api from "@/api/print";

import AddPrinterWizard from "./AddPrinterWizard";

jest.mock("@/api/print");

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "printers.wizard.title": "Add printer",
      "printers.wizard.nameLabel": "Printer name",
      "printers.wizard.roleLabel": "Role",
      "printers.wizard.roleBillReceipt": "Bill / Receipt",
      "printers.roleLabels.kitchen": "Kitchen",
      "printers.roleLabels.bar": "Bar",
      "printers.wizard.transportLabel": "Transport",
      "printers.wizard.transportBrowser": "Browser (any OS-installed printer)",
      "printers.wizard.transportCloudprnt":
        "CloudPRNT (Star / Epson cloud-poll) — coming soon",
      "printers.wizard.paperWidthLabel": "Paper width",
      "printers.wizard.paper80": "80 mm",
      "printers.wizard.paper58": "58 mm",
      "printers.wizard.cancel": "Cancel",
      "printers.wizard.save": "Save",
    };
    return map[key] ?? key;
  },
}));

describe("AddPrinterWizard", () => {
  it("creates a browser-transport printer through the wizard", async () => {
    (api.createPrinter as jest.Mock).mockResolvedValueOnce({ id: 1, name: "Front" });
    const onCreated = jest.fn();
    render(
      <AddPrinterWizard businessId={42} open onClose={() => {}} onCreated={onCreated} />,
    );

    fireEvent.change(screen.getByLabelText(/printer name/i), { target: { value: "Front" } });
    // transport defaults to "browser" — no need to click the radio
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(api.createPrinter).toHaveBeenCalledWith(42, expect.objectContaining({
      name: "Front",
      transport: "browser",
      role: "bill",
      paper_width_mm: 80,
    })));
    await waitFor(() => expect(onCreated).toHaveBeenCalled());
  });

  it("disables the CloudPRNT option in Sprint 1 and shows a coming-soon hint", () => {
    render(<AddPrinterWizard businessId={42} open onClose={() => {}} onCreated={() => {}} />);
    expect(screen.getByLabelText(/cloudprnt/i)).toBeDisabled();
    expect(screen.getByText(/sprint 2|coming/i)).toBeInTheDocument();
  });

  // L3-33 residual: create wizard must offer kitchen/bar, not only bill.
  it("renders role select with bill, kitchen, and bar options", () => {
    render(
      <AddPrinterWizard businessId={42} open onClose={() => {}} onCreated={() => {}} />,
    );
    expect(screen.getByTestId("add-printer-role")).toBeInTheDocument();
    // Labels may appear more than once (trigger + listbox); require at least one.
    expect(screen.getAllByText("Bill / Receipt").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Kitchen").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Bar").length).toBeGreaterThan(0);
  });

  it("submits kitchen role when selected", async () => {
    (api.createPrinter as jest.Mock).mockResolvedValueOnce({ id: 2, name: "Expo" });
    const onCreated = jest.fn();
    render(
      <AddPrinterWizard businessId={42} open onClose={() => {}} onCreated={onCreated} />,
    );
    fireEvent.change(screen.getByLabelText(/printer name/i), {
      target: { value: "Expo" },
    });
    // Drive role via the Select's listbox if openable; fallback: source contract
    // that kitchen key is present is covered above — force role state by selecting.
    const roleSelect = screen.getByTestId("add-printer-role");
    fireEvent.click(roleSelect);
    const kitchen = await screen.findAllByText("Kitchen");
    fireEvent.click(kitchen[kitchen.length - 1]);
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() =>
      expect(api.createPrinter).toHaveBeenCalledWith(
        42,
        expect.objectContaining({ name: "Expo", role: "kitchen" }),
      ),
    );
  });
});
