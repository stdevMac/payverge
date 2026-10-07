"use client";

import React, { createContext, useContext, useCallback, useRef } from "react";
import toast from "react-hot-toast";
import { Check, X, AlertTriangle, Info } from "lucide-react";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import { getChromeTranslation } from "@/i18n/operatorChromeCatalog";

type ToastType = "success" | "error" | "warning" | "info";

interface Toast {
  id: string;
  type: ToastType;
  title: string;
  message?: string;
  duration?: number;
}

interface ToastContextType {
  showToast: (toast: Omit<Toast, "id">) => void;
  showSuccess: (title: string, message?: string, duration?: number) => void;
  showError: (title: string, message?: string, duration?: number) => void;
  showWarning: (title: string, message?: string, duration?: number) => void;
  showInfo: (title: string, message?: string, duration?: number) => void;
}

const ToastContext = createContext<ToastContextType | undefined>(undefined);

const MAX_VISIBLE_TOASTS = 4;
const URGENT_TYPES: ReadonlySet<ToastType> = new Set(["error", "warning"]);

const isUrgentToast = (type: ToastType): boolean => URGENT_TYPES.has(type);

const toastDuration = (duration?: number): number => {
  if (duration === 0) return Number.POSITIVE_INFINITY;
  return duration ?? 5000;
};

const formatToastBody = (title: string, message?: string): string =>
  message ? `${title}\n${message}` : title;

export const useToast = () => {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return context;
};

interface ToastProviderProps {
  children: React.ReactNode;
}

export const ToastProvider: React.FC<ToastProviderProps> = ({ children }) => {
  const { locale } = useSimpleLocale();
  const visibleRef = useRef<Array<{ id: string; type: ToastType }>>([]);

  const evictIfNeeded = useCallback((incomingType: ToastType) => {
    const visible = visibleRef.current;
    if (visible.length < MAX_VISIBLE_TOASTS) return;
    let removeIndex = visible.findIndex((item) => !isUrgentToast(item.type));
    if (removeIndex === -1 && isUrgentToast(incomingType)) {
      removeIndex = 0;
    } else if (removeIndex === -1) {
      removeIndex = 0;
    }
    const [evicted] = visible.splice(removeIndex, 1);
    if (evicted) toast.remove(evicted.id);
  }, []);

  const track = useCallback((id: string, type: ToastType) => {
    visibleRef.current.push({ id, type });
  }, []);

  const showCustom = useCallback(
    (next: Omit<Toast, "id">) => {
      evictIfNeeded(next.type);
      const id = toast.custom(
        (toastApi) => (
          <div
            role={next.type === "error" ? "alert" : "status"}
            className="flex max-w-md items-start gap-3 rounded-xl border border-warm-200 bg-white px-4 py-3 shadow-lg"
          >
            <span className="mt-0.5 shrink-0" aria-hidden>
              {next.type === "success" ? (
                <Check className="h-5 w-5 text-brand" />
              ) : next.type === "error" ? (
                <X className="h-5 w-5 text-rose-700" />
              ) : next.type === "warning" ? (
                <AlertTriangle className="h-5 w-5 text-amber-700" />
              ) : (
                <Info className="h-5 w-5 text-brand" />
              )}
            </span>
            <div className="min-w-0 flex-1">
              <h4 className="text-sm font-semibold text-ink-950">{next.title}</h4>
              {next.message ? (
                <p className="mt-1 text-sm text-ink-600">{next.message}</p>
              ) : null}
            </div>
            <button
              type="button"
              aria-label={String(
                getChromeTranslation("common.dismissToast", locale, {
                  title: next.title,
                }),
              )}
              onClick={(event) => {
                event.stopPropagation();
                toast.dismiss(toastApi.id);
                visibleRef.current = visibleRef.current.filter(
                  (item) => item.id !== toastApi.id,
                );
              }}
              className="shrink-0 text-ink-400 hover:text-ink-700"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        ),
        { duration: toastDuration(next.duration) },
      );
      track(id, next.type);
    },
    [evictIfNeeded, locale, track],
  );

  const showToast = useCallback(
    (next: Omit<Toast, "id">) => {
      if (next.type === "success") {
        evictIfNeeded("success");
        const id = toast.success(formatToastBody(next.title, next.message), {
          duration: toastDuration(next.duration),
        });
        track(id, "success");
        return;
      }
      if (next.type === "error") {
        evictIfNeeded("error");
        const id = toast.error(formatToastBody(next.title, next.message), {
          duration: toastDuration(next.duration),
        });
        track(id, "error");
        return;
      }
      showCustom(next);
    },
    [evictIfNeeded, showCustom, track],
  );

  const showSuccess = useCallback(
    (title: string, message?: string, duration?: number) => {
      showToast({ type: "success", title, message, duration });
    },
    [showToast],
  );

  const showError = useCallback(
    (title: string, message?: string, duration?: number) => {
      showToast({ type: "error", title, message, duration });
    },
    [showToast],
  );

  const showWarning = useCallback(
    (title: string, message?: string, duration?: number) => {
      showToast({ type: "warning", title, message, duration });
    },
    [showToast],
  );

  const showInfo = useCallback(
    (title: string, message?: string, duration?: number) => {
      showToast({ type: "info", title, message, duration });
    },
    [showToast],
  );

  return (
    <ToastContext.Provider
      value={{
        showToast,
        showSuccess,
        showError,
        showWarning,
        showInfo,
      }}
    >
      {children}
    </ToastContext.Provider>
  );
};
