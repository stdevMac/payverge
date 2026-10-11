/** @jest-environment jsdom */

import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import toast from "react-hot-toast";

import StripeConfig from "../StripeConfig";
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
    startStripeOAuth: jest.fn(),
    disablePlugin: jest.fn(),
  },
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
    Switch: ({ isSelected, onValueChange }: any) => (
      <input
        type="checkbox"
        checked={!!isSelected}
        onChange={(e) => onValueChange?.(e.target.checked)}
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
  id: 3,
  name: "stripe",
  display_name: "Stripe",
  description: "Cards",
  image: "/images/plugins/stripe-logo.png",
  category: "payment",
  version: "1.0.0",
  features: "",
  is_enabled: true,
  config: "{}",
};

function renderConfig(
  config: Record<string, unknown> = {},
  overrides: Partial<React.ComponentProps<typeof StripeConfig>> = {},
) {
  return render(
    <StripeConfig
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

describe("StripeConfig OAuth UX", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    Array.from(mockSearchParams.keys()).forEach((k) =>
      mockSearchParams.delete(k),
    );
  });

  it("shows Connect hero when disconnected and starts OAuth", async () => {
    (businessPluginAPI.startStripeOAuth as jest.Mock).mockResolvedValue({
      authorization_url:
        "https://connect.stripe.com/oauth/authorize?response_type=code&client_id=ca_x",
    });
    const navigateTo = jest.fn();

    renderConfig({}, { navigateTo });

    expect(
      screen.getByRole("button", { name: /connectWithStripe/i }),
    ).toBeInTheDocument();
    // Manual fallback form is always available when disconnected.
    expect(screen.getAllByText(/manualAdvanced/i).length).toBeGreaterThan(0);
    expect(
      screen.getByPlaceholderText("secretKeyPlaceholder"),
    ).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: /connectWithStripe/i }),
      );
    });

    await waitFor(() => {
      expect(businessPluginAPI.startStripeOAuth).toHaveBeenCalledWith("42");
    });
    expect(navigateTo).toHaveBeenCalledWith(
      "https://connect.stripe.com/oauth/authorize?response_type=code&client_id=ca_x",
    );
  });

  it("shows connected chip and account id for oauth mode", () => {
    renderConfig({
      connection_mode: "oauth",
      oauth_status: "connected",
      stripe_user_id: "acct_12345",
      live_mode: true,
      access_token: "••••••••",
    });

    expect(screen.getByText("connectedChip")).toBeInTheDocument();
    expect(screen.getByText(/acct_12345/)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /connectWithStripe/i }),
    ).not.toBeInTheDocument();
    // Manual secret key inputs are hidden when OAuth-connected.
    expect(
      screen.queryByPlaceholderText("secretKeyPlaceholder"),
    ).not.toBeInTheDocument();
  });

  it("keeps manual secret key fields usable as fallback when disconnected", () => {
    renderConfig({});
    expect(screen.getByPlaceholderText("secretKeyPlaceholder")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("publishableKeyPlaceholder"),
    ).toBeInTheDocument();
  });

  // A 200 with no authorization_url (platform credentials unset server-side)
  // used to leave the button spinning forever: the early return skipped the
  // isConnecting reset, so the operator could not retry without a reload.
  it("re-enables the connect button when the start call returns no URL", async () => {
    (businessPluginAPI.startStripeOAuth as jest.Mock).mockResolvedValue({
      authorization_url: "",
    });
    const navigateTo = jest.fn();

    renderConfig({}, { navigateTo });
    const button = screen.getByRole("button", { name: /connectWithStripe/i });

    await act(async () => {
      fireEvent.click(button);
    });

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled();
    });
    expect(navigateTo).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: /connectWithStripe/i }),
    ).not.toBeDisabled();
  });

  it("toasts on stripe_connect success query param", async () => {
    mockSearchParams.set("stripe_connect", "success");
    renderConfig({});

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalled();
    });
    expect(mockReplace).toHaveBeenCalled();
  });

  it("shows reauth banner when oauth_status is reauth_required", () => {
    renderConfig({
      connection_mode: "oauth",
      oauth_status: "reauth_required",
      stripe_user_id: "acct_reauth",
    });

    expect(screen.getByText("reauthRequired")).toBeInTheDocument();
    expect(screen.getByText("reauthBannerTitle")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /reconnect/i })).toBeInTheDocument();
  });
});
