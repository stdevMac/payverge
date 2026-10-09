/** @jest-environment jsdom */
/**
 * L8 PDFDigitizer findings:
 *  - F13: the extraction poll interval is owned by a ref and torn down on
 *         unmount (tab switch / modal close) so it can't keep firing setState /
 *         hitting the API against an unmounted component.
 *  - F14: oversized files are rejected at selection time before any rendering /
 *         upload, with a translated error.
 *
 * #576: snapshot file bytes at selection time so a stale File handle cannot
 * throw a raw NotFoundError at extract, and retry keeps the selected name.
 */

import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mockGetPdfDocument = jest.fn();

jest.mock("pdfjs-dist/legacy/build/pdf.mjs", () => ({
    GlobalWorkerOptions: {},
    getDocument: (...args: unknown[]) => mockGetPdfDocument(...args),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
    const { getTranslation } = jest.requireActual("@/i18n/getTranslation") as typeof import("@/i18n/getTranslation");
    const harness = () =>
        ((globalThis as { __pdfDigitizerI18n?: { locale: string; useRealTranslations: boolean } })
            .__pdfDigitizerI18n ?? { locale: "en", useRealTranslations: false });
    return {
        useSimpleLocale: () => ({ locale: harness().locale, setLocale: jest.fn() }),
        getTranslation: (key: string, locale?: string, params?: Record<string, string | number>) => {
            const { locale: activeLocale, useRealTranslations } = harness();
            if (useRealTranslations) {
                return getTranslation(
                    key,
                    (locale ?? activeLocale) as "en" | "es" | "es-AR",
                    params,
                );
            }
            let v = key;
            if (params) {
                Object.entries(params).forEach(([k, val]) => {
                    v = `${v}|${k}=${val}`;
                });
            }
            return v;
        },
    };
});

// Flaky-test root cause (#576 "retry after a later extract error ..."): the
// component swaps views inside <AnimatePresence mode="wait">, so the error view
// mounts only after the processing view's exit animation completes. framer-motion
// drives that from a module-level requestAnimationFrame loop; tests in this file
// that use jest fake timers can leave that singleton loop believing a frame is
// already scheduled, after which exit animations never finish and the stale
// "loading-pdf" view stays on screen (the error view and "Try again" never
// mount) for the rest of the file. View-swap animation is irrelevant to what
// these tests assert, so AnimatePresence renders its current children directly.
jest.mock("framer-motion", () => {
    const actual = jest.requireActual("framer-motion") as typeof import("framer-motion");
    const React = jest.requireActual("react") as typeof import("react");
    return {
        ...actual,
        AnimatePresence: ({ children }: { children?: React.ReactNode }) =>
            React.createElement(React.Fragment, null, children),
    };
});

jest.mock("@/utils/apiError", () => ({ errMessage: (e: unknown) => (e as Error)?.message ?? "" }));

jest.mock("@/api/business", () => ({
    startMenuExtraction: jest.fn(),
    uploadMenuPage: jest.fn(),
    processMenuExtraction: jest.fn(),
    getMenuExtractionJob: jest.fn(),
}));

import PDFDigitizer from "../PDFDigitizer";
import { getTranslation } from "@/i18n/getTranslation";
import {
    startMenuExtraction,
    uploadMenuPage,
    processMenuExtraction,
    getMenuExtractionJob,
} from "@/api/business";

const mockStart = startMenuExtraction as jest.Mock;
const mockUpload = uploadMenuPage as jest.Mock;
const mockProcess = processMenuExtraction as jest.Mock;
const mockJob = getMenuExtractionJob as jest.Mock;

function renderDigitizer(onExtracted = jest.fn()) {
    return render(
        <PDFDigitizer
            businessId={42}
            onExtracted={onExtracted}
        />,
    );
}

function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (reason?: unknown) => void;
    const promise = new Promise<T>((res, rej) => {
        resolve = res;
        reject = rej;
    });
    return { promise, resolve, reject };
}

// Build a File with a controllable byte size.
function makeFile(name: string, type: string, sizeBytes: number): File {
    const f = new File(["x"], name, { type });
    Object.defineProperty(f, "size", { value: sizeBytes });
    return f;
}

function staleFileHandleError(): Error {
    const err = new Error(
        "A requested file or directory could not be found at the time an operation was processed.",
    );
    err.name = "NotFoundError";
    return err;
}

function makePdfFile(name = "menu.pdf", byteLength = 45 * 1024): File {
    const bytes = new Uint8Array(byteLength);
    bytes.set([0x25, 0x50, 0x44, 0x46]); // %PDF
    return new File([bytes], name, { type: "application/pdf" });
}

function attachArrayBuffer(
    file: File,
    impl: ArrayBuffer | (() => Promise<ArrayBuffer>),
): jest.Mock<Promise<ArrayBuffer>, []> {
    const fn = jest.fn(
        typeof impl === "function" ? impl : () => Promise.resolve(impl.slice(0)),
    );
    Object.defineProperty(file, "arrayBuffer", { value: fn });
    return fn;
}

function stubCanvasExport() {
    jest.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({} as CanvasRenderingContext2D);
    jest.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation((cb) => {
        cb?.(new Blob(["img"], { type: "image/jpeg" }));
    });
}

function mockOnePagePdf() {
    mockGetPdfDocument.mockReturnValue({
        promise: Promise.resolve({
            numPages: 1,
            getPage: jest.fn().mockResolvedValue({
                getViewport: () => ({ width: 8, height: 8 }),
                render: () => ({ promise: Promise.resolve() }),
            }),
        }),
    });
}

describe("PDFDigitizer (L8)", () => {
    beforeEach(() => {
        jest.clearAllMocks();
        (globalThis as { __pdfDigitizerI18n?: { locale: string; useRealTranslations: boolean } })
            .__pdfDigitizerI18n = { locale: "en", useRealTranslations: false };
        jest.spyOn(console, "error").mockImplementation(() => {});
    });

    afterEach(() => {
        (console.error as jest.Mock).mockRestore?.();
        jest.useRealTimers();
    });

    it("F14: rejects an oversized image file at selection with a translated error", async () => {
        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;

        const big = makeFile("huge.png", "image/png", 20 * 1024 * 1024); // 20MB
        await act(async () => {
            fireEvent.change(input, { target: { files: [big] } });
        });

        // The translated fileTooLarge error appears with the {max} param.
        expect(
            screen.getByText(/aiMenuOnboarding\.pdfDigitizer\.error\.fileTooLarge\|max=15/),
        ).toBeInTheDocument();
        // The over-size file was NOT accepted (no extract button shown).
        expect(
            screen.queryByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        ).not.toBeInTheDocument();
    });

    it("F14: accepts an in-limit image file (shows the extract button)", async () => {
        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;

        const ok = makeFile("menu.png", "image/png", 2 * 1024 * 1024); // 2MB
        await act(async () => {
            fireEvent.change(input, { target: { files: [ok] } });
        });

        expect(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        ).toBeInTheDocument();
    });

    it("labels extraction progress for assistive technology", async () => {
        mockStart.mockReturnValue(new Promise(() => {}));
        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, {
                target: {
                    files: [makeFile("menu.png", "image/png", 1024)],
                },
            });
        });

        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );

        expect(
            await screen.findByRole("progressbar", {
                name: "aiMenuOnboarding.pdfDigitizer.processing.progressLabel",
            }),
        ).toBeInTheDocument();
    });

    it("F13: stops polling on unmount (no further getMenuExtractionJob calls)", async () => {
        jest.useFakeTimers();
        mockStart.mockResolvedValue({ job_id: "job-1" });
        mockUpload.mockResolvedValue({});
        mockProcess.mockResolvedValue({});
        // Always "processing" so the poll keeps running until torn down.
        mockJob.mockResolvedValue({ status: "processing" });

        const { unmount } = renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        const ok = makeFile("menu.png", "image/png", 1 * 1024 * 1024);
        await act(async () => {
            fireEvent.change(input, { target: { files: [ok] } });
        });

        const extractBtn = await screen.findByText(
            /aiMenuOnboarding\.pdfDigitizer\.extractButton/,
        );
        await act(async () => {
            fireEvent.click(extractBtn);
        });
        // Let the start/upload promises settle so the poll interval is created.
        await act(async () => {
            await Promise.resolve();
            await Promise.resolve();
            await Promise.resolve();
            jest.advanceTimersByTime(2000);
            await Promise.resolve();
        });

        // Poll has fired at least once.
        await waitFor(() => expect(mockJob.mock.calls.length).toBeGreaterThan(0));
        const callsBeforeUnmount = mockJob.mock.calls.length;

        // Unmount mid-extraction; the interval must be cleared by the cleanup.
        act(() => {
            unmount();
        });
        await act(async () => {
            jest.advanceTimersByTime(10_000);
            await Promise.resolve();
        });

        // No additional poll calls after unmount.
        expect(mockJob.mock.calls.length).toBe(callsBeforeUnmount);
    });

    it("handles a rejected extraction initiation before polling begins", async () => {
        jest.useFakeTimers();
        mockStart.mockResolvedValue({ job_id: "job-1" });
        mockUpload.mockResolvedValue({});

        let rejectInitiation!: (reason?: unknown) => void;
        mockProcess.mockImplementation(
            () => new Promise((_resolve, reject) => {
                rejectInitiation = reject;
            }),
        );

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        const ok = makeFile("menu.png", "image/png", 1 * 1024 * 1024);
        await act(async () => {
            fireEvent.change(input, { target: { files: [ok] } });
        });

        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockProcess).toHaveBeenCalledWith(42, "job-1"));

        // Polling must wait until the extraction initiation request succeeds.
        await act(async () => {
            jest.advanceTimersByTime(10_000);
            await Promise.resolve();
        });
        expect(mockJob).not.toHaveBeenCalled();

        // A 401/500/network rejection immediately enters the translated,
        // retryable error state without waiting for a poll tick.
        jest.useRealTimers();
        await act(async () => {
            rejectInitiation(new Error("Request failed with status code 500"));
            await Promise.resolve();
        });
        expect(
            await screen.findByText("aiMenuOnboarding.pdfDigitizer.error.startExtractionFailed"),
        ).toBeInTheDocument();
        expect(
            screen.getByText("aiMenuOnboarding.pdfDigitizer.error.tryAgain"),
        ).toBeInTheDocument();

        jest.useFakeTimers();
        await act(async () => {
            jest.advanceTimersByTime(10_000);
            await Promise.resolve();
        });
        expect(mockJob).not.toHaveBeenCalled();
    });

    it("does not start polling after unmount while extraction initiation is pending", async () => {
        jest.useFakeTimers();
        mockStart.mockResolvedValue({ job_id: "job-1" });
        mockUpload.mockResolvedValue({});
        mockJob.mockResolvedValue({ status: "processing" });

        let resolveInitiation!: (value: { status: string }) => void;
        mockProcess.mockImplementation(
            () => new Promise(resolve => {
                resolveInitiation = resolve;
            }),
        );

        const { unmount } = renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        const ok = makeFile("menu.png", "image/png", 1 * 1024 * 1024);
        await act(async () => {
            fireEvent.change(input, { target: { files: [ok] } });
        });

        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockProcess).toHaveBeenCalledWith(42, "job-1"));

        unmount();
        await act(async () => {
            resolveInitiation({ status: "processing" });
            await Promise.resolve();
            jest.advanceTimersByTime(10_000);
            await Promise.resolve();
        });

        expect(mockJob).not.toHaveBeenCalled();
    });

    it("stops snapshot work when the file read resolves after unmount", async () => {
        const fileRead = deferred<ArrayBuffer>();
        const pdf = makeFile("menu.pdf", "application/pdf", 1 * 1024 * 1024);
        const arrayBuffer = jest.fn(() => fileRead.promise);
        Object.defineProperty(pdf, "arrayBuffer", { value: arrayBuffer });

        const { unmount } = renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, { target: { files: [pdf] } });
        await waitFor(() => expect(arrayBuffer).toHaveBeenCalledTimes(1));
        expect(
            screen.queryByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        ).not.toBeInTheDocument();

        unmount();
        await act(async () => {
            fileRead.resolve(new ArrayBuffer(8));
            await fileRead.promise;
        });

        expect(mockGetPdfDocument).not.toHaveBeenCalled();
        expect(mockStart).not.toHaveBeenCalled();
    });

    it("does not upload pages when job creation resolves after unmount", async () => {
        const jobCreation = deferred<{ job_id: string }>();
        mockStart.mockReturnValue(jobCreation.promise);

        const { unmount } = renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, {
            target: { files: [makeFile("menu.png", "image/png", 1 * 1024 * 1024)] },
        });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockStart).toHaveBeenCalledWith(42));

        unmount();
        await act(async () => {
            jobCreation.resolve({ job_id: "job-1" });
            await jobCreation.promise;
        });

        expect(mockUpload).not.toHaveBeenCalled();
        expect(mockProcess).not.toHaveBeenCalled();
    });

    it("does not start extraction when an upload resolves after unmount", async () => {
        const upload = deferred<object>();
        mockStart.mockResolvedValue({ job_id: "job-1" });
        mockUpload.mockReturnValue(upload.promise);

        const { unmount } = renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, {
            target: { files: [makeFile("menu.png", "image/png", 1 * 1024 * 1024)] },
        });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockUpload).toHaveBeenCalledTimes(1));

        unmount();
        await act(async () => {
            upload.resolve({});
            await upload.promise;
        });

        expect(mockProcess).not.toHaveBeenCalled();
        expect(mockJob).not.toHaveBeenCalled();
    });

    it("ignores an in-flight completed poll that resolves after unmount", async () => {
        jest.useFakeTimers();
        const poll = deferred<{ status: string; extracted_menu: string }>();
        const onExtracted = jest.fn();
        mockStart.mockResolvedValue({ job_id: "job-1" });
        mockUpload.mockResolvedValue({});
        mockProcess.mockResolvedValue({});
        mockJob.mockReturnValue(poll.promise);

        const { unmount } = renderDigitizer(onExtracted);
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, {
            target: { files: [makeFile("menu.png", "image/png", 1 * 1024 * 1024)] },
        });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await act(async () => {
            await Promise.resolve();
            await Promise.resolve();
            await Promise.resolve();
            jest.advanceTimersByTime(2000);
            await Promise.resolve();
        });
        expect(mockJob).toHaveBeenCalledTimes(1);

        unmount();
        await act(async () => {
            poll.resolve({ status: "completed", extracted_menu: '{"categories":[]}' });
            await poll.promise;
        });

        expect(onExtracted).not.toHaveBeenCalled();
    });

    it("stops PDF conversion work when getDocument resolves after Cancel", async () => {
        const pdfDoc = deferred<{ numPages: number; getPage: jest.Mock }>();
        mockGetPdfDocument.mockReturnValue({ promise: pdfDoc.promise });

        const pdf = makePdfFile();
        attachArrayBuffer(pdf, new ArrayBuffer(45 * 1024));

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, { target: { files: [pdf] } });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockGetPdfDocument).toHaveBeenCalledTimes(1));

        fireEvent.click(
            await screen.findByText("aiMenuOnboarding.pdfDigitizer.processing.cancel"),
        );
        await act(async () => {
            pdfDoc.resolve({ numPages: 1, getPage: jest.fn() });
            await pdfDoc.promise;
        });

        expect(mockStart).not.toHaveBeenCalled();
    });

    it("does not upload pages when job creation resolves after Cancel", async () => {
        const jobCreation = deferred<{ job_id: string }>();
        mockStart.mockReturnValue(jobCreation.promise);

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, {
            target: { files: [makeFile("menu.png", "image/png", 1 * 1024 * 1024)] },
        });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockStart).toHaveBeenCalledWith(42));

        fireEvent.click(
            await screen.findByText("aiMenuOnboarding.pdfDigitizer.processing.cancel"),
        );
        await act(async () => {
            jobCreation.resolve({ job_id: "job-1" });
            await jobCreation.promise;
        });

        expect(mockUpload).not.toHaveBeenCalled();
        expect(mockProcess).not.toHaveBeenCalled();
    });

    it("does not start extraction when an upload resolves after Cancel", async () => {
        const upload = deferred<object>();
        mockStart.mockResolvedValue({ job_id: "job-1" });
        mockUpload.mockReturnValue(upload.promise);

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, {
            target: { files: [makeFile("menu.png", "image/png", 1 * 1024 * 1024)] },
        });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockUpload).toHaveBeenCalledTimes(1));

        fireEvent.click(
            await screen.findByText("aiMenuOnboarding.pdfDigitizer.processing.cancel"),
        );
        await act(async () => {
            upload.resolve({});
            await upload.promise;
        });

        expect(mockProcess).not.toHaveBeenCalled();
        expect(mockJob).not.toHaveBeenCalled();
    });

    it("does not start polling when extraction initiation resolves after Cancel", async () => {
        const initiation = deferred<{ status: string }>();
        mockStart.mockResolvedValue({ job_id: "job-1" });
        mockUpload.mockResolvedValue({});
        mockProcess.mockReturnValue(initiation.promise);
        mockJob.mockResolvedValue({ status: "processing" });

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, {
            target: { files: [makeFile("menu.png", "image/png", 1 * 1024 * 1024)] },
        });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockProcess).toHaveBeenCalledWith(42, "job-1"));

        fireEvent.click(
            await screen.findByText("aiMenuOnboarding.pdfDigitizer.processing.cancel"),
        );
        jest.useFakeTimers();
        await act(async () => {
            initiation.resolve({ status: "processing" });
            await initiation.promise;
            jest.advanceTimersByTime(10_000);
            await Promise.resolve();
        });

        expect(mockJob).not.toHaveBeenCalled();
    });

    it("ignores an in-flight completed poll that resolves after Cancel", async () => {
        const initiation = deferred<{ status: string }>();
        const poll = deferred<{ status: string; extracted_menu: string }>();
        const onExtracted = jest.fn();
        mockStart.mockResolvedValue({ job_id: "job-1" });
        mockUpload.mockResolvedValue({});
        mockProcess.mockReturnValue(initiation.promise);
        mockJob.mockReturnValue(poll.promise);

        renderDigitizer(onExtracted);
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        fireEvent.change(input, {
            target: { files: [makeFile("menu.png", "image/png", 1 * 1024 * 1024)] },
        });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        const cancel = await screen.findByText(
            "aiMenuOnboarding.pdfDigitizer.processing.cancel",
        );
        jest.useFakeTimers();
        await act(async () => {
            initiation.resolve({ status: "processing" });
            await initiation.promise;
            await Promise.resolve();
            jest.advanceTimersByTime(2000);
            await Promise.resolve();
        });
        expect(mockJob).toHaveBeenCalledTimes(1);

        fireEvent.click(cancel);
        await act(async () => {
            poll.resolve({ status: "completed", extracted_menu: '{"categories":[]}' });
            await poll.promise;
        });

        expect(onExtracted).not.toHaveBeenCalled();
        expect(screen.queryByText("aiMenuOnboarding.pdfDigitizer.success.title")).not.toBeInTheDocument();
    });
});

describe("PDFDigitizer file snapshot (#576)", () => {
    beforeEach(() => {
        jest.clearAllMocks();
        (globalThis as { __pdfDigitizerI18n?: { locale: string; useRealTranslations: boolean } })
            .__pdfDigitizerI18n = { locale: "en", useRealTranslations: false };
        jest.spyOn(console, "error").mockImplementation(() => {});
    });

    afterEach(() => {
        (console.error as jest.Mock).mockRestore?.();
        jest.useRealTimers();
    });

    it("snapshots a valid small PDF on select and extracts from those bytes", async () => {
        stubCanvasExport();
        mockOnePagePdf();
        mockStart.mockReturnValue(new Promise(() => {}));

        const pdf = makePdfFile("carta.pdf", 45 * 1024);
        const arrayBuffer = attachArrayBuffer(pdf, new ArrayBuffer(45 * 1024));

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [pdf] } });
        });

        expect(arrayBuffer).toHaveBeenCalledTimes(1);
        expect(await screen.findByText("carta.pdf")).toBeInTheDocument();
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );

        await waitFor(() => expect(mockGetPdfDocument).toHaveBeenCalledTimes(1));
        const payload = mockGetPdfDocument.mock.calls[0][0] as { data: ArrayBuffer };
        expect(payload.data.byteLength).toBe(45 * 1024);
        expect(arrayBuffer).toHaveBeenCalledTimes(1);
        await waitFor(() => expect(mockStart).toHaveBeenCalledWith(42));
        expect(screen.queryByText(/NotFoundError/)).not.toBeInTheDocument();
    });

    it("does not mark a PDF ready until the byte snapshot resolves", async () => {
        const fileRead = deferred<ArrayBuffer>();
        const pdf = makePdfFile();
        const arrayBuffer = jest.fn(() => fileRead.promise);
        Object.defineProperty(pdf, "arrayBuffer", { value: arrayBuffer });

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [pdf] } });
        });

        expect(arrayBuffer).toHaveBeenCalledTimes(1);
        expect(screen.getByText("menu.pdf")).toBeInTheDocument();
        expect(
            screen.queryByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        ).not.toBeInTheDocument();
        expect(
            screen.queryByText(/aiMenuOnboarding\.pdfDigitizer\.processing\.extracting/),
        ).not.toBeInTheDocument();

        await act(async () => {
            fileRead.resolve(new ArrayBuffer(45 * 1024));
            await fileRead.promise;
        });

        expect(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        ).toBeInTheDocument();
        expect(mockStart).not.toHaveBeenCalled();
    });

    it("delayed extract still uses the snapshot after the File handle goes stale", async () => {
        stubCanvasExport();
        mockOnePagePdf();
        mockStart.mockReturnValue(new Promise(() => {}));

        const bytes = new Uint8Array(45 * 1024);
        bytes.set([0x25, 0x50, 0x44, 0x46]);
        const pdf = new File([bytes], "menu.pdf", { type: "application/pdf" });
        const arrayBuffer = attachArrayBuffer(pdf, () => Promise.resolve(bytes.buffer.slice(0)));
        arrayBuffer
            .mockResolvedValueOnce(bytes.buffer.slice(0))
            .mockRejectedValue(staleFileHandleError());

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [pdf] } });
        });
        expect(arrayBuffer).toHaveBeenCalledTimes(1);
        expect(await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/)).toBeInTheDocument();

        await act(async () => {
            await Promise.resolve();
            await Promise.resolve();
        });

        fireEvent.click(
            screen.getByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );

        await waitFor(() => expect(mockGetPdfDocument).toHaveBeenCalledTimes(1));
        expect(arrayBuffer).toHaveBeenCalledTimes(1);
        await waitFor(() => expect(mockStart).toHaveBeenCalledWith(42));
        expect(screen.queryByText(/NotFoundError/)).not.toBeInTheDocument();
    });

    it("maps a stale file handle to localized copy and never starts AI work", async () => {
        const pdf = makePdfFile();
        Object.defineProperty(pdf, "arrayBuffer", {
            value: jest.fn().mockRejectedValue(staleFileHandleError()),
        });

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [pdf] } });
        });

        expect(
            await screen.findByText("aiMenuOnboarding.pdfDigitizer.error.fileUnreadable"),
        ).toBeInTheDocument();
        expect(screen.getByText("menu.pdf")).toBeInTheDocument();
        expect(
            screen.queryByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        ).not.toBeInTheDocument();
        expect(screen.queryByText(/NotFoundError/)).not.toBeInTheDocument();
        expect(
            screen.queryByText(/aiMenuOnboarding\.pdfDigitizer\.processing\.extracting/),
        ).not.toBeInTheDocument();
        expect(
            screen.queryByText(/aiMenuOnboarding\.pdfDigitizer\.processing\.uploading/),
        ).not.toBeInTheDocument();
        expect(mockStart).not.toHaveBeenCalled();
        expect(mockProcess).not.toHaveBeenCalled();
    });

    it("retry after a later extract error reuses the snapshot and keeps the filename", async () => {
        stubCanvasExport();
        mockOnePagePdf();
        mockStart.mockRejectedValueOnce(new Error("network down"));

        const pdf = makePdfFile("menu.pdf");
        const arrayBuffer = attachArrayBuffer(pdf, new ArrayBuffer(45 * 1024));

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [pdf] } });
        });
        fireEvent.click(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );

        expect(
            await screen.findByText("aiMenuOnboarding.pdfDigitizer.error.tryAgain"),
        ).toBeInTheDocument();
        expect(screen.getByText("network down")).toBeInTheDocument();

        fireEvent.click(screen.getByText("aiMenuOnboarding.pdfDigitizer.error.tryAgain"));

        expect(await screen.findByText("menu.pdf")).toBeInTheDocument();
        expect(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        ).toBeInTheDocument();
        expect(arrayBuffer).toHaveBeenCalledTimes(1);

        mockStart.mockReturnValue(new Promise(() => {}));
        fireEvent.click(
            screen.getByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        );
        await waitFor(() => expect(mockStart).toHaveBeenCalledTimes(2));
        expect(arrayBuffer).toHaveBeenCalledTimes(1);
        expect(screen.queryByText(/NotFoundError/)).not.toBeInTheDocument();
    });

    it("retry after an unreadable file keeps the filename and allows reselection", async () => {
        const stale = makePdfFile("menu.pdf");
        Object.defineProperty(stale, "arrayBuffer", {
            value: jest.fn().mockRejectedValue(staleFileHandleError()),
        });

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [stale] } });
        });
        expect(
            await screen.findByText("aiMenuOnboarding.pdfDigitizer.error.fileUnreadable"),
        ).toBeInTheDocument();
        expect(screen.getByText("menu.pdf")).toBeInTheDocument();

        const recovered = makePdfFile("menu.pdf");
        await act(async () => {
            fireEvent.change(input, { target: { files: [recovered] } });
        });

        expect(await screen.findByText("menu.pdf")).toBeInTheDocument();
        expect(
            await screen.findByText(/aiMenuOnboarding\.pdfDigitizer\.extractButton/),
        ).toBeInTheDocument();
        expect(
            screen.queryByText("aiMenuOnboarding.pdfDigitizer.error.fileUnreadable"),
        ).not.toBeInTheDocument();
        expect(mockStart).not.toHaveBeenCalled();
    });

    it("shows ES-AR voseo copy for an unreadable file and never leaks NotFoundError", async () => {
        (globalThis as { __pdfDigitizerI18n?: { locale: string; useRealTranslations: boolean } })
            .__pdfDigitizerI18n = { locale: "es-AR", useRealTranslations: true };

        const pdf = makePdfFile("menu.pdf");
        Object.defineProperty(pdf, "arrayBuffer", {
            value: jest.fn().mockRejectedValue(staleFileHandleError()),
        });

        renderDigitizer();
        const input = document.querySelector('input[type="file"]') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [pdf] } });
        });

        const copy = getTranslation(
            "aiMenuOnboarding.pdfDigitizer.error.fileUnreadable",
            "es-AR",
        ) as string;
        expect(copy).toMatch(/Volvé/);
        expect(copy).not.toMatch(/\bVuelve\b/);
        expect(copy).not.toMatch(/NotFoundError/);
        expect(await screen.findByText(copy)).toBeInTheDocument();
        expect(screen.getByText("menu.pdf")).toBeInTheDocument();
        expect(screen.queryByText(/NotFoundError/)).not.toBeInTheDocument();
        expect(screen.queryByText(/La IA está extrayendo/)).not.toBeInTheDocument();
        expect(mockStart).not.toHaveBeenCalled();
    });
});
