"use client";

import React from "react";
import { Copy, Pencil } from "lucide-react";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
export interface UserBubbleProps {
  content: string;
  onCopy: (text: string) => void;
  onEdit: (text: string) => void;
}

export default function UserBubble({
  content,
  onCopy,
  onEdit,
}: UserBubbleProps) {
  const { locale } = useSimpleLocale();
  const t = (key: string): string => {
    const value = getTranslation(`directorConsole.${key}`, locale);
    return Array.isArray(value) ? value[0] || key : (value as string);
  };
  return (
    <div data-testid="dc-user-bubble" className="group flex justify-end gap-2">
      <div className="rounded-2xl rounded-br-md bg-brand/10 text-ink-900 px-4 py-3 max-w-[72ch] whitespace-pre-wrap text-body">
        <span className="sr-only">{t("chat.speakerYou")}: </span>
        {content}
      </div>
      <div className="opacity-0 group-hover:opacity-100 flex flex-col gap-1 transition-opacity">
        <button
          type="button"
          aria-label={t("actions.copy")}
          onClick={() => onCopy(content)}
          className="p-1 rounded text-ink-400 hover:text-ink-700"
        >
          <Copy className="w-3.5 h-3.5" />
        </button>
        <button
          type="button"
          aria-label={t("actions.edit")}
          onClick={() => onEdit(content)}
          className="p-1 rounded text-ink-400 hover:text-ink-700"
        >
          <Pencil className="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  );
}
