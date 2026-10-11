/** @jest-environment jsdom */
/**
 * ScanClient localization — uses production guest-messages for Spanish chrome
 * and the real QRCodeScanner path for camera denial (GUEST-004).
 *
 * Also covers lookup-error announcement: role=alert, aria-invalid /
 * aria-describedby pairing, and focus return to the manual-code field.
 */
import React from "react";
import fs from "fs";
import path from "path";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import esMessages from "@/i18n/guest-messages/es.json";
import esARMessages from "@/i18n/guest-messages/es-AR.json";
import enMessages from "@/i18n/guest-messages/en.json";
import ScanClient from "./ScanClient";

const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
}));

const mockValidateTableCode = jest.fn();
jest.mock("../../utils/qrValidation", () => ({
  validateTableCode: (...args: unknown[]) => mockValidateTableCode(...args),
}));

function mockNestedT(messages: Record<string, unknown>, key: string): string {
  const parts = key.split(".");
  let cur: unknown = messages;
  for (const part of parts) {
    if (!cur || typeof cur !== "object" || !(part in (cur as object))) {
      return key;
    }
    cur = (cur as Record<string, unknown>)[part];
  }
  return typeof cur === "string" ? cur : key;
}

/** Active production messages for this suite (es by default). */
let mockActiveMessages: Record<string, unknown> = esMessages as Record<
  string,
  unknown
>;

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "es",
    t: (key: string) => mockNestedT(mockActiveMessages, key),
  }),
}));

// Real QRCodeScanner is used for camera-denial. NextUI stubs keep the tree light.
jest.mock("@nextui-org/react", () => {
  const ReactActual = require("react");
  return {
    Card: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    CardBody: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    Spinner: () => <div data-testid="spinner" />,
    Input: ReactActual.forwardRef(function MockScanInput(
      {
        placeholder,
        value,
        onValueChange,
        onKeyDown,
        isDisabled,
        isInvalid,
        id,
        "aria-describedby": ariaDescribedBy,
        "aria-invalid": ariaInvalid,
        "aria-errormessage": ariaErrorMessage,
      }: {
        placeholder?: string;
        value?: string;
        onValueChange?: (v: string) => void;
        onKeyDown?: (e: { key: string }) => void;
        isDisabled?: boolean;
        isInvalid?: boolean;
        id?: string;
        "aria-describedby"?: string;
        "aria-invalid"?: boolean | "true" | "false";
        "aria-errormessage"?: string;
      },
      ref: React.Ref<HTMLInputElement>,
    ) {
      const invalid =
        ariaInvalid === true ||
        ariaInvalid === "true" ||
        (ariaInvalid == null && isInvalid);
      return ReactActual.createElement("input", {
        ref,
        id,
        placeholder,
        value,
        disabled: isDisabled,
        "aria-invalid": invalid ? true : undefined,
        "aria-describedby": ariaDescribedBy,
        "aria-errormessage": ariaErrorMessage,
        onChange: (e: { target: { value: string } }) =>
          onValueChange?.(e.target.value),
        onKeyDown,
      });
    }),
    Button: ({
      children,
      onPress,
      isDisabled,
      isLoading,
      "aria-label": ariaLabel,
    }: {
      children?: React.ReactNode;
      onPress?: () => void;
      isDisabled?: boolean;
      isLoading?: boolean;
      "aria-label"?: string;
    }) =>
      ReactActual.createElement(
        "button",
        {
          type: "button",
          onClick: onPress,
          disabled: isDisabled || isLoading,
          "aria-label": ariaLabel,
        },
        children,
      ),
  };
});

describe("ScanClient localization (production guest-messages)", () => {
  const realGUM = navigator.mediaDevices;

  beforeAll(() => {
    if (!("inert" in HTMLElement.prototype)) {
      Object.defineProperty(HTMLElement.prototype, "inert", {
        configurable: true,
        enumerable: true,
        get() {
          return this.hasAttribute("inert");
        },
        set(value: boolean) {
          if (value) this.setAttribute("inert", "");
          else this.removeAttribute("inert");
        },
      });
    }
  });

  beforeEach(() => {
    jest.clearAllMocks();
    mockActiveMessages = esMessages as Record<string, unknown>;
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: {
        getUserMedia: jest.fn().mockResolvedValue({
          getTracks: () => [{ stop: jest.fn() }],
        }),
      },
    });
    delete (window as unknown as { BarcodeDetector?: unknown }).BarcodeDetector;
    HTMLMediaElement.prototype.play = jest.fn().mockResolvedValue(undefined);
  });

  afterEach(() => {
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: realGUM,
    });
  });

  it("renders Spanish first-frame chrome from production es.json without English titles", () => {
    render(<ScanClient />);
    expect(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.header.title")),
    ).toBeInTheDocument();
    expect(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.actions.scanQr")),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(
        mockNestedT(enMessages as Record<string, unknown>, "scan.header.title"),
      ),
    ).not.toBeInTheDocument();
  });

  it("localizes invalid / not-found table codes from production messages", async () => {
    mockValidateTableCode.mockResolvedValue({
      isValid: false,
      errorCode: "notFound",
    });
    render(<ScanClient />);
    fireEvent.change(
      screen.getByPlaceholderText(
        mockNestedT(mockActiveMessages, "scan.actions.manualPlaceholder"),
      ),
      { target: { value: "ZZZZZZ" } },
    );
    fireEvent.click(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.actions.accessTable")),
    );
    await waitFor(() => {
      expect(
        screen.getByText(mockNestedT(mockActiveMessages, "scan.errors.notFound")),
      ).toBeInTheDocument();
    });
  });

  it("localizes inactive table codes from production messages", async () => {
    mockValidateTableCode.mockResolvedValue({
      isValid: false,
      errorCode: "inactive",
    });
    render(<ScanClient />);
    fireEvent.change(
      screen.getByPlaceholderText(
        mockNestedT(mockActiveMessages, "scan.actions.manualPlaceholder"),
      ),
      { target: { value: "TABLE01" } },
    );
    fireEvent.click(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.actions.accessTable")),
    );
    await waitFor(() => {
      expect(
        screen.getByText(mockNestedT(mockActiveMessages, "scan.errors.inactive")),
      ).toBeInTheDocument();
    });
  });

  it("localizes network / validation failures from production messages", async () => {
    mockValidateTableCode.mockRejectedValue(new Error("network"));
    render(<ScanClient />);
    fireEvent.change(
      screen.getByPlaceholderText(
        mockNestedT(mockActiveMessages, "scan.actions.manualPlaceholder"),
      ),
      { target: { value: "TABLE01" } },
    );
    fireEvent.click(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.actions.accessTable")),
    );
    await waitFor(() => {
      expect(
        screen.getByText(
          mockNestedT(mockActiveMessages, "scan.errors.validationFailed"),
        ),
      ).toBeInTheDocument();
    });
  });

  it("shows production Spanish camera-denied copy via real QRCodeScanner", async () => {
    (navigator.mediaDevices.getUserMedia as jest.Mock).mockRejectedValueOnce(
      new Error("denied"),
    );
    const expected = mockNestedT(mockActiveMessages, "scan.cameraDeniedError");
    // Structural: production es.json must define the key (not sentence-case leaf).
    expect(expected).not.toBe("scan.cameraDeniedError");
    expect(expected.toLowerCase()).not.toBe("camera denied error");

    render(<ScanClient />);
    // Opens the real QRCodeScanner modal.
    fireEvent.click(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.actions.scanQr")),
    );
    // Real scanner start button (production Spanish).
    fireEvent.click(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.scanner.startScanning")),
    );

    await waitFor(() => {
      expect(screen.getByText(expected)).toBeInTheDocument();
    });
    expect(screen.queryByText("Camera denied error")).not.toBeInTheDocument();
  });

  it("opens the scanner as a dialog and dismisses it on Escape (#391)", () => {
    render(<ScanClient />);
    const scanQr = mockNestedT(mockActiveMessages, "scan.actions.scanQr");
    const scanButton = screen.getByRole("button", { name: scanQr });
    scanButton.focus();
    fireEvent.click(scanButton);

    const title = mockNestedT(mockActiveMessages, "scan.scanner.title");
    const dialog = screen.getByRole("dialog", { name: title });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog.contains(document.activeElement as Node)).toBe(true);

    let inertAncestor: HTMLElement | null = scanButton;
    while (inertAncestor && !inertAncestor.inert) {
      inertAncestor = inertAncestor.parentElement;
    }
    expect(inertAncestor?.inert).toBe(true);

    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows production es-AR camera-denied copy via real QRCodeScanner", async () => {
    mockActiveMessages = esARMessages as Record<string, unknown>;
    (navigator.mediaDevices.getUserMedia as jest.Mock).mockRejectedValueOnce(
      new Error("denied"),
    );
    const expected = mockNestedT(mockActiveMessages, "scan.cameraDeniedError");
    expect(expected).toMatch(/c[aá]mara/i);

    render(<ScanClient />);
    fireEvent.click(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.actions.scanQr")),
    );
    fireEvent.click(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.scanner.startScanning")),
    );
    await waitFor(() => {
      expect(screen.getByText(expected)).toBeInTheDocument();
    });
  });
});

function tableCodeInput(): HTMLElement {
  return screen.getByPlaceholderText(
    mockNestedT(mockActiveMessages, "scan.actions.manualPlaceholder"),
  );
}

function submitManualCode(code: string) {
  fireEvent.change(tableCodeInput(), { target: { value: code } });
  fireEvent.click(
    screen.getByText(mockNestedT(mockActiveMessages, "scan.actions.accessTable")),
  );
}

async function expectAnnouncedError(messageKey: string) {
  const message = mockNestedT(mockActiveMessages, messageKey);
  const alert = await screen.findByRole("alert");
  expect(alert).toHaveTextContent(message);
  expect(alert).toHaveAttribute("aria-live", "assertive");
  const input = tableCodeInput();
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(input.getAttribute("aria-describedby")).toBe(alert.id);
  expect(document.getElementById(alert.id)).toBe(alert);
  await waitFor(() => expect(input).toHaveFocus());
}

describe("ScanClient lookup error accessibility", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockActiveMessages = esMessages as Record<string, unknown>;
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: {
        getUserMedia: jest.fn().mockResolvedValue({
          getTracks: () => [{ stop: jest.fn() }],
        }),
      },
    });
  });

  it("announces not-found, marks the field invalid, and returns focus to the code input", async () => {
    mockValidateTableCode.mockResolvedValue({
      isValid: false,
      errorCode: "notFound",
    });
    render(<ScanClient />);
    submitManualCode("QA-INVALID-20260814");
    await expectAnnouncedError("scan.errors.notFound");
  });

  it("announces inactive, validation-failed, and invalid-format lookups", async () => {
    const cases: Array<{ errorCode: string; key: string }> = [
      { errorCode: "inactive", key: "scan.errors.inactive" },
      { errorCode: "failed", key: "scan.errors.validationFailed" },
      { errorCode: "format", key: "scan.errors.invalidCode" },
    ];
    for (const { errorCode, key } of cases) {
      mockValidateTableCode.mockResolvedValue({ isValid: false, errorCode });
      const { unmount } = render(<ScanClient />);
      submitManualCode("TABLE01");
      await expectAnnouncedError(key);
      unmount();
    }

    mockValidateTableCode.mockRejectedValue(new Error("network"));
    render(<ScanClient />);
    submitManualCode("TABLE01");
    await expectAnnouncedError("scan.errors.validationFailed");
  });

  it("announces empty-code on Enter and focuses the field", async () => {
    render(<ScanClient />);
    const input = tableCodeInput();
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.keyDown(input, { key: "Enter" });
    await expectAnnouncedError("scan.errors.emptyCode");
    expect(mockValidateTableCode).not.toHaveBeenCalled();
  });

  it("clears stale error semantics before a new lookup, then re-announces the result", async () => {
    mockValidateTableCode.mockResolvedValue({
      isValid: false,
      errorCode: "notFound",
    });
    render(<ScanClient />);
    submitManualCode("BADCODE");
    await expectAnnouncedError("scan.errors.notFound");

    let resolveValidation: (value: {
      isValid: boolean;
      errorCode: string;
    }) => void = () => undefined;
    mockValidateTableCode.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveValidation = resolve;
        }),
    );

    fireEvent.click(
      screen.getByText(
        mockNestedT(mockActiveMessages, "scan.actions.accessTable"),
      ),
    );

    await waitFor(() => {
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      expect(tableCodeInput()).not.toHaveAttribute("aria-invalid", "true");
      expect(tableCodeInput()).not.toHaveAttribute("aria-describedby");
    });

    resolveValidation({ isValid: false, errorCode: "inactive" });
    await expectAnnouncedError("scan.errors.inactive");
  });

  it("announces scanner camera-denial through the same live region and recovers focus", async () => {
    (navigator.mediaDevices.getUserMedia as jest.Mock).mockRejectedValueOnce(
      new Error("denied"),
    );
    render(<ScanClient />);
    fireEvent.click(
      screen.getByText(mockNestedT(mockActiveMessages, "scan.actions.scanQr")),
    );
    fireEvent.click(
      screen.getByText(
        mockNestedT(mockActiveMessages, "scan.scanner.startScanning"),
      ),
    );
    await expectAnnouncedError("scan.cameraDeniedError");
  });

  it("announces English not-found copy from production guest-messages", async () => {
    mockActiveMessages = enMessages as Record<string, unknown>;
    mockValidateTableCode.mockResolvedValue({
      isValid: false,
      errorCode: "notFound",
    });
    render(<ScanClient />);
    submitManualCode("QA-INVALID-20260814");
    await expectAnnouncedError("scan.errors.notFound");
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Table not found. Check the code and try again.",
    );
  });

  it("defines announced scan error copy in every guest locale", () => {
    const dir = path.join(__dirname, "../../i18n/guest-messages");
    const files = fs
      .readdirSync(dir)
      .filter((file) => file.endsWith(".json") && !file.startsWith("."));
    expect(files.length).toBeGreaterThanOrEqual(21);
    const errorKeys = [
      "invalidCode",
      "validationFailed",
      "emptyCode",
      "notFound",
      "inactive",
    ];
    for (const file of files) {
      const messages = JSON.parse(
        fs.readFileSync(path.join(dir, file), "utf8"),
      ) as {
        scan?: { errors?: Record<string, string>; cameraDeniedError?: string };
      };
      for (const key of errorKeys) {
        const value = messages.scan?.errors?.[key];
        expect(typeof value).toBe("string");
        expect(value?.trim()).not.toBe("");
      }
      expect(typeof messages.scan?.cameraDeniedError).toBe("string");
      expect(messages.scan?.cameraDeniedError?.trim()).not.toBe("");
    }
  });
});
