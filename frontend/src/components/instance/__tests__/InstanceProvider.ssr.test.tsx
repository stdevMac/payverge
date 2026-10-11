/** @jest-environment node */
import React from "react";
import { renderToString } from "react-dom/server";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import { resetInstanceCacheForTests, useInstance } from "@/hooks/useInstance";
import InstanceProvider from "@/components/instance/InstanceProvider";

afterEach(() => resetInstanceCacheForTests());

function Probe() {
  const { productName, isOff } = useInstance();
  return (
    <p>
      {productName}:{isOff("ai") ? "ai-off" : "ai-unknown-or-on"}
    </p>
  );
}

describe("InstanceProvider on the server render", () => {
  const info = parseInstanceInfo({
    product_name: "Casa Pepe",
    registration_mode: "invite",
    features: { ai: false },
  });

  it("gives the SSR HTML the same instance answer the client seed will", () => {
    const html = renderToString(
      <InstanceProvider value={info}>
        <Probe />
      </InstanceProvider>,
    );
    expect(html).toContain("Casa Pepe");
    expect(html).toContain("ai-off");
  });

  it("without a provider the server render stays 'unknown' (never off)", () => {
    const html = renderToString(<Probe />);
    expect(html).toContain("Payverge");
    expect(html).toContain("ai-unknown-or-on");
  });
});
