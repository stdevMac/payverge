/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import DirectorMessage from "../DirectorMessage";

describe("DirectorMessage image-free regression guard (P2-12)", () => {
  it("does NOT render an <img> for markdown image syntax in an assistant message", () => {
    const { container } = render(
      <DirectorMessage role="assistant" content={"![x](https://evil.example/i.png)"} />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain("https://evil.example/i.png");
  });

  it("does NOT render an <img> when an image is embedded mid-paragraph", () => {
    const { container } = render(
      <DirectorMessage role="assistant" content={"Sales rose. ![pixel](https://evil.example/track.gif) Great week!"} />,
    );
    expect(container.querySelector("img")).toBeNull();
  });

  it("does NOT render an anchor for markdown link syntax (no link injection vector)", () => {
    const { container } = render(
      <DirectorMessage role="assistant" content={"See [report](https://evil.example/leak)"} />,
    );
    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("https://evil.example/leak");
  });

  it("still renders supported formatting (bold) so the guard is not vacuous", () => {
    const { container } = render(
      <DirectorMessage role="assistant" content={"**Revenue** is up"} />,
    );
    expect(container.querySelector("strong")).not.toBeNull();
    expect(container.querySelector("img")).toBeNull();
  });
});
