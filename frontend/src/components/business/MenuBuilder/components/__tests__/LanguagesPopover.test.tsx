/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import { LanguagesPopover } from "../LanguagesPopover";
import type {
  BusinessLanguage,
  SupportedLanguage,
} from "../../../../../api/currency";

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
}));

jest.mock("@/components/business/modals/ConfirmationModal", () => ({
  __esModule: true,
  default: ({
    isOpen,
    title,
    description,
    onConfirm,
  }: {
    isOpen: boolean;
    title: string;
    description: string;
    onConfirm: () => void;
  }) =>
    isOpen ? (
      <div>
        <p>{title}</p>
        <p>{description}</p>
        <button type="button" onClick={() => void onConfirm()}>
          confirm-relabel
        </button>
      </div>
    ) : null,
}));

jest.mock("@nextui-org/react", () => {
  const React = jest.requireActual("react");
  return {
    Button: ({
      children,
      onPress,
      isDisabled,
    }: {
      children: React.ReactNode;
      onPress?: () => void;
      isDisabled?: boolean;
    }) => (
      <button type="button" disabled={isDisabled} onClick={onPress}>
        {children}
      </button>
    ),
    Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    Popover: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    PopoverTrigger: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    PopoverContent: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    Select: ({
      children,
      onSelectionChange,
      "aria-label": ariaLabel,
      selectionMode,
    }: {
      children: React.ReactNode;
      onSelectionChange?: (keys: Set<string> | string[]) => void;
      "aria-label"?: string;
      selectionMode?: string;
    }) => (
      <div data-testid={ariaLabel} role="group" aria-label={ariaLabel}>
        {React.Children.map(children, (child: React.ReactNode) => {
          if (!React.isValidElement(child)) return child;
          const element = child as React.ReactElement<{ value?: string; onSelect?: () => void }>;
          const value = element.props.value;
          return React.cloneElement(
            element,
            {
              onSelect: () => {
                if (!value) return;
                if (selectionMode === "multiple") {
                  onSelectionChange?.([value]);
                  return;
                }
                onSelectionChange?.(new Set([value]));
              },
            },
          );
        })}
      </div>
    ),
    SelectItem: ({
      children,
      onSelect,
    }: {
      children: React.ReactNode;
      onSelect?: () => void;
    }) => (
      <button type="button" onClick={onSelect}>
        {children}
      </button>
    ),
  };
});

const supportedLanguages: SupportedLanguage[] = [
  {
    id: 1,
    code: "en",
    name: "English",
    native_name: "English",
    is_active: true,
    created_at: "",
    updated_at: "",
  },
  {
    id: 2,
    code: "de",
    name: "German",
    native_name: "Deutsch",
    is_active: true,
    created_at: "",
    updated_at: "",
  },
  {
    id: 3,
    code: "es",
    name: "Spanish",
    native_name: "Español",
    is_active: true,
    created_at: "",
    updated_at: "",
  },
];

const businessLanguages: BusinessLanguage[] = [
  {
    id: 1,
    business_id: 7,
    language_code: "en",
    is_default: true,
    display_order: 0,
    created_at: "",
    updated_at: "",
  },
];

function renderPopover(
  overrides: Partial<React.ComponentProps<typeof LanguagesPopover>> = {},
) {
  const setSelectedLanguages = jest.fn();
  const setDefaultLanguage = jest.fn();
  const handleLanguageUpdate = jest.fn();

  const view = render(
    <LanguagesPopover
      tString={(key) => key}
      isLocked
      supportedLanguages={supportedLanguages}
      businessLanguages={businessLanguages}
      selectedLanguages={["en"]}
      setSelectedLanguages={setSelectedLanguages}
      defaultLanguage="en"
      setDefaultLanguage={setDefaultLanguage}
      isLanguageLoading={false}
      hasLanguageChanges={false}
      handleLanguageUpdate={handleLanguageUpdate}
      {...overrides}
    />,
  );

  return {
    ...view,
    setSelectedLanguages,
    setDefaultLanguage,
    handleLanguageUpdate,
  };
}

describe("LanguagesPopover", () => {
  it("keeps locked-tier selections draft-local until save+confirm", () => {
    const { setSelectedLanguages, setDefaultLanguage, handleLanguageUpdate } =
      renderPopover();

    fireEvent.click(screen.getByRole("button", { name: "Deutsch" }));

    expect(setSelectedLanguages).not.toHaveBeenCalled();
    expect(setDefaultLanguage).not.toHaveBeenCalled();
    expect(handleLanguageUpdate).not.toHaveBeenCalled();

    fireEvent.click(
      screen.getByRole("button", { name: "languages.saveLanguages" }),
    );
    expect(handleLanguageUpdate).not.toHaveBeenCalled();
    expect(
      screen.getByText("languages.relabelConfirmTitle"),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "confirm-relabel" }));
    expect(handleLanguageUpdate).toHaveBeenCalledWith(["de"], "de");
    expect(setSelectedLanguages).not.toHaveBeenCalled();
    expect(setDefaultLanguage).not.toHaveBeenCalled();
  });

  it("prompts for a new default instead of promoting set order", () => {
    const { handleLanguageUpdate } = renderPopover({
      isLocked: false,
      selectedLanguages: ["en", "de"],
      defaultLanguage: "en",
      businessLanguages: [
        businessLanguages[0],
        {
          ...businessLanguages[0],
          id: 2,
          language_code: "de",
          is_default: false,
          display_order: 1,
        },
      ],
    });

    const supported = screen.getByTestId("languages.supportedLanguages");
    fireEvent.click(
      Array.from(supported.querySelectorAll("button")).find(
        (button) => button.textContent === "Deutsch",
      )!,
    );

    expect(screen.getByText("languages.chooseNewDefault")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "languages.saveLanguages" }),
    );
    expect(handleLanguageUpdate).not.toHaveBeenCalled();
  });
});
