/** @jest-environment jsdom */
/* eslint-disable @next/next/no-img-element */
// Issue #597: the table-QR menu carousel gets the same loading treatment as
// the storefront menu — a pulse skeleton while the active image is in
// flight, cleared on load, and the utensils fallback (never a stuck
// skeleton) on error.
import { render, fireEvent } from "@testing-library/react";
import { ImageCarousel } from "../ImageCarousel";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
  }),
}));

jest.mock("next/image", () => {
  const React = jest.requireActual("react");
  // forwardRef: the component attaches a ref to detect already-cached images.
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

describe("ImageCarousel loading skeleton (#597)", () => {
  test("shows a pulse skeleton over the frame until the image loads", () => {
    const { container } = render(
      <ImageCarousel images={["/a.jpg"]} itemName="Dish" />,
    );
    const skeleton = container.querySelector(SKELETON) as HTMLElement;
    expect(skeleton).toBeTruthy();
    expect(skeleton.className).toMatch(/animate-pulse/);
    expect(skeleton.className).toMatch(/motion-reduce:animate-none/);
    expect(skeleton.className).toMatch(/pointer-events-none/);

    fireEvent.load(container.querySelector("img") as HTMLImageElement);
    expect(container.querySelector(SKELETON)).toBeNull();
    expect(container.querySelector("img")).toBeTruthy();
  });

  test("single image error shows the utensils placeholder, not a stuck skeleton", () => {
    const { container } = render(
      <ImageCarousel images={["/broken.jpg"]} itemName="Dish" />,
    );
    fireEvent.error(container.querySelector("img") as HTMLImageElement);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).toBeTruthy(); // utensils icon
    expect(container.querySelector(SKELETON)).toBeNull();
  });

  test("when the active image errors, the skeleton follows the replacement frame instead of sticking", () => {
    const { container } = render(
      <ImageCarousel images={["/broken.jpg", "/b.jpg"]} itemName="Dish" />,
    );
    const first = container.querySelector(
      'img[src="/broken.jpg"]',
    ) as HTMLImageElement;
    fireEvent.error(first);
    // Carousel snapped to /b.jpg, which is still in flight → skeleton shown.
    expect(container.querySelector('img[src="/broken.jpg"]')).toBeNull();
    expect(container.querySelector(SKELETON)).toBeTruthy();
    fireEvent.load(
      container.querySelector('img[src="/b.jpg"]') as HTMLImageElement,
    );
    expect(container.querySelector(SKELETON)).toBeNull();
  });

  test("no images renders the placeholder with no skeleton", () => {
    const { container } = render(<ImageCarousel images={[]} itemName="Dish" />);
    expect(container.querySelector("svg")).toBeTruthy();
    expect(container.querySelector(SKELETON)).toBeNull();
  });
});
