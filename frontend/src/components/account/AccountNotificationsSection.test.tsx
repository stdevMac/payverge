/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import {
  AccountNotificationsSection,
  type AccountNotificationsCopy,
} from "./AccountNotificationsSection";
import { getBusinessNotificationSettingsPath } from "@/utils/businessUrl";

const copy: AccountNotificationsCopy = {
  sectionTitle: "Email & notifications",
  sectionDescription: "Choose which emails each business sends you.",
  manageLink: "Manage notifications",
  emptyTitle: "No businesses yet",
  emptyDescription: "Create a business to manage its notification settings.",
  errorMessage: "We couldn't load your businesses.",
  loadingLabel: "Loading your businesses…",
};

const businesses = [
  { id: 7, business_id: "mara-core-kitchen", name: "Core Kitchen" },
  { id: 9, name: "Second Spot" },
];

describe("AccountNotificationsSection", () => {
  it("lists each business with a deep link to its notification settings", () => {
    render(
      <AccountNotificationsSection
        businesses={businesses}
        loading={false}
        error={false}
        copy={copy}
      />,
    );

    expect(screen.getByText("Email & notifications")).toBeInTheDocument();
    expect(screen.getByText("Core Kitchen")).toBeInTheDocument();
    expect(screen.getByText("Second Spot")).toBeInTheDocument();

    const links = screen.getAllByRole("link", { name: /Manage notifications/i });
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute(
      "href",
      getBusinessNotificationSettingsPath(businesses[0]),
    );
    expect(links[1]).toHaveAttribute(
      "href",
      "/business/9/dashboard?tab=settings&section=notifications",
    );
  });

  it("shows the empty state when the operator has no businesses", () => {
    render(
      <AccountNotificationsSection
        businesses={[]}
        loading={false}
        error={false}
        copy={copy}
      />,
    );

    expect(screen.getByText("No businesses yet")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /Manage notifications/i }),
    ).not.toBeInTheDocument();
  });

  it("shows a loading label while businesses load", () => {
    render(
      <AccountNotificationsSection
        businesses={[]}
        loading
        error={false}
        copy={copy}
      />,
    );

    expect(screen.getByText("Loading your businesses…")).toBeInTheDocument();
  });

  it("shows an error message when the load fails", () => {
    render(
      <AccountNotificationsSection
        businesses={[]}
        loading={false}
        error
        copy={copy}
      />,
    );

    expect(
      screen.getByText("We couldn't load your businesses."),
    ).toBeInTheDocument();
  });
});
