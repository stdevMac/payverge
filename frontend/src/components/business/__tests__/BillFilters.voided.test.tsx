/** @jest-environment jsdom */

import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { BillFilters } from "../BillFilters";

const t = (key: string) =>
  ({
    allStatuses: "All Statuses",
    paid: "Paid",
    closed: "Closed",
    voided: "Voided",
    "filters.filterByStatusAria": "Filter by status",
    "search.billHistoryPlaceholder": "Search bill history...",
  })[key] ?? key;

it("offers a Voided option in the history status filter so voided bills stay reachable", async () => {
  const onStatusFilterChange = jest.fn();
  render(
    <BillFilters
      searchQuery=""
      onSearchChange={jest.fn()}
      dateFrom=""
      onDateFromChange={jest.fn()}
      dateTo=""
      onDateToChange={jest.fn()}
      statusFilter="all"
      onStatusFilterChange={onStatusFilterChange}
      showStatusFilter
      onReset={jest.fn()}
      tString={t}
    />,
  );

  // Open the NextUI Select trigger.
  const trigger = screen.getByRole("button", { name: /Filter by status/ });
  await userEvent.click(trigger);

  // The Voided option must be listed as a selectable status.
  expect(
    await screen.findByRole("option", { name: /Voided/ }),
  ).toBeInTheDocument();
});
