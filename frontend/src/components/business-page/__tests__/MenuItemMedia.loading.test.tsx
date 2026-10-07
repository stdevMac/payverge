/** @jest-environment jsdom */
/* eslint-disable @next/next/no-img-element */
// Issue #597: storefront dish photos must show a pulse skeleton while the
// CDN image is in flight, clear it on load, and keep the no-media fallback
// (never a stuck skeleton) on error.
import { render, fireEvent } from "@testing-library/react";
import MenuItemMedia from "../MenuItemMedia";

jest.mock("next/image", () => {
  const React = jest.requireActual("react");
  const MockNextImage = React.forwardRef((props: any, ref: any) => {
    const imageProps = { ...props };
    delete imageProps.fill;
    delete imageProps.priority;
    delete imageProps.unoptimized;
    return <img {...imageProps} ref={ref} alt={props.alt ?? ""} />;
  });
  MockNextImage.displayName = "MockNextImage";
  return { __esModule: true, default: MockNextImage };
});

const SKELETON = '[data-testid="image-loading-skeleton"]';

describe("MenuItemMedia loading skeleton (#597)", () => {
  test("shows a pulse skeleton until the image loads", () => {
    const { container } = render(
      <MenuItemMedia images={["/a.jpg"]} alt="dish" />,
    );
    const skeleton = container.querySelector(SKELETON) as HTMLElement;
    expect(skeleton).toBeTruthy();
    // CSS pulse, frozen for reduced-motion users.
    expect(skeleton.className).toMatch(/animate-pulse/);
    expect(skeleton.className).toMatch(/motion-reduce:animate-none/);
    // Never intercepts taps on the card.
    expect(skeleton.className).toMatch(/pointer-events-none/);
    expect(skeleton.getAttribute("aria-hidden")).toBe("true");
  });

  test("clears the skeleton when the image load event fires", () => {
    const { container } = render(
      <MenuItemMedia images={["/a.jpg"]} alt="dish" />,
    );
    const img = container.querySelector("img") as HTMLImageElement;
    fireEvent.load(img);
    expect(container.querySelector(SKELETON)).toBeNull();
    // Image stays rendered.
    expect(container.querySelector("img")).toBeTruthy();
  });

  test("error path shows the no-media fallback, not a stuck skeleton", () => {
    const { container, getByText } = render(
      <MenuItemMedia images={["/broken.jpg"]} alt="dish" noMediaLabel="No image" />,
    );
    const img = container.querySelector("img") as HTMLImageElement;
    fireEvent.error(img);
    expect(getByText("No image")).toBeInTheDocument();
    expect(container.querySelector(SKELETON)).toBeNull();
  });

  test("does not re-show the skeleton when returning to an already-loaded image", () => {
    const { container, getByLabelText } = render(
      <MenuItemMedia images={["/a.jpg", "/b.jpg"]} alt="dish" />,
    );
    // Load first image, move to the second (in flight), come back to first.
    fireEvent.load(container.querySelector("img") as HTMLImageElement);
    fireEvent.click(getByLabelText("Image 2 of 2"));
    expect(container.querySelector(SKELETON)).toBeTruthy();
    fireEvent.click(getByLabelText("Image 1 of 2"));
    expect(container.querySelector(SKELETON)).toBeNull();
  });
});
