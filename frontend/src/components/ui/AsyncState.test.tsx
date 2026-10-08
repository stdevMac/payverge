/** @jest-environment jsdom */
import fs from "node:fs";
import path from "node:path";
import { fireEvent, render, screen } from "@testing-library/react";
import { PanelFailure, RouteLoadingFallback } from "./AsyncState";

describe("shared async states", () => {
  it("renders a non-blank, polite route loading region", () => {
    const { container } = render(<RouteLoadingFallback />);

    const status = screen.getByRole("status");
    expect(status).toHaveAttribute("aria-live", "polite");
    expect(status).toHaveAttribute("aria-busy", "true");
    expect(container.querySelectorAll("[data-skeleton-line]").length).toBeGreaterThan(0);
  });

  it("announces a panel failure and exposes a working retry", () => {
    const onRetry = jest.fn();
    render(
      <PanelFailure
        title="Could not load bills"
        message="Your other dashboard panels are still available."
        retryLabel="Try again"
        onRetry={onRetry}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Your other dashboard panels are still available.",
    );
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});

function sourceFiles(root: string): string[] {
  return fs.readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const fullPath = path.join(root, entry.name);
    if (entry.isDirectory()) return sourceFiles(fullPath);
    return /\.tsx$/.test(entry.name) ? [fullPath] : [];
  });
}

describe("Suspense loading policy", () => {
  it("does not allow a blank null fallback in application routes", () => {
    const appRoot = path.resolve(__dirname, "../../app");
    const offenders = sourceFiles(appRoot)
      .filter((filePath) => /fallback\s*=\s*\{\s*null\s*\}/.test(fs.readFileSync(filePath, "utf8")))
      .map((filePath) => path.relative(appRoot, filePath));

    expect(offenders).toEqual([]);
  });
});
