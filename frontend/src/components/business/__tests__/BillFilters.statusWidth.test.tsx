/** @jest-environment jsdom */
/** #122 — Bill History status filter must not squeeze "All Statuses". */
import "@testing-library/jest-dom";
import { fireEvent, render, screen } from "@testing-library/react";

import { BillFilters } from "../BillFilters";

const t = (key: string) =>
  ({
    allStatuses: "All Statuses",
    paid: "Paid",
    closed: "Closed",
    voided: "Voided",
    "filters.filterByStatusAria": "Filter by status",
    "search.billHistoryPlaceholder": "Search bill history...",
    reset: "Reset",
    "filters.fromDate": "From",
    "filters.toDate": "To",
    "filters.to": "to",
    "filters.fromDateAria": "From date",
    "filters.toDateAria": "To date",
  })[key] ?? key;

it("gives the status filter enough width for All Statuses", () => {
  render(
    <BillFilters
      searchQuery=""
      onSearchChange={jest.fn()}
      dateFrom=""
      onDateFromChange={jest.fn()}
      dateTo=""
      onDateToChange={jest.fn()}
      statusFilter="all"
      onStatusFilterChange={jest.fn()}
      showStatusFilter
      onReset={jest.fn()}
      tString={t}
    />,
  );

  const wrap = screen.getByTestId("bill-history-status-filter");
  expect(wrap.className).toMatch(/lg:w-52/);
  expect(wrap.className).not.toMatch(/lg:w-40/);

  const trigger = screen.getByRole("button", { name: /Filter by status/ });
  expect(trigger.textContent).toMatch(/All Statuses/);
});

it("hides idle date pickers below sm when collapseIdleDates is set", () => {
  render(
    <BillFilters
      searchQuery=""
      onSearchChange={jest.fn()}
      dateFrom=""
      onDateFromChange={jest.fn()}
      dateTo=""
      onDateToChange={jest.fn()}
      onReset={jest.fn()}
      collapseIdleDates
      tString={t}
    />,
  );
  const dates = screen.getByTestId("bill-filters-dates");
  expect(dates.className).toMatch(/hidden/);
  expect(dates.className).toMatch(/sm:flex/);
});

it("keeps date pickers visible when a date filter is already applied", () => {
  render(
    <BillFilters
      searchQuery=""
      onSearchChange={jest.fn()}
      dateFrom="2026-08-01"
      onDateFromChange={jest.fn()}
      dateTo=""
      onDateToChange={jest.fn()}
      onReset={jest.fn()}
      collapseIdleDates
      tString={t}
    />,
  );
  const dates = screen.getByTestId("bill-filters-dates");
  expect(dates.className).toMatch(/^flex /);
  expect(dates.className).not.toMatch(/hidden/);
});

it("lets a phone operator expand idle date filters", () => {
  render(
    <BillFilters
      searchQuery=""
      onSearchChange={jest.fn()}
      dateFrom=""
      onDateFromChange={jest.fn()}
      dateTo=""
      onDateToChange={jest.fn()}
      onReset={jest.fn()}
      collapseIdleDates
      tString={t}
    />,
  );
  const toggle = screen.getByTestId("bill-filters-dates-toggle");
  expect(screen.getByTestId("bill-filters-dates").className).toMatch(/hidden/);
  fireEvent.click(toggle);
  expect(screen.getByTestId("bill-filters-dates").className).toMatch(/^flex /);
  expect(screen.queryByTestId("bill-filters-dates-toggle")).toBeNull();
});
