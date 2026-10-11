/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import MenuItemMedia from "../MenuItemMedia";

// Capture the props next/image receives so we can assert `unoptimized` is gone.
const imgProps: Record<string, unknown>[] = [];
jest.mock("next/image", () => {
  const ReactActual = jest.requireActual("react");
  // forwardRef: the component attaches a ref to detect already-cached images.
  const MockNextImage = ReactActual.forwardRef(
    (props: Record<string, unknown>, ref: unknown) => {
      imgProps.push(props);
      const { fill: _fill, ...imgAttrs } = props;
      // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
      return <img {...(imgAttrs as any)} ref={ref as any} />;
    },
  );
  MockNextImage.displayName = "MockNextImage";
  return { __esModule: true, default: MockNextImage };
});

describe("MenuItemMedia — optimized images (PERF-2)", () => {
  beforeEach(() => {
    imgProps.length = 0;
  });

  it("does NOT pass unoptimized to next/image", () => {
    render(
      <MenuItemMedia
        images={["/media/menu/steak.jpg"]}
        alt="Steak"
      />,
    );
    // The menu media renders exactly one real <NextImage> (single valid URL,
    // no carousel dots). Filter to the rendered image props.
    const rendered = imgProps.filter(
      (p): p is Record<string, unknown> =>
        p != null && (p as Record<string, unknown>).src != null,
    );
    expect(rendered).toHaveLength(1);
    expect(rendered[0].unoptimized).toBeFalsy();
    // The error fallback wiring must be preserved.
    expect(typeof rendered[0].onError).toBe("function");
  });

  it("serves a runtime-only upload host unoptimized instead of a broken optimizer URL", () => {
    render(
      <MenuItemMedia
        images={["https://bucket.example.test/menu/steak.jpg"]}
        alt="Steak"
      />,
    );
    const rendered = imgProps.filter((p) => p != null && p.src != null);
    expect(rendered[0].unoptimized).toBe(true);
  });
});
