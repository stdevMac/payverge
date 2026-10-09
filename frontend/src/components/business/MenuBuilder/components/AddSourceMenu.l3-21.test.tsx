/** @jest-environment jsdom */
/**
 * L3-21 residual: the AI import entry points follow the instance, not a plan.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { AddSourceMenu } from "./AddSourceMenu";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";

afterEach(() => resetInstanceCacheForTests());

function renderMenu() {
  return render(
    <AddSourceMenu
      tString={(k) => k}
      onAddCategory={jest.fn()}
      onAIWizard={jest.fn()}
      onPDFImport={jest.fn()}
    />,
  );
}

describe("L3-21 AddSourceMenu AI affordance", () => {
  it("offers Import with no plan lock when AI is available", () => {
    renderMenu();
    expect(screen.getByTestId("menu-import-trigger")).toBeInTheDocument();
    expect(screen.queryByTestId("menu-import-lock")).not.toBeInTheDocument();
  });

  it("hides the AI import entry when the server has AI off", () => {
    setInstanceForTests(
      parseInstanceInfo({ registration_mode: "invite", features: { ai: false } })!,
    );
    renderMenu();
    expect(screen.queryByTestId("menu-import-trigger")).not.toBeInTheDocument();
  });
});
