/** @jest-environment jsdom */
/* eslint-disable @next/next/no-img-element */
import { render, fireEvent, screen } from "@testing-library/react";
import { ImageCarousel } from "../ImageCarousel";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
  }),
}));

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

describe("ImageCarousel accessibility", () => {
  test("hides inactive slides and keeps nav buttons visible for keyboard and touch", () => {
    const { container } = render(
      <ImageCarousel images={["/a.jpg", "/b.jpg", "/c.jpg"]} itemName="Dish" />,
    );

    const images = Array.from(container.querySelectorAll("img"));
    expect(images).toHaveLength(3);
    const hidden = images.filter(
      (img) => img.getAttribute("aria-hidden") === "true",
    );
    const visible = images.filter((img) => !img.hasAttribute("aria-hidden"));
    expect(hidden).toHaveLength(2);
    expect(visible).toHaveLength(1);
    expect(visible[0].getAttribute("src")).toBe("/a.jpg");

    fireEvent.click(
      screen.getByRole("button", { name: "accessibility.nextImage" }),
    );

    const after = Array.from(container.querySelectorAll("img"));
    const hiddenAfter = after.filter(
      (img) => img.getAttribute("aria-hidden") === "true",
    );
    const visibleAfter = after.filter((img) => !img.hasAttribute("aria-hidden"));
    expect(hiddenAfter).toHaveLength(2);
    expect(visibleAfter).toHaveLength(1);
    expect(visibleAfter[0].getAttribute("src")).toBe("/b.jpg");

    for (const name of [
      "accessibility.previousImage",
      "accessibility.nextImage",
    ]) {
      expect(screen.getByRole("button", { name }).className).toContain(
        "group-focus-within:opacity-100",
      );
    }
  });
});
