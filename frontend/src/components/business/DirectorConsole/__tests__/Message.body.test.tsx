/** @jest-environment jsdom */
/**
 * Audit B5 — Director's Console body rendering.
 *
 * Prior behaviour: when an assistant message arrived without a
 * `structured_response` payload, `DirectorConsoleDashboard` rendered the
 * raw markdown string inside a `<p>` with `whitespace-pre-wrap`. Section
 * headings, lists, and emphasis all surfaced as literal `##`, `-`, and
 * `**` characters instead of rendering as structural HTML — and the
 * paragraph body between headings often visually collapsed into the
 * heading line, making operators see only the section titles.
 *
 * This test renders the extracted `DirectorMessage` component with a
 * realistic markdown payload and asserts that the body content
 * (paragraph text, list items, bold copy) is actually present in the
 * DOM under the section headings.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

import DirectorMessage from "../DirectorMessage";

describe("Director's Console message body rendering", () => {
  it("renders paragraph + list content under section headings", () => {
    const markdown = [
      "## Diagnosis",
      "Sales dropped 12% on Tuesday driven by a slow lunch.",
      "",
      "## Action Plan",
      "- Run a lunch promo Tuesday",
      "- Notify regulars",
      "",
      "**Expected impact:** AED 800 recovery.",
    ].join("\n");

    render(<DirectorMessage content={markdown} role="assistant" />);

    expect(screen.getByText(/Sales dropped 12%/i)).toBeInTheDocument();
    expect(screen.getByText(/Run a lunch promo Tuesday/i)).toBeInTheDocument();
    expect(screen.getByText(/Notify regulars/i)).toBeInTheDocument();
    expect(screen.getByText(/AED 800 recovery/i)).toBeInTheDocument();
    // Section headings should be rendered as headings, not as raw "##".
    expect(screen.getByRole("heading", { name: /Diagnosis/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Action Plan/i })).toBeInTheDocument();
  });

  it("renders user messages as plain text without markdown processing", () => {
    render(
      <DirectorMessage
        content={"Why did **sales** drop?"}
        role="user"
      />,
    );

    // User text is rendered verbatim — markers are not structural HTML.
    expect(screen.getByText(/Why did \*\*sales\*\* drop\?/)).toBeInTheDocument();
    expect(screen.queryByRole("heading")).not.toBeInTheDocument();
  });
});
