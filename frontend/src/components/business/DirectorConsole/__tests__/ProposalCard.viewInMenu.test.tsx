/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

jest.mock("@/api/directorConsole", () => ({
  applyDirectorAction: jest.fn(() =>
    Promise.resolve({ applied: true, result: {} }),
  ),
  undoDirectorAction: jest.fn(),
  directorActionErrorInfo: () => ({ status: 0, code: "" }),
}));

import ProposalCard from "../ProposalCard";

const proposal = {
  id: "p1",
  kind: "menu.adjust_prices",
  title: "Raise dessert prices",
  description: "",
  requires_reconfirm: false,
  warnings: [],
  preview: null,
} as never;

it("offers View in Menu after a menu proposal is applied", async () => {
  const onViewInMenu = jest.fn();
  render(
    <ProposalCard
      proposal={proposal}
      businessId={1}
      t={(key: string) => key}
      onDismiss={jest.fn()}
      onViewInMenu={onViewInMenu}
    />,
  );
  fireEvent.click(screen.getByTestId("dc-proposal-apply"));
  const view = await screen.findByTestId("dc-proposal-view-in-menu");
  fireEvent.click(view);
  expect(onViewInMenu).toHaveBeenCalled();
});
