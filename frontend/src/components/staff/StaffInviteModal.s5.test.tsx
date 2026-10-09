/** @jest-environment jsdom */
/**
 * L5-16 — single localized validation system; real email (not includes("@")).
 */
import React, { useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import StaffInviteModal from "./StaffInviteModal";

const tString = (k: string) => k;

function Harness() {
  const [inviteForm, setInviteForm] = useState<{
    email: string;
    name: string;
    role: "manager" | "server" | "host" | "kitchen";
  }>({
    email: "",
    name: "",
    role: "server",
  });
  const handleInviteStaff = jest.fn();
  return (
    <>
      <StaffInviteModal
        isOpen
        onClose={jest.fn()}
        inviteForm={inviteForm}
        setInviteForm={setInviteForm}
        inviteLoading={false}
        getRoleLabel={(r) => r}
        getRoleDescription={(r) => `${r}-desc`}
        tString={tString}
        handleInviteStaff={handleInviteStaff}
      />
      <span data-testid="invite-calls">{handleInviteStaff.mock.calls.length}</span>
    </>
  );
}

describe("L5-16 StaffInviteModal validation", () => {
  it("email is type=text (no native type=email English bubble)", () => {
    render(<Harness />);
    const email = screen.getByTestId("staff-invite-email");
    expect(email).toHaveAttribute("type", "text");
    expect(email).toHaveAttribute("inputmode", "email");
  });

  it('rejects weak includes("@") emails like "a@" with localized error', async () => {
    render(<Harness />);
    fireEvent.change(screen.getByTestId("staff-invite-name"), {
      target: { value: "Sam" },
    });
    fireEvent.change(screen.getByTestId("staff-invite-email"), {
      target: { value: "a@" },
    });
    fireEvent.click(screen.getByText("modals.invite.sendInvitation"));
    await waitFor(() => {
      expect(screen.getByText("error.emailInvalid")).toBeInTheDocument();
    });
    expect(screen.getByTestId("invite-calls")).toHaveTextContent("0");
  });

  it("requires name with localized inline error (not dual toast system)", async () => {
    render(<Harness />);
    fireEvent.change(screen.getByTestId("staff-invite-email"), {
      target: { value: "ok@example.com" },
    });
    fireEvent.click(screen.getByText("modals.invite.sendInvitation"));
    await waitFor(() => {
      expect(screen.getByText("error.nameRequired")).toBeInTheDocument();
    });
  });
});
