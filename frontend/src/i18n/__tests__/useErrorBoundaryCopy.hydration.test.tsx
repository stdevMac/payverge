/** @jest-environment jsdom */
import React from "react";
import { act } from "react";
import { renderToString } from "react-dom/server";
import { hydrateRoot } from "react-dom/client";
import { useErrorBoundaryCopy } from "../useErrorBoundaryCopy";
import { __errorBoundaryCopyForTests as COPY } from "../errorBoundaryCopy";

function Probe() {
  const copy = useErrorBoundaryCopy();
  return <h1>{copy.title}</h1>;
}

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

describe("useErrorBoundaryCopy hydration", () => {
  const originalLang = document.documentElement.lang;

  afterEach(() => {
    document.documentElement.lang = originalLang;
    window.history.replaceState({}, "", "/");
  });

  it("hydrates English server HTML without a mismatch, then switches locale", async () => {
    // Server render: no window locale available, English copy.
    const serverHtml = renderToString(<Probe />);
    expect(serverHtml).toContain(COPY.en.title);

    // Browser is Spanish.
    document.documentElement.lang = "es";
    window.history.replaceState({}, "", "/t/abc?lang=es");

    const container = document.createElement("div");
    container.innerHTML = serverHtml;
    document.body.appendChild(container);

    const errorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    const recoverable: unknown[] = [];
    await act(async () => {
      hydrateRoot(container, <Probe />, {
        onRecoverableError: (err) => recoverable.push(err),
      });
    });

    expect(recoverable).toEqual([]);
    // Any console.error during hydration (mismatch warnings included) fails.
    expect(errorSpy.mock.calls.map((args) => String(args[0]))).toEqual([]);
    expect(container.textContent).toBe(COPY.es.title);

    errorSpy.mockRestore();
    container.remove();
  });
});
