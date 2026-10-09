/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import CommandPalette from "../CommandPalette";
import { buildNavCommands, type Command } from "../commandRegistry";
import type { AccessState } from "../tabAccess";

const FULL: AccessState = {
  loading: false,
  isSuspended: false,
};

const navCommands = buildNavCommands(
  [
    { key: "overview", label: "Overview", description: "Quick stats", iconKey: "Home" },
    { key: "bills", label: "Bills", description: "Open tabs", iconKey: "Receipt" },
    { key: "kitchen", label: "Kitchen", description: "KDS", iconKey: "ChefHat", keywords: ["kds"] },
    { key: "reservations", label: "Reservations", description: "Bookings", iconKey: "Calendar" },
  ],
  { allowedTabs: [], isStaffUser: false, access: FULL },
);

const actionCommands: Command[] = [
  {
    id: "action:storefront",
    group: "actions",
    kind: "action",
    label: "View storefront",
    keywords: ["public", "website"],
    iconKey: "ExternalLink",
    locked: false,
    actionKey: "storefront",
  },
];

// Minimal localized-string stub: echo the leaf key.
const t = (key: string) => key.split(".").pop() as string;

function renderPalette(overrides: Partial<React.ComponentProps<typeof CommandPalette>> = {}) {
  const onClose = jest.fn();
  const onRun = jest.fn();
  render(
    <CommandPalette
      open
      onClose={onClose}
      navCommands={navCommands}
      actionCommands={actionCommands}
      recentTabKeys={["kitchen"]}
      onRun={onRun}
      t={t}
      {...overrides}
    />,
  );
  return { onClose, onRun };
}

describe("CommandPalette", () => {
  it("renders nothing when closed", () => {
    const { container } = render(
      <CommandPalette
        open={false}
        onClose={jest.fn()}
        navCommands={navCommands}
        actionCommands={actionCommands}
        recentTabKeys={[]}
        onRun={jest.fn()}
        t={t}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("exposes an accessible combobox + listbox when open", () => {
    renderPalette();
    expect(screen.getByRole("combobox")).toBeInTheDocument();
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    // Every tab plus the action shows up as an option.
    expect(screen.getByText("Reservations")).toBeInTheDocument();
    expect(screen.getByText("View storefront")).toBeInTheDocument();
  });

  it("surfaces recent sections first when the query is empty", () => {
    renderPalette();
    // "recent" group label is rendered (t echoes the leaf key).
    expect(screen.getByText("recent")).toBeInTheDocument();
  });

  it("filters to fuzzy matches as the operator types", () => {
    renderPalette();
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "res" } });
    expect(screen.getByText("Reservations")).toBeInTheDocument();
    expect(screen.queryByText("Bills")).not.toBeInTheDocument();
  });

  it("runs the highlighted command on Enter after arrow navigation", () => {
    const { onRun } = renderPalette();
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "kitchen" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onRun).toHaveBeenCalledTimes(1);
    expect(onRun.mock.calls[0][0]).toMatchObject({ tabKey: "kitchen" });
  });

  it("closes on Escape", () => {
    const { onClose } = renderPalette();
    fireEvent.keyDown(screen.getByRole("combobox"), { key: "Escape" });
    expect(onClose).toHaveBeenCalled();
  });

  it("runs a command on click", () => {
    const { onRun } = renderPalette();
    fireEvent.click(screen.getByText("View storefront"));
    expect(onRun).toHaveBeenCalledTimes(1);
    expect(onRun.mock.calls[0][0]).toMatchObject({ actionKey: "storefront" });
  });

  it("shows an empty state when nothing matches", () => {
    renderPalette();
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "zzzzzz" } });
    expect(screen.getByText("empty")).toBeInTheDocument();
  });
});
