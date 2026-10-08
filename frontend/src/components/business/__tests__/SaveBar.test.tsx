/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SaveBar from "@/components/business/SaveBar";

const labels = {
  save: "Save changes",
  saving: "Saving…",
  unsaved: "You have unsaved changes",
  auto: "Changes saved automatically",
  clean: "No unsaved changes",
};

describe("SaveBar", () => {
  it("fires onSave when the button is pressed while dirty", async () => {
    const onSave = jest.fn();
    render(
      <SaveBar
        mode="button"
        isSaving={false}
        dirty
        onSave={onSave}
        labels={labels}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(onSave).toHaveBeenCalled();
    expect(screen.getByText("You have unsaved changes")).toBeInTheDocument();
  });

  it("does not claim autosave when explicit-save mode is clean (#380)", () => {
    render(
      <SaveBar
        mode="button"
        isSaving={false}
        dirty={false}
        onSave={() => {}}
        labels={labels}
      />,
    );
    expect(screen.queryByText(/saved automatically/i)).toBeNull();
    expect(screen.queryByText(labels.auto)).toBeNull();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();
    expect(screen.getByTestId("save-bar-all-saved")).toHaveTextContent(
      "No unsaved changes",
    );
    expect(screen.queryByText("You have unsaved changes")).toBeNull();
  });

  it("disables the save button when dirty === false and shows idle clean copy", () => {
    render(
      <SaveBar
        mode="button"
        isSaving={false}
        dirty={false}
        onSave={() => {}}
        labels={labels}
      />,
    );
    const btn = screen.getByRole("button", { name: "Save changes" });
    expect(btn).toBeDisabled();
    expect(screen.queryByText("You have unsaved changes")).toBeNull();
    expect(screen.getByTestId("save-bar-all-saved")).toHaveTextContent(
      "No unsaved changes",
    );
  });

  it("enables the save button when dirty and not saving", () => {
    render(
      <SaveBar
        mode="button"
        isSaving={false}
        dirty
        onSave={() => {}}
        labels={labels}
      />,
    );
    expect(screen.getByRole("button", { name: "Save changes" })).toBeEnabled();
  });

  it("disables the save button while isSaving even if dirty", () => {
    render(
      <SaveBar
        mode="button"
        isSaving
        dirty
        onSave={() => {}}
        labels={labels}
      />,
    );
    // NextUI loading state exposes the saving label; button must stay disabled.
    const btn = screen.getByRole("button", { name: /Saving/i });
    expect(btn).toBeDisabled();
  });

  it("does not fire onSave when the button is clean/disabled", async () => {
    const onSave = jest.fn();
    render(
      <SaveBar
        mode="button"
        isSaving={false}
        dirty={false}
        onSave={onSave}
        labels={labels}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(onSave).not.toHaveBeenCalled();
  });

  it("renders a passive indicator in auto mode with no button", () => {
    render(
      <SaveBar
        mode="auto"
        isSaving={false}
        dirty={false}
        onSave={() => {}}
        labels={labels}
      />,
    );
    expect(screen.getByText("Changes saved automatically")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByText("No unsaved changes")).toBeNull();
  });
});
