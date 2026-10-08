/** @jest-environment jsdom */
/**
 * L3-33: EditPrinterModal must expose a Rol select (mirrors AddPrinterWizard)
 * and include role on the update payload.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mockUpdatePrinter = jest.fn().mockResolvedValue({});
jest.mock("@/api/print", () => ({
  updatePrinter: (...args: unknown[]) => mockUpdatePrinter(...args),
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

import EditPrinterModal from "./EditPrinterModal";
import type { Printer } from "@/api/print";

const printer = {
  id: 7,
  name: "Front",
  role: "bill",
  paper_width_mm: 80,
  enabled: true,
  transport: "browser",
} as unknown as Printer;

describe("L3-33 EditPrinterModal role select", () => {
  beforeEach(() => {
    mockUpdatePrinter.mockClear();
  });

  it("renders role select with current role", () => {
    render(
      <EditPrinterModal
        businessId={1}
        printer={printer}
        open
        onClose={jest.fn()}
        onSaved={jest.fn()}
      />,
    );
    expect(screen.getByTestId("edit-printer-role")).toBeInTheDocument();
  });

  it("submits role on save", async () => {
    const user = userEvent.setup();
    const onSaved = jest.fn();
    render(
      <EditPrinterModal
        businessId={1}
        printer={{ ...printer, role: "kitchen" }}
        open
        onClose={jest.fn()}
        onSaved={onSaved}
      />,
    );
    const save = screen.getByRole("button", { name: "printers.edit.save" });
    await user.click(save);
    await waitFor(() => {
      expect(mockUpdatePrinter).toHaveBeenCalled();
    });
    const payload = mockUpdatePrinter.mock.calls[0][2];
    expect(payload).toEqual(expect.objectContaining({ role: "kitchen" }));
  });
});
