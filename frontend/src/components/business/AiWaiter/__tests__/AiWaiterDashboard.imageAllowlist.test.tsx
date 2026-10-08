/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";

// AiWaiterDashboard now imports HybridAuthProvider, which transitively pulls in
// wagmi (ESM). Stub useAuth so importing renderChatContent doesn't evaluate it.
jest.mock("@/providers/HybridAuthProvider", () => ({ useAuth: () => ({ staffData: null }) }));

import { renderChatContent } from "../AiWaiterDashboard";

describe("AiWaiterDashboard.renderChatContent image allowlist", () => {
  const hosts = new Set<string>(["assets.payverge.io"]);

  it("renders an allowed image host as <img>", () => {
    const { container } = render(<>{renderChatContent("![x](https://assets.payverge.io/a.jpg)", hosts)}</>);
    expect(container.querySelector("img")).not.toBeNull();
    expect(container.querySelector("img")?.getAttribute("src")).toBe("https://assets.payverge.io/a.jpg");
  });

  it("renders a hostile host as plain text, not <img>", () => {
    const { container } = render(<>{renderChatContent("![p](https://evil.example.com/t.gif)", hosts)}</>);
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain("evil.example.com/t.gif");
  });

  it("strips disallowed image markdown in operator-transcript mode (E5)", () => {
    const { container } = render(
      <>{renderChatContent("Here you go ![p](https://evil.example.com/t.gif) enjoy", hosts, true)}</>,
    );
    expect(container.querySelector("img")).toBeNull();
    // Raw markdown syntax and the untrusted URL must not leak into the transcript.
    expect(container.textContent).not.toContain("![p]");
    expect(container.textContent).not.toContain("evil.example.com");
    // Surrounding prose is preserved.
    expect(container.textContent).toContain("Here you go");
    expect(container.textContent).toContain("enjoy");
  });
});
