/** @jest-environment jsdom */
import React, { useState } from "react";
import { fireEvent, render, screen, act } from "@testing-library/react";
import {
  GuestTranslationProvider,
  useGuestTranslation,
} from "../GuestTranslationProvider";

const seen = {
  consumerRenders: 0,
  t: undefined as unknown,
  setLanguage: undefined as unknown,
};

function Probe() {
  const { t, setLanguage } = useGuestTranslation();
  seen.t = t;
  seen.setLanguage = setLanguage;
  return <span data-testid="probe">{t("errors.networkError")}</span>;
}

const MemoConsumer = React.memo(function MemoConsumer() {
  const { t } = useGuestTranslation();
  seen.consumerRenders += 1;
  return <span data-testid="consumer">{t("errors.networkError")}</span>;
});

function Parent() {
  const [tick, setTick] = useState(0);
  return (
    <div>
      <button type="button" onClick={() => setTick((n) => n + 1)}>
        bump
      </button>
      <span data-testid="tick">{tick}</span>
      <GuestTranslationProvider initialLanguage="en">
        <Probe />
        <MemoConsumer />
      </GuestTranslationProvider>
    </div>
  );
}

describe("GuestTranslationProvider render stability", () => {
  const originalLang = document.documentElement.lang;
  const originalDir = document.documentElement.dir;

  beforeEach(() => {
    seen.consumerRenders = 0;
    seen.t = undefined;
    seen.setLanguage = undefined;
  });

  afterEach(() => {
    document.documentElement.lang = originalLang;
    document.documentElement.dir = originalDir;
  });

  it("does not re-render a memo consumer when the parent re-renders, and keeps t and setLanguage stable", async () => {
    render(<Parent />);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByTestId("probe")).toHaveTextContent(
      "Network connection error",
    );
    const rendersAfterMount = seen.consumerRenders;
    const tAfterMount = seen.t;
    const setLanguageAfterMount = seen.setLanguage;
    expect(typeof tAfterMount).toBe("function");
    expect(typeof setLanguageAfterMount).toBe("function");
    expect(rendersAfterMount).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "bump" }));

    expect(screen.getByTestId("tick")).toHaveTextContent("1");
    expect(seen.consumerRenders).toBe(rendersAfterMount);
    expect(seen.t).toBe(tAfterMount);
    expect(seen.setLanguage).toBe(setLanguageAfterMount);
  });
});
