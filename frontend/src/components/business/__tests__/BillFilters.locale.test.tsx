/** @jest-environment jsdom */

import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";

import { BillFilters } from "../BillFilters";

it("renders labelled locale-aware bill date controls", () => {
  render(
    <BillFilters
      searchQuery=""
      onSearchChange={jest.fn()}
      dateFrom="2026-07-08"
      onDateFromChange={jest.fn()}
      dateTo="2026-07-18"
      onDateToChange={jest.fn()}
      onReset={jest.fn()}
      tString={(key) =>
        ({
          "filters.fromDateAria": "From date",
          "filters.fromDate": "From date",
          "filters.toDateAria": "To date",
          "filters.toDate": "To date",
        })[key] ?? key
      }
    />,
  );
  expect(screen.getByRole("group", { name: /From date/ })).toHaveTextContent(
    /07.*08.*2026/,
  );
  expect(screen.getByRole("group", { name: /To date/ })).toHaveTextContent(
    /07.*18.*2026/,
  );
  expect(document.querySelector('input[type="date"]')).toBeNull();
});
