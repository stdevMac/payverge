/** @jest-environment jsdom */
import React from "react";
import { render, screen, act } from "@testing-library/react";
import {
  CookieConsentProvider,
  useCookieConsent,
  useSuppressCookieBanner,
} from "./CookieConsentContext";
import { CONSENT_STORAGE_KEY } from "@/lib/analytics/consentGate";

function Probe() {
  const { ready, hasConsent, accept, decline, reset, consent } =
    useCookieConsent();
  return (
    <div>
      <span data-testid="ready">{String(ready)}</span>
      <span data-testid="decided">{String(consent !== null)}</span>
      <span data-testid="analytics">{String(hasConsent("analytics"))}</span>
      <span data-testid="marketing">{String(hasConsent("marketing"))}</span>
      <span data-testid="essential">{String(hasConsent("essential"))}</span>
      <button onClick={() => accept()}>accept</button>
      <button onClick={() => decline()}>decline</button>
      <button onClick={() => reset()}>reset</button>
    </div>
  );
}

describe("CookieConsentProvider", () => {
  beforeEach(() => localStorage.clear());

  it("starts undecided with all non-essential consent false; essential always true", () => {
    render(
      <CookieConsentProvider>
        <Probe />
      </CookieConsentProvider>,
    );
    expect(screen.getByTestId("decided").textContent).toBe("false");
    expect(screen.getByTestId("analytics").textContent).toBe("false");
    expect(screen.getByTestId("marketing").textContent).toBe("false");
    expect(screen.getByTestId("essential").textContent).toBe("true");
  });

  it("decline() persists analytics=false and marks decided", () => {
    render(
      <CookieConsentProvider>
        <Probe />
      </CookieConsentProvider>,
    );
    act(() => {
      screen.getByText("decline").click();
    });
    expect(screen.getByTestId("analytics").textContent).toBe("false");
    expect(screen.getByTestId("decided").textContent).toBe("true");
    const stored = JSON.parse(localStorage.getItem(CONSENT_STORAGE_KEY)!);
    expect(stored.analytics).toBe(false);
  });

  it("accept() persists analytics=true", () => {
    render(
      <CookieConsentProvider>
        <Probe />
      </CookieConsentProvider>,
    );
    act(() => {
      screen.getByText("accept").click();
    });
    expect(screen.getByTestId("analytics").textContent).toBe("true");
    const stored = JSON.parse(localStorage.getItem(CONSENT_STORAGE_KEY)!);
    expect(stored.analytics).toBe(true);
  });

  it("a later decline withdraws both optional categories", () => {
    render(
      <CookieConsentProvider>
        <Probe />
      </CookieConsentProvider>,
    );
    act(() => screen.getByText("accept").click());
    act(() => screen.getByText("decline").click());
    expect(screen.getByTestId("analytics").textContent).toBe("false");
    expect(screen.getByTestId("marketing").textContent).toBe("false");
    const stored = JSON.parse(localStorage.getItem(CONSENT_STORAGE_KEY)!);
    expect(stored.analytics).toBe(false);
    expect(stored.marketing).toBe(false);
  });

  it("hydrates from a previously persisted decision", () => {
    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: true,
        marketing: false,
        decidedAt: new Date().toISOString(),
      }),
    );
    render(
      <CookieConsentProvider>
        <Probe />
      </CookieConsentProvider>,
    );
    expect(screen.getByTestId("decided").textContent).toBe("true");
    expect(screen.getByTestId("analytics").textContent).toBe("true");
  });

  it("reset() clears the decision so the banner can reopen", () => {
    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: true,
        marketing: false,
        decidedAt: new Date().toISOString(),
      }),
    );
    render(
      <CookieConsentProvider>
        <Probe />
      </CookieConsentProvider>,
    );
    act(() => {
      screen.getByText("reset").click();
    });
    expect(screen.getByTestId("decided").textContent).toBe("false");
    expect(localStorage.getItem(CONSENT_STORAGE_KEY)).toBeNull();
  });
});

describe("useSuppressCookieBanner", () => {
  function Suppressor({ active }: { active: boolean }) {
    useSuppressCookieBanner(active);
    return null;
  }
  function SuppressedProbe() {
    const { bannerSuppressed } = useCookieConsent();
    return <span data-testid="suppressed">{String(bannerSuppressed)}</span>;
  }
  function Tree({ a, b }: { a: boolean; b: boolean }) {
    return (
      <CookieConsentProvider>
        <Suppressor active={a} />
        <Suppressor active={b} />
        <SuppressedProbe />
      </CookieConsentProvider>
    );
  }

  it("suppresses while any surface is active and releases once all close", () => {
    const view = render(<Tree a={false} b={false} />);
    expect(screen.getByTestId("suppressed").textContent).toBe("false");
    view.rerender(<Tree a b={false} />);
    expect(screen.getByTestId("suppressed").textContent).toBe("true");
    view.rerender(<Tree a b />);
    view.rerender(<Tree a={false} b />);
    // One modal closing must not unhide the banner over the other.
    expect(screen.getByTestId("suppressed").textContent).toBe("true");
    view.rerender(<Tree a={false} b={false} />);
    expect(screen.getByTestId("suppressed").textContent).toBe("false");
  });

  it("releases on unmount", () => {
    function Mounted({ show }: { show: boolean }) {
      return (
        <CookieConsentProvider>
          {show && <Suppressor active />}
          <SuppressedProbe />
        </CookieConsentProvider>
      );
    }
    const view = render(<Mounted show />);
    expect(screen.getByTestId("suppressed").textContent).toBe("true");
    view.rerender(<Mounted show={false} />);
    expect(screen.getByTestId("suppressed").textContent).toBe("false");
  });
});
