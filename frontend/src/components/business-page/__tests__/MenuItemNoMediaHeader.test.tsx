/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import MenuItemNoMediaHeader from "../MenuItemNoMediaHeader";

describe("MenuItemNoMediaHeader", () => {
  test("renders a 4:3 placeholder block matching image-card media area", () => {
    // Cards in a CSS grid with align-items: stretch take their row's tallest
    // height. Image cards reserve a 4:3 block; no-media cards must reserve
    // the same so they don't stretch into hundreds of pixels of empty
    // whitespace below their content.
    const { container } = render(<MenuItemNoMediaHeader label="No image" />);
    const root = container.firstElementChild as HTMLElement;
    expect(root).toBeTruthy();
    expect(root.className).toMatch(/aspect-\[4\/3\]/);
    expect(root.getAttribute("role")).toBe("img");
    expect(root.getAttribute("aria-label")).toBe("No image");
    expect(screen.getByText("No image")).toBeInTheDocument();
  });

  test("No image label uses AA-safe ink-700 (not ink-400/500 that fail under card opacity)", () => {
    // Release smoke (public-business-accessibility) failed when the label was
    // text-ink-500 on bg-ink-100 and a closed-mode card applied opacity-80:
    // effective contrast dropped to ~3.5:1 (#868076 on #f4f3ee). ink-700 keeps
    // ≥4.5:1 even under that composite.
    const { container } = render(<MenuItemNoMediaHeader label="No image" />);
    const label = screen.getByText("No image");
    expect(label.className).toMatch(/text-ink-700/);
    expect(label.className).not.toMatch(/text-ink-(?:400|500)\b/);
    const root = container.firstElementChild as HTMLElement;
    // Parent must not set a weak text-* that the label could inherit.
    expect(root.className).not.toMatch(/text-ink-400/);
  });
});
