/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";
import { useHashTabs } from "@/hooks/useHashTabs";

// Helpers to manage location.hash between tests
function setHash(hash: string) {
  // jsdom supports direct assignment to window.location.hash
  window.location.hash = hash;
}

function clearHash() {
  window.location.hash = "";
}

afterEach(() => {
  clearHash();
  // Remove any ?tab= query params
  window.history.replaceState(null, "", window.location.pathname);
});

describe("useHashTabs — #delivery deep-link retry (R15)", () => {
  it("activates the delivery tab immediately when #delivery is in the URL and delivery is already in validTabs", () => {
    setHash("#delivery");

    const { result } = renderHook(() =>
      useHashTabs({ validTabs: ["about", "menu", "delivery", "contact"], defaultTab: "about" }),
    );

    expect(result.current.activeTab).toBe("delivery");
  });

  it("activates #delivery immediately while feature tabs are still loading", () => {
    setHash("#delivery");

    const { result, rerender } = renderHook(
      ({
        validTabs,
        tabsReady,
      }: {
        validTabs: string[];
        tabsReady: boolean;
      }) => useHashTabs({ validTabs, defaultTab: "about", tabsReady }),
      {
        initialProps: {
          validTabs: ["about", "menu", "contact"],
          tabsReady: false,
        },
      },
    );

    expect(result.current.activeTab).toBe("delivery");

    act(() => {
      rerender({
        validTabs: ["about", "menu", "delivery", "contact"],
        tabsReady: true,
      });
    });

    expect(result.current.activeTab).toBe("delivery");
  });

  it("falls back to the default tab when a lazy hash is unavailable after tabs settle", () => {
    setHash("#reservations");

    const { result, rerender } = renderHook(
      ({
        validTabs,
        tabsReady,
      }: {
        validTabs: string[];
        tabsReady: boolean;
      }) => useHashTabs({ validTabs, defaultTab: "about", tabsReady }),
      {
        initialProps: {
          validTabs: ["about", "menu", "contact"],
          tabsReady: false,
        },
      },
    );

    expect(result.current.activeTab).toBe("reservations");

    act(() => {
      rerender({
        validTabs: ["about", "menu", "contact"],
        tabsReady: true,
      });
    });

    expect(result.current.activeTab).toBe("about");
    expect(window.location.hash).toBe("#about");
  });

  it("hashchange event activates the delivery tab after mount", () => {
    const { result } = renderHook(() =>
      useHashTabs({ validTabs: ["about", "menu", "delivery", "contact"], defaultTab: "about" }),
    );

    expect(result.current.activeTab).toBe("about");

    act(() => {
      window.location.hash = "#delivery";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    expect(result.current.activeTab).toBe("delivery");
  });

  it("changeTab clears any pending hash deep-link and activates the new tab", () => {
    setHash("#delivery");

    // Mount before delivery is in validTabs (pending state)
    const { result, rerender } = renderHook(
      ({ validTabs }: { validTabs: string[] }) =>
        useHashTabs({ validTabs, defaultTab: "about" }),
      { initialProps: { validTabs: ["about", "menu", "contact"] } },
    );

    // User manually navigates to menu before delivery loads
    act(() => {
      result.current.changeTab("menu");
    });

    // Now delivery loads — pending hash should NOT override the user's choice
    act(() => {
      rerender({ validTabs: ["about", "menu", "delivery", "contact"] });
    });

    expect(result.current.activeTab).toBe("menu");
  });

  // PG-15.3: history.pushState alone never fires hashchange (jsdom + browsers).
  // changeTab must notify listeners so AiWaiter/Concierge close on tab switch.
  it("changeTab dispatches hashchange after pushState (PG-15.3)", () => {
    const { result } = renderHook(() =>
      useHashTabs({
        validTabs: ["about", "menu", "delivery", "contact"],
        defaultTab: "about",
      }),
    );

    let hashChangeCount = 0;
    const onHash = () => {
      hashChangeCount += 1;
    };
    window.addEventListener("hashchange", onHash);

    act(() => {
      result.current.changeTab("menu");
    });

    window.removeEventListener("hashchange", onHash);

    expect(result.current.activeTab).toBe("menu");
    expect(window.location.hash).toBe("#menu");
    expect(hashChangeCount).toBeGreaterThan(0);
  });

  it("does not activate an invalid tab from a stale hash", () => {
    setHash("#nonexistent");

    const { result } = renderHook(() =>
      useHashTabs({ validTabs: ["about", "menu", "delivery", "contact"], defaultTab: "about" }),
    );

    expect(result.current.activeTab).toBe("about");
    expect(window.location.hash).toBe("#about");
  });

  it("treats an invalid runtime hash as the default tab instead of keeping prior content", () => {
    setHash("#reservations");

    const { result } = renderHook(() =>
      useHashTabs({
        validTabs: ["about", "menu", "reservations", "contact"],
        defaultTab: "about",
      }),
    );

    expect(result.current.activeTab).toBe("reservations");

    act(() => {
      window.location.hash = "#not-a-section";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    expect(result.current.activeTab).toBe("about");
    expect(window.location.hash).toBe("#about");
  });

  it("does not keep a previous tab when history restores an invalid hash", () => {
    const { result } = renderHook(() =>
      useHashTabs({
        validTabs: ["about", "menu", "reservations", "contact"],
        defaultTab: "about",
      }),
    );

    act(() => {
      result.current.changeTab("reservations");
    });
    expect(result.current.activeTab).toBe("reservations");

    act(() => {
      window.history.replaceState(null, "", "#not-a-section");
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    expect(result.current.activeTab).toBe("about");
    expect(window.location.hash).toBe("#about");
  });

  // CTA coverage lives in DeliveryAvailableCard.test.tsx:
  // "opens the GuestDeliveryOrder modal when the CTA button is pressed" and
  // "calls onQuoteReady and closes the modal when a quote context is emitted".
  test.todo("CTA flow: covered by DeliveryAvailableCard.test.tsx colocated tests");
});
