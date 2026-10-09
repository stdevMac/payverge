/** @jest-environment jsdom */
/* eslint-disable @next/next/no-img-element */
// Issue #597: the older MenuImage/MenuCarousel helpers in PublicMenuDisplay
// (item-details modal) get the same loading treatment as MenuItemMedia — a
// pulse skeleton while the image is in flight, cleared on load, and the
// utensils fallback (never a stuck skeleton) on error.
import { render, fireEvent } from "@testing-library/react";
import { MenuImage, MenuCarousel } from "../PublicMenuDisplay";

const t = (k: string) => k;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t }),
}));

// NextUI's Image probes loading through an off-DOM Image() that jsdom never
// completes, so substitute a plain forwardRef <img> that surfaces the
// onLoad/onError/ref wiring MenuImage relies on in the browser.
jest.mock("@nextui-org/react", () => {
  const actual = jest.requireActual("@nextui-org/react");
  const React = jest.requireActual("react");
  const MockNextUIImage = React.forwardRef((props: any, ref: any) => {
    const imageProps = { ...props };
    delete imageProps.classNames;
    delete imageProps.isBlurred;
    delete imageProps.isZoomed;
    delete imageProps.removeWrapper;
    delete imageProps.disableSkeleton;
    return <img {...imageProps} ref={ref} alt={props.alt ?? ""} />;
  });
  MockNextUIImage.displayName = "MockNextUIImage";
  return { ...actual, Image: MockNextUIImage };
});

const SKELETON = '[data-testid="image-loading-skeleton"]';

describe("MenuImage loading skeleton (#597)", () => {
  test("shows a pulse skeleton until the image loads, then clears it", () => {
    const { container } = render(
      <MenuImage src="/dish.jpg" alt="dish" className="w-full h-full" />,
    );
    const skeleton = container.querySelector(SKELETON) as HTMLElement;
    expect(skeleton).toBeTruthy();
    expect(skeleton.className).toMatch(/animate-pulse/);
    expect(skeleton.className).toMatch(/motion-reduce:animate-none/);

    fireEvent.load(container.querySelector("img") as HTMLImageElement);
    expect(container.querySelector(SKELETON)).toBeNull();
    expect(container.querySelector("img")).toBeTruthy();
  });

  test("error path shows the utensils fallback, not a stuck skeleton", () => {
    const { container } = render(
      <MenuImage src="/broken.jpg" alt="dish" className="w-full h-full" />,
    );
    fireEvent.error(container.querySelector("img") as HTMLImageElement);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).toBeTruthy(); // utensils icon
    expect(container.querySelector(SKELETON)).toBeNull();
  });

  test("missing src renders the fallback with no skeleton", () => {
    const { container } = render(
      <MenuImage src={undefined} alt="dish" className="w-full h-full" />,
    );
    expect(container.querySelector("svg")).toBeTruthy();
    expect(container.querySelector(SKELETON)).toBeNull();
  });
});

describe("MenuCarousel loading skeleton (#597)", () => {
  test("carousel frame shows a skeleton until the current image loads", () => {
    const { container } = render(
      <MenuCarousel images={["/a.jpg"]} alt="dish" className="aspect-square" />,
    );
    expect(container.querySelector(SKELETON)).toBeTruthy();
    fireEvent.load(container.querySelector("img") as HTMLImageElement);
    expect(container.querySelector(SKELETON)).toBeNull();
  });

  test("empty carousel renders the fallback with no skeleton", () => {
    const { container } = render(
      <MenuCarousel images={[]} alt="dish" className="aspect-square" />,
    );
    expect(container.querySelector("svg")).toBeTruthy();
    expect(container.querySelector(SKELETON)).toBeNull();
  });
});
