/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ErrorBoundary } from "react-error-boundary";
import TabErrorFallback, {
  isChunkLoadError,
  shouldHardReloadForChunkError,
  chunkReloadStorageKey,
} from "../TabErrorFallback";

// English passthrough — fallback receives a `tString` prop so it stays
// localized via the surviving dashboard provider in production.
const tString = (key: string) => key;

function Boom(): React.JSX.Element {
  throw new Error("tab exploded");
}

function memoryStorage(): Pick<Storage, "getItem" | "setItem"> & {
  data: Record<string, string>;
} {
  const data: Record<string, string> = {};
  return {
    data,
    getItem: (k: string) => data[k] ?? null,
    setItem: (k: string, v: string) => {
      data[k] = v;
    },
  };
}

describe("isChunkLoadError", () => {
  it("detects ChunkLoadError by name", () => {
    const err = new Error("Loading chunk 8436 failed");
    err.name = "ChunkLoadError";
    expect(isChunkLoadError(err)).toBe(true);
  });

  it("detects Loading chunk failed by message", () => {
    expect(isChunkLoadError(new Error("Loading chunk 8436 failed"))).toBe(
      true,
    );
  });

  it("returns false for ordinary errors", () => {
    expect(isChunkLoadError(new Error("tab exploded"))).toBe(false);
    expect(isChunkLoadError(null)).toBe(false);
  });
});

describe("shouldHardReloadForChunkError", () => {
  it("returns true once for a ChunkLoadError and sets the guard key", () => {
    const storage = memoryStorage();
    const err = new Error("Loading chunk 8436 failed");
    err.name = "ChunkLoadError";

    expect(shouldHardReloadForChunkError(err, storage, "default")).toBe(true);
    expect(storage.data[chunkReloadStorageKey("default")]).toBe("1");
    // Second attempt must soft-reset so a broken deploy cannot loop.
    expect(shouldHardReloadForChunkError(err, storage, "default")).toBe(false);
  });

  it("returns false for non-chunk errors", () => {
    const storage = memoryStorage();
    expect(
      shouldHardReloadForChunkError(new Error("boom"), storage, "default"),
    ).toBe(false);
    expect(Object.keys(storage.data)).toHaveLength(0);
  });
});

describe("TabErrorFallback", () => {
  it("contains a child throw inside the boundary and renders the localized retry", () => {
    const spy = jest.spyOn(console, "error").mockImplementation(() => {});
    render(
      <div>
        <nav data-testid="sidebar">sidebar stays</nav>
        <ErrorBoundary
          FallbackComponent={(props) => (
            <TabErrorFallback {...props} tString={tString} />
          )}
        >
          <Boom />
        </ErrorBoundary>
      </div>,
    );
    // Sibling chrome survives — the throw did not unwind past the boundary.
    expect(screen.getByTestId("sidebar")).toBeInTheDocument();
    // Localized title + body keys are rendered by the fallback.
    expect(screen.getByText("error.title")).toBeInTheDocument();
    expect(screen.getByText("error.tabCrashed")).toBeInTheDocument();
    spy.mockRestore();
  });

  it("calls resetErrorBoundary when the retry button is pressed for normal errors", () => {
    const reset = jest.fn();
    render(
      <TabErrorFallback
        error={new Error("boom")}
        resetErrorBoundary={reset}
        tString={tString}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "error.retry" }));
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it("hard-reload path is taken for ChunkLoadError when guard is clear", () => {
    // Drive the pure decision used by the button handler (location.reload is
    // non-configurable under jsdom).
    sessionStorage.clear();
    const err = new Error("Loading chunk 8436 failed");
    err.name = "ChunkLoadError";
    expect(
      shouldHardReloadForChunkError(err, sessionStorage, "default"),
    ).toBe(true);
    expect(sessionStorage.getItem(chunkReloadStorageKey("default"))).toBe("1");

    // After guard is set, the UI path soft-resets instead of reloading again.
    const reset = jest.fn();
    render(
      <TabErrorFallback
        error={err}
        resetErrorBoundary={reset}
        tString={tString}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "error.retry" }));
    expect(reset).toHaveBeenCalledTimes(1);
  });
});
