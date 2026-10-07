/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import {
  parseInstanceInfo,
  type InstanceInfo,
} from "@/lib/instance/instanceInfo";
import { instanceOffFeatureForTab } from "@/lib/instance/featureGates";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";
import FeatureUnavailable from "@/components/instance/FeatureUnavailable";
import MenuFrontDoor from "@/components/business/onboarding/MenuFrontDoor";
import PluginConfigFactory from "@/components/business/plugins/PluginConfigFactory";
import { PLUGIN } from "@/constants/plugins";
import { axiosInstance } from "@/api/tools/instance";
import WhatsAppChannelStatus from "@/components/business/AiWaiter/WhatsAppChannelStatus";
import { TopMenuShell } from "@/components/ui/top-menu/TopMenuShell";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";

function instance(features: Record<string, boolean>): InstanceInfo {
  const info = parseInstanceInfo({
    registration_mode: "invite",
    features,
  });
  if (!info) throw new Error("fixture must parse");
  return info;
}

afterEach(() => resetInstanceCacheForTests());

describe("instanceOffFeatureForTab", () => {
  it("gates AI and fiscal rails only when the server confirmed off", () => {
    const off = instance({ ai: false, fiscal_ar: false });
    expect(instanceOffFeatureForTab("ai-waiter", off)).toBe("ai");
    expect(instanceOffFeatureForTab("director-console", off)).toBe("ai");
    expect(instanceOffFeatureForTab("fiscal", off)).toBe("fiscal_ar");
    expect(instanceOffFeatureForTab("menu", off)).toBeNull();
    expect(instanceOffFeatureForTab("ai-waiter", null)).toBeNull();
    expect(
      instanceOffFeatureForTab("ai-waiter", instance({ ai: true })),
    ).toBeNull();
  });
});

describe("FeatureUnavailable", () => {
  it("renders one panel with a docs link for the missing integration", () => {
    render(<FeatureUnavailable feature="ai" />);
    const panel = screen.getByTestId("feature-unavailable-ai");
    expect(panel).toBeInTheDocument();
    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toContain("docs/self-hosting/ai.md");
  });
});

describe("MenuFrontDoor without AI", () => {
  it("offers only the manual tile", () => {
    setInstanceForTests(instance({ ai: false }));
    render(
      <MenuFrontDoor
        tString={(k) => k}
        onAddCategoryOpen={jest.fn()}
      />,
    );
    const headings = screen
      .getAllByRole("heading", { level: 4 })
      .map((h) => h.textContent);
    expect(headings).toEqual(["ai.emptyState.manual.title"]);
  });
});

describe("PluginConfigFactory gates", () => {
  const props = {
    config: {},
    onConfigChange: jest.fn(),
    onSave: jest.fn(),
    onCancel: jest.fn(),
  };

  it.each([PLUGIN.usdcPayment, PLUGIN.crossChainPayment])(
    "explains instead of configuring %s when crypto is off",
    (name) => {
      setInstanceForTests(instance({ crypto: false }));
      render(
        <PluginConfigFactory
          {...props}
          plugin={{ name } as React.ComponentProps<typeof PluginConfigFactory>["plugin"]}
        />,
      );
      expect(screen.getByTestId("feature-unavailable-crypto")).toBeInTheDocument();
    },
  );

  it("explains instead of configuring Telegram when telegram is off", () => {
    setInstanceForTests(instance({ telegram: false }));
    render(
      <PluginConfigFactory
        {...props}
        plugin={{ name: PLUGIN.telegram } as React.ComponentProps<typeof PluginConfigFactory>["plugin"]}
      />,
    );
    expect(screen.getByTestId("feature-unavailable-telegram")).toBeInTheDocument();
  });
});

describe("email-off gates (F2)", () => {
  it.each([PLUGIN.dailyEmailReport, PLUGIN.weeklyEmailReport])(
    "explains instead of configuring %s when email is off",
    (name) => {
      setInstanceForTests(instance({ email: false }));
      render(
        <PluginConfigFactory
          config={{}}
          onConfigChange={jest.fn()}
          onSave={jest.fn()}
          onCancel={jest.fn()}
          plugin={{ name } as React.ComponentProps<typeof PluginConfigFactory>["plugin"]}
        />,
      );
      expect(screen.getByTestId("feature-unavailable-email")).toBeInTheDocument();
    },
  );
});

describe("WhatsApp channel card (F2)", () => {
  it("renders nothing and makes no status call when whatsapp is off", () => {
    const get = jest.spyOn(axiosInstance, "get");
    setInstanceForTests(instance({ whatsapp: false }));
    const { container } = render(<WhatsAppChannelStatus businessId={1} locale="en" />);
    expect(container).toBeEmptyDOMElement();
    expect(get).not.toHaveBeenCalled();
    get.mockRestore();
  });
});

describe("TopMenuShell branding (F2)", () => {
  it("shows the operator logo and product name from /instance", () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Trattoria OS",
        logo_url: "https://cdn.trattoria.example/logo.svg",
        registration_mode: "invite",
        features: {},
      }) as InstanceInfo,
    );
    render(
      <SimpleTranslationProvider initialLocale="en">
        <TopMenuShell pathname="/login" scrolled={false} labels={{} as React.ComponentProps<typeof TopMenuShell>["labels"]} showPublicNav={false} />
      </SimpleTranslationProvider>,
    );
    const logo = screen.getByTestId("instance-logo");
    expect(logo.getAttribute("src")).toBe("https://cdn.trattoria.example/logo.svg");
    expect(logo.getAttribute("alt")).toBe("Trattoria OS");
  });
});
