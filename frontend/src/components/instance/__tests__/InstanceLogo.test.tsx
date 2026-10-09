/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";
import InstanceLogo from "@/components/instance/InstanceLogo";

afterEach(() => resetInstanceCacheForTests());

describe("InstanceLogo", () => {
  it("renders the operator logo with the product name as alt", () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Casa Pepe",
        logo_url: "https://cdn.example.com/logo.png",
        registration_mode: "invite",
        features: {},
      }),
    );
    render(<InstanceLogo fallback={() => <span>fallback</span>} />);
    const img = screen.getByTestId("instance-logo");
    expect(img).toHaveAttribute("src", "https://cdn.example.com/logo.png");
    expect(img).toHaveAttribute("alt", "Casa Pepe");
    expect(screen.queryByText("fallback")).toBeNull();
  });

  it("falls back to the bundled logo, still named for the instance", () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Casa Pepe",
        registration_mode: "invite",
        features: {},
      }),
    );
    render(
      <InstanceLogo fallback={(alt) => <span>{`bundled:${alt}`}</span>} />,
    );
    expect(screen.getByText("bundled:Casa Pepe")).toBeInTheDocument();
    expect(screen.queryByTestId("instance-logo")).toBeNull();
  });
});
