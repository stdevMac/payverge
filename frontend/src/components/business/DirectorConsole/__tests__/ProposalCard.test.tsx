/** @jest-environment jsdom */
/**
 * Phase 5 proposal card (spec §5.7): renders the AI-proposed disclosure,
 * server-computed diff preview and warnings; Apply commits via the actions
 * API (with the two-press reconfirm flow for large swings); 409 means the
 * menu drifted and the card goes terminal; applied cards offer Undo.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

import ProposalCard from "../ProposalCard";
import type { DirectorProposedAction } from "@/api/directorConsole";
import {
  applyDirectorAction,
  undoDirectorAction,
} from "@/api/directorConsole";

jest.mock("@/api/directorConsole", () => {
  const actual = jest.requireActual("@/api/directorConsole");
  return {
    ...actual,
    applyDirectorAction: jest.fn(),
    undoDirectorAction: jest.fn(),
  };
});

const mockedApply = applyDirectorAction as jest.Mock;
const mockedUndo = undoDirectorAction as jest.Mock;

const t = (key: string, params?: Record<string, string | number>) => {
  let out = key;
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      out = out.replace(`{${k}}`, String(v));
    });
  }
  return out;
};

function makeProposal(
  overrides: Partial<DirectorProposedAction> = {},
): DirectorProposedAction {
  return {
    id: "pa_card1",
    kind: "menu.adjust_prices",
    title: "Raise dessert prices 5%",
    description: "Desserts are underpriced vs. category peers.",
    preview: {
      affected_count: 2,
      summary: "2 prices change",
      examples: [
        { name: "Tiramisu", before: 8, after: 8.4 },
        { name: "Flan", before: 6, after: 6.3 },
      ],
    },
    warnings: ["Large swing on Flan"],
    menu_version: 3,
    requires_reconfirm: false,
    expires_at: "2026-06-11T23:00:00Z",
    ...overrides,
  };
}

const applyResult = {
  applied: true,
  result: { new_menu_version: 4, proposal_id: "pa_card1", audit_id: 1, kind: "menu.adjust_prices" },
};

describe("ProposalCard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders disclosure, kind, preview diff and warnings", () => {
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    expect(screen.getByTestId("dc-proposal-disclosure")).toHaveTextContent(
      "proposal.disclosure",
    );
    // Dotted kinds are flattened to underscores so the operator lookup (which
    // splits keys on ".") can resolve them. (R3-AI-6)
    expect(screen.getByText("proposal.kinds.menu_adjust_prices")).toBeInTheDocument();
    expect(screen.getByText("Raise dessert prices 5%")).toBeInTheDocument();
    expect(screen.getByText(/proposal.summaryAffected/)).toBeInTheDocument();
    expect(screen.getByText(/proposal.affectedCount/)).toBeInTheDocument();
    expect(screen.queryByText(/item\(s\)/)).not.toBeInTheDocument();
    expect(screen.getAllByTestId("dc-proposal-example")).toHaveLength(2);
    expect(screen.getByText("Large swing on Flan")).toBeInTheDocument();
  });

  it("localizes boolean preview values for availability proposals", () => {
    render(
      <ProposalCard
        proposal={makeProposal({
          kind: "menu.set_availability",
          preview: {
            affected_count: 1,
            summary: "1 item changes",
            examples: [{ name: "Tiramisu", before: true, after: false }],
          },
        })}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );
    expect(screen.getByText("proposal.valueAvailable")).toBeInTheDocument();
    expect(screen.getByText("proposal.valueUnavailable")).toBeInTheDocument();
  });

  it("applies on click and then offers undo", async () => {
    mockedApply.mockResolvedValue(applyResult);
    const onApplied = jest.fn();
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
        onApplied={onApplied}
      />,
    );

    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    await waitFor(() => {
      expect(mockedApply).toHaveBeenCalledWith(42, "pa_card1", false);
    });
    expect(await screen.findByTestId("dc-proposal-undo")).toBeInTheDocument();
    expect(onApplied).toHaveBeenCalledWith("pa_card1");
    // The apply/dismiss controls are gone once applied.
    expect(screen.queryByTestId("dc-proposal-apply")).not.toBeInTheDocument();
  });

  it("requires a second confirming press when requires_reconfirm is set", async () => {
    mockedApply.mockResolvedValue(applyResult);
    render(
      <ProposalCard
        proposal={makeProposal({ requires_reconfirm: true })}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    // First press arms the confirm step without calling the API.
    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    expect(mockedApply).not.toHaveBeenCalled();
    expect(screen.getByTestId("dc-proposal-notice")).toHaveTextContent(
      "proposal.reconfirmHint",
    );
    expect(screen.getByTestId("dc-proposal-apply")).toHaveTextContent(
      "proposal.confirmApply",
    );

    // Second press applies with reconfirm=true.
    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    await waitFor(() => {
      expect(mockedApply).toHaveBeenCalledWith(42, "pa_card1", true);
    });
    expect(await screen.findByTestId("dc-proposal-undo")).toBeInTheDocument();
  });

  it("goes terminal with the menu-changed message on apply 409", async () => {
    mockedApply.mockRejectedValue({
      response: { status: 409, data: { code: "menu_changed" } },
    });
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    expect(await screen.findByText("proposal.errors.menuChanged")).toBeInTheDocument();
    // No retry: apply is gone, only dismiss remains.
    expect(screen.queryByTestId("dc-proposal-apply")).not.toBeInTheDocument();
    expect(screen.getByTestId("dc-proposal-dismiss")).toBeInTheDocument();
  });

  it("arms the confirm step when the server demands reconfirmation (428)", async () => {
    mockedApply.mockRejectedValue({
      response: { status: 428, data: { code: "reconfirm_required" } },
    });
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    expect(await screen.findByText("proposal.reconfirmHint")).toBeInTheDocument();
    expect(screen.getByTestId("dc-proposal-apply")).toHaveTextContent(
      "proposal.confirmApply",
    );
  });

  it("undoes an applied proposal", async () => {
    mockedApply.mockResolvedValue(applyResult);
    mockedUndo.mockResolvedValue({ undone: true, result: applyResult.result });
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    fireEvent.click(await screen.findByTestId("dc-proposal-undo"));
    await waitFor(() => {
      expect(mockedUndo).toHaveBeenCalledWith(42, "pa_card1");
    });
    expect(await screen.findByText("proposal.undoneNote")).toBeInTheDocument();
    expect(screen.queryByTestId("dc-proposal-undo")).not.toBeInTheDocument();
  });

  it("keeps undo visible but disabled when the menu drifted since apply", async () => {
    mockedApply.mockResolvedValue(applyResult);
    mockedUndo.mockRejectedValue({
      response: { status: 409, data: { code: "menu_changed_since_apply" } },
    });
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    fireEvent.click(await screen.findByTestId("dc-proposal-undo"));
    expect(await screen.findByText("proposal.errors.undoStale")).toBeInTheDocument();
    // The affordance never vanishes (audit L4-17) — it stays visible but
    // disabled so the operator sees undo existed and why it's unavailable.
    expect(screen.getByTestId("dc-proposal-undo")).toBeDisabled();
    // Still shows as applied.
    expect(screen.getByText("proposal.appliedNote")).toBeInTheDocument();
  });

  it("keeps undo visible but disabled when the undo window elapsed (410)", async () => {
    mockedApply.mockResolvedValue(applyResult);
    mockedUndo.mockRejectedValue({
      response: { status: 410, data: { code: "undo_window_elapsed" } },
    });
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    fireEvent.click(await screen.findByTestId("dc-proposal-undo"));
    expect(await screen.findByText("proposal.errors.undoExpired")).toBeInTheDocument();
    expect(screen.getByTestId("dc-proposal-undo")).toBeDisabled();
    expect(screen.getByText("proposal.appliedNote")).toBeInTheDocument();
  });

  it("renders a zero-match proposal without an Apply button and with a no-match notice", () => {
    render(
      <ProposalCard
        proposal={makeProposal({
          preview: { affected_count: 0, summary: "No items affected.", examples: [] },
          warnings: ['No items matched scope "item:x"'],
        })}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    // Applying a zero-match proposal can only fail (409 no_items_match) — the
    // card must not offer it (audit L4-17).
    expect(screen.queryByTestId("dc-proposal-apply")).not.toBeInTheDocument();
    expect(screen.getByTestId("dc-proposal-nomatch")).toHaveTextContent(
      "proposal.noMatchNotice",
    );
    expect(screen.getByTestId("dc-proposal-dismiss")).toBeInTheDocument();
  });

  it("shows the dedicated no-items-match message on apply 409 no_items_match", async () => {
    mockedApply.mockRejectedValue({
      response: { status: 409, data: { code: "no_items_match" } },
    });
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("dc-proposal-apply"));
    // Not the misleading "menu changed" copy — nothing changed; the proposal
    // simply matches no items.
    expect(
      await screen.findByText("proposal.errors.noItemsMatch"),
    ).toBeInTheDocument();
    expect(screen.queryByText("proposal.errors.menuChanged")).not.toBeInTheDocument();
    expect(screen.queryByTestId("dc-proposal-apply")).not.toBeInTheDocument();
    expect(screen.getByTestId("dc-proposal-dismiss")).toBeInTheDocument();
  });

  it("dismiss hands the proposal id back to the parent", () => {
    const onDismiss = jest.fn();
    render(
      <ProposalCard
        proposal={makeProposal()}
        businessId={42}
        t={t}
        onDismiss={onDismiss}
      />,
    );
    fireEvent.click(screen.getByTestId("dc-proposal-dismiss"));
    expect(onDismiss).toHaveBeenCalledWith("pa_card1");
  });
});
