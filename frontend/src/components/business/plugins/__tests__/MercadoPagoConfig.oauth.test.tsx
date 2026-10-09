/** @jest-environment jsdom */

import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import toast from "react-hot-toast";

import MercadoPagoConfig from "../MercadoPagoConfig";
import { businessPluginAPI } from "@/api/plugins";

const mockReplace = jest.fn();
const mockSearchParams = new URLSearchParams();

jest.mock("next/navigation", () => ({
  useSearchParams: () => mockSearchParams,
  useRouter: () => ({ replace: mockReplace }),
  usePathname: () => "/business/42/plugins",
}));

jest.mock("@/api/plugins", () => ({
  businessPluginAPI: {
    startMercadoPagoOAuth: jest.fn(),
    testPluginConnection: jest.fn(),
    disablePlugin: jest.fn(),
  },
}));

jest.mock("@/constants/plugins", () => ({
  PLUGIN: { mercadopago: "mercadopago" },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: jest.fn(),
    error: jest.fn(),
  },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key.split(".").pop() || key,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element
    return <img alt={String(props.alt ?? "")} src={String(props.src ?? "")} />;
  },
}));

jest.mock("@nextui-org/react", () => {
  const React = jest.requireActual("react");
  return {
    Button: ({
      children,
      onPress,
      isLoading,
      isDisabled,
      startContent: _startContent,
      endContent: _endContent,
      color: _color,
      variant: _variant,
      size: _size,
      className,
      ...rest
    }: any) => (
      <button
        type="button"
        className={className}
        onClick={onPress}
        disabled={isDisabled || isLoading}
        {...rest}
      >
        {children}
      </button>
    ),
    Chip: ({ children }: any) => <span>{children}</span>,
    Input: ({
      label,
      "aria-label": ariaLabel,
      value,
      onValueChange,
      description: _description,
      errorMessage: _errorMessage,
      isInvalid: _isInvalid,
      labelPlacement: _labelPlacement,
      variant: _variant,
      type,
      placeholder,
    }: any) => (
      <label>
        {label}
        <input
          type={type}
          placeholder={placeholder}
          aria-label={
            ariaLabel || (typeof label === "string" ? label : undefined)
          }
          value={value}
          onChange={(e) => onValueChange?.(e.target.value)}
        />
      </label>
    ),
    Link: ({ children, href }: any) => <a href={href}>{children}</a>,
    Select: ({ children, "aria-label": ariaLabel }: any) => (
      <div aria-label={ariaLabel}>{children}</div>
    ),
    SelectItem: ({ children }: any) => <div>{children}</div>,
    Slider: ({ "aria-label": ariaLabel, value, onChange }: any) => (
      <input
        type="range"
        aria-label={ariaLabel}
        value={value}
        onChange={(e) => onChange?.(Number(e.target.value))}
      />
    ),
    Accordion: ({ children }: any) => <div>{children}</div>,
    AccordionItem: ({ title, children, "aria-label": ariaLabel }: any) => (
      <div>
        <button type="button" aria-label={ariaLabel}>
          {title}
        </button>
        {children}
      </div>
    ),
    Modal: ({ children, isOpen }: any) =>
      isOpen ? <div data-testid="disconnect-modal">{children}</div> : null,
    ModalContent: ({ children }: any) => <div>{children}</div>,
    ModalHeader: ({ children }: any) => <div>{children}</div>,
    ModalBody: ({ children }: any) => <div>{children}</div>,
    ModalFooter: ({ children }: any) => <div>{children}</div>,
    useDisclosure: () => {
      const [isOpen, setIsOpen] = React.useState(false);
      return {
        isOpen,
        onOpen: () => setIsOpen(true),
        onClose: () => setIsOpen(false),
      };
    },
  };
});

const plugin = {
  id: 7,
  name: "mercadopago",
  display_name: "Mercado Pago",
  description: "LATAM payments",
  image: "/images/plugins/mercadopago.png",
  category: "payment",
  version: "1.0.0",
  features: "",
  is_enabled: true,
  config: "{}",
};

function renderConfig(
  config: Record<string, unknown> = {},
  overrides: Partial<React.ComponentProps<typeof MercadoPagoConfig>> = {},
) {
  return render(
    <MercadoPagoConfig
      plugin={plugin}
      config={config}
      onConfigChange={jest.fn()}
      onSave={jest.fn()}
      onCancel={jest.fn()}
      businessId="42"
      {...overrides}
    />,
  );
}

describe("MercadoPagoConfig OAuth UX", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    Array.from(mockSearchParams.keys()).forEach((k) =>
      mockSearchParams.delete(k),
    );
  });

  it("shows Connect hero when disconnected and starts OAuth", async () => {
    (businessPluginAPI.startMercadoPagoOAuth as jest.Mock).mockResolvedValue({
      authorization_url: "https://auth.mercadopago.com/authorize?x=1",
    });
    const navigateTo = jest.fn();

    renderConfig({}, { navigateTo });

    expect(
      screen.getByRole("button", { name: /connectWithMercadoPago/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /manualAdvanced/i }),
    ).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: /connectWithMercadoPago/i }),
      );
    });

    await waitFor(() => {
      expect(businessPluginAPI.startMercadoPagoOAuth).toHaveBeenCalledWith(
        "42",
      );
    });
    expect(navigateTo).toHaveBeenCalledWith(
      "https://auth.mercadopago.com/authorize?x=1",
    );
  });

  // A 200 with no authorization_url (platform credentials unset server-side)
  // used to leave the button spinning forever: the early return skipped the
  // isConnecting reset, so the operator could not retry without a reload.
  it("re-enables the connect button when the start call returns no URL", async () => {
    (businessPluginAPI.startMercadoPagoOAuth as jest.Mock).mockResolvedValue({
      authorization_url: "",
    });
    const navigateTo = jest.fn();

    renderConfig({}, { navigateTo });

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: /connectWithMercadoPago/i }),
      );
    });

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled();
    });
    expect(navigateTo).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: /connectWithMercadoPago/i }),
    ).not.toBeDisabled();
  });

  it("shows connected chip and account details for oauth mode", () => {
    renderConfig({
      connection_mode: "oauth",
      oauth_status: "connected",
      mp_user_id: "999001",
      country: "AR",
      live_mode: true,
      access_token: "••••••••",
      refresh_token: "••••••••",
    });

    expect(screen.getByText("connectedChip")).toBeInTheDocument();
    expect(screen.getByText(/999001/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /testConnection/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /disconnect/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /connectWithMercadoPago/i }),
    ).not.toBeInTheDocument();
  });

  it("shows reauth banner and reconnect when oauth_status is reauth_required", () => {
    renderConfig({
      connection_mode: "oauth",
      oauth_status: "reauth_required",
      mp_user_id: "999001",
      access_token: "••••••••",
      refresh_token: "••••••••",
    });

    expect(screen.getByText("reauthRequired")).toBeInTheDocument();
    expect(screen.getByText("reauthBannerTitle")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /reconnect/i }),
    ).toBeInTheDocument();
  });

  it("toasts on mp_connect success query param", async () => {
    mockSearchParams.set("mp_connect", "success");
    renderConfig({});

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalled();
    });
    expect(mockReplace).toHaveBeenCalled();
  });

  it("disables the plugin after disconnect confirm", async () => {
    (businessPluginAPI.disablePlugin as jest.Mock).mockResolvedValue(undefined);
    const onCancel = jest.fn();

    renderConfig(
      {
        connection_mode: "oauth",
        oauth_status: "connected",
        mp_user_id: "1",
        access_token: "••••••••",
        refresh_token: "••••••••",
      },
      { onCancel },
    );

    fireEvent.click(screen.getByRole("button", { name: /^disconnect$/i }));
    expect(screen.getByTestId("disconnect-modal")).toBeInTheDocument();

    const buttons = screen.getAllByRole("button", { name: /^disconnect$/i });
    await act(async () => {
      fireEvent.click(buttons[buttons.length - 1]);
    });

    await waitFor(() => {
      expect(businessPluginAPI.disablePlugin).toHaveBeenCalledWith("42", 7);
    });
    expect(onCancel).toHaveBeenCalled();
  });
});
