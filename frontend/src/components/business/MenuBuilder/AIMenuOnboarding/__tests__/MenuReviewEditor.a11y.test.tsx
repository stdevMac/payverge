/** @jest-environment jsdom */

import React from "react";
import { render, screen } from "@testing-library/react";
import { computeAccessibleName } from "dom-accessibility-api";
import type { ExtractedMenu } from "@/api/business";
import { getTranslation } from "@/i18n/getTranslation";
import type { OperatorLocale } from "@/i18n/localeRegistry";

let mockLocale: OperatorLocale = "en";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const { getTranslation: realGetTranslation } = jest.requireActual(
    "@/i18n/getTranslation",
  ) as typeof import("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: mockLocale, setLocale: jest.fn() }),
    getTranslation: (
      key: string,
      locale?: OperatorLocale,
      params?: Record<string, string | number>,
    ) => realGetTranslation(key, locale ?? mockLocale, params),
  };
});

jest.mock("framer-motion", () => {
  const MotionDiv = ({
    children,
    className,
  }: React.PropsWithChildren<{ className?: string }>) => (
    <div className={className}>{children}</div>
  );
  return {
    motion: { div: MotionDiv },
    AnimatePresence: ({ children }: React.PropsWithChildren) => <>{children}</>,
  };
});

jest.mock("@/api/business", () => ({
  importExtractedMenu: jest.fn(),
  regenerateMenuItemImage: jest.fn(),
}));

jest.mock("@nextui-org/react", () => {
  const pass = ({
    children,
    ...props
  }: React.PropsWithChildren<Record<string, unknown>>) => (
    <div {...props}>{children}</div>
  );

  return {
    Button: ({
      children,
      "aria-label": ariaLabel,
      onPress,
      isDisabled,
      startContent,
    }: React.PropsWithChildren<{
      "aria-label"?: string;
      onPress?: () => void;
      isDisabled?: boolean;
      startContent?: React.ReactNode;
    }>) => (
      <button
        type="button"
        aria-label={ariaLabel}
        disabled={isDisabled}
        onClick={onPress}
      >
        {startContent}
        {children}
      </button>
    ),
    Card: pass,
    CardBody: pass,
    Input: ({
      "aria-label": ariaLabel,
      label,
      value,
      onChange,
      onValueChange,
      placeholder,
      type,
    }: {
      "aria-label"?: string;
      label?: React.ReactNode;
      value?: string;
      onChange?: React.ChangeEventHandler<HTMLInputElement>;
      onValueChange?: (value: string) => void;
      placeholder?: string;
      type?: string;
    }) => (
      <input
        // NextUI Input writes aria-label=" " when neither label nor aria-label is set.
        aria-label={
          ariaLabel || (typeof label === "string" ? label : " ")
        }
        value={value ?? ""}
        placeholder={placeholder}
        type={type ?? "text"}
        onChange={(e) => {
          onChange?.(e);
          onValueChange?.(e.target.value);
        }}
      />
    ),
    Textarea: ({
      "aria-label": ariaLabel,
      label,
      value,
      onChange,
      placeholder,
    }: {
      "aria-label"?: string;
      label?: React.ReactNode;
      value?: string;
      onChange?: React.ChangeEventHandler<HTMLTextAreaElement>;
      placeholder?: string;
    }) => (
      <textarea
        aria-label={
          ariaLabel || (typeof label === "string" ? label : " ")
        }
        value={value ?? ""}
        placeholder={placeholder}
        onChange={onChange}
      />
    ),
    Chip: ({ children }: React.PropsWithChildren) => <span>{children}</span>,
    Accordion: ({ children }: React.PropsWithChildren) => <div>{children}</div>,
    AccordionItem: ({
      title,
      children,
      "aria-label": ariaLabel,
    }: React.PropsWithChildren<{
      title?: React.ReactNode;
      "aria-label"?: string;
    }>) => (
      <section aria-label={ariaLabel}>
        <div>{title}</div>
        {children}
      </section>
    ),
    Image: ({ alt }: { alt?: string }) => (
      <span role="img" aria-label={alt} />
    ),
    Tooltip: ({ children }: React.PropsWithChildren) => <>{children}</>,
    Modal: () => null,
    ModalContent: pass,
    ModalHeader: pass,
    ModalBody: pass,
    ModalFooter: pass,
    useDisclosure: () => ({
      isOpen: false,
      onOpen: jest.fn(),
      onClose: jest.fn(),
    }),
  };
});

import MenuReviewEditor from "../MenuReviewEditor";

const extractedMenu: ExtractedMenu = {
  restaurant_name: "La Parilla",
  currency: "ARS",
  categories: [
    {
      name: "Entradas",
      items: [
        {
          name: "Milanesa",
          price: "12",
          description: "Breaded cutlet",
          category: "Entradas",
          allergens: ["dairy", "eggs", "so2"],
          add_ons: [],
        },
        {
          name: "Empanada",
          price: "4",
          description: "Beef pastry",
          category: "Entradas",
          allergens: ["fish", "crustaceans"],
          add_ons: [],
        },
      ],
    },
  ],
};

function renderEditor(locale: OperatorLocale = "en") {
  mockLocale = locale;
  return render(
    <MenuReviewEditor
      businessId={7}
      extractedMenu={extractedMenu}
      onImport={jest.fn()}
      onCancel={jest.fn()}
    />,
  );
}

function namesOf(role: "textbox" | "button"): string[] {
  return screen.getAllByRole(role).map((el) => computeAccessibleName(el));
}

describe("MenuReviewEditor accessible names (#575)", () => {
  it("gives every name and description field a purpose + identity name", () => {
    renderEditor("en");

    expect(
      screen.getByRole("textbox", { name: /category name.*entradas/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /item name.*milanesa/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /item name.*empanada/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /description.*milanesa/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /description.*empanada/i }),
    ).toBeInTheDocument();

    for (const name of namesOf("textbox")) {
      expect(name.trim()).not.toBe("");
    }
  });

  it("includes each item name on its image-generation button", () => {
    renderEditor("en");

    const milanesa = screen.getByRole("button", {
      name: /generate ai image.*milanesa/i,
    });
    const empanada = screen.getByRole("button", {
      name: /generate ai image.*empanada/i,
    });
    expect(milanesa).toBeInTheDocument();
    expect(empanada).toBeInTheDocument();
    expect(computeAccessibleName(milanesa)).not.toBe(
      computeAccessibleName(empanada),
    );
  });

  it("does not leave Spanish review fields named only as a space", () => {
    renderEditor("es-AR");

    for (const name of namesOf("textbox")) {
      expect(name.trim().length).toBeGreaterThan(1);
    }
    expect(
      screen.getByRole("button", { name: /imagen.*milanesa/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /imagen.*empanada/i }),
    ).toBeInTheDocument();
  });
});

describe("MenuReviewEditor allergen chips (#574)", () => {
  it.each([
    ["en", ["Dairy", "Eggs", "Sulphur Dioxide", "Fish", "Crustaceans"]],
    ["es", ["Lácteos", "Huevos", "Dióxido de azufre", "Pescado", "Crustáceos"]],
    [
      "es-AR",
      ["Lácteos", "Huevos", "Dióxido de azufre", "Pescado", "Crustáceos"],
    ],
  ] as const)(
    "shows restaurant-ready allergen labels in %s, never raw tokens",
    (locale, labels) => {
      renderEditor(locale);

      for (const label of labels) {
        expect(screen.getByText(label)).toBeInTheDocument();
      }
      for (const token of ["dairy", "eggs", "fish", "crustaceans", "so2"]) {
        expect(screen.queryByText(token)).not.toBeInTheDocument();
      }
    },
  );

  it("resolves the same tokens through operator i18n in en/es/es-AR", () => {
    const expected = {
      en: {
        dairy: "Dairy",
        eggs: "Eggs",
        fish: "Fish",
        crustaceans: "Crustaceans",
        so2: "Sulphur Dioxide",
      },
      es: {
        dairy: "Lácteos",
        eggs: "Huevos",
        fish: "Pescado",
        crustaceans: "Crustáceos",
        so2: "Dióxido de azufre",
      },
      "es-AR": {
        dairy: "Lácteos",
        eggs: "Huevos",
        fish: "Pescado",
        crustaceans: "Crustáceos",
        so2: "Dióxido de azufre",
      },
    } as const;

    for (const locale of ["en", "es", "es-AR"] as const) {
      for (const token of ["dairy", "eggs", "fish", "crustaceans", "so2"] as const) {
        const label = getTranslation(
          `businessDashboard.dashboard.menuBuilder.items.allergenNames.${token}`,
          locale,
        );
        expect(label).toBe(expected[locale][token]);
        expect(label).not.toBe(token);
        expect(label).not.toMatch(/^so2$/i);
      }
    }
  });
});
