/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import ContactEditor from "@/components/business/ContactEditor";

// A passthrough t that returns the key's leaf so labels are findable by text.
const t = (key: string) => {
  const leaf = key.split(".").pop() || key;
  const map: Record<string, string> = {
    phoneNumber: "Phone Number",
    websiteUrl: "Website URL",
    streetAddress: "Street Address",
    city: "City",
    state: "State/Province",
    postalCode: "Postal Code",
    country: "Country",
    instagram: "Instagram",
    facebook: "Facebook",
  };
  return map[leaf] || leaf;
};

const baseProps = {
  phone: "",
  website: "",
  address: { street: "", city: "", state: "", postal_code: "", country: "" },
  socialMedia: {},
  onPhoneChange: jest.fn(),
  onWebsiteChange: jest.fn(),
  onAddressChange: jest.fn(),
  onSocialChange: jest.fn(),
  t,
};

describe("ContactEditor", () => {
  beforeEach(() => jest.clearAllMocks());

  it("fires onPhoneChange / onWebsiteChange with the typed value", () => {
    render(<ContactEditor {...baseProps} />);
    fireEvent.change(screen.getByRole("textbox", { name: /phone/i }), { target: { value: "+1555" } });
    expect(baseProps.onPhoneChange).toHaveBeenCalledWith("+1555");
    fireEvent.change(screen.getByRole("textbox", { name: /website/i }), { target: { value: "https://x.test" } });
    expect(baseProps.onWebsiteChange).toHaveBeenCalledWith("https://x.test");
  });

  it("fires onAddressChange with the field key + value", () => {
    render(<ContactEditor {...baseProps} />);
    fireEvent.change(screen.getByRole("textbox", { name: /street/i }), { target: { value: "1 A St" } });
    expect(baseProps.onAddressChange).toHaveBeenCalledWith("street", "1 A St");
  });

  it("fires onSocialChange with the platform + value", () => {
    render(<ContactEditor {...baseProps} />);
    fireEvent.change(screen.getByRole("textbox", { name: /instagram/i }), { target: { value: "mycafe" } });
    expect(baseProps.onSocialChange).toHaveBeenCalledWith("instagram", "mycafe");
  });

  it("normalizes a full URL paste to a bare handle on input", () => {
    render(<ContactEditor {...baseProps} />);
    fireEvent.change(screen.getByRole("textbox", { name: /facebook/i }), {
      target: { value: "https://www.facebook.com/payverge/" },
    });
    expect(baseProps.onSocialChange).toHaveBeenCalledWith("facebook", "payverge");
  });

  it("normalizes @handles and host/path forms on input", () => {
    render(<ContactEditor {...baseProps} />);
    fireEvent.change(screen.getByRole("textbox", { name: /instagram/i }), {
      target: { value: "@mycafe" },
    });
    expect(baseProps.onSocialChange).toHaveBeenCalledWith("instagram", "mycafe");
  });

  it("renders a dirty full-URL stored value as the bare handle", () => {
    render(
      <ContactEditor
        {...baseProps}
        socialMedia={{ facebook: "https://www.facebook.com/payverge/" }}
      />,
    );
    expect(screen.getByRole("textbox", { name: /facebook/i })).toHaveValue("payverge");
  });
});
