/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import RecurringTemplatesPanel from "./RecurringTemplatesPanel";
import { accountingApi } from "@/api/accounting";

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    listRecurringTemplates: jest.fn(),
    createRecurringTemplate: jest.fn(),
    updateRecurringTemplate: jest.fn(),
    deleteRecurringTemplate: jest.fn(),
    listAccountingCategories: jest.fn(),
  },
}));

const listTemplates = accountingApi.listRecurringTemplates as jest.Mock;
const createTemplate = accountingApi.createRecurringTemplate as jest.Mock;
const listCategories = accountingApi.listAccountingCategories as jest.Mock;

describe("RecurringTemplatesPanel L6-13 shared fields", () => {
  beforeEach(() => {
    listTemplates.mockResolvedValue([]);
    createTemplate.mockResolvedValue({ id: 1 });
    listCategories.mockResolvedValue({
      defaults: [{ key: "rent" }, { key: "utilities" }],
      custom: [{ key: "custom_supplies" }],
    });
  });

  function getCategorySelect(): HTMLSelectElement {
    const containers = screen.getAllByTestId("hidden-select-container");
    for (const container of containers) {
      const label = container.querySelector("label");
      if (label?.textContent?.includes("entryForm.category")) {
        const select = container.querySelector("select");
        if (select) return select;
      }
    }
    throw new Error("hidden category select not found");
  }

  function getFieldInput(label: string): HTMLInputElement | HTMLTextAreaElement {
    const nodes = screen.getAllByLabelText(label);
    const control = nodes.find(
      (n) =>
        n instanceof HTMLInputElement || n instanceof HTMLTextAreaElement,
    );
    if (!control) throw new Error(`input/textarea for ${label} not found`);
    return control;
  }

  it("uses category Select and posts notes/reference/currency", async () => {
    render(
      <RecurringTemplatesPanel businessId="42" currency="ARS" t={(k) => k} />,
    );

    fireEvent.click(screen.getByText("recurring.manage"));
    await waitFor(() => expect(listTemplates).toHaveBeenCalledWith("42"));
    fireEvent.click(screen.getByTestId("recurring-create-toggle"));

    await waitFor(() => {
      const opts = Array.from(getCategorySelect().querySelectorAll("option")).map(
        (o) => o.getAttribute("value"),
      );
      expect(opts).toContain("rent");
    });
    expect(screen.getByTestId("ledger-entry-fields")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("other")).toBeNull();

    fireEvent.change(getCategorySelect(), { target: { value: "rent" } });
    fireEvent.change(getFieldInput("entryForm.description"), {
      target: { value: "Monthly rent" },
    });
    fireEvent.change(getFieldInput("entryForm.amount"), {
      target: { value: "1200" },
    });
    fireEvent.change(getFieldInput("entryForm.notes"), {
      target: { value: "Lease building A" },
    });
    fireEvent.change(getFieldInput("entryForm.reference"), {
      target: { value: "INV-9" },
    });

    fireEvent.click(screen.getByTestId("recurring-create-submit"));

    await waitFor(() => expect(createTemplate).toHaveBeenCalled());
    const payload = createTemplate.mock.calls[0][1];
    expect(payload).toMatchObject({
      description: "Monthly rent",
      amount: 1200,
      currency: "ARS",
      notes: "Lease building A",
      reference: "INV-9",
      category: "rent",
    });
  });
});
