/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));
jest.mock("@/api/business", () => ({
  checkCustomURLAvailability: jest.fn().mockResolvedValue({ available: true }),
}));

import CustomURLInput, {
  isReservedPublicSlug,
} from "@/components/business/CustomURLInput";
import { checkCustomURLAvailability } from "@/api/business";

describe("isReservedPublicSlug", () => {
  it.each(["admin", "api", "demo", "b", "internal", "demo-lounge", "admin-1", "b-x", "API"])(
    "rejects reserved prefix %s",
    (slug) => {
      expect(isReservedPublicSlug(slug)).toBe(true);
    },
  );

  it.each(["payverge-core-demo-kitchen", "my-restaurant", "aurora", "demoed", "cab"])(
    "allows non-reserved slug %s",
    (slug) => {
      expect(isReservedPublicSlug(slug)).toBe(false);
    },
  );
});

describe("CustomURLInput live-page zone", () => {
  it("shows an Open link for a saved slug without waiting on the availability check", () => {
    render(
      <CustomURLInput value="my-cafe" onChange={() => {}} businessId={1} savedUrl="my-cafe" />,
    );
    const link = screen.getByRole("link", { name: /open live page/i });
    expect(link).toHaveAttribute("href", "https://payverge.io/b/my-cafe");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("builds the live link and URL prefix from the runtime PUBLIC_URL", () => {
    const w = window as unknown as { __PAYVERGE_ENV__?: Record<string, string> };
    const previous = w.__PAYVERGE_ENV__;
    w.__PAYVERGE_ENV__ = { PUBLIC_URL: "https://pos.example.test" };
    try {
      render(
        <CustomURLInput value="my-cafe" onChange={() => {}} businessId={1} savedUrl="my-cafe" />,
      );
      expect(screen.getByRole("link", { name: /open live page/i })).toHaveAttribute(
        "href",
        "https://pos.example.test/b/my-cafe",
      );
      expect(screen.getByText("pos.example.test/b/")).toBeInTheDocument();
      expect(document.body.textContent).not.toContain("payverge.io");
    } finally {
      w.__PAYVERGE_ENV__ = previous;
    }
  });

  it("does not render the live-page zone when no slug is saved yet", () => {
    render(<CustomURLInput value="" onChange={() => {}} businessId={1} savedUrl="" />);
    expect(screen.queryByRole("link", { name: /open live page/i })).not.toBeInTheDocument();
  });

  it("shows 'current URL' state when the typed slug equals the saved slug", async () => {
    (checkCustomURLAvailability as jest.Mock).mockClear();
    render(
      <CustomURLInput
        value="my-lounge"
        onChange={() => {}}
        businessId={9}
        savedUrl="my-lounge"
      />,
    );
    expect(await screen.findByText(/tu URL actual|current URL/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/disponible|is available|tomada|taken/i),
    ).toBeNull();
    // The availability API must NOT be called for the business's own slug.
    await waitFor(() => {
      expect(checkCustomURLAvailability).not.toHaveBeenCalled();
    });
  });
});

describe("CustomURLInput reserved prefixes (Finding 35)", () => {
  beforeEach(() => {
    (checkCustomURLAvailability as jest.Mock).mockClear();
  });

  // "b" alone is shorter than the 2-char minimum, so the reserved list is
  // exercised via "b-x" (and equals-checks for the longer tokens).
  it.each(["demo", "admin", "api", "b-x", "internal", "demo-admin-1-ai-pro"])(
    "rejects reserved candidate %s without calling the API",
    async (slug) => {
      render(
        <CustomURLInput value={slug} onChange={() => {}} businessId={1} savedUrl="my-cafe" />,
      );

      expect(
        await screen.findByText(/reserved|not available|no está disponible|reservad/i),
      ).toBeInTheDocument();

      await waitFor(() => {
        expect(checkCustomURLAvailability).not.toHaveBeenCalled();
      });
    },
  );

  it("still checks availability for a normal candidate slug", async () => {
    (checkCustomURLAvailability as jest.Mock).mockResolvedValue({ available: true });
    render(
      <CustomURLInput value="aurora-cafe" onChange={() => {}} businessId={1} savedUrl="my-cafe" />,
    );

    await waitFor(() => {
      expect(checkCustomURLAvailability).toHaveBeenCalledWith("aurora-cafe", 1);
    });
    expect(await screen.findByText(/available|disponible/i)).toBeInTheDocument();
  });
});
