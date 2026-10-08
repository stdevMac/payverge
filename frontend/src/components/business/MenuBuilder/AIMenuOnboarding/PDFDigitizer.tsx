"use client";

import React, { useEffect, useRef, useState } from "react";
import { Button, Progress, Chip } from "@nextui-org/react";
import { Upload, FileText, AlertCircle, CheckCircle2, FileUp, Cpu, Languages, Sparkles } from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";
import {
    startMenuExtraction,
    uploadMenuPage,
    processMenuExtraction,
    getMenuExtractionJob,
    ExtractedMenu
} from "@/api/business";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { errMessage } from "@/utils/apiError";
import { PremiumPanel } from "../../premium";

interface PDFDigitizerProps {
    businessId: number;
    onExtracted: (menu: ExtractedMenu) => void;
}

interface PageImage {
    pageNumber: number;
    blob: Blob;
    mimeType: string;
}

type ProcessingStep = "idle" | "loading-pdf" | "converting" | "uploading" | "extracting" | "complete" | "error";

// F14: bound the input so a huge multi-page menu PDF can't freeze/OOM the tab
// (operators are often on tablets/phones). The dropzone copy already advertises
// "up to 12 pages", so 12 is the page cap; 15 MB covers a generous scanned menu.
const MAX_FILE_BYTES = 15 * 1024 * 1024;
const MAX_FILE_MB = 15;
const MAX_PDF_PAGES = 12;

type SelectedMenuFile = {
    name: string;
    type: string;
    size: number;
    bytes: ArrayBuffer | null;
};

function isBrowserFileReadError(err: unknown): boolean {
    if (typeof err !== "object" || err === null) return false;
    const name =
        "name" in err && typeof (err as { name: unknown }).name === "string"
            ? (err as { name: string }).name
            : "";
    const message = err instanceof Error ? err.message : "";
    return (
        /^(NotFoundError|NotReadableError|SecurityError)$/i.test(name) ||
        /NotFoundError|NotReadableError/i.test(message)
    );
}

function readBlobBytes(file: Blob): Promise<ArrayBuffer> {
    if (typeof file.arrayBuffer === "function") {
        return file.arrayBuffer();
    }
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            if (reader.result instanceof ArrayBuffer) {
                resolve(reader.result);
                return;
            }
            reject(new Error("Failed to read file"));
        };
        reader.onerror = () => {
            reject(reader.error ?? new Error("Failed to read file"));
        };
        reader.readAsArrayBuffer(file);
    });
}

export default function PDFDigitizer({
    businessId,
    onExtracted,
}: PDFDigitizerProps) {
    const [selected, setSelected] = useState<SelectedMenuFile | null>(null);
    const [isReadingFile, setIsReadingFile] = useState(false);
    const [step, setStep] = useState<ProcessingStep>("idle");
    const [progress, setProgress] = useState(0);
    const [error, setError] = useState<string | null>(null);
    const [isDragging, setIsDragging] = useState(false);
    const { locale } = useSimpleLocale();
    const t = (key: string, params?: Record<string, string | number>) => getTranslation(`aiMenuOnboarding.pdfDigitizer.${key}`, locale, params);

    // F13: own the poll interval in a ref so it can be torn down on unmount
    // (tab switch / modal close) instead of running for up to 5 minutes against
    // an unmounted component.
    const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const isMountedRef = useRef(false);
    const runGenerationRef = useRef(0);
    const readGenerationRef = useRef(0);

    const stopPolling = () => {
        if (pollIntervalRef.current) {
            clearInterval(pollIntervalRef.current);
            pollIntervalRef.current = null;
        }
    };

    useEffect(() => {
        isMountedRef.current = true;
        return () => {
            isMountedRef.current = false;
            runGenerationRef.current += 1;
            readGenerationRef.current += 1;
            stopPolling();
        };
    }, []);

    const isRunActive = (runGeneration: number) =>
        isMountedRef.current && runGenerationRef.current === runGeneration;

    // F14: reject oversized files / over-long PDFs at selection time, before any
    // canvas rendering or upload happens. Returns true when the file is OK.
    const validateFile = (candidate: File): boolean => {
        if (candidate.size > MAX_FILE_BYTES) {
            setError(t("error.fileTooLarge", { max: MAX_FILE_MB }) as string);
            return false;
        }
        return true;
    };

    const snapshotSelectedFile = async (candidate: File) => {
        if (!validateFile(candidate)) return;
        const readGeneration = readGenerationRef.current + 1;
        readGenerationRef.current = readGeneration;
        setSelected({
            name: candidate.name,
            type: candidate.type,
            size: candidate.size,
            bytes: null,
        });
        setIsReadingFile(true);
        setError(null);
        setStep("idle");

        try {
            // Cloud-staged File handles can throw NotFoundError if we defer
            // arrayBuffer() until Extract. Snapshot now, before marking ready.
            const bytes = await readBlobBytes(candidate);
            if (!isMountedRef.current || readGeneration !== readGenerationRef.current) return;
            setSelected({
                name: candidate.name,
                type: candidate.type,
                size: candidate.size,
                bytes: bytes.slice(0),
            });
            setIsReadingFile(false);
        } catch {
            if (!isMountedRef.current || readGeneration !== readGenerationRef.current) return;
            setSelected({
                name: candidate.name,
                type: candidate.type,
                size: candidate.size,
                bytes: null,
            });
            setIsReadingFile(false);
            setError(t("error.fileUnreadable") as string);
        }
    };

    const handleDrop = (e: React.DragEvent) => {
        e.preventDefault();
        setIsDragging(false);
        const droppedFile = e.dataTransfer.files[0];
        if (droppedFile && (droppedFile.type === "application/pdf" || droppedFile.type.startsWith("image/"))) {
            void snapshotSelectedFile(droppedFile);
        } else {
            setError(t("error.invalidFile") as string);
        }
    };

    const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
        const selectedFile = e.target.files?.[0];
        if (selectedFile) {
            void snapshotSelectedFile(selectedFile);
        }
        e.target.value = "";
    };

    const convertPdfToImages = async (
        pdfBytes: ArrayBuffer,
        runGeneration: number,
    ): Promise<PageImage[]> => {
        // Use the legacy build to avoid top-level await issues
        const pdfjsLib = await import("pdfjs-dist/legacy/build/pdf.mjs");
        if (!isRunActive(runGeneration)) return [];
        pdfjsLib.GlobalWorkerOptions.workerSrc = "/pdf/pdf.worker.min.mjs";

        const data = pdfBytes.slice(0);
        if (!isRunActive(runGeneration)) return [];
        const pdf = await pdfjsLib.getDocument({ data }).promise;
        if (!isRunActive(runGeneration)) return [];

        // F14: refuse over-long PDFs before rendering any pages — a 30-100 page
        // menu would render dozens of full-resolution canvases and stall the tab.
        if (pdf.numPages > MAX_PDF_PAGES) {
            throw new Error(t("error.tooManyPages", { max: MAX_PDF_PAGES }) as string);
        }

        const images: PageImage[] = [];

        for (let i = 1; i <= pdf.numPages; i++) {
            if (!isRunActive(runGeneration)) return images;
            setProgress(Math.round((i / pdf.numPages) * 30));
            const page = await pdf.getPage(i);
            if (!isRunActive(runGeneration)) return images;
            const scale = 2;
            const viewport = page.getViewport({ scale });
            const canvas = document.createElement("canvas");
            const context = canvas.getContext("2d")!;
            canvas.width = viewport.width;
            canvas.height = viewport.height;
            await page.render({ canvasContext: context, viewport }).promise;
            if (!isRunActive(runGeneration)) return images;
            const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, "image/jpeg", 0.8));
            if (!isRunActive(runGeneration)) return images;
            if (blob) {
                images.push({ pageNumber: i, blob, mimeType: "image/jpeg" });
            }
        }
        return images;
    };

    const messageForProcessError = (err: unknown): string => {
        if (isBrowserFileReadError(err)) {
            return t("error.fileUnreadable") as string;
        }
        const raw = errMessage(err) ?? "";
        if (/NotFoundError|NotReadableError/i.test(raw)) {
            return t("error.fileUnreadable") as string;
        }
        return raw || (t("error.processFileFailed") as string);
    };

    const processFile = async () => {
        if (!selected?.bytes) {
            setError(t("error.fileUnreadable") as string);
            return;
        }
        const snapshot = selected.bytes;
        const mimeType = selected.type;
        const runGeneration = runGenerationRef.current + 1;
        runGenerationRef.current = runGeneration;
        stopPolling();

        try {
            setStep("converting");
            setProgress(0);
            setError(null);

            let pageImages: PageImage[];

            if (mimeType === "application/pdf") {
                setStep("loading-pdf");
                pageImages = await convertPdfToImages(snapshot, runGeneration);
            } else {
                setStep("loading-pdf");
                pageImages = [{
                    pageNumber: 1,
                    blob: new Blob([snapshot], { type: mimeType }),
                    mimeType,
                }];
                setProgress(30);
            }
            if (!isRunActive(runGeneration)) return;

            setStep("uploading");
            const { job_id } = await startMenuExtraction(businessId);
            if (!isRunActive(runGeneration)) return;

            const totalPages = pageImages.length;
            for (let i = 0; i < totalPages; i++) {
                if (!isRunActive(runGeneration)) return;
                const img = pageImages[i];
                await uploadMenuPage(businessId, job_id, img.blob, img.pageNumber);
                if (!isRunActive(runGeneration)) return;
                const uploadProgress = 30 + Math.round(((i + 1) / totalPages) * 50);
                setProgress(uploadProgress);
            }

            if (!isRunActive(runGeneration)) return;
            setStep("extracting");
            setProgress(85);

            try {
                await processMenuExtraction(businessId, job_id);
            } catch {
                if (!isRunActive(runGeneration)) return;
                setStep("error");
                setError(t("error.startExtractionFailed") as string);
                return;
            }
            if (!isRunActive(runGeneration)) return;

            let attempts = 0;
            const maxAttempts = 150;

            // F13: stored in a ref so unmount cleanup (tab switch / modal close)
            // and the Cancel button can stop the poll. Any prior poll is cleared
            // first so we never run two loops at once.
            stopPolling();
            pollIntervalRef.current = setInterval(async () => {
                attempts++;
                try {
                    const job = await getMenuExtractionJob(businessId, job_id);
                    if (!isRunActive(runGeneration)) return;
                    if (job.status === 'completed') {
                        stopPolling();
                        setProgress(100);
                        setStep("complete");
                        if (job.extracted_menu) {
                            try {
                                const menu = JSON.parse(job.extracted_menu);
                                onExtracted(menu);
                            } catch (e) {
                                setError(t("error.parseExtractedMenu") as string);
                                setStep("error");
                            }
                        }
                    } else if (job.status === 'failed') {
                        stopPolling();
                        setStep("error");
                        setError(job.error_message || (t("error.extractionFailed") as string));
                    } else {
                        setProgress(prev => Math.min(prev + 0.5, 98));
                    }
                    if (attempts >= maxAttempts) {
                        stopPolling();
                        setStep("error");
                        setError(t("error.timeout") as string);
                    }
                } catch (e) {
                    if (isRunActive(runGeneration)) console.error(e);
                }
            }, 2000);

        } catch (err: unknown) {
            if (!isRunActive(runGeneration)) return;
            setStep("error");
            setError(messageForProcessError(err));
        }
    };

    const resetState = () => {
        runGenerationRef.current += 1;
        readGenerationRef.current += 1;
        stopPolling();
        setSelected(null);
        setIsReadingFile(false);
        setStep("idle");
        setProgress(0);
        setError(null);
    };

    const returnToReady = () => {
        runGenerationRef.current += 1;
        stopPolling();
        setIsReadingFile(false);
        setStep("idle");
        setProgress(0);
        if (selected?.bytes) {
            setError(null);
        }
    };


    return (
        <div className="space-y-6">
            <AnimatePresence mode="wait">
                {step === "idle" && (
                    <motion.div
                        key="idle"
                        initial={{ opacity: 0, y: 10 }}
                        animate={{ opacity: 1, y: 0 }}
                        exit={{ opacity: 0, scale: 0.95 }}
                        className="space-y-6"
                    >
                        <div
                            className={`
                                group relative rounded-3xl border-3 border-dashed p-10 text-center transition-all duration-300 sm:p-16
                                ${isDragging
                                    ? "border-brand bg-brand/5 scale-[1.01] shadow-2xl shadow-brand/10"
                                    : "border-warm-200 bg-white/70 hover:border-brand/50 hover:bg-brand/[0.02]"
                                }
                            `}
                            role="button"
                            tabIndex={-1}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter' || e.key === ' ') {
                                    e.preventDefault();
                                    (e.currentTarget.querySelector('input') as HTMLInputElement)?.click();
                                }
                            }}
                            onDragOver={(e) => { e.preventDefault(); setIsDragging(true); }}
                            onDragLeave={() => setIsDragging(false)}
                            onDrop={handleDrop}
                        >
                            <input
                                type="file"
                                accept=".pdf,image/*"
                                className="absolute inset-0 z-10 h-full w-full cursor-pointer opacity-0"
                                onChange={handleFileSelect}
                            />

                            <motion.div
                                initial={{ opacity: 0, scale: 0.9 }}
                                animate={{ opacity: 1, scale: 1 }}
                            >
                                <motion.div
                                    className="flex flex-col items-center justify-center pt-5 pb-6 text-center"
                                    animate={isDragging ? { scale: 1.02 } : { scale: 1 }}
                                >
                                    <div className={`mb-4 rounded-2xl p-4 transition-colors duration-300 ${isDragging ? "bg-brand/10 text-brand" : "bg-warm-100 text-ink-500"}`}>
                                        <Upload className={`h-10 w-10 transition-transform duration-500 ${isDragging ? "scale-110 rotate-5" : ""}`} />
                                    </div>
                                    <p className="mb-2 font-title text-lg font-semibold tracking-0 text-ink-950">
                                        {t("dropzone.title")}
                                    </p>
                                    <p className="text-sm text-ink-600">
                                        {t("dropzone.subtitle")}
                                    </p>
                                </motion.div>
                            </motion.div>

                            {selected && (
                                <motion.div
                                    initial={{ opacity: 0, scale: 0.9 }}
                                    animate={{ opacity: 1, scale: 1 }}
                                    className="mt-8 flex flex-col items-center justify-center gap-2"
                                >
                                    <Chip
                                        color="primary"
                                        variant="flat"
                                        className="h-10 bg-brand/10 px-4 font-medium text-brand"
                                        onClose={() => resetState()}
                                    >
                                        {selected.name}
                                    </Chip>
                                    {isReadingFile && (
                                        <p className="text-sm text-ink-600">{t("readingFile")}</p>
                                    )}
                                </motion.div>
                            )}
                        </div>

                        {/* Selection-time validation errors (invalid file, too large,
                            too many pages) stay on the dropzone — they never reach the
                            dedicated error step, so surface them here. */}
                        {error && (
                            <div className="flex items-center justify-center gap-2 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-medium text-rose-700">
                                <AlertCircle className="h-4 w-4 flex-shrink-0" />
                                <span>{error}</span>
                            </div>
                        )}

                        {selected?.bytes && !isReadingFile && (
                            <motion.div
                                initial={{ opacity: 0, y: 10 }}
                                animate={{ opacity: 1, y: 0 }}
                                className="flex justify-center"
                            >
                                <Button
                                    size="lg"
                                    className="h-14 rounded-2xl bg-brand px-12 font-semibold text-white shadow-xl shadow-brand/20 transition-all duration-300 hover:bg-brand-dark hover:shadow-brand/40"
                                    onPress={processFile}
                                    startContent={<Sparkles className="h-5 w-5" />}
                                >
                                    {t("extractButton")}
                                </Button>
                            </motion.div>
                        )}
                    </motion.div>
                )}

                {(["loading-pdf", "converting", "uploading", "extracting"] as ProcessingStep[]).includes(step) && (
                    <motion.div
                        key="processing"
                        initial={{ opacity: 0, scale: 0.95 }}
                        animate={{ opacity: 1, scale: 1 }}
                        exit={{ opacity: 0, y: -20 }}
                        className="w-full"
                    >
                        <PremiumPanel className="overflow-hidden" withTexture>
                            {/* Scanning beam effect during extraction */}
                            {step === "extracting" && (
                                <motion.div
                                    className="absolute inset-x-0 z-20 h-1 bg-brand/50"
                                    animate={{ top: ["0%", "100%", "0%"] }}
                                    transition={{ duration: 3, repeat: Infinity, ease: "linear" }}
                                />
                            )}

                            <div className="p-10">
                                <div className="flex flex-col items-center gap-6 mb-8 text-center">
                                    <div className="flex h-16 w-16 items-center justify-center rounded-2xl border border-warm-200 bg-white text-brand shadow-xl">
                                        {step === "loading-pdf" && <FileText className="h-8 w-8 motion-safe:animate-pulse" />}
                                        {step === "converting" && <Languages className="h-8 w-8 animate-bounce" />}
                                        {step === "uploading" && <FileUp className="h-8 w-8" />}
                                        {step === "extracting" && <Cpu className="h-8 w-8 rotate-animation" />}
                                    </div>

                                    <div>
                                        <p className="mb-2 font-title text-2xl font-semibold tracking-0 text-ink-950">
                                            {t(`processing.${step === "loading-pdf" ? "loadingPdf" : step === "converting" ? "converting" : step === "uploading" ? "uploading" : "extracting"}`)}
                                        </p>
                                        <p className="mx-auto max-w-sm leading-relaxed text-ink-600">
                                            {step === "extracting"
                                                ? (progress > 90 ? t("processing.finalizing") : t("processing.extractingSubtitle"))
                                                : t("processing.waiting")}
                                        </p>
                                    </div>
                                </div>

                                <div className="max-w-md mx-auto space-y-2">
                                    <div className="flex justify-between text-sm font-bold text-brand mb-1">
                                        <span>{t("processing.progressLabel")}</span>
                                        <span>{Math.round(progress)}%</span>
                                    </div>
                                    <Progress
                                        value={progress}
                                        aria-label={t("processing.progressLabel") as string}
                                        size="md"
                                        classNames={{
                                            indicator: "bg-brand shadow-brand/20",
                                            track: "bg-warm-100"
                                        }}
                                    />
                                    <div className="flex justify-center pt-2">
                                        <Button
                                            size="sm"
                                            variant="light"
                                            className="text-xs font-medium text-ink-600 hover:bg-brand/10 hover:text-brand"
                                            onPress={returnToReady}
                                        >
                                            {t("processing.cancel") as string}
                                        </Button>
                                    </div>
                                </div>
                            </div>
                        </PremiumPanel>
                    </motion.div>
                )}

                {step === "error" && (
                    <motion.div
                        key="error"
                        initial={{ opacity: 0, x: -10 }}
                        animate={{ opacity: 1, x: 0, y: [0, -5, 5, -5, 0] }}
                        className="w-full"
                    >
                        <PremiumPanel tone="urgent" className="p-8" withTexture>
                                <div className="flex flex-col items-center text-center gap-4">
                                    <div className="flex h-16 w-16 items-center justify-center rounded-2xl border border-rose-200 bg-rose-50 text-rose-600">
                                        <AlertCircle className="h-8 w-8" />
                                    </div>
                                    <div>
                                        <p className="mb-1 font-title text-xl font-semibold tracking-0 text-rose-700">
                                            {t("error.title")}
                                        </p>
                                        <p className="mx-auto max-w-sm text-ink-600">{error}</p>
                                    </div>
                                    <Button
                                        className="mt-2 border border-rose-200 bg-white font-semibold text-rose-700 shadow-sm hover:bg-rose-50"
                                        onPress={returnToReady}
                                    >
                                        {t("error.tryAgain")}
                                    </Button>
                                </div>
                        </PremiumPanel>
                    </motion.div>
                )}

                {step === "complete" && (
                    <motion.div
                        key="complete"
                        initial={{ opacity: 0, scale: 0.9 }}
                        animate={{ opacity: 1, scale: 1 }}
                        className="w-full"
                    >
                        <PremiumPanel className="overflow-hidden p-8" withTexture>
                                <div className="flex flex-col items-center text-center gap-4">
                                    <motion.div
                                        initial={{ scale: 0 }}
                                        animate={{ scale: 1 }}
                                        transition={{ type: "spring", damping: 12 }}
                                        className="mx-auto mb-6 flex h-20 w-20 items-center justify-center rounded-2xl border border-emerald-200 bg-emerald-50 text-emerald-700"
                                    >
                                        <CheckCircle2 className="h-12 w-12" />
                                    </motion.div>
                                    <div>
                                        <p className="mb-1 font-title text-xl font-semibold tracking-0 text-emerald-700">
                                            {t("success.title")}
                                        </p>
                                        <p className="text-ink-600">{t("success.subtitle")}</p>
                                    </div>
                                </div>
                        </PremiumPanel>
                    </motion.div>
                )}
            </AnimatePresence>

            <style jsx>{`
                @keyframes rotate-animation {
                    from { transform: rotate(0deg); }
                    to { transform: rotate(360deg); }
                }
                .rotate-animation {
                    animation: rotate-animation 4s linear infinite;
                }
            `}</style>
        </div>
    );
}
