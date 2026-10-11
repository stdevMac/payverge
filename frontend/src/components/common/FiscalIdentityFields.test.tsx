/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";

import {
  FiscalIdentityFields,
  emptyFiscalIdentity,
  validateFiscalIdentity,
  fiscalIdentityToPayload,
  type FiscalIdentityValue,
} from "./FiscalIdentityFields";

// Minimal identity translator: the component resolves keys like
// "fiscalCustomer.toggle" — return the key itself so assertions can target it.
const t = (key: string) => key;

// Controlled test harness: the parent owns the value, mirroring the real
// CartModal/BillDetailsModal wiring.
function Harness({
  onState,
}: {
  onState?: (value: FiscalIdentityValue, valid: boolean) => void;
}) {
  const [value, setValue] = React.useState<FiscalIdentityValue>(
    emptyFiscalIdentity(),
  );
  const [valid, setValid] = React.useState(true);
  return (
    <>
      <FiscalIdentityFields
        value={value}
        onChange={(next, isValid) => {
          setValue(next);
          setValid(isValid);
          onState?.(next, isValid);
        }}
        t={t}
      />
      <span data-testid="valid">{valid ? "valid" : "invalid"}</span>
    </>
  );
}

describe("validateFiscalIdentity (pure)", () => {
  it("treats an empty identity as valid (optional at checkout)", () => {
    expect(validateFiscalIdentity(emptyFiscalIdentity())).toBeNull();
  });

  it("rejects a 10-digit CUIT", () => {
    const v: FiscalIdentityValue = {
      ...emptyFiscalIdentity(),
      docType: "CUIT",
      docNumber: "2012345678",
    };
    expect(validateFiscalIdentity(v)).toBe("docNumber");
  });

  it("accepts an 11-digit CUIT (with separators)", () => {
    const v: FiscalIdentityValue = {
      ...emptyFiscalIdentity(),
      docType: "CUIT",
      docNumber: "20-12345678-3",
    };
    expect(validateFiscalIdentity(v)).toBeNull();
  });

  it("rejects a 6-digit DNI and accepts a 7-8 digit DNI", () => {
    expect(
      validateFiscalIdentity({
        ...emptyFiscalIdentity(),
        docType: "DNI",
        docNumber: "123456",
      }),
    ).toBe("docNumber");
    expect(
      validateFiscalIdentity({
        ...emptyFiscalIdentity(),
        docType: "DNI",
        docNumber: "12345678",
      }),
    ).toBeNull();
  });

  it("maps a populated identity to a payload, trimming and omitting blanks", () => {
    const payload = fiscalIdentityToPayload({
      docType: "CUIT",
      docNumber: " 20-12345678-3 ",
      taxCondition: "responsable_inscripto",
      name: "  ACME SA  ",
      email: "",
    });
    expect(payload).toEqual({
      fiscal_customer_doc_type: "CUIT",
      fiscal_customer_doc_number: "20-12345678-3",
      fiscal_customer_tax_condition: "responsable_inscripto",
      fiscal_customer_name: "ACME SA",
    });
  });

  it("emits an empty payload for an empty identity (clears to NULL)", () => {
    expect(fiscalIdentityToPayload(emptyFiscalIdentity())).toEqual({});
  });
});

describe("FiscalIdentityFields (UI)", () => {
  it("is collapsed by default — fields hidden behind the toggle", () => {
    render(<Harness />);
    // The toggle/disclosure is present...
    expect(
      screen.getByText("fiscalCustomer.toggle"),
    ).toBeInTheDocument();
    // ...but the doc-number input is not rendered until expanded.
    expect(
      screen.queryByLabelText("fiscalCustomer.docNumberLabel"),
    ).not.toBeInTheDocument();
  });

  it("reveals the fields when the toggle is pressed", () => {
    render(<Harness />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));
    expect(
      screen.getByLabelText("fiscalCustomer.docNumberLabel"),
    ).toBeInTheDocument();
  });

  it("shows an inline error and reports invalid for a 10-digit CUIT", () => {
    const onState = jest.fn();
    render(<Harness onState={onState} />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));

    // Default doc type is CUIT for this test path: select it explicitly via the
    // hidden native select the component renders for testability.
    const docTypeSelect = screen.getByLabelText(
      "fiscalCustomer.docTypeLabel",
    ) as HTMLSelectElement;
    fireEvent.change(docTypeSelect, { target: { value: "CUIT" } });

    const docNumber = screen.getByLabelText(
      "fiscalCustomer.docNumberLabel",
    ) as HTMLInputElement;
    fireEvent.change(docNumber, { target: { value: "2012345678" } });

    // Inline error surfaces and is announced; the input points at that alert.
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("fiscalCustomer.errorCuit");
    expect(docNumber).toHaveAttribute("aria-describedby", alert.id);
    // And the last reported validity is invalid (blocks submit).
    const lastCall = onState.mock.calls[onState.mock.calls.length - 1];
    expect(lastCall[1]).toBe(false);
  });

  it("reports valid and the right payload for an 11-digit CUIT", () => {
    const onState = jest.fn();
    render(<Harness onState={onState} />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));

    const docTypeSelect = screen.getByLabelText(
      "fiscalCustomer.docTypeLabel",
    ) as HTMLSelectElement;
    fireEvent.change(docTypeSelect, { target: { value: "CUIT" } });

    const docNumber = screen.getByLabelText(
      "fiscalCustomer.docNumberLabel",
    ) as HTMLInputElement;
    fireEvent.change(docNumber, { target: { value: "20123456783" } });

    const taxSelect = screen.getByLabelText(
      "fiscalCustomer.taxConditionLabel",
    ) as HTMLSelectElement;
    fireEvent.change(taxSelect, { target: { value: "responsable_inscripto" } });

    expect(
      screen.queryByText("fiscalCustomer.errorCuit"),
    ).not.toBeInTheDocument();

    const lastCall = onState.mock.calls[onState.mock.calls.length - 1];
    const [value, valid] = lastCall as [FiscalIdentityValue, boolean];
    expect(valid).toBe(true);
    expect(fiscalIdentityToPayload(value)).toEqual({
      fiscal_customer_doc_type: "CUIT",
      fiscal_customer_doc_number: "20123456783",
      fiscal_customer_tax_condition: "responsable_inscripto",
    });
  });

  it("leaves the identity optional — empty fields stay valid (below threshold)", () => {
    const onState = jest.fn();
    render(<Harness onState={onState} />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));

    // No input — the form stays valid because identity is optional.
    expect(screen.getByTestId("valid")).toHaveTextContent("valid");
    expect(
      screen.queryByText("fiscalCustomer.errorCuit"),
    ).not.toBeInTheDocument();
  });

  it("renders the tax condition options", () => {
    render(<Harness />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));
    const taxSelect = screen.getByLabelText(
      "fiscalCustomer.taxConditionLabel",
    );
    expect(
      within(taxSelect).getByText("fiscalCustomer.taxConsumidorFinal"),
    ).toBeInTheDocument();
    expect(
      within(taxSelect).getByText("fiscalCustomer.taxResponsableInscripto"),
    ).toBeInTheDocument();
  });
});

describe("FiscalIdentityFields — actions slot (#103)", () => {
  it("hides actions while collapsed and shows them only when expanded", () => {
    render(
      <FiscalIdentityFields
        value={emptyFiscalIdentity()}
        onChange={() => {}}
        t={t}
        actions={<button type="button">fiscalCustomer.save</button>}
      />,
    );

    expect(screen.queryByTestId("fiscal-identity-actions")).toBeNull();
    expect(screen.queryByText("fiscalCustomer.save")).toBeNull();

    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));

    expect(screen.getByTestId("fiscal-identity-actions")).toBeInTheDocument();
    expect(screen.getByText("fiscalCustomer.save")).toBeInTheDocument();
  });
});

describe("FiscalIdentityFields — Responsable Inscripto ⇄ CUIT coupling (T11)", () => {
  it("shows a soft CUIT hint when condition is responsable_inscripto and doc type is empty", () => {
    const onState = jest.fn();
    render(<Harness onState={onState} />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));

    const taxSelect = screen.getByLabelText(
      "fiscalCustomer.taxConditionLabel",
    ) as HTMLSelectElement;
    fireEvent.change(taxSelect, { target: { value: "responsable_inscripto" } });

    // The soft hint surfaces (no doc type chosen → backend would downgrade to
    // factura B, but the user likely intended factura A).
    const hint = screen.getByRole("note");
    expect(hint).toHaveTextContent("fiscalCustomer.cuitHint");
    expect(hint.className).toContain("text-amber-900");
    expect(taxSelect).toHaveAttribute("aria-describedby", hint.id);

    // It is a SOFT hint — the form is still submittable (validity unchanged).
    expect(screen.getByTestId("valid")).toHaveTextContent("valid");
    const lastCall = onState.mock.calls[onState.mock.calls.length - 1];
    expect(lastCall[1]).toBe(true);
    // And it must NOT be the hard doc-shape error.
    expect(
      screen.queryByText("fiscalCustomer.errorCuit"),
    ).not.toBeInTheDocument();
  });

  it("shows the soft CUIT hint when condition is responsable_inscripto but doc type is DNI", () => {
    const onState = jest.fn();
    render(<Harness onState={onState} />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));

    const docTypeSelect = screen.getByLabelText(
      "fiscalCustomer.docTypeLabel",
    ) as HTMLSelectElement;
    fireEvent.change(docTypeSelect, { target: { value: "DNI" } });

    const taxSelect = screen.getByLabelText(
      "fiscalCustomer.taxConditionLabel",
    ) as HTMLSelectElement;
    fireEvent.change(taxSelect, { target: { value: "responsable_inscripto" } });

    expect(screen.getByText("fiscalCustomer.cuitHint")).toBeInTheDocument();

    // Still soft: a valid-shaped DNI with RI must NOT block submit.
    const docNumber = screen.getByLabelText(
      "fiscalCustomer.docNumberLabel",
    ) as HTMLInputElement;
    fireEvent.change(docNumber, { target: { value: "12345678" } });
    expect(screen.getByTestId("valid")).toHaveTextContent("valid");
    const lastCall = onState.mock.calls[onState.mock.calls.length - 1];
    expect(lastCall[1]).toBe(true);
  });

  it("hides the CUIT hint once the doc type is CUIT (or CUIL)", () => {
    render(<Harness />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));

    const taxSelect = screen.getByLabelText(
      "fiscalCustomer.taxConditionLabel",
    ) as HTMLSelectElement;
    fireEvent.change(taxSelect, { target: { value: "responsable_inscripto" } });
    expect(screen.getByText("fiscalCustomer.cuitHint")).toBeInTheDocument();

    const docTypeSelect = screen.getByLabelText(
      "fiscalCustomer.docTypeLabel",
    ) as HTMLSelectElement;
    fireEvent.change(docTypeSelect, { target: { value: "CUIT" } });
    expect(
      screen.queryByText("fiscalCustomer.cuitHint"),
    ).not.toBeInTheDocument();

    fireEvent.change(docTypeSelect, { target: { value: "CUIL" } });
    expect(
      screen.queryByText("fiscalCustomer.cuitHint"),
    ).not.toBeInTheDocument();
  });

  it("does not show the CUIT hint for other tax conditions (monotributo / consumidor final)", () => {
    render(<Harness />);
    fireEvent.click(screen.getByText("fiscalCustomer.toggle"));

    const taxSelect = screen.getByLabelText(
      "fiscalCustomer.taxConditionLabel",
    ) as HTMLSelectElement;
    fireEvent.change(taxSelect, { target: { value: "monotributo" } });
    expect(
      screen.queryByText("fiscalCustomer.cuitHint"),
    ).not.toBeInTheDocument();

    fireEvent.change(taxSelect, { target: { value: "" } });
    expect(
      screen.queryByText("fiscalCustomer.cuitHint"),
    ).not.toBeInTheDocument();
  });
});

describe("fiscal email capture", () => {
  it("accepts an empty email (optional field)", () => {
    const v = { ...emptyFiscalIdentity(), email: "" };
    expect(validateFiscalIdentity(v)).toBeNull();
  });

  it("flags a malformed email", () => {
    const v = { ...emptyFiscalIdentity(), email: "not-an-email" };
    expect(validateFiscalIdentity(v)).toBe("email");
  });

  it("accepts a well-formed email", () => {
    const v = { ...emptyFiscalIdentity(), email: "guest@example.com" };
    expect(validateFiscalIdentity(v)).toBeNull();
  });

  it.each(["guest@example", "guest@.com", "guest@example.", "a@b@c.com", "gu est@example.com"])(
    "rejects %s",
    (email) => {
      expect(validateFiscalIdentity({ ...emptyFiscalIdentity(), email })).toBe("email");
    },
  );

  it("accepts a short domain with an inner dot", () => {
    expect(
      validateFiscalIdentity({ ...emptyFiscalIdentity(), email: "a@b.co" }),
    ).toBeNull();
  });

  it("rejects a long dotted string without backtracking", () => {
    const email = `a@${".".repeat(50_000)}@`;
    const started = Date.now();
    expect(validateFiscalIdentity({ ...emptyFiscalIdentity(), email })).toBe("email");
    expect(Date.now() - started).toBeLessThan(1000);
  });

  it("maps email into the wire payload, trimmed", () => {
    const v = { ...emptyFiscalIdentity(), email: "  guest@example.com  " };
    expect(fiscalIdentityToPayload(v).fiscal_customer_email).toBe(
      "guest@example.com",
    );
  });

  it("omits a blank email from the payload", () => {
    const v = { ...emptyFiscalIdentity(), email: "   " };
    expect("fiscal_customer_email" in fiscalIdentityToPayload(v)).toBe(false);
  });
});
