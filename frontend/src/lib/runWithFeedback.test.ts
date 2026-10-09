import toast from "react-hot-toast";
import { runWithFeedback } from "./runWithFeedback";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

describe("runWithFeedback", () => {
  beforeEach(() => jest.clearAllMocks());

  it("returns the result and shows a success toast", async () => {
    const result = await runWithFeedback(async () => 42, {
      success: "Saved",
      error: "Save failed",
    });
    expect(result).toBe(42);
    expect(toast.success).toHaveBeenCalledWith("Saved");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("returns undefined and shows an error toast on failure", async () => {
    const result = await runWithFeedback(
      async () => {
        throw new Error("boom");
      },
      { error: "Save failed" },
    );
    expect(result).toBeUndefined();
    expect(toast.error).toHaveBeenCalledWith("Save failed");
  });

  it("drives the busy setter around the action, including on failure", async () => {
    const busy: boolean[] = [];
    await runWithFeedback(
      async () => {
        throw new Error("boom");
      },
      { error: "x", setBusy: (b) => busy.push(b) },
    );
    expect(busy).toEqual([true, false]);
  });

  it("uses a custom notifier when provided (ToastContext consumers)", async () => {
    const notifier = { success: jest.fn(), error: jest.fn() };
    await runWithFeedback(async () => 1, { success: "ok", error: "no", notifier });
    expect(notifier.success).toHaveBeenCalledWith("ok");
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("rethrows when rethrow=true (callers that keep modals open)", async () => {
    await expect(
      runWithFeedback(
        async () => {
          throw new Error("boom");
        },
        { error: "no", rethrow: true },
      ),
    ).rejects.toThrow("boom");
    expect(toast.error).toHaveBeenCalledWith("no");
  });
});
