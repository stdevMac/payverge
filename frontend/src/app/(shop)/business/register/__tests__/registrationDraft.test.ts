import {
  buildRegistrationDraft,
  splitRegistrationDraft,
} from "../_registrationDraft";

describe("registration draft persistence", () => {
  const formData = {
    name: "La Parrilla",
    owner_name: "Ana",
    email: "ana@example.com",
    business_type: "cafe",
    address: {
      street: "",
      city: "Buenos Aires",
      state: "",
      postal_code: "",
      country: "AR",
    },
  };

  it("snapshots the full funnel state, not just name/owner/email", () => {
    expect(buildRegistrationDraft({ formData })).toEqual(formData);
  });

  it("restores only the known form fields from a stored draft", () => {
    const formFields = splitRegistrationDraft({
      name: "La Parrilla",
      owner_name: "Ana",
      email: "ana@example.com",
      business_type: "cafe",
      address: { country: "AR", city: "Buenos Aires" },
      plan_code: "ai_pro",
      billing_cycle: "annual",
    });
    expect(formFields).toEqual({
      name: "La Parrilla",
      owner_name: "Ana",
      email: "ana@example.com",
      business_type: "cafe",
      address: { country: "AR", city: "Buenos Aires" },
    });
  });

  it("tolerates the 3-field draft shape", () => {
    expect(
      splitRegistrationDraft({
        name: "Old Draft",
        owner_name: "Ana",
        email: "ana@example.com",
      }),
    ).toEqual({
      name: "Old Draft",
      owner_name: "Ana",
      email: "ana@example.com",
    });
  });

  it("rejects non-object garbage", () => {
    expect(splitRegistrationDraft(null)).toEqual({});
    expect(splitRegistrationDraft("corrupt")).toEqual({});
    expect(splitRegistrationDraft([1, 2])).toEqual({});
  });
});
