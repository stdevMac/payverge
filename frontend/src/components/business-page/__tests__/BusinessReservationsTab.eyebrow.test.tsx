/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import BusinessReservationsTab from "../BusinessReservationsTab";

jest.mock("../GuestReservationForm", () => ({
  __esModule: true,
  default: () => <div data-testid="reservation-form" />,
}));

describe("BusinessReservationsTab eyebrow (#508)", () => {
  it("does not use the same string for badge and heading", () => {
    render(
      <BusinessReservationsTab
        customUrl="test"
        businessName="Test"
        reservationsEnabled
        reservationPartnerLinks={[]}
        designSettings={{
          primary_color: "#1a6b6a",
          corner_radius: "medium",
        }}
        t={(key: string) => key}
      />,
    );

    expect(screen.getByText("businessPage.reservations")).toBeInTheDocument();
    expect(screen.getAllByText("businessPage.reservations")).toHaveLength(1);
  });
});
