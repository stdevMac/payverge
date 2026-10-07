/** @jest-environment jsdom */
/**
 * Wave 4 Task 19: approval queue shows guest no-show/visit history.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (
    key: string,
    _l?: string,
    params?: Record<string, string | number>,
  ) => (params ? `${key}:${JSON.stringify(params)}` : key),
}));

import ReservationApprovalQueue from "../ReservationApprovalQueue";

const pending = [
  {
    id: 1,
    customer_name: "Ana",
    customer_email: "ana@x.io",
    party_size: 2,
    reservation_time: new Date(Date.now() + 86400000).toISOString(),
    created_at: new Date().toISOString(),
    status: "pending",
  },
] as never;

it("shows prior no-shows and visits when history is provided", () => {
  render(
    <ReservationApprovalQueue
      pending={pending}
      onApprove={jest.fn()}
      onDecline={jest.fn()}
      busyId={null}
      guestHistory={{ "ana@x.io": { prior_no_shows: 2, prior_visits: 5 } }}
    />,
  );
  expect(
    screen.getByText(/approvalQueue\.history\.line.*"noShows":2.*"visits":5/),
  ).toBeInTheDocument();
});
