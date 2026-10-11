import { isBusinessStepComplete, isValidEmail } from "../_stepValidation";

describe("registration business-step validation", () => {
  const complete = {
    name: "Café Rio",
    owner_name: "Mara",
    email: "owner@example.com",
    address: { country: "AR" },
  };

  it("accepts a fully-filled business step including country", () => {
    expect(isBusinessStepComplete(complete)).toBe(true);
  });

  it("rejects a missing country (would silently default to USD/UTC)", () => {
    expect(isBusinessStepComplete({ ...complete, address: { country: "" } })).toBe(false);
    expect(isBusinessStepComplete({ ...complete, address: { country: "   " } })).toBe(false);
    expect(isBusinessStepComplete({ ...complete, address: undefined })).toBe(false);
  });

  it("still rejects the pre-existing required fields", () => {
    expect(isBusinessStepComplete({ ...complete, name: " " })).toBe(false);
    expect(isBusinessStepComplete({ ...complete, owner_name: "" })).toBe(false);
    expect(isBusinessStepComplete({ ...complete, email: "not-an-email" })).toBe(false);
  });

  it("validates email format", () => {
    expect(isValidEmail("a@b.co")).toBe(true);
    expect(isValidEmail("nope")).toBe(false);
  });
});
