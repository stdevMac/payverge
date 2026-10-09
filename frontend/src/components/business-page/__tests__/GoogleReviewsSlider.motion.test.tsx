/** @jest-environment jsdom */
import { act, render } from "@testing-library/react";
import React from "react";
import GoogleReviewsSlider from "../GoogleReviewsSlider";
import { getBusinessGoogleReviews } from "@/api/googleReviews";

let mockCurrentLanguage = "en";

jest.mock("@/api/googleReviews", () => ({
  getBusinessGoogleReviews: jest.fn().mockResolvedValue({
    reviews: Array.from({ length: 5 }, (_, i) => ({
      author_name: `Reviewer ${i}`,
      rating: 5,
      relative_time_description: "1 week ago",
      text: "Great",
    })),
  }),
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    // Deliberately new every render: provider callback identity must not cause
    // an API reload loop when the translated fallback text is unchanged.
    t: (k: string) => `${mockCurrentLanguage}:${k}`,
    currentLanguage: mockCurrentLanguage,
  }),
}));

const getBusinessGoogleReviewsMock = getBusinessGoogleReviews as jest.Mock;

beforeEach(() => {
  jest.useFakeTimers();
  mockCurrentLanguage = "en";
  getBusinessGoogleReviewsMock.mockClear();
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: jest.fn().mockImplementation((q: string) => ({
      matches: q.includes("reduce"),
      media: q,
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })),
  });
});

afterEach(() => {
  jest.useRealTimers();
});

describe("GoogleReviewsSlider reduced motion", () => {
  test("does not schedule auto-slide intervals when prefers-reduced-motion is set", async () => {
    const setIntervalSpy = jest.spyOn(global, "setInterval");
    await act(async () => {
      render(
        <GoogleReviewsSlider
          businessName="x"
          customUrl="x"
          googlePlaceId="abc"
          googleReviewsEnabled
           
          designSettings={{ primaryColor: "#000", secondaryColor: "#111" }}
        />,
      );
    });
    await act(async () => {
      jest.advanceTimersByTime(0);
    });
    expect(setIntervalSpy).not.toHaveBeenCalled();
    setIntervalSpy.mockRestore();
  });

  test("reloads once with the fresh locale without looping on t identity", async () => {
    const props = {
      businessName: "x",
      customUrl: "cafe",
      googlePlaceId: "abc",
      googleReviewsEnabled: true,
      designSettings: { primaryColor: "#000", secondaryColor: "#111" },
    };
    let view: ReturnType<typeof render>;
    await act(async () => {
      view = render(<GoogleReviewsSlider {...props} />);
    });
    expect(getBusinessGoogleReviewsMock).toHaveBeenCalledTimes(1);
    expect(getBusinessGoogleReviewsMock).toHaveBeenLastCalledWith("cafe", "en");

    mockCurrentLanguage = "es";
    await act(async () => {
      view!.rerender(<GoogleReviewsSlider {...props} />);
    });

    expect(getBusinessGoogleReviewsMock).toHaveBeenCalledTimes(2);
    expect(getBusinessGoogleReviewsMock).toHaveBeenLastCalledWith("cafe", "es");
  });
});
