/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import InviteLinkFallbackModal from "../InviteLinkFallbackModal";

jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
import { toast } from "react-hot-toast";

const tString = (key: string) => key;
const URL_UNDER_TEST =
  "https://payverge.io/staff/accept-invitation?token=abc123";

describe("InviteLinkFallbackModal (P2-21)", () => {
  it("shows the invitation link read-only and copies it", async () => {
    const writeText = jest.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });

    render(
      <InviteLinkFallbackModal
        isOpen
        email="new.hire@example.com"
        invitationUrl={URL_UNDER_TEST}
        onClose={jest.fn()}
        tString={tString}
      />,
    );

    const input = screen.getByLabelText("inviteFallback.linkLabel");
    expect(input).toHaveValue(URL_UNDER_TEST);
    expect(input).toHaveAttribute("readonly");

    fireEvent.click(screen.getByLabelText("inviteFallback.copy"));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(URL_UNDER_TEST));
    expect(toast.success).toHaveBeenCalledWith("inviteFallback.copied");
  });

  it("renders nothing when closed", () => {
    render(
      <InviteLinkFallbackModal
        isOpen={false}
        email=""
        invitationUrl=""
        onClose={jest.fn()}
        tString={tString}
      />,
    );
    expect(screen.queryByLabelText("inviteFallback.linkLabel")).toBeNull();
  });
});
