/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { GenericPluginConfig } from "./PluginConfigFactory";

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...(props as any)} />;
  },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es" }),
  getTranslation: (key: string) => {
    if (key.endsWith(".enable")) return "Activar";
    if (key.endsWith(".save")) return "Guardar";
    if (key.endsWith(".cancel")) return "Cancelar";
    if (key.endsWith(".noConfigRequired")) return "Sin config";
    return key;
  },
}));

jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    utils: {
      getTranslatedContent: (plugin: any, field: string, lang: string) => {
        if (lang === "es" && field === "display_name") return "Informes diarios";
        if (lang === "es" && field === "description")
          return "Resumen diario por correo";
        if (field === "display_name") return plugin.display_name;
        if (field === "description") return plugin.description;
        return "";
      },
    },
  },
}));

const plugin = {
  id: 9,
  name: "daily_email_report",
  display_name: "Daily Email Report",
  description: "Daily summary email",
  image: "/images/plugins/daily.png",
  category: "reporting",
  version: "1.0.0",
  features: "[]",
  is_enabled: false,
  config: "{}",
};

describe("GenericPluginConfig translation + CTA (L6-31)", () => {
  it("renders translated display_name and description, not registry English", () => {
    render(
      <GenericPluginConfig
        plugin={plugin}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />,
    );
    expect(screen.getByText("Informes diarios")).toBeInTheDocument();
    expect(screen.getByText("Resumen diario por correo")).toBeInTheDocument();
    expect(screen.queryByText("Daily Email Report")).not.toBeInTheDocument();
  });

  it("uses Enable CTA when plugin is not enabled", () => {
    render(
      <GenericPluginConfig
        plugin={plugin}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Activar" })).toBeInTheDocument();
  });

  it("uses Save CTA when plugin is already enabled", () => {
    render(
      <GenericPluginConfig
        plugin={{ ...plugin, is_enabled: true }}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Guardar" })).toBeInTheDocument();
  });
});
