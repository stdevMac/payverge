/**
 * @jest-environment jsdom
 */
import React from "react";
import { render } from "@testing-library/react";
import { WalletBridge } from "../WalletBridge";
import {
  WALLET_BRIDGE_UNAVAILABLE,
  resetWalletBridge,
  useWalletBridgeStore,
} from "../walletBridgeStore";

const mockSign = jest.fn();
let mockAccount: {
  address: `0x${string}` | undefined;
  isConnected: boolean;
  status: string;
} = { address: undefined, isConnected: false, status: "disconnected" };

jest.mock("wagmi", () => ({
  useAccount: () => mockAccount,
  useChainId: () => 8453,
  useSignMessage: () => ({ signMessageAsync: mockSign }),
}));

describe("WalletBridge", () => {
  beforeEach(() => resetWalletBridge());

  it("starts unavailable before any wallet stack mounts", () => {
    expect(useWalletBridgeStore.getState()).toEqual(WALLET_BRIDGE_UNAVAILABLE);
  });

  it("publishes the connected wagmi account", () => {
    mockAccount = { address: "0xabc", isConnected: true, status: "connected" };
    render(<WalletBridge />);
    expect(useWalletBridgeStore.getState()).toEqual({
      available: true,
      address: "0xabc",
      isConnected: true,
      chainId: 8453,
      signMessageAsync: mockSign,
    });
  });

  it("publishes a real disconnect as available + not connected", () => {
    mockAccount = {
      address: undefined,
      isConnected: false,
      status: "disconnected",
    };
    render(<WalletBridge />);
    const state = useWalletBridgeStore.getState();
    expect(state.available).toBe(true);
    expect(state.isConnected).toBe(false);
  });

  it.each(["reconnecting", "connecting"])(
    "stays unavailable while wagmi is %s",
    (status) => {
      mockAccount = { address: undefined, isConnected: false, status };
      render(<WalletBridge />);
      expect(useWalletBridgeStore.getState().available).toBe(false);
    },
  );

  it("resets to unavailable when the wallet stack unmounts", () => {
    mockAccount = { address: "0xabc", isConnected: true, status: "connected" };
    const { unmount } = render(<WalletBridge />);
    expect(useWalletBridgeStore.getState().available).toBe(true);
    unmount();
    expect(useWalletBridgeStore.getState()).toEqual(WALLET_BRIDGE_UNAVAILABLE);
  });
});
