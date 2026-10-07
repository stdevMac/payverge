import {
  isWorkerCancelMessage,
  isWorkerInMessage,
  isWorkerOutMessage,
  isWorkerStartMessage,
  type WorkerDoneMessage,
  type WorkerErrorMessage,
  type WorkerProgressMessage,
  type WorkerStartMessage,
} from "./workerProtocol";

const baseStart: WorkerStartMessage = {
  type: "start",
  renderInput: {
    kit: "editorial",
    composition: "photoBottomStack",
    aspect: "4:5",
    photoUrl: "https://cdn.example.com/dish.jpg",
    slots: { dishName: "Milanesa" },
    palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
  },
  preset: "pushIn",
  photo: null,
  logo: null,
};

describe("workerProtocol", () => {
  it("accepts a well-formed start message", () => {
    const msg: WorkerStartMessage = { ...baseStart };
    expect(isWorkerInMessage(msg)).toBe(true);
    expect(isWorkerStartMessage(msg)).toBe(true);
    expect(isWorkerCancelMessage(msg)).toBe(false);
  });

  it("accepts cancel", () => {
    expect(isWorkerInMessage({ type: "cancel" })).toBe(true);
    expect(isWorkerCancelMessage({ type: "cancel" })).toBe(true);
    expect(isWorkerStartMessage({ type: "cancel" })).toBe(false);
  });

  it("rejects malformed start payloads", () => {
    expect(isWorkerInMessage({ type: "start" })).toBe(false);
    expect(isWorkerInMessage({ type: "start", renderInput: {}, preset: 1 })).toBe(
      false,
    );
    expect(isWorkerInMessage({ type: "nope" })).toBe(false);
    expect(isWorkerInMessage(null)).toBe(false);
    expect(isWorkerInMessage("start")).toBe(false);
  });

  it("accepts progress / done / error out-messages", () => {
    const progress: WorkerProgressMessage = { type: "progress", fraction: 0.5 };
    const done: WorkerDoneMessage = {
      type: "done",
      video: new ArrayBuffer(4),
      videoMime: "video/mp4",
    };
    const error: WorkerErrorMessage = {
      type: "error",
      message: "motion_unsupported",
      code: "unsupported",
    };
    expect(isWorkerOutMessage(progress)).toBe(true);
    expect(isWorkerOutMessage(done)).toBe(true);
    expect(isWorkerOutMessage(error)).toBe(true);
  });

  it("rejects malformed out-messages", () => {
    expect(isWorkerOutMessage({ type: "progress" })).toBe(false);
    expect(isWorkerOutMessage({ type: "done", video: "x", videoMime: "video/mp4" })).toBe(
      false,
    );
    expect(isWorkerOutMessage({ type: "error" })).toBe(false);
    expect(isWorkerOutMessage({ type: "other" })).toBe(false);
  });
});
