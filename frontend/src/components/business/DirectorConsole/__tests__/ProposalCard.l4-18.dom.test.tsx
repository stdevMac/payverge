/** @jest-environment jsdom */
/**
 * D1 / L4-18: ProposalCard must render localized summary/warnings for Spanish
 * operators — not raw English Go prose from preview.summary / warnings.
 * Asserts the mounted DOM, not proposalCopy helper existence.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import ProposalCard from "../ProposalCard";
import type { DirectorProposedAction } from "@/api/directorConsole";

jest.mock("@/api/directorConsole", () => {
  const actual = jest.requireActual("@/api/directorConsole");
  return {
    ...actual,
    applyDirectorAction: jest.fn(),
    undoDirectorAction: jest.fn(),
  };
});

const tEs = (key: string, params?: Record<string, string | number>) => {
  if (
    key === "proposal.summaryAffected" ||
    key === "proposal.summaryAffected_other"
  ) {
    return `Vista previa de ${params?.count}`;
  }
  if (key === "proposal.summaryAffected_one") {
    return `Vista previa de ${params?.count} elemento.`;
  }
  if (key === "proposal.warnings.largeChange") return "Cambio grande.";
  if (key === "proposal.warnings.generic") return "Revisa la advertencia.";
  if (key === "proposal.kinds.menu_adjust_prices") return "Ajustar precios";
  if (key === "proposal.titleWithCount" || key === "proposal.titleWithCount_other") {
    return `${params?.kind} · ${params?.count}`;
  }
  if (key === "proposal.titleWithCount_one") {
    return `${params?.kind} · ${params?.count} ítem`;
  }
  if (key === "proposal.descAffected" || key === "proposal.descAffected_other") {
    return `${params?.count} elementos se verían afectados.`;
  }
  if (key === "proposal.descAffected_one") {
    return `${params?.count} elemento se vería afectado.`;
  }
  if (key === "proposal.affectedCount" || key === "proposal.affectedCount_other") {
    return `${params?.count} afectados`;
  }
  if (key === "proposal.affectedCount_one") {
    return `${params?.count} afectado`;
  }
  if (key === "proposal.disclosure") return "Propuesta de Sage";
  return key;
};

function makeProposal(
  overrides: Partial<DirectorProposedAction> = {},
): DirectorProposedAction {
  return {
    id: "pa_l418",
    kind: "menu.adjust_prices",
    title: "Raise dessert prices 5%",
    description: "Desserts are underpriced vs. category peers.",
    preview: {
      affected_count: 3,
      summary: "Raised 3 item(s) by 10%.",
      examples: [{ name: "Flan", before: 6, after: 6.6 }],
    },
    warnings: ["Large swing on Flan"],
    menu_version: 3,
    requires_reconfirm: false,
    expires_at: "2026-08-20T23:00:00Z",
    ...overrides,
  };
}

describe("ProposalCard L4-18 Spanish DOM (D1)", () => {
  it("renders Spanish summary and warning, never raw Go English prose", () => {
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={48}
        t={tEs}
        locale="es"
        onDismiss={jest.fn()}
      />,
    );

    expect(screen.getByText(/Vista previa de 3/)).toBeInTheDocument();
    expect(screen.getByText("Cambio grande.")).toBeInTheDocument();
    expect(
      screen.queryByText(/Raised 3 item\(s\) by 10%/i),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/Large swing on Flan/i)).not.toBeInTheDocument();
    // Title/description also rebuilt for non-en (not English server strings).
    expect(
      screen.queryByText("Raise dessert prices 5%"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(/Desserts are underpriced/i),
    ).not.toBeInTheDocument();
  });
});
