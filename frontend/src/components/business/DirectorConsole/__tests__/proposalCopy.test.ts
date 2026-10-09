import { getTranslation } from "@/i18n/getTranslation";
import {
  isEnglishLocale,
  localizedProposalDescription,
  localizedProposalSummary,
  localizedProposalTitle,
} from "../proposalCopy";

const locales = ["en", "es", "es-AR"] as const;

function tFor(locale: (typeof locales)[number]) {
  return (key: string, params?: Record<string, string | number>) =>
    String(getTranslation(`directorConsole.${key}`, locale, params));
}

function proposal(count: number) {
  return {
    kind: "menu.adjust_prices" as const,
    title: "Raise all prices by 10%",
    description: `Applies to ${count} item(s).`,
    preview: {
      affected_count: count,
      examples: null,
      summary: `Raised ${count} item(s) by 10%.`,
    },
  };
}

describe("proposalCopy (L4-18)", () => {
  const stubT = (key: string, params?: Record<string, string | number>) =>
    params ? `${key}:${JSON.stringify(params)}` : key;

  const many = proposal(12);

  it("keeps English server titles for en locale", () => {
    expect(isEnglishLocale("en")).toBe(true);
    expect(localizedProposalTitle(many, "en", stubT)).toBe(many.title);
  });

  it("rebuilds Spanish titles from kind + count instead of English prose", () => {
    expect(isEnglishLocale("es")).toBe(false);
    const title = localizedProposalTitle(many, "es", stubT);
    expect(title).not.toMatch(/Raise all prices/);
    expect(title).toContain("proposal.titleWithCount_other");
    const desc = localizedProposalDescription(many, "es", stubT);
    expect(desc).not.toMatch(/Applies to/);
    expect(desc).toContain("proposal.descAffected_other");
  });

  it.each(locales)(
    "%s proposal desc/summary use singular at 1 and never (s)",
    (locale) => {
      const t = tFor(locale);
      const zeroDesc = localizedProposalDescription(proposal(0), locale, t);
      const oneDesc = localizedProposalDescription(proposal(1), locale, t);
      const twoDesc = localizedProposalDescription(proposal(2), locale, t);
      const zeroSummary = localizedProposalSummary(proposal(0), locale, t);
      const oneSummary = localizedProposalSummary(proposal(1), locale, t);
      const twoSummary = localizedProposalSummary(proposal(2), locale, t);
      const oneTitle = localizedProposalTitle(proposal(1), locale, t);

      for (const value of [
        zeroDesc,
        oneDesc,
        twoDesc,
        zeroSummary,
        oneSummary,
        twoSummary,
        oneTitle,
      ]) {
        expect(value).not.toMatch(/\(s\)/);
      }

      if (locale === "en") {
        expect(oneTitle).toBe("Raise all prices by 10%");
        expect(zeroDesc).toBe("No items would be affected.");
        expect(oneDesc).toBe("Applies to 1 item.");
        expect(twoDesc).toBe("Applies to 2 items.");
        expect(zeroSummary).toBe("No items would change.");
        expect(oneSummary).toBe("Preview of 1 item.");
        expect(twoSummary).toBe("Preview of 2 items.");
      } else {
        expect(oneTitle).toBe("Ajuste de precios · 1 ítem");
        expect(zeroDesc).toBe("Ningún ítem se vería afectado.");
        expect(oneDesc).toBe("Se aplica a 1 ítem.");
        expect(twoDesc).toBe("Se aplica a 2 ítems.");
        expect(zeroSummary).toBe("Ningún elemento cambiaría.");
        expect(oneSummary).toBe("Vista previa de 1 elemento.");
        expect(twoSummary).toBe("Vista previa de 2 elementos.");
      }
    },
  );
});
