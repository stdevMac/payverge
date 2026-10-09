import {
  localizedProposalSummary,
  localizedProposalWarning,
} from "../proposalCopy";

const t = (key: string, params?: Record<string, string | number>) => {
  if (key === "proposal.summaryAffected" || key === "proposal.summaryAffected_other") {
    return `Vista previa de ${params?.count}`;
  }
  if (key === "proposal.summaryAffected_one") {
    return `Vista previa de ${params?.count} elemento.`;
  }
  if (key === "proposal.summaryNone") return "Ningún elemento cambiaría.";
  if (key === "proposal.warnings.noMatch") return "Ningún elemento coincide.";
  if (key === "proposal.warnings.largeChange") return "Cambio grande.";
  if (key === "proposal.warnings.generic") return "Revisa la advertencia.";
  return key;
};

describe("L4-18 proposal summary + warnings localization", () => {
  it("rebuilds summary for Spanish operators instead of English Go prose", () => {
    const summary = localizedProposalSummary(
      {
        kind: "menu.adjust_prices",
        preview: {
          summary: "Raised 3 item(s) by 10%.",
          affected_count: 3,
          examples: [],
        } as any,
      },
      "es",
      t,
    );
    expect(summary).toBe("Vista previa de 3");
    expect(summary).not.toMatch(/Raised|item\(s\)/i);
  });

  it("maps known English warnings and never returns raw English in es", () => {
    expect(
      localizedProposalWarning('No items matched scope "item:x"', "es", t),
    ).toBe("Ningún elemento coincide.");
    expect(localizedProposalWarning("Large swing on Flan", "es", t)).toBe(
      "Cambio grande.",
    );
    expect(localizedProposalWarning("Some unknown English warning", "es", t)).toBe(
      "Revisa la advertencia.",
    );
  });

  it("rebuilds English count summaries from singular/other keys, never (s)", () => {
    const tEn = (key: string, params?: Record<string, string | number>) => {
      if (key === "proposal.summaryAffected_other") {
        return `Preview of ${params?.count} items.`;
      }
      if (key === "proposal.summaryAffected_one") {
        return `Preview of ${params?.count} item.`;
      }
      return key;
    };
    expect(
      localizedProposalSummary(
        {
          kind: "menu.adjust_prices",
          preview: {
            summary: "Raised 3 item(s) by 10%.",
            affected_count: 3,
            examples: [],
          } as any,
        },
        "en",
        tEn,
      ),
    ).toBe("Preview of 3 items.");
    expect(
      localizedProposalSummary(
        {
          kind: "menu.adjust_prices",
          preview: {
            summary: "Raised 1 item(s) by 10%.",
            affected_count: 1,
            examples: [],
          } as any,
        },
        "en",
        tEn,
      ),
    ).toBe("Preview of 1 item.");
  });
});
