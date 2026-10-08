/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import StaffInviteModal from "./StaffInviteModal";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";

function renderModal() {
  return render(
    <StaffInviteModal
      isOpen
      onClose={jest.fn()}
      inviteForm={{ email: "", name: "", role: "server" }}
      setInviteForm={jest.fn()}
      inviteLoading={false}
      getRoleLabel={(r) => r}
      getRoleDescription={(r) => `${r}-desc`}
      tString={(k) => k}
      handleInviteStaff={jest.fn()}
    />,
  );
}

describe("StaffInviteModal note vs features.email", () => {
  afterEach(() => resetInstanceCacheForTests());

  it("promises an email invitation when email is on", () => {
    setInstanceForTests(
      parseInstanceInfo({
        registration_mode: "invite",
        features: { email: true },
      }),
    );
    renderModal();
    expect(
      screen.getAllByText(/modals\.invite\.noteDescription$/).length,
    ).toBeGreaterThan(0);
    expect(
      screen.queryByText(/modals\.invite\.noteDescriptionNoEmail/),
    ).toBeNull();
  });

  it("points at the copyable invite link when email is off", () => {
    setInstanceForTests(
      parseInstanceInfo({
        registration_mode: "invite",
        features: { email: false },
      }),
    );
    renderModal();
    expect(
      screen.getAllByText(/modals\.invite\.noteDescriptionNoEmail/).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText(/modals\.invite\.noteDescription$/)).toBeNull();
  });
});
