/** @jest-environment jsdom */
/* eslint-disable @next/next/no-img-element */
import { render } from "@testing-library/react";
import MenuItemMedia from "../MenuItemMedia";

jest.mock("next/image", () => {
  const React = jest.requireActual("react");
  // forwardRef: the component attaches a ref to detect already-cached images.
  const MockNextImage = React.forwardRef((props: any, ref: any) => {
    const { fill } = props;
    const imageProps = { ...props };
    delete imageProps.fill;
    delete imageProps.priority;
    delete imageProps.unoptimized;
    return <img {...imageProps} ref={ref} alt={props.alt ?? ""} data-fill={String(!!fill)} />;
  });
  MockNextImage.displayName = "MockNextImage";
  return { __esModule: true, default: MockNextImage };
});

describe("MenuItemMedia", () => {
  test("renders next/image with fill inside aspect-[4/3] when images are present", () => {
    const { container } = render(<MenuItemMedia images={["/a.jpg"]} alt="dish" />);
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper.className).toMatch(/aspect-\[4\/3\]/);
    expect(wrapper.className).toMatch(/relative/);
    const img = wrapper.querySelector("img[data-fill='true']");
    expect(img).toBeTruthy();
  });

  test("renders MenuItemNoMediaHeader when images is empty", () => {
    const { container, getByText } = render(<MenuItemMedia images={[]} alt="dish" noMediaLabel="No image" />);
    expect(getByText("No image")).toBeInTheDocument();
    expect(container.querySelector("[data-fill='true']")).toBeNull();
  });
});
