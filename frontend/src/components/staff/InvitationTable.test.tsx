/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import InvitationTable from "./InvitationTable";
import * as StaffAPI from "../../api/staff";
import type { StaffInvitation } from "../../api/staff";

const baseInvitation: StaffInvitation = {
  id: 11,
  business_id: 42,
  name: "Robin",
  email: "robin@example.com",
  role: "server",
  status: "pending",
  expires_at: "2026-07-09T00:00:00Z",
  created_at: "2026-07-02T00:00:00Z",
  updated_at: "2026-07-02T00:00:00Z",
} as StaffInvitation;

jest.mock("../../api/staff", () => {
  const actual = jest.requireActual("../../api/staff");
  return {
    ...actual,
    getInvitationLink: jest.fn(),
  };
});

const mockedGetInvitationLink = StaffAPI.getInvitationLink as jest.MockedFunction<
  typeof StaffAPI.getInvitationLink
>;

const tString = (key: string) => key;
const getRoleLabel = (role: string) => role;
const formatDate = (d: string) => d;

describe("InvitationTable", () => {
  beforeEach(() => {
    mockedGetInvitationLink.mockReset();
  });

  it("keeps expired invitations visible with an 'Invite again' action", async () => {
    const onResend = jest.fn();
    render(
      <InvitationTable
        invitations={[
          baseInvitation,
          { ...baseInvitation, id: 12, name: "Sam", email: "sam@example.com", expires_at: "2026-06-01T00:00:00Z" },
        ]}
        businessId="42"
        getRoleLabel={getRoleLabel}
        tString={tString}
        formatDate={formatDate}
        isInvitationExpired={(expiresAt) => expiresAt < "2026-07-03"}
        handleResendInvitation={onResend}
      />,
    );
    // The expired row is not silently dropped…
    expect(screen.getByText("Sam")).toBeInTheDocument();
    expect(
      screen.getByText("invitations.status.expired"),
    ).toBeInTheDocument();
    // …and recovery is one tap.
    const inviteAgain = screen.getByRole("button", {
      name: "invitations.inviteAgain",
    });
    await userEvent.click(inviteAgain);
    expect(onResend).toHaveBeenCalledWith(12, "Sam");
    // The header explains the lifecycle.
    expect(screen.getByText("invitations.expiryHint")).toBeInTheDocument();
  });

  it("copies the invite link via staff:invite-gated link endpoint", async () => {
    const writeText = jest.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    mockedGetInvitationLink.mockResolvedValue({
      invitation_id: 11,
      invitation_url: `${window.location.origin}/staff/accept-invitation?token=tok-11`,
      expires_at: baseInvitation.expires_at,
    });
    render(
      <InvitationTable
        invitations={[baseInvitation]}
        businessId="42"
        getRoleLabel={getRoleLabel}
        tString={tString}
        formatDate={formatDate}
        isInvitationExpired={() => false}
        handleResendInvitation={jest.fn()}
      />,
    );
    const copy = screen.getByRole("button", { name: "invitations.copyLink" });
    await userEvent.click(copy);
    expect(mockedGetInvitationLink).toHaveBeenCalledWith("42", 11);
    expect(writeText).toHaveBeenCalledWith(
      `${window.location.origin}/staff/accept-invitation?token=tok-11`,
    );
    // The button confirms in place.
    expect(
      await screen.findByRole("button", { name: "invitations.copied" }),
    ).toBeInTheDocument();
  });
});
