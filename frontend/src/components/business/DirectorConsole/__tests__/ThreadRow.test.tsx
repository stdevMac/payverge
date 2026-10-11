/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import ThreadRow from "../ThreadRow";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "directorConsole.threadActions.menu": "Thread actions",
      "directorConsole.threadActions.rename": "Rename",
      "directorConsole.threadActions.pin": "Pin",
      "directorConsole.threadActions.unpin": "Unpin",
      "directorConsole.threadActions.export": "Export markdown",
      "directorConsole.threadActions.archive": "Archive",
      "directorConsole.threadActions.delete": "Delete permanently",
      "directorConsole.threadActions.deleteTitle": "Delete this thread?",
      "directorConsole.threadActions.deleteDescription": "Permanent.",
      "directorConsole.threadActions.deleteConfirm": "Delete",
      "directorConsole.threadActions.renameTitle": "Rename thread",
      "directorConsole.threadActions.cancel": "Cancel",
      "directorConsole.threadActions.save": "Save",
      "directorConsole.threads.pinnedLabel": "Pinned",
    };
    return map[key] ?? key;
  },
}));

const baseProps = {
  title: "Demo Director Briefing",
  timestampLabel: "Jul 8, 2026",
  pinned: false,
  selected: false,
  onSelect: jest.fn(),
  onRename: jest.fn(),
  onPinToggle: jest.fn(),
  onArchive: jest.fn(),
  onDelete: jest.fn(),
  onExport: jest.fn(),
  pinnedLabel: "Pinned",
};

describe("ThreadRow", () => {
  beforeEach(() => jest.clearAllMocks());

  it("selects the thread when the main area is clicked", () => {
    render(<ThreadRow {...baseProps} />);
    fireEvent.click(
      screen.getByRole("button", { name: /Demo Director Briefing/i }),
    );
    expect(baseProps.onSelect).toHaveBeenCalledTimes(1);
  });

  it("does not select when opening the actions menu", () => {
    render(<ThreadRow {...baseProps} />);
    fireEvent.click(screen.getByRole("button", { name: /thread actions/i }));
    expect(baseProps.onSelect).not.toHaveBeenCalled();
  });

  it("shows a pin icon when pinned", () => {
    render(<ThreadRow {...baseProps} pinned selected />);
    expect(screen.getByLabelText(/pinned/i)).toBeInTheDocument();
  });

  it("always renders the actions trigger (not hover-only)", () => {
    render(<ThreadRow {...baseProps} />);
    expect(
      screen.getByRole("button", { name: /thread actions/i }),
    ).toBeVisible();
  });

  it("fires pin from the menu without selecting", async () => {
    render(<ThreadRow {...baseProps} />);
    fireEvent.click(screen.getByRole("button", { name: /thread actions/i }));
    fireEvent.click(await screen.findByText(/^Pin$/));
    expect(baseProps.onPinToggle).toHaveBeenCalled();
    expect(baseProps.onSelect).not.toHaveBeenCalled();
  });
});
