/** @jest-environment jsdom */
import React from "react";
import { renderToString } from "react-dom/server";
import { hydrateRoot, type Root } from "react-dom/client";
import { act } from "@testing-library/react";

import { getSiteUrl } from "@/config/publicConfig";

import { useSiteUrl } from "./useSiteUrl";

function HookProbe() {
  const siteUrl = useSiteUrl();
  return <a href={`${siteUrl}/contact`}>{siteUrl}</a>;
}

function DirectProbe() {
  const siteUrl = getSiteUrl();
  return <a href={`${siteUrl}/contact`}>{siteUrl}</a>;
}

/** Server-render, then hydrate in the same document, as Next does. */
function serverThenHydrate(element: React.ReactElement) {
  const html = renderToString(element);
  const container = document.createElement("div");
  container.innerHTML = html;
  document.body.appendChild(container);
  const recoverable: unknown[] = [];
  let root: Root | undefined;
  act(() => {
    root = hydrateRoot(container, element, {
      onRecoverableError: (error) => recoverable.push(error),
    });
  });
  return {
    html,
    container,
    recoverable,
    cleanup: () => {
      act(() => root?.unmount());
      container.remove();
    },
  };
}

describe("useSiteUrl", () => {
  let consoleError: jest.SpyInstance;

  beforeEach(() => {
    consoleError = jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    consoleError.mockRestore();
    delete window.__PAYVERGE_ENV__;
  });

  it("hydrates from the server value, then shows the page origin when PUBLIC_URL is unset", () => {
    // What serializePublicEnvScript publishes for an unset PUBLIC_URL.
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "", API_URL: "/api/v1" };
    expect(window.location.origin).not.toBe("http://localhost:3000");

    const run = serverThenHydrate(<HookProbe />);
    try {
      expect(run.html).toContain("http://localhost:3000");
      expect(run.recoverable).toEqual([]);
      expect(consoleError).not.toHaveBeenCalled();
      const link = run.container.querySelector("a");
      expect(link?.textContent).toBe(window.location.origin);
      expect(link?.getAttribute("href")).toBe(`${window.location.origin}/contact`);
    } finally {
      run.cleanup();
    }
  });

  it("is the published PUBLIC_URL on both sides when one is configured", () => {
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "https://pos.example.test" };

    const run = serverThenHydrate(<HookProbe />);
    try {
      expect(run.html).toContain("https://pos.example.test");
      expect(run.recoverable).toEqual([]);
      expect(run.container.querySelector("a")?.getAttribute("href")).toBe(
        "https://pos.example.test/contact",
      );
    } finally {
      run.cleanup();
    }
  });

  it("guards a real mismatch: getSiteUrl() during render diverges from the server HTML", () => {
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "", API_URL: "/api/v1" };
    // A real server renders the dev default; under jsdom renderToString runs
    // with a window, so hand-build the server HTML instead.
    const container = document.createElement("div");
    container.innerHTML =
      '<a href="http://localhost:3000/contact">http://localhost:3000</a>';
    document.body.appendChild(container);
    const recoverable: unknown[] = [];
    let root: Root | undefined;
    act(() => {
      root = hydrateRoot(container, <DirectProbe />, {
        onRecoverableError: (error) => recoverable.push(error),
      });
    });
    try {
      expect(recoverable.length).toBeGreaterThan(0);
    } finally {
      act(() => root?.unmount());
      container.remove();
    }
  });
});
