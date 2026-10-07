/** @jest-environment jsdom */

import React from "react";
import { render, screen } from "@testing-library/react";
import GenericLegalTemplate, {
  type LegalKind,
} from "@/components/legal/GenericLegalTemplate";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";

function instance(support = "help@trattoria.example") {
  return parseInstanceInfo({
    product_name: "Trattoria OS",
    company_name: "Trattoria SRL",
    legal_entity: "Trattoria Hospitality SRL",
    support_email: support,
    public_url: "https://pos.trattoria.example",
    registration_mode: "invite",
    features: {},
  });
}

function renderTemplate(kind: LegalKind) {
  return render(
    <SimpleTranslationProvider initialLocale="en">
      <GenericLegalTemplate kind={kind} />
    </SimpleTranslationProvider>,
  );
}

afterEach(() => resetInstanceCacheForTests());

describe("legal pages render the labelled generic template", () => {
  it.each([
    ["privacy", "Privacy notice"],
    ["terms", "Terms of use"],
    ["refund", "Refunds"],
  ] as const)("%s renders the template filled from /instance", (kind, title) => {
    setInstanceForTests(instance());
    const { container } = renderTemplate(kind);
    expect(screen.getByTestId("legal-template-banner")).toHaveTextContent(/Template\./);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(title);
    const text = container.textContent ?? "";
    expect(text).toContain("Trattoria Hospitality SRL");
    expect(text).toContain("help@trattoria.example");
    expect(text).not.toMatch(/payverge\.io/i);
  });

  it("says the contact is not configured instead of inventing one", () => {
    setInstanceForTests(instance(""));
    renderTemplate("privacy");
    expect(screen.getByTestId("legal-template-banner")).toHaveTextContent(
      "(contact address not configured)",
    );
  });
});
