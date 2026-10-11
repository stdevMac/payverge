/** @jest-environment jsdom */

import React, { useRef, useState } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { PrintFineTunePanel } from "./PrintFineTunePanel";

const tString = (key: string) => key;

const sections = [
  {
    id: "layout",
    label: "print.fineTune.layout",
    content: (
      <>
        <button type="button">first control</button>
        <button type="button">second control</button>
      </>
    ),
  },
  {
    id: "identity",
    label: "print.fineTune.identity",
    content: <button type="button">last control</button>,
  },
];

function Harness() {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  return (
    <div>
      <button type="button" onClick={() => setOpen(true)}>
        print.fineTune.open
      </button>
      <PrintFineTunePanel
        isOpen={open}
        sections={sections}
        tString={tString}
        panelRef={panelRef}
        onClose={() => setOpen(false)}
      />
    </div>
  );
}

describe("PrintFineTunePanel", () => {
  it("stays out of the flow until it is opened", () => {
    render(
      <PrintFineTunePanel
        isOpen={false}
        sections={sections}
        tString={tString}
        onClose={jest.fn()}
      />,
    );

    expect(
      screen.queryByTestId("print-fine-tune-panel"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("first control")).not.toBeInTheDocument();
  });

  it("groups every advanced control under labelled sections in one dialog", () => {
    render(
      <PrintFineTunePanel
        isOpen
        sections={sections}
        tString={tString}
        onClose={jest.fn()}
      />,
    );

    const panel = screen.getByRole("dialog", { name: "print.fineTune.title" });
    expect(panel).toHaveAttribute("aria-modal", "true");
    expect(within(panel).getByText("print.fineTune.help")).toBeVisible();
    for (const section of sections) {
      expect(
        within(panel).getByRole("region", { name: section.label }),
      ).toBeInTheDocument();
    }
    expect(within(panel).getByText("last control")).toBeVisible();
  });

  it("moves focus in on open, cycles it, and hands it back on close", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const trigger = screen.getByRole("button", { name: "print.fineTune.open" });

    await user.click(trigger);
    const closeButtons = screen.getAllByRole("button", {
      name: "print.fineTune.close",
    });
    await waitFor(() => expect(closeButtons.at(-1)).toHaveFocus());

    // Tab wraps from the last control back to the first inside the dialog,
    // so focus never escapes to the page behind the drawer.
    const done = screen.getByRole("button", { name: "print.fineTune.done" });
    done.focus();
    await user.tab();
    expect(closeButtons.at(-1)).toHaveFocus();

    await user.tab({ shift: true });
    expect(done).toHaveFocus();

    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(
        screen.queryByTestId("print-fine-tune-panel"),
      ).not.toBeInTheDocument(),
    );
    expect(trigger).toHaveFocus();
  });

  it("closes when the quiet scrim behind it is clicked", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(
      screen.getByRole("button", { name: "print.fineTune.open" }),
    );
    const overlay = screen.getByTestId("print-fine-tune-overlay");
    await user.click(
      within(overlay).getAllByRole("button", {
        name: "print.fineTune.close",
      })[0],
    );

    await waitFor(() =>
      expect(
        screen.queryByTestId("print-fine-tune-panel"),
      ).not.toBeInTheDocument(),
    );
  });
});
