/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import DeliveryTrackingPage from "./page";
import { guestDeliveryApi } from "@/api/delivery";
import type { PublicDeliveryTrackingDto } from "@/api/delivery";

jest.mock("next/navigation", () => ({
  useParams: () => ({ deliveryNumber: "DEL-X" }),
  useSearchParams: () => ({ get: () => null }),
}));

jest.mock("@/api/delivery", () => ({
  guestDeliveryApi: {
    track: jest.fn(),
  },
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  GuestTranslationProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
    setLanguage: jest.fn(),
    availableLanguages: {},
    setBusinessId: jest.fn(),
  }),
}));

jest.mock("@nextui-org/react", () => ({
  Card: ({ children, className }: any) => (
    <div className={className}>{children}</div>
  ),
  CardBody: ({ children }: any) => <div>{children}</div>,
  Spinner: () => <div data-testid="loading-spinner" />,
  Button: ({ children, onPress, ...rest }: any) => (
    <button type="button" onClick={onPress} {...rest}>
      {children}
    </button>
  ),
}));

const mockTrack = guestDeliveryApi.track as jest.Mock;

function buildTracking(
  overrides: Partial<PublicDeliveryTrackingDto> = {},
): PublicDeliveryTrackingDto {
  return {
    delivery_number: "DEL-X",
    status: "preparing",
    business_name: "Test Bistro",
    ...overrides,
  };
}

describe("delivery stage label contrast", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("uses AA text colors on the current and completed stage labels", async () => {
    mockTrack.mockResolvedValue(buildTracking({ status: "preparing" }));
    render(<DeliveryTrackingPage />);

    const current = await screen.findByText("deliveryTracking.stages.preparing");
    expect(current.className).toContain("text-amber-800");
    expect(current.className).not.toContain("text-amber-500");

    const completed = screen.getByText("deliveryTracking.stages.received");
    expect(completed.className).toContain("text-emerald-800");
    expect(completed.className).not.toContain("text-emerald-600");
  });
});
