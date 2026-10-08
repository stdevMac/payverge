/** @jest-environment jsdom */
import { render, screen, fireEvent } from "@testing-library/react";
import BannerImageUploader from "../BannerImageUploader";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));
jest.mock("@/api/uploads", () => ({ uploadFile: jest.fn() }));

describe("BannerImageUploader — removal hygiene (INT-7) + a11y overlay (A11Y-14)", () => {
  it("removing a banner splices it out (no empty-string slot) and keeps the rest", () => {
    const onBannersChange = jest.fn();
    render(
      <BannerImageUploader
        bannerImages={["https://example.com/a.jpg", "https://example.com/b.jpg"]}
        onBannersChange={onBannersChange}
        businessId={1}
      />,
    );
    // The first filled slot's Remove control (aria-label resolves to the key).
    const removeButtons = screen.getAllByRole("button", { name: /buttons\.remove/i });
    fireEvent.click(removeButtons[0]);
    // Spliced, not blanked: ["b"], with NO empty strings.
    expect(onBannersChange).toHaveBeenCalledWith(["https://example.com/b.jpg"]);
    const arg = onBannersChange.mock.calls[0][0] as string[];
    expect(arg).not.toContain("");
    expect(arg.filter((x) => !x).length).toBe(0);
  });

  it("the Replace overlay is revealed on keyboard focus, not just hover (A11Y-14)", () => {
    const { container } = render(
      <BannerImageUploader
        bannerImages={["https://example.com/a.jpg"]}
        onBannersChange={jest.fn()}
        businessId={1}
      />,
    );
    const overlay = container.querySelector('[class*="group-hover:opacity-100"]');
    expect(overlay).not.toBeNull();
    expect(overlay!.className).toContain("group-focus-within:opacity-100");
  });
});
