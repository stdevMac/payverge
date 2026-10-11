/** @jest-environment jsdom */

import React from "react";
import { render, screen, act } from "@testing-library/react";
import toast, { Toaster } from "react-hot-toast";
import { ToastProvider, useToast } from "../ToastContext";

const ToastTrigger: React.FC<{
  onReady?: (api: ReturnType<typeof useToast>) => void;
}> = ({ onReady }) => {
  const toastApi = useToast();
  React.useEffect(() => {
    onReady?.(toastApi);
  }, [onReady, toastApi]);
  return null;
};

describe("ToastContext accessibility", () => {
  afterEach(() => {
    act(() => {
      toast.remove();
    });
  });

  it("does not mount the legacy right-4 NextUI stack", () => {
    const { container } = render(
      <ToastProvider>
        <div>content</div>
      </ToastProvider>,
    );

    expect(container.querySelector(".fixed.top-20.right-4")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("announces success toasts without role=alert", () => {
    let api: ReturnType<typeof useToast> | undefined;
    render(
      <ToastProvider>
        <Toaster />
        <ToastTrigger onReady={(a) => (api = a)} />
      </ToastProvider>,
    );

    act(() => {
      api!.showSuccess("ok", "saved successfully");
    });

    expect(screen.getByText(/ok/)).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("renders error toasts through the shared toaster", () => {
    let api: ReturnType<typeof useToast> | undefined;
    render(
      <ToastProvider>
        <Toaster />
        <ToastTrigger onReady={(a) => (api = a)} />
      </ToastProvider>,
    );

    act(() => {
      api!.showError("oh no", "something broke");
    });

    expect(screen.getByText(/oh no/)).toBeInTheDocument();
    expect(screen.getByText(/something broke/)).toBeInTheDocument();
  });

  it("caps visible toasts and evicts the oldest low-priority one first (R3-AI-9)", () => {
    let api: ReturnType<typeof useToast> | undefined;
    render(
      <ToastProvider>
        <Toaster />
        <ToastTrigger onReady={(a) => (api = a)} />
      </ToastProvider>,
    );

    act(() => {
      for (let i = 1; i <= 6; i++) {
        api!.showInfo(`info-${i}`, undefined, 0);
      }
    });

    expect(screen.queryByText("info-1")).toBeNull();
    expect(screen.queryByText("info-2")).toBeNull();
    expect(screen.getByText("info-3")).toBeInTheDocument();
    expect(screen.getByText("info-6")).toBeInTheDocument();
  });

  it("keeps urgent toasts on screen while evicting low-priority ones (R3-AI-9)", () => {
    let api: ReturnType<typeof useToast> | undefined;
    render(
      <ToastProvider>
        <Toaster />
        <ToastTrigger onReady={(a) => (api = a)} />
      </ToastProvider>,
    );

    act(() => {
      api!.showWarning("urgent-alert", undefined, 0);
      for (let i = 1; i <= 5; i++) {
        api!.showInfo(`info-${i}`, undefined, 0);
      }
    });

    expect(screen.getByText("urgent-alert")).toBeInTheDocument();
    expect(screen.queryByText("info-1")).toBeNull();
    expect(screen.queryByText("info-2")).toBeNull();
  });
});
