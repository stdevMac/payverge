/** @jest-environment jsdom */
import React from "react";
import { render, act } from "@testing-library/react";
import toast from "react-hot-toast";
import { ToastProvider, useToast } from "../ToastContext";

jest.mock("react-hot-toast", () => {
  const success = jest.fn();
  const error = jest.fn();
  const custom = jest.fn();
  const dismiss = jest.fn();
  const toastFn = Object.assign(jest.fn(), { success, error, custom, dismiss });
  return { __esModule: true, default: toastFn };
});

const Probe: React.FC<{
  onReady: (api: ReturnType<typeof useToast>) => void;
}> = ({ onReady }) => {
  const api = useToast();
  React.useEffect(() => {
    onReady(api);
  }, [api, onReady]);
  return <span>ready</span>;
};

describe("ToastContext react-hot-toast adapter", () => {
  it("exposes the same hook API and does not render the old right-4 stack", () => {
    let api: ReturnType<typeof useToast> | undefined;
    const { container } = render(
      <ToastProvider>
        <Probe onReady={(next) => (api = next)} />
      </ToastProvider>,
    );

    expect(api).toEqual(
      expect.objectContaining({
        showToast: expect.any(Function),
        showSuccess: expect.any(Function),
        showError: expect.any(Function),
        showWarning: expect.any(Function),
        showInfo: expect.any(Function),
      }),
    );
    expect(container.querySelector(".fixed.top-20.right-4")).toBeNull();
    expect(container.innerHTML).not.toMatch(/top-20 right-4/);

    act(() => {
      api!.showSuccess("Saved", "Menu published");
    });

    expect(toast.success).toHaveBeenCalled();
    expect(container.querySelector(".fixed.top-20.right-4")).toBeNull();
  });
});
