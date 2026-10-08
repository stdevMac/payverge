/** @jest-environment jsdom */

import React from "react";
import { render, screen } from "@testing-library/react";
import { AssistantRichText } from "../AssistantRichText";
import {
  assistantUrlTransform,
  classifyAssistantHref,
} from "../assistantLinks";

describe("AssistantRichText", () => {
  it("renders assistant Markdown structure and GFM without flattening the response", () => {
    render(
      <AssistantRichText
        content={`# Weekly plan

## Priorities

1. First
2. Second

- Alpha
- Beta

Use **bold**, *emphasis*, and \`inline()\`.

\`\`\`ts
const total = 2;
\`\`\`

> Keep service warm.

~~obsolete~~

| Metric | Value |
| --- | ---: |
| Guests | 12 |

[Register](/business/register)`}
      />,
    );

    expect(
      screen.getByRole("heading", { level: 3, name: "Weekly plan" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 4, name: "Priorities" }),
    ).toBeInTheDocument();
    expect(document.querySelector("ol")).toBeInTheDocument();
    expect(document.querySelector("ul")).toBeInTheDocument();
    expect(screen.getByText("bold").tagName).toBe("STRONG");
    expect(screen.getByText("emphasis").tagName).toBe("EM");
    expect(screen.getByText("inline()").tagName).toBe("CODE");
    expect(screen.getByText("const total = 2;").closest("pre")).not.toBeNull();
    expect(
      screen.getByText("Keep service warm.").closest("blockquote"),
    ).not.toBeNull();
    expect(screen.getByText("obsolete").tagName).toBe("DEL");
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Register" })).toHaveAttribute(
      "href",
      "/business/register",
    );
  });

  it("keeps a javascript link readable but inert while preserving a registered route", () => {
    render(
      <AssistantRichText content="[Register](/business/register) [Bad](javascript:alert(1))" />,
    );

    expect(screen.getByRole("link", { name: "Register" })).toHaveAttribute(
      "href",
      "/business/register",
    );
    expect(screen.queryByRole("link", { name: "Bad" })).not.toBeInTheDocument();
    expect(screen.getByText("Bad")).toBeVisible();
  });

  it.each([
    ["javascript:alert(1)", "scheme"],
    ["data:text/html,<script>alert(1)</script>", "data"],
    ["file:///etc/passwd", "file"],
    ["blob:https://payverge.io/id", "blob"],
    ["//evil.example/path", "protocol relative"],
    ["https://user:pass@example.com/private", "credentials"],
    ["https://example.com\\@evil.test", "backslash"],
    ["https://example.com/%5Cescape", "encoded backslash"],
    ["/business\\..\\admin", "internal backslash"],
    ["\u0001https://example.com", "leading control"],
    ["https://example.com/\u007fescape", "embedded control"],
    ["https://example.com/%0Aescape", "encoded control"],
    ["/api/v1/users", "api"],
    ["/business/restaurant-1", "business"],
    ["/admin", "admin"],
    ["/account", "account"],
    ["/staff/login", "staff"],
    ["/t/table-code", "table"],
    ["/b/restaurant", "storefront"],
    ["pricing", "relative without slash"],
  ])("blocks %s (%s)", (url) => {
    expect(classifyAssistantHref(url)).toBe("blocked");
    expect(assistantUrlTransform(url)).toBe("");
  });

  it("does not create executable or embedded elements from hostile Markdown", () => {
    const { container } = render(
      <AssistantRichText
        content={`[Data](data:text/html,boom)
[File](file:///etc/passwd)
[Blob](blob:https://payverge.io/id)
[Protocol](//evil.example/path)
[Credentials](https://user:pass@example.com/private)
[Unknown](/api/v1/users)
![Tracking pixel](https://evil.example/pixel.png)

<script>alert(1)</script>
<style>body { display: none }</style>
<iframe src="https://evil.example"></iframe>
<object data="https://evil.example"></object>
<svg onload="alert(1)"><script>alert(2)</script></svg>
<form action="https://evil.example"><button formaction="javascript:alert(1)">Send</button></form>
<a href="/business/register" onclick="alert(1)">raw link</a>`}
      />,
    );

    expect(container.querySelector("a")).not.toBeInTheDocument();
    expect(
      container.querySelector(
        "img,script,style,iframe,object,svg,form,button,[onclick],[onload]",
      ),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Tracking pixel")).toBeInTheDocument();
  });

  it("opens only valid absolute HTTP(S) links in a protected new tab", () => {
    render(
      <AssistantRichText content="See [the guide](https://docs.example.com/guide?q=1#start)." />,
    );

    const link = screen.getByRole("link", { name: /the guide/i });
    expect(link).toHaveAttribute(
      "href",
      "https://docs.example.com/guide?q=1#start",
    );
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(link).toHaveTextContent("↗");
  });

  it("allows registered localized public routes and preserves query/hash", () => {
    render(
      <AssistantRichText content="[Registro](/es-ar/business/register?plan=pro#form) [Inicio](/es)" />,
    );

    expect(screen.getByRole("link", { name: "Registro" })).toHaveAttribute(
      "href",
      "/es-ar/business/register?plan=pro#form",
    );
    expect(screen.getByRole("link", { name: "Inicio" })).toHaveAttribute(
      "href",
      "/es",
    );
  });

  it.each([
    "/pricing",
    "/features",
    "/blog",
    "/blog/restaurant-ops",
    "/es/blog/restaurant-ops",
    "/es-ar/pricing",
    "/contact",
    "/tools/roi-calculator",
  ])("blocks the removed marketing route %s", (url) => {
    expect(classifyAssistantHref(url)).toBe("blocked");
    expect(assistantUrlTransform(url)).toBe("");
  });

  it("leaves malformed Markdown readable", () => {
    const { container } = render(
      <AssistantRichText content="Start **unfinished and [broken]( still here" />,
    );

    expect(container).toHaveTextContent("Start");
    expect(container).toHaveTextContent("unfinished");
    expect(container).toHaveTextContent("broken");
    expect(container).toHaveTextContent("still here");
  });

  it("renders plain_text as whitespace-preserving text without raw markdown markers", () => {
    const content =
      "# Plain **bold** [Register](/business/register)\n<script>alert(1)</script>";
    const { container } = render(
      <AssistantRichText content={content} format="plain_text" />,
    );

    expect(container.firstElementChild).toHaveClass("whitespace-pre-wrap");
    // Markers flattened; no HTML injection or rich nodes.
    expect(container).toHaveTextContent("Plain bold Register");
    expect(container).toHaveTextContent("<script>alert(1)</script>");
    expect(container).not.toHaveTextContent("**");
    expect(
      container.querySelector("h1,h2,h3,strong,a,script"),
    ).not.toBeInTheDocument();
  });

  it("uses logical spacing and borders for RTL-safe rich content", () => {
    const { container } = render(
      <AssistantRichText
        content={"> Note\n\n- One\n\n[Docs](https://example.com)"}
      />,
    );

    expect(container.querySelector("blockquote")).toHaveClass(
      "border-s-4",
      "ps-4",
    );
    expect(container.querySelector("ul")).toHaveClass("ps-6");
    expect(
      screen.getByRole("link", { name: /Docs/ }).lastElementChild,
    ).toHaveClass("ms-1");
  });
});
