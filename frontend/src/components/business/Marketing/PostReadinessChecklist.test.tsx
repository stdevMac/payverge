/** @jest-environment jsdom */

import React from "react";
import { render, screen } from "@testing-library/react";
import {
  computeExportReady,
  PostReadinessChecklist,
} from "./PostReadinessChecklist";

const t = (key: string) => key;

describe("computeExportReady", () => {
  it.each([
    {
      name: "all green",
      flags: {
        photoReady: true,
        captionReady: true,
        previewReady: true,
        previewBroken: false,
      },
      ready: true,
    },
    {
      name: "preview still rendering",
      flags: {
        photoReady: true,
        captionReady: true,
        previewReady: false,
        previewBroken: false,
      },
      ready: false,
    },
    {
      name: "preview broken never greens",
      flags: {
        photoReady: true,
        captionReady: true,
        previewReady: true,
        previewBroken: true,
      },
      ready: false,
    },
    {
      name: "missing photo",
      flags: {
        photoReady: false,
        captionReady: true,
        previewReady: true,
        previewBroken: false,
      },
      ready: false,
    },
    {
      name: "missing caption",
      flags: {
        photoReady: true,
        captionReady: false,
        previewReady: true,
        previewBroken: false,
      },
      ready: false,
    },
  ])("$name → $ready", ({ flags, ready }) => {
    expect(computeExportReady(flags)).toBe(ready);
  });
});

describe("PostReadinessChecklist", () => {
  it("marks ready false and surfaces broken label when preview is broken", () => {
    render(
      <PostReadinessChecklist
        photoReady
        captionReady
        previewReady={false}
        previewBroken
        t={t}
      />,
    );
    const list = screen.getByTestId("post-readiness-checklist");
    expect(list).toHaveAttribute("data-export-ready", "false");
    expect(list).toHaveAttribute("data-preview-broken", "true");
    expect(
      screen.getByText("editor.checklist.previewBroken"),
    ).toBeInTheDocument();
    expect(
      list.querySelector('[data-step="ready"]'),
    ).toHaveAttribute("data-ready", "false");
  });

  it("marks all steps ready only when every gate passes", () => {
    render(
      <PostReadinessChecklist
        photoReady
        captionReady
        previewReady
        previewBroken={false}
        t={t}
      />,
    );
    const list = screen.getByTestId("post-readiness-checklist");
    expect(list).toHaveAttribute("data-export-ready", "true");
    for (const step of ["photo", "caption", "preview", "ready"]) {
      expect(list.querySelector(`[data-step="${step}"]`)).toHaveAttribute(
        "data-ready",
        "true",
      );
    }
  });
});
