/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AIImageTools } from "./AIImageTools";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";

// This component substitutes {limit}/{hours} into daily-limit copy at render
// time, so (unlike the repo's usual bare-key passthrough — see
// OrderDetailDrawer.test.tsx) the mock needs param-aware strings to make the
// substituted values visible to screen.getByText. No real copy exists yet for
// these keys (Task 11 owns the production message-file text); these are
// placeholder sentences for assertion purposes only, not a preview of shipped
// copy.
const tString = (key: string, params?: Record<string, string | number>) => {
  const map: Record<string, string> = {
    "items.aiImageTools.dailyLimit.title":
      "You've generated {limit} AI images today, which is the daily limit.",
    "items.aiImageTools.dailyLimit.resets": "It resets in about {hours} hours.",
    "items.aiImageTools.dailyLimit.resetsHour": "It resets in about an hour.",
    "items.aiImageTools.dailyLimit.resetsSoon":
      "It resets in less than an hour.",
    "items.aiImageTools.dailyLimit.contact":
      "Need more today? Get in touch and we'll raise it.",
  };
  let val = map[key] ?? key;
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      val = val.replace(`{${k}}`, String(v));
    });
  }
  return val;
};

const baseProps = {
  tString,
  itemName: "Burger",
  itemDescription: "Beef, cheese, lettuce",
  images: [] as string[],
  isGeneratingBreakdown: false,
  isGeneratingPhoto: false,
  isEnhancing: false,
  onBreakdown: jest.fn(),
  onGenerate: jest.fn(),
  onEnhance: jest.fn(),
};

beforeEach(() => jest.clearAllMocks());
afterEach(() => resetInstanceCacheForTests());

it("renders all three tool CTAs", () => {
  render(<AIImageTools {...baseProps} />);
  expect(
    screen.getByText("items.aiImageTools.breakdown.cta"),
  ).toBeInTheDocument();
  expect(
    screen.getByText("items.aiImageTools.generate.cta"),
  ).toBeInTheDocument();
  expect(
    screen.getByText("items.aiImageTools.enhance.cta"),
  ).toBeInTheDocument();
});

it("gives each information control a unique contextual accessible name", () => {
  render(<AIImageTools {...baseProps} />);

  const controls = ["breakdown", "generate", "enhance"].map((tool) => {
    const name = `items.aiImageTools.${tool}.info.ariaLabel`;
    return screen.getByRole("button", { name });
  });

  const names = controls.map((control) => control.getAttribute("aria-label"));
  expect(new Set(names).size).toBe(controls.length);
  expect(
    screen.queryByRole("button", { name: "items.aiImageTools.infoCta" }),
  ).not.toBeInTheDocument();
});

it("enhances directly when there is exactly one image", async () => {
  const onEnhance = jest.fn();
  render(
    <AIImageTools
      {...baseProps}
      images={["https://x/a.png"]}
      onEnhance={onEnhance}
    />,
  );
  await userEvent.click(screen.getByText("items.aiImageTools.enhance.cta"));
  expect(onEnhance).toHaveBeenCalledWith("https://x/a.png");
});

it("opens a picker and enhances the chosen image when there are several", async () => {
  const onEnhance = jest.fn();
  render(
    <AIImageTools
      {...baseProps}
      images={["https://x/a.png", "https://x/b.png"]}
      onEnhance={onEnhance}
    />,
  );
  await userEvent.click(screen.getByText("items.aiImageTools.enhance.cta"));
  expect(
    screen.getByText("items.aiImageTools.enhance.picker.title"),
  ).toBeInTheDocument();
  await userEvent.click(
    screen.getByRole("button", {
      name: "items.aiImageTools.enhance.picker.option 2",
    }),
  );
  await userEvent.click(
    screen.getByText("items.aiImageTools.enhance.picker.confirm"),
  );
  expect(onEnhance).toHaveBeenCalledWith("https://x/b.png");
});

it("disables enhance when there are no images", () => {
  render(<AIImageTools {...baseProps} images={[]} />);
  const btn = screen
    .getByText("items.aiImageTools.enhance.cta")
    .closest("button");
  expect(btn).toBeDisabled();
});

it("renders nothing when the server has no AI provider configured", () => {
  setInstanceForTests(
    parseInstanceInfo({ registration_mode: "invite", features: { ai: false } })!,
  );
  const { container } = render(
    <AIImageTools {...baseProps} images={["https://x/a.png"]} />,
  );
  expect(container).toBeEmptyDOMElement();
});

it("shows nothing about limits during normal use", () => {
  render(<AIImageTools {...baseProps} />);
  expect(screen.queryByText(/reached today's limit/i)).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /buy/i }),
  ).not.toBeInTheDocument();
});

it("explains the limit and the reset only once it is hit", () => {
  render(
    <AIImageTools
      {...baseProps}
      dailyLimitReached={{ dailyLimit: 500, resetsInSeconds: 21600 }}
    />,
  );
  const status = screen.getByRole("status");
  expect(status).toHaveTextContent(/500/);
  expect(status).toHaveTextContent(/about 6 hours/i);
});

it("uses singular reset copy for a one-hour window", () => {
  render(
    <AIImageTools
      {...baseProps}
      dailyLimitReached={{ dailyLimit: 500, resetsInSeconds: 3600 }}
    />,
  );
  expect(screen.getByRole("status")).toHaveTextContent(
    /resets in about an hour/i,
  );
});

it("uses soon-reset copy for a sub-hour window", () => {
  render(
    <AIImageTools
      {...baseProps}
      dailyLimitReached={{ dailyLimit: 500, resetsInSeconds: 300 }}
    />,
  );
  expect(screen.getByRole("status")).toHaveTextContent(
    /resets in less than an hour/i,
  );
});
