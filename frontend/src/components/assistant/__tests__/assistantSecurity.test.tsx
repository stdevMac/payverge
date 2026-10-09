/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { AssistantRichText } from "../AssistantRichText";
import { AssistantSources } from "../AssistantSources";
import {
  assistantUrlTransform,
  classifyAssistantHref,
} from "../assistantLinks";

const sourceLabels = {
  usedSources: (count: number) => `Used ${count} sources`,
  sourcesRegion: (count: number) => `${count} verified sources`,
  sourceOrigin: (origin: string) => `Origin: ${origin}`,
  externalSource: (hostname: string) => `External: ${hostname}`,
};

describe("assistant rendering security", () => {
  it("renders registered internal, normal HTTPS, and verified source destinations", () => {
    render(
      <>
        <AssistantRichText content="[Register](/business/register) [Guide](https://docs.example.com/guide?q=1#start)" />
        <AssistantSources
          labels={sourceLabels}
          sources={[
            {
              id: "source-menu",
              type: "dashboard_guide",
              title: "Verified menu guide",
              href: "/business/7/dashboard?tab=menu",
              origin: "ops_guide_catalog",
              retrieved_at: "2026-08-08T12:00:00Z",
            },
          ]}
        />
      </>,
    );

    expect(screen.getByRole("link", { name: "Register" })).toHaveAttribute(
      "href",
      "/business/register",
    );
    const guide = screen.getByRole("link", { name: /Guide/ });
    expect(guide).toHaveAttribute(
      "href",
      "https://docs.example.com/guide?q=1#start",
    );
    expect(guide).toHaveAttribute("target", "_blank");
    expect(guide).toHaveAttribute("rel", "noopener noreferrer");

    fireEvent.click(screen.getByRole("button", { name: "Used 1 sources" }));
    expect(
      screen.getByRole("link", { name: "Verified menu guide" }),
    ).toHaveAttribute("href", "/business/7/dashboard?tab=menu");
  });

  it.each([
    ["javascript:alert(1)", "script scheme"],
    ["data:text/html,<script>alert(1)</script>", "data scheme"],
    ["//evil.example/path", "protocol-relative URL"],
    ["https://user:pass@example.com/private", "credentialed URL"],
    ["https://payverge.io。evil.example/path", "Unicode host separator"],
    ["/business/7/dashboard?tab=secret-admin", "unknown internal route"],
  ])("keeps %s (%s) inert", (href) => {
    expect(classifyAssistantHref(href)).toBe("blocked");
    expect(assistantUrlTransform(href)).toBe("");

    render(<AssistantRichText content={`[Hostile](${href})`} />);
    expect(screen.getByText("Hostile")).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "Hostile" }),
    ).not.toBeInTheDocument();
  });

  it("renders hostile HTML, embeds, images, and handler syntax as inert content", () => {
    const { container } = render(
      <AssistantRichText
        content={`Useful answer remains.

![Tracking pixel](https://evil.example/pixel.png)

<script>alert(1)</script>
<img src="x" onerror="alert(2)">
<iframe src="https://evil.example"></iframe>
<a href="/business/register" onclick="alert(3)">raw handler link</a>`}
      />,
    );

    expect(screen.getByText("Useful answer remains.")).toBeVisible();
    expect(screen.getByText("Tracking pixel")).toBeVisible();
    expect(
      container.querySelector(
        "script,img,iframe,[onerror],[onclick],a[href='/business/register']",
      ),
    ).not.toBeInTheDocument();
  });

  it("keeps hostile and invalid-route verified sources readable without anchors", () => {
    render(
      <AssistantSources
        labels={sourceLabels}
        sources={[
          {
            id: "source-hostile",
            type: "knowledge_entry",
            title: "Readable source title",
            href: "https://user:pass@example.com/private",
            origin: "knowledge_base",
            retrieved_at: "2026-08-08T12:00:00Z",
          },
          {
            id: "source-unknown-route",
            type: "dashboard_guide",
            title: "Unknown dashboard guide",
            href: "/business/7/dashboard?tab=secret-admin",
            origin: "ops_guide_catalog",
            retrieved_at: "2026-08-08T12:00:00Z",
          },
          {
            id: "source-duplicate-route",
            type: "dashboard_guide",
            title: "Duplicate dashboard route",
            href: "/business/7/dashboard?tab=menu&tab=settings",
            origin: "ops_guide_catalog",
            retrieved_at: "2026-08-08T12:00:00Z",
          },
          {
            id: "source-conflicting-identity",
            type: "dashboard_guide",
            title: "Conflicting business identity",
            href: "/business/7/dashboard?tab=menu&business_id=999",
            origin: "ops_guide_catalog",
            retrieved_at: "2026-08-08T12:00:00Z",
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Used 4 sources" }));
    expect(screen.getByText("Readable source title")).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "Readable source title" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Unknown dashboard guide")).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "Unknown dashboard guide" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Duplicate dashboard route")).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "Duplicate dashboard route" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Conflicting business identity")).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "Conflicting business identity" }),
    ).not.toBeInTheDocument();
  });
});
