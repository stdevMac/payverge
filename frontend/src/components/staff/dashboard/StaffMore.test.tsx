/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import StaffMore, { type StaffMoreLabels } from "./StaffMore";

const labels: StaffMoreLabels = {
  title: "More",
  profileEntry: "Profile",
  profileHint: "h",
  installAppEntry: "Install Payverge",
  installAppHint: "Add it to this device",
  availabilityEntry: "Availability",
  availabilityHint: "h",
  timeOffEntry: "Time off",
  timeOffHint: "h",
  coverageEntry: "Coverage",
  coverageHint: "h",
  announcementsEntry: "Announcements",
  announcementsHint: "h",
  logbookEntry: "Logbook",
  logbookHint: "h",
  checklistsEntry: "Checklists",
  checklistsHint: "h",
  onboardingEntry: "Onboarding",
  onboardingHint: "h",
  documentsEntry: "Documents",
  documentsHint: "h",
  hoursEntry: "Hours",
  hoursHint: "h",
  recognitionEntry: "Recognition",
  recognitionHint: "h",
  pollsEntry: "Polls",
  pollsHint: "h",
  groupResources: "Resources",
  groupCommunity: "Community",
  groupAccount: "Account",
  back: "Back",
};

const renderProps = {
  renderProfile: () => null,
  renderAvailability: () => null,
  renderTimeOff: () => null,
  renderCoverage: () => null,
  renderAnnouncements: () => <div>ANNOUNCEMENTS SURFACE</div>,
  renderLogbook: () => null,
  renderChecklists: () => null,
  renderOnboarding: () => null,
  renderDocuments: () => null,
  renderRecognition: () => null,
  renderHours: () => null,
  renderPolls: () => null,
};

test("badges announcements + checklists menu entries with their counts", () => {
  render(
    <StaffMore
      labels={labels}
      announcementsBadge={3}
      checklistsBadge={0}
      {...renderProps}
    />,
  );
  // A pill on the announcements entry showing 3; none on checklists (0).
  const badges = screen.getAllByTestId("menu-badge");
  expect(badges).toHaveLength(1);
  expect(badges[0]).toHaveTextContent("3");
  // Money-free.
  expect(document.body.textContent).not.toMatch(/\$\d/);
});

test("caps a large badge at 99+", () => {
  render(
    <StaffMore
      labels={labels}
      announcementsBadge={0}
      checklistsBadge={250}
      {...renderProps}
    />,
  );
  expect(screen.getByTestId("menu-badge")).toHaveTextContent("99+");
});

test("renders no badges when both counts are zero", () => {
  render(<StaffMore labels={labels} {...renderProps} />);
  expect(screen.queryByTestId("menu-badge")).not.toBeInTheDocument();
});

test("groups the menu into Resources, Community, and Account clusters", () => {
  render(<StaffMore labels={labels} {...renderProps} />);
  // Group headers break up the flat list.
  expect(
    screen.getByRole("heading", { name: "Resources" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("heading", { name: "Community" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Account" })).toBeInTheDocument();

  // Profile lives under the Account group; Documents under Resources.
  const account = screen.getByRole("heading", { name: "Account" });
  const accountList = account.nextElementSibling as HTMLElement;
  expect(accountList).toHaveTextContent("Profile");

  // All twelve surfaces are still reachable.
  for (const entry of [
    "Availability",
    "Time off",
    "Coverage",
    "Hours",
    "Checklists",
    "Documents",
    "Onboarding",
    "Logbook",
    "Announcements",
    "Recognition",
    "Polls",
    "Profile",
  ]) {
    expect(screen.getByText(entry)).toBeInTheDocument();
  }
});

test("deep-links into a section via the controlled section prop", () => {
  render(
    <StaffMore labels={labels} section="announcements" {...renderProps} />,
  );
  expect(screen.getByText("ANNOUNCEMENTS SURFACE")).toBeInTheDocument();
  // The back control returns to the menu.
  expect(screen.getByRole("button", { name: "Back" })).toBeInTheDocument();
});

test("places install beside Profile in the Account group", () => {
  const onInstallApp = jest.fn();
  render(
    <StaffMore labels={labels} onInstallApp={onInstallApp} {...renderProps} />,
  );
  const account = screen.getByRole("heading", { name: "Account" });
  const list = account.nextElementSibling as HTMLElement;
  expect(list).toHaveTextContent("Install Payverge");
  fireEvent.click(screen.getByRole("button", { name: /Install Payverge/i }));
  expect(onInstallApp).toHaveBeenCalledTimes(1);
});

test("keeps a pending install action visible, disabled, and busy", () => {
  render(
    <StaffMore
      labels={labels}
      onInstallApp={jest.fn()}
      installAppPending
      {...renderProps}
    />,
  );
  const action = screen.getByRole("button", { name: /Install Payverge/i });
  expect(action).toBeDisabled();
  expect(action).toHaveAttribute("aria-busy", "true");
});
