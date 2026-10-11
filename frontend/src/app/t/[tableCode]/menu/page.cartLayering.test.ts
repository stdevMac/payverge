import fs from "fs";
import path from "path";

describe("guest menu cart pointer layering", () => {
  const menuSource = fs.readFileSync(
    path.resolve(__dirname, "page.tsx"),
    "utf8",
  );
  const callWaiterSource = fs.readFileSync(
    path.resolve(
      __dirname,
      "../../../../components/guest/CallWaiterButton.tsx",
    ),
    "utf8",
  );
  const landingSource = fs.readFileSync(
    path.resolve(
      __dirname,
      "../../../../components/guest/GuestTableView.tsx",
    ),
    "utf8",
  );
  const billSource = fs.readFileSync(
    path.resolve(__dirname, "../bill/page.tsx"),
    "utf8",
  );
  const profileSource = fs.readFileSync(
    path.resolve(__dirname, "../profile/page.tsx"),
    "utf8",
  );
  const themeSource = fs.readFileSync(
    path.resolve(__dirname, "../../../../styles/theme.css"),
    "utf8",
  );
  const aiWaiterSource = fs.readFileSync(
    path.resolve(
      __dirname,
      "../../../../components/guest/AiWaiter.tsx",
    ),
    "utf8",
  );

  it("passes pointer events through the waiter-call gutter while keeping its controls interactive", () => {
    expect(menuSource).toMatch(
      /Raise a hand[\s\S]*?<div className="pointer-events-none relative z-40[^"]*"[\s\S]*?<CallWaiterButton/,
    );
    expect(callWaiterSource).toMatch(
      /className="pointer-events-auto group relative z-10/,
    );
    expect(callWaiterSource).toMatch(
      /className="pointer-events-auto relative z-10/,
    );
    expect(billSource).toMatch(
      /pointer-events-none relative z-\[60\][\s\S]*?<CallWaiterButton/,
    );
  });

  it("clears guest chrome with --guest-nav-height instead of magic rem pads", () => {
    expect(themeSource).toMatch(/--guest-nav-height:\s*5\.5rem/);
    // Landing / bill / profile / menu content shells
    expect(landingSource).toMatch(
      /pb-\[calc\(var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)\+env\(safe-area-inset-bottom\)/,
    );
    expect(billSource).toMatch(
      /pb-\[calc\(var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)\+env\(safe-area-inset-bottom\)/,
    );
    expect(profileSource).toMatch(
      /pb-\[calc\(var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)\+env\(safe-area-inset-bottom\)\)\]/,
    );
    expect(menuSource).toMatch(
      /pb-\[calc\(var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)\+env\(safe-area-inset-bottom\)/,
    );
    // Cart FAB + quote alert sit above the dock via the same token
    expect(menuSource).toMatch(
      /var\(--guest-nav-height,\s*5\.5rem\)\s*\+\s*var\(--cookie-banner-height,\s*0px\)\s*\+\s*env\(safe-area-inset-bottom\)/,
    );
    // No legacy magic 9rem/12rem dock pads on these shells
    expect(landingSource).not.toMatch(
      /pb-\[calc\(9rem\+env\(safe-area-inset-bottom\)\)\]/,
    );
    expect(billSource).not.toMatch(
      /pb-\[calc\(9rem\+env\(safe-area-inset-bottom\)\)\]/,
    );
    expect(menuSource).not.toMatch(
      /pb-\[calc\(12rem\+env\(safe-area-inset-bottom\)\)\]/,
    );
  });

  it("AiWaiter FAB bottoms with --guest-nav-height, not magic 96px-only offset", () => {
    expect(aiWaiterSource).toMatch(
      /bottom-\[calc\(var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)\+env\(safe-area-inset-bottom\)\+0\.75rem\)\]/,
    );
    // Lift further when the cart pill is visible so FAB/nudge don't cover Place Order.
    expect(aiWaiterSource).toMatch(
      /bottom-\[calc\(var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)\+env\(safe-area-inset-bottom\)\+5\.5rem\)\]/,
    );
    expect(aiWaiterSource).not.toMatch(
      /bottom-\[calc\(env\(safe-area-inset-bottom\)\+96px\)\]/,
    );
    expect(aiWaiterSource).not.toMatch(
      /bottom-\[calc\(env\(safe-area-inset-bottom\)\+148px\)\]/,
    );
  });

  it("landing footer rail is marked and shell pads clear the dock", () => {
    expect(landingSource).toContain('data-testid="landing-footer-rail"');
    expect(landingSource).toMatch(
      /pb-\[calc\(var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)\+env\(safe-area-inset-bottom\)\+5\.5rem\)\]/,
    );
  });
});
