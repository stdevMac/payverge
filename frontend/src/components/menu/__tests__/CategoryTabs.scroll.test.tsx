/** @jest-environment jsdom */
import { render, fireEvent, screen } from "@testing-library/react";
import CategoryTabs from "../CategoryTabs";

describe("CategoryTabs scroll behavior", () => {
  const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;
  const originalMatchMedia = window.matchMedia;

  afterEach(() => {
    // Restore prototype between tests so leaked mocks don't bleed.
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      value: originalScrollIntoView,
      configurable: true,
    });
    window.matchMedia = originalMatchMedia;
    // Reset the hash between tests.
    history.replaceState(null, "", " ");
  });

  it("scrolls to category section and updates URL hash on click", () => {
    const section = document.createElement("section");
    section.id = "cat-starters";
    section.textContent = "starters";
    document.body.appendChild(section);

    const scrollIntoView = jest.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      value: scrollIntoView,
      configurable: true,
    });

    render(
      <CategoryTabs categories={[{ slug: "starters", name: "Starters" }]} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Starters" }));

    expect(scrollIntoView).toHaveBeenCalledWith({
      behavior: "smooth",
      block: "start",
    });
    expect(window.location.hash).toBe("#cat-starters");

    section.remove();
  });

  it("scrolls with auto behavior when the user prefers reduced motion", () => {
    const section = document.createElement("section");
    section.id = "cat-starters";
    section.textContent = "starters";
    document.body.appendChild(section);

    const scrollIntoView = jest.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      value: scrollIntoView,
      configurable: true,
    });
    window.matchMedia = jest.fn().mockImplementation((query: string) => ({
      matches: query === "(prefers-reduced-motion: reduce)",
      media: query,
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
    })) as unknown as typeof window.matchMedia;

    render(
      <CategoryTabs categories={[{ slug: "starters", name: "Starters" }]} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Starters" }));

    expect(scrollIntoView).toHaveBeenCalledWith({
      behavior: "auto",
      block: "start",
    });

    section.remove();
  });

  it("highlights the active category and invokes onSelect when provided", () => {
    const onSelect = jest.fn();
    render(
      <CategoryTabs
        categories={[
          { slug: "starters", name: "Starters" },
          { slug: "mains", name: "Mains" },
        ]}
        activeIndex={1}
        onSelect={onSelect}
      />,
    );

    const mains = screen.getByRole("button", { name: "Mains" });
    expect(mains).toHaveAttribute("aria-current", "true");

    fireEvent.click(screen.getByRole("button", { name: "Starters" }));
    expect(onSelect).toHaveBeenCalledWith(0, { slug: "starters", name: "Starters" });
  });

  it("does not throw if the target section is missing from the DOM", () => {
    render(
      <CategoryTabs categories={[{ slug: "ghost", name: "Ghost" }]} />,
    );

    expect(() =>
      fireEvent.click(screen.getByRole("button", { name: "Ghost" })),
    ).not.toThrow();
    // Hash should be untouched when there is no matching section.
    expect(window.location.hash).toBe("");
  });
});
