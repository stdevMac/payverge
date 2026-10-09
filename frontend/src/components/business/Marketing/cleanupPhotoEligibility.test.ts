import { isCleanupEligiblePhotoUrl } from "./cleanupPhotoEligibility";

describe("isCleanupEligiblePhotoUrl (L4-20)", () => {
  it("rejects empty and non-https", () => {
    expect(isCleanupEligiblePhotoUrl("")).toBe(false);
    expect(isCleanupEligiblePhotoUrl("http://bucket.example/x.jpg")).toBe(
      false,
    );
  });

  it("rejects Unsplash demo hosts", () => {
    expect(
      isCleanupEligiblePhotoUrl(
        "https://images.unsplash.com/photo-123?w=800",
      ),
    ).toBe(false);
    expect(
      isCleanupEligiblePhotoUrl("https://plus.unsplash.com/premium_photo-1"),
    ).toBe(false);
  });

  it("accepts https gallery-like hosts", () => {
    expect(
      isCleanupEligiblePhotoUrl(
        "https://payverge-public.s3.us-east-1.amazonaws.com/menu/1.jpg",
      ),
    ).toBe(true);
  });
});
