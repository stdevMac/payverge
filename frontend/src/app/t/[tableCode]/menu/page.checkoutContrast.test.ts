import fs from "fs";
import path from "path";

describe("guest checkout contrast", () => {
  const menuPageSource = fs.readFileSync(
    path.resolve(__dirname, "page.tsx"),
    "utf8",
  );
  const cartSource = fs.readFileSync(
    path.resolve(__dirname, "_components/CartModal.tsx"),
    "utf8",
  );
  const menuViewsSource = fs.readFileSync(
    path.resolve(__dirname, "../../../../components/guest/GuestMenuViews.tsx"),
    "utf8",
  );
  const fiscalFieldsSource = fs.readFileSync(
    path.resolve(
      __dirname,
      "../../../../components/common/FiscalIdentityFields.tsx",
    ),
    "utf8",
  );

  it("keeps checkout foregrounds opaque and on high-contrast semantic tokens", () => {
    expect(cartSource).not.toContain("opacity: 0");
    expect(cartSource).not.toContain("opacity: 1");
    expect(cartSource).toContain('className="text-xs text-ink-600"');
    expect(cartSource).toContain(
      'className="text-xs text-rose-700 hover:bg-rose-50 hover:text-rose-800"',
    );
    expect(cartSource).toContain(
      'className="flex items-center gap-2 text-sm font-medium text-brand-dark hover:text-brand-800"',
    );
    expect(cartSource).toContain(
      'className="flex-1 h-12 rounded-xl bg-ink-950 font-semibold text-white shadow-sm hover:bg-ink-900"',
    );
    expect(fiscalFieldsSource).toContain(
      'toggle: "text-brand-dark hover:text-brand-800"',
    );
  });

  it("does not fade the added-item confirmation below accessible contrast", () => {
    expect(menuViewsSource).toContain(
      'className="bg-emerald-800 text-white px-4 py-2 rounded-full flex items-center gap-2"',
    );
    expect(menuViewsSource).not.toContain(
      "bg-emerald-600 text-white px-4 py-2 rounded-full flex items-center gap-2 motion-safe:animate-pulse",
    );
  });

  it("gives the menu-load spinner a status role and hides the quote spinner", () => {
    expect(menuPageSource.match(/<Spinner/g)).toHaveLength(2);
    expect(menuPageSource.match(/<Spinner[\s\S]*?role="status"/g)).toHaveLength(
      1,
    );
    expect(menuPageSource).toContain('<Spinner size="sm" color="white" aria-hidden />');
  });
});
