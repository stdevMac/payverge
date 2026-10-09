/** @jest-environment jsdom */
import React from "react";
import { render, fireEvent, waitFor } from "@testing-library/react";

const mockUploadFile = jest.fn();
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();

jest.mock("@/api/uploads", () => ({
  uploadFile: (...a: unknown[]) => mockUploadFile(...a),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: mockShowSuccess, showError: mockShowError }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

import GalleryImageUploader from "../GalleryImageUploader";

function imageFile(name: string) {
  return new File(["x"], name, { type: "image/png" });
}

beforeEach(() => jest.clearAllMocks());

it("keeps already-uploaded images when a later file in the batch fails (F12)", async () => {
  // First upload succeeds; second rejects.
  mockUploadFile
    .mockResolvedValueOnce({ location: "https://cdn/one.png" })
    .mockRejectedValueOnce(new Error("network blip"));

  const onImagesChange = jest.fn();

  const { container } = render(
    <GalleryImageUploader
      businessId={1}
      images={[]}
      onImagesChange={onImagesChange}
    />,
  );

  const input = container.querySelector(
    'input[type="file"]',
  ) as HTMLInputElement;

  fireEvent.change(input, {
    target: { files: [imageFile("one.png"), imageFile("two.png")] },
  });

  // The error for the failed file must surface.
  await waitFor(() => expect(mockShowError).toHaveBeenCalled());

  // The successfully-uploaded first image must NOT be discarded.
  await waitFor(() => expect(onImagesChange).toHaveBeenCalled());
  const applied = onImagesChange.mock.calls.at(-1)![0];
  expect(applied).toHaveLength(1);
  expect(applied[0].image_url).toBe("https://cdn/one.png");
});
