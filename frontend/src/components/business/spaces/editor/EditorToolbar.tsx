"use client";

import React from "react";
import {
  Button,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
} from "@nextui-org/react";
import {
  ArrowLeft,
  Check,
  CloudOff,
  Grid3x3,
  Hand,
  Loader2,
  Maximize2,
  MousePointer2,
  Save,
  ZoomIn,
  ZoomOut,
  AlertTriangle,
  Cloud,
} from "lucide-react";
import type { AutosaveStatus, EditorTool, PreviewMode } from "./types";
import { PreviewModes } from "./PreviewModes";
import { EDITOR_TOOLBAR_STYLE } from "./editorLayout";

interface EditorToolbarProps {
  t: (key: string, params?: Record<string, string | number>) => string;
  spaceName: string;
  tool: EditorTool;
  onToolChange: (tool: EditorTool) => void;
  snapGrid: boolean;
  onSnapGridChange: (v: boolean) => void;
  canUndo: boolean;
  canRedo: boolean;
  onUndo: () => void;
  onRedo: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
  onBringForward: () => void;
  onSendBackward: () => void;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFit: () => void;
  autosaveStatus: AutosaveStatus;
  onSaveNow: () => void;
  onPublish: () => void;
  onDiscard: () => void;
  publishing?: boolean;
  onBack: () => void;
  previewMode: PreviewMode;
  onPreviewModeChange: (m: PreviewMode) => void;
  hasSelection: boolean;
  /** True when selection includes layout elements (layer order only applies to elements). */
  canLayerOrder?: boolean;
  warningsCount: number;
}

function statusLabel(
  status: AutosaveStatus,
  t: EditorToolbarProps["t"],
): { text: string; className: string; icon: React.ReactNode } {
  switch (status) {
    case "saving":
      return {
        text: t("editor.saving"),
        className: "text-ink-500",
        icon: <Loader2 className="h-3.5 w-3.5 animate-spin" />,
      };
    case "saved":
      return {
        text: t("editor.saved"),
        className: "text-emerald-700",
        icon: <Check className="h-3.5 w-3.5" />,
      };
    case "dirty":
      return {
        text: t("editor.unsaved"),
        className: "text-amber-800",
        icon: <Cloud className="h-3.5 w-3.5" />,
      };
    case "offline":
      return {
        text: t("editor.offline"),
        className: "text-ink-500",
        icon: <CloudOff className="h-3.5 w-3.5" />,
      };
    case "failed":
      return {
        text: t("editor.saveFailed"),
        className: "text-rose-700",
        icon: <AlertTriangle className="h-3.5 w-3.5" />,
      };
    case "conflict":
      return {
        text: t("editor.revisionConflict"),
        className: "text-rose-700",
        icon: <AlertTriangle className="h-3.5 w-3.5" />,
      };
    default:
      return {
        text: t("editor.saved"),
        className: "text-ink-500",
        icon: <Check className="h-3.5 w-3.5" />,
      };
  }
}

function ToolBtn({
  active,
  onClick,
  label,
  children,
  disabled,
  testId,
}: {
  active?: boolean;
  onClick: () => void;
  label: string;
  children: React.ReactNode;
  disabled?: boolean;
  testId?: string;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={active}
      disabled={disabled}
      data-testid={testId}
      onClick={onClick}
      className={`inline-flex h-8 w-8 items-center justify-center rounded-lg transition-colors disabled:opacity-40 ${
        active
          ? "bg-brand/15 text-brand"
          : "text-ink-600 hover:bg-warm-100 hover:text-ink-900"
      }`}
    >
      {children}
    </button>
  );
}

export function EditorToolbar({
  t,
  spaceName,
  tool,
  onToolChange,
  snapGrid,
  onSnapGridChange,
  canUndo,
  canRedo,
  onUndo,
  onRedo,
  onDuplicate,
  onDelete,
  onBringForward,
  onSendBackward,
  onZoomIn,
  onZoomOut,
  onFit,
  autosaveStatus,
  onSaveNow,
  onPublish,
  onDiscard,
  publishing,
  onBack,
  previewMode,
  onPreviewModeChange,
  hasSelection,
  canLayerOrder = false,
  warningsCount,
}: EditorToolbarProps) {
  const status = statusLabel(autosaveStatus, t);

  return (
    <header
      className="gap-2 border-b border-warm-200 bg-white px-3 py-2"
      style={EDITOR_TOOLBAR_STYLE}
      data-testid="editor-toolbar"
    >
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1.5 rounded-full px-2 py-1.5 text-sm font-medium text-ink-700 hover:bg-warm-50"
        data-testid="editor-back"
      >
        <ArrowLeft className="h-4 w-4" />
        <span className="hidden sm:inline">{t("editor.backToSpaces")}</span>
      </button>

      <div className="min-w-0 flex-1">
        <h1 className="truncate text-sm font-semibold text-ink-900">
          {spaceName}
        </h1>
        <p className="text-[11px] text-ink-500">{t("editor.title")}</p>
      </div>

      <PreviewModes
        mode={previewMode}
        onChange={onPreviewModeChange}
        t={t}
      />

      <div className="flex items-center gap-0.5 rounded-xl border border-warm-200 bg-warm-50/60 p-0.5">
        <ToolBtn
          active={tool === "select"}
          onClick={() => onToolChange("select")}
          label={t("editor.selectTool")}
          testId="tool-select"
        >
          <MousePointer2 className="h-4 w-4" />
        </ToolBtn>
        <ToolBtn
          active={tool === "pan"}
          onClick={() => onToolChange("pan")}
          label={t("editor.panTool")}
          testId="tool-pan"
        >
          <Hand className="h-4 w-4" />
        </ToolBtn>
        <ToolBtn
          active={snapGrid}
          onClick={() => onSnapGridChange(!snapGrid)}
          label={t("editor.gridSnap")}
          testId="tool-snap"
        >
          <Grid3x3 className="h-4 w-4" />
        </ToolBtn>
      </div>

      <div className="flex items-center gap-0.5 rounded-xl border border-warm-200 bg-warm-50/60 p-0.5">
        <ToolBtn onClick={onZoomOut} label={t("editor.zoomOut")}>
          <ZoomOut className="h-4 w-4" />
        </ToolBtn>
        <ToolBtn onClick={onZoomIn} label={t("editor.zoomIn")}>
          <ZoomIn className="h-4 w-4" />
        </ToolBtn>
        <ToolBtn onClick={onFit} label={t("editor.fitView")} testId="tool-fit">
          <Maximize2 className="h-4 w-4" />
        </ToolBtn>
      </div>

      <div
        className={`hidden items-center gap-1.5 text-xs sm:flex ${status.className}`}
        data-testid="autosave-status"
      >
        {status.icon}
        <span className="max-w-[10rem] truncate">{status.text}</span>
      </div>

      {warningsCount > 0 && (
        <span
          className="rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-900"
          title={t("warnings.generic")}
        >
          {t("editor.warningsCount", { count: warningsCount })}
        </span>
      )}

      <button
        type="button"
        onClick={onSaveNow}
        className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-ink-600 hover:bg-warm-100"
        title={t("editor.saveNow")}
        aria-label={t("editor.saveNow")}
        data-testid="tool-save"
      >
        <Save className="h-4 w-4" />
      </button>

      <Dropdown>
        <DropdownTrigger>
          <Button size="sm" variant="bordered" radius="full" className="border-warm-200">
            {t("editor.more")}
          </Button>
        </DropdownTrigger>
        <DropdownMenu aria-label={t("editor.more")}>
          <DropdownItem key="undo" isDisabled={!canUndo} onPress={onUndo}>
            {t("editor.undo")}
          </DropdownItem>
          <DropdownItem key="redo" isDisabled={!canRedo} onPress={onRedo}>
            {t("editor.redo")}
          </DropdownItem>
          <DropdownItem
            key="duplicate"
            isDisabled={!hasSelection}
            onPress={onDuplicate}
          >
            {t("editor.duplicate")}
          </DropdownItem>
          <DropdownItem
            key="delete"
            isDisabled={!hasSelection}
            onPress={onDelete}
          >
            {t("editor.delete")}
          </DropdownItem>
          <DropdownItem
            key="bring-forward"
            isDisabled={!canLayerOrder}
            onPress={onBringForward}
          >
            {t("editor.bringForward")}
          </DropdownItem>
          <DropdownItem
            key="send-backward"
            isDisabled={!canLayerOrder}
            onPress={onSendBackward}
          >
            {t("editor.sendBackward")}
          </DropdownItem>
          <DropdownItem key="discard" className="text-danger" color="danger" onPress={onDiscard}>
            {t("editor.discard")}
          </DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Button
        size="sm"
        radius="full"
        className="bg-brand font-medium text-white"
        isLoading={publishing}
        onPress={onPublish}
        data-testid="editor-publish"
      >
        {publishing ? t("editor.publishing") : t("editor.publish")}
      </Button>
    </header>
  );
}
