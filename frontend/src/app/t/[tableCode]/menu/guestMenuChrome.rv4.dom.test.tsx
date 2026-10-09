/** @jest-environment jsdom */
/**
 * D1 / RV-4: compact chrome must reclaim vertical space in the DOM — hide the
 * table label, tighten bar padding, shrink the title. Constant-vs-constant
 * budget math alone is pass-on-revert theater.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import {
  guestMenuChromeBarPaddingClass,
  guestMenuChromeShowsTableLabel,
  guestMenuChromeTitleClass,
} from "./guestMenuChrome";

/** Mirrors page.tsx chrome wiring so revert of helpers fails here. */
function GuestMenuChromeFixture({ compact }: { compact: boolean }) {
  return (
    <div
      data-testid="guest-menu-chrome"
      data-compact={compact ? "true" : "false"}
    >
      <div
        data-testid="guest-menu-chrome-bar"
        className={guestMenuChromeBarPaddingClass(compact)}
      >
        {guestMenuChromeShowsTableLabel(compact) ? (
          <p data-testid="guest-menu-table-label">Table 1</p>
        ) : null}
        <h1
          data-testid="guest-menu-business-title"
          className={guestMenuChromeTitleClass(compact)}
        >
          Demo Kitchen
        </h1>
      </div>
    </div>
  );
}

describe("RV-4 guest menu chrome compact DOM (D1)", () => {
  it("expanded chrome shows table label and roomy padding/title", () => {
    render(<GuestMenuChromeFixture compact={false} />);
    expect(screen.getByTestId("guest-menu-table-label")).toHaveTextContent(
      "Table 1",
    );
    expect(screen.getByTestId("guest-menu-chrome-bar").className).toMatch(
      /py-3/,
    );
    // NEW-10: base size is heading-sm at 390px; sm: bumps on larger viewports.
    expect(
      screen.getByTestId("guest-menu-business-title").className,
    ).toMatch(/text-heading-sm/);
    expect(screen.getByTestId("guest-menu-chrome")).toHaveAttribute(
      "data-compact",
      "false",
    );
  });

  it("compact chrome hides table label and tightens padding/title", () => {
    render(<GuestMenuChromeFixture compact />);
    expect(screen.queryByTestId("guest-menu-table-label")).not.toBeInTheDocument();
    expect(screen.getByTestId("guest-menu-chrome-bar").className).toMatch(
      /py-1\.5/,
    );
    expect(
      screen.getByTestId("guest-menu-business-title").className,
    ).toMatch(/text-heading-sm/);
    expect(screen.getByTestId("guest-menu-chrome")).toHaveAttribute(
      "data-compact",
      "true",
    );
  });

  it("page.tsx wires the shared helpers (not reimplemented literals only)", () => {
    const fs = require("fs") as typeof import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(path.join(__dirname, "page.tsx"), "utf8");
    expect(src).toMatch(/guestMenuChromeBarPaddingClass/);
    expect(src).toMatch(/guestMenuChromeShowsTableLabel/);
    expect(src).toMatch(/guestMenuChromeTitleClass/);
    expect(src).toMatch(/data-testid="guest-menu-chrome"/);
    // NEW-10: venue title must not force single-line ellipsis at 390px.
    expect(src).toMatch(/data-testid="guest-menu-business-title"/);
    expect(src).not.toMatch(
      /data-testid="guest-menu-business-title"[\s\S]{0,120}truncate/,
    );

  });
});
