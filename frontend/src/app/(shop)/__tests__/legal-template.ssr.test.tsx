/**
 * @jest-environment node
 *
 * F1 regression: the legal pages render the generic template on the server,
 * so the server HTML never carries upstream product-site legal text, even
 * when /instance is unreachable.
 */

import React from "react";
import { renderToString } from "react-dom/server";
import GenericLegalTemplate from "@/components/legal/GenericLegalTemplate";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import { genericLegalMetadata } from "@/lib/instance/legalPage";

const selfHost = parseInstanceInfo({
  product_name: "Trattoria OS",
  legal_entity: "Trattoria Hospitality SRL",
  company_name: "Trattoria SRL",
  support_email: "help@trattoria.example",
  public_url: "https://pos.trattoria.example",
  registration_mode: "invite",
  features: {},
});

function ssr(el: React.ReactElement): string {
  return renderToString(
    <SimpleTranslationProvider initialLocale="en">{el}</SimpleTranslationProvider>,
  );
}

describe("legal pages render the template on the server", () => {
  it.each(["privacy", "terms", "refund"] as const)(
    "%s server HTML is the filled template",
    (kind) => {
      const html = ssr(
        <GenericLegalTemplate kind={kind} serverInstance={selfHost} />,
      );
      expect(html).toContain("legal-template-banner");
      expect(html).toContain("Trattoria Hospitality SRL");
      expect(html).toContain("help@trattoria.example");
      expect(html).not.toMatch(/Payverge provides|trademarks of our company/);
    },
  );

  it("server HTML still renders the template when /instance is unknown", () => {
    const html = ssr(<GenericLegalTemplate kind="privacy" serverInstance={null} />);
    expect(html).toContain("legal-template-banner");
  });

  it("metadata for the template is neutral and noindex", () => {
    const meta = genericLegalMetadata("privacy", "en", selfHost);
    expect(meta.title).toBe("Privacy notice — Trattoria OS");
    expect(meta.robots).toEqual({ index: false, follow: false });
  });
});
