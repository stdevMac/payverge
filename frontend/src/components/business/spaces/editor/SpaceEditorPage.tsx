"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Button, Input, Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, useDisclosure } from "@nextui-org/react";
import toast from "react-hot-toast";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import {
  parseLayoutDocument,
  spacesApi,
  type LayoutDocument,
  type Space,
  type SpaceTableRef,
} from "@/api/spaces";
import { createTableWithQR } from "@/api/business";
import type { LayoutElementType, TableShape } from "@/api/spaces";
import ConfirmationModal from "../../modals/ConfirmationModal";
import { EditorToolbar } from "./EditorToolbar";
import { ElementPalette } from "./ElementPalette";
import { FloorCanvas } from "./FloorCanvas";
import { PropertiesPanel } from "./PropertiesPanel";
import {
  DEFAULT_ROOM_HEIGHT_MM,
  DEFAULT_ROOM_WIDTH_MM,
  toMm,
  type Point,
} from "./geometry";
import { useAutosave } from "./hooks/useAutosave";
import { useCanvasViewport } from "./hooks/useCanvasViewport";
import { useEditorState } from "./hooks/useEditorState";
import { useKeyboardShortcuts } from "./hooks/useKeyboardShortcuts";
import {
  editorToLayout,
  layoutToEditor,
  type EditorDocument,
  type EditorTool,
  type PreviewMode,
} from "./types";
import { autoPlaceMissingTables } from "../autoPlaceTables";
import {
  EDITOR_CANVAS_COLUMN_STYLE,
  EDITOR_PALETTE_COLUMN_STYLE,
  EDITOR_PROPERTIES_COLUMN_STYLE,
  EDITOR_WORKSPACE_EDIT_STYLE,
  EDITOR_WORKSPACE_PREVIEW_STYLE,
} from "./editorLayout";

export interface SpaceEditorPageProps {
  businessId: number;
  spaceId: number;
  onClose: () => void;
  /** Preferred entry path from create flow. */
  initialPath?: "scan" | "draw" | null;
}

export default function SpaceEditorPage({
  businessId,
  spaceId,
  onClose,
  initialPath = null,
}: SpaceEditorPageProps) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`spacesTables.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [loading, setLoading] = useState(true);
  const [space, setSpace] = useState<Space | null>(null);
  const [tables, setTables] = useState<SpaceTableRef[]>([]);
  const [draftRevision, setDraftRevision] = useState(0);
  const [tool, setTool] = useState<EditorTool>("select");
  const [snapGrid, setSnapGrid] = useState(true);
  const [previewMode, setPreviewMode] = useState<PreviewMode>("edit");
  const [publishing, setPublishing] = useState(false);
  const [drawingPoints, setDrawingPoints] = useState<Point[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [conflictOpen, setConflictOpen] = useState(false);

  const roomModal = useDisclosure();
  const [roomW, setRoomW] = useState("12");
  const [roomH, setRoomH] = useState("8");
  const discardModal = useDisclosure();
  const publishModal = useDisclosure();

  const emptyDoc = useMemo(
    () =>
      layoutToEditor(null, {
        width_mm: 0,
        height_mm: 0,
        measurement_unit: "m",
      }),
    [],
  );

  const editor = useEditorState({ initial: emptyDoc });
  const { replaceDocument } = editor;
  const viewport = useCanvasViewport();
  const fittedRef = useRef(false);

  const getLayout = useCallback((): LayoutDocument => {
    return editorToLayout(editor.docRef.current);
  }, [editor.docRef]);

  const autosave = useAutosave({
    businessId,
    spaceId,
    draftRevision,
    onRevisionChange: setDraftRevision,
    getLayout,
    dirtyToken: editor.historyRevision,
    enabled: !loading && Boolean(space),
  });

  useEffect(() => {
    if (autosave.status === "conflict") {
      setConflictOpen(true);
    }
  }, [autosave.status]);

  // beforeunload when dirty
  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (autosave.isDirty || autosave.status === "dirty" || autosave.status === "saving") {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [autosave.isDirty, autosave.status]);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const [detail, draft] = await Promise.all([
        spacesApi.get(businessId, spaceId),
        spacesApi.getLayoutDraft(businessId, spaceId),
      ]);
      setSpace(detail.space);
      setTables(detail.tables);
      setDraftRevision(draft.draft_revision);
      const layout =
        parseLayoutDocument(draft.layout) ||
        parseLayoutDocument(detail.space.draft_layout_json) ||
        null;
      const doc = layoutToEditor(layout, {
        width_mm: draft.width_mm ?? detail.space.width_mm,
        height_mm: draft.height_mm ?? detail.space.height_mm,
        measurement_unit:
          draft.measurement_unit || detail.space.measurement_unit,
      });
      // Seed empty room dimensions if none yet and draw path
      if (!doc.width_mm || !doc.height_mm) {
        if (initialPath === "draw" || detail.tables.length > 0) {
          doc.width_mm = DEFAULT_ROOM_WIDTH_MM;
          doc.height_mm = DEFAULT_ROOM_HEIGHT_MM;
          doc.boundary = {
            points_mm: [
              { x: 0, y: 0 },
              { x: doc.width_mm, y: 0 },
              { x: doc.width_mm, y: doc.height_mm },
              { x: 0, y: doc.height_mm },
            ],
            closed: true,
          };
        }
      }
      // Seed assigned-but-unplaced tables in this session so venues with
      // existing QR tables don't start from an empty canvas (#236).
      // Do not PUT on open — persist only on explicit Save (or a later
      // user edit that trips autosave). Card counts already include
      // assigned tables via spaceCardStats without a draft write.
      const seeded = autoPlaceMissingTables(doc, detail.tables);
      replaceDocument(seeded.doc);
      fittedRef.current = false;
    } catch (err) {
      setLoadError(getSafeApiErrorMessage(err, t("toasts.loadError")));
    } finally {
      setLoading(false);
    }
  }, [businessId, initialPath, replaceDocument, spaceId, t]);

  useEffect(() => {
    void load();
  }, [load]);

  // Fit viewport when doc dimensions known
  useEffect(() => {
    if (loading || fittedRef.current) return;
    const w = editor.doc.width_mm;
    const h = editor.doc.height_mm;
    if (w > 0 && h > 0) {
      // Approximate canvas size; FloorCanvas also sizes itself
      viewport.fitToView(w, h, 900, 600, 64);
      fittedRef.current = true;
    }
  }, [loading, editor.doc.width_mm, editor.doc.height_mm, viewport]);

  /**
   * Candidates (table_id 0 / missing) are left for the backend atomic publish
   * materialize path — creating tables here then failing draft put orphans QR
   * rows. Palette "add table" still creates real tables immediately.
   */
  const ensurePendingTableIds = useCallback(
    async (doc: EditorDocument): Promise<EditorDocument> => doc,
    [],
  );

  const handleAddTableShape = useCallback(
    async (shape: TableShape) => {
      try {
        const name = `T${editor.doc.tables.length + 1}`;
        const created = await createTableWithQR(businessId, {
          name,
          capacity: shape === "bar" ? 6 : 4,
        });
        await spacesApi.assignTables(businessId, spaceId, {
          table_ids: [created.id],
        });
        const cx = (editor.doc.width_mm || DEFAULT_ROOM_WIDTH_MM) / 2 - 450;
        const cy = (editor.doc.height_mm || DEFAULT_ROOM_HEIGHT_MM) / 2 - 450;
        editor.addTable({
          table_id: created.id,
          name: created.name || name,
          shape,
          x_mm: Math.max(200, cx),
          y_mm: Math.max(200, cy),
          max_capacity: shape === "bar" ? 6 : 4,
          visible_seat_count: shape === "bar" ? 6 : 4,
        });
        setTables((prev) => [
          ...prev,
          {
            id: created.id,
            business_id: businessId,
            name: created.name || name,
            table_code: created.table_code || "",
            capacity: created.capacity || 4,
            is_active: true,
            space_id: spaceId,
          },
        ]);
        toast.success(t("editor.tableAdded"));
      } catch (err) {
        toast.error(getSafeApiErrorMessage(err, t("editor.tableAddError")));
      }
    },
    [businessId, spaceId, editor, t],
  );

  const handleAddElement = useCallback(
    (type: LayoutElementType) => {
      const at = {
        x: (editor.doc.width_mm || DEFAULT_ROOM_WIDTH_MM) * 0.15,
        y: (editor.doc.height_mm || DEFAULT_ROOM_HEIGHT_MM) * 0.15,
      };
      editor.addElement(type, at);
      setTool("select");
    },
    [editor],
  );

  const handlePolygonComplete = useCallback(
    (points: Point[]) => {
      if (tool === "draw-region") {
        editor.addRegion(points);
      } else if (tool === "draw-polygon" || tool === "draw-rect-room") {
        const xs = points.map((p) => p.x);
        const ys = points.map((p) => p.y);
        const minX = Math.min(...xs);
        const minY = Math.min(...ys);
        const maxX = Math.max(...xs);
        const maxY = Math.max(...ys);
        // Normalize to origin for room boundary
        const width = Math.max(1000, maxX - minX);
        const height = Math.max(1000, maxY - minY);
        const normalized =
          tool === "draw-rect-room"
            ? [
                { x: 0, y: 0 },
                { x: width, y: 0 },
                { x: width, y: height },
                { x: 0, y: height },
              ]
            : points.map((p) => ({ x: p.x - minX, y: p.y - minY }));
        editor.setRoomSize(width, height, {
          points_mm: normalized,
          closed: true,
        });
      }
      setDrawingPoints([]);
      setTool("select");
    },
    [tool, editor],
  );

  const handleRoomFromModal = () => {
    const unit = (space?.measurement_unit || "m") as "m" | "ft";
    const w = toMm(Number(roomW) || 12, unit);
    const h = toMm(Number(roomH) || 8, unit);
    editor.setRoomSize(Math.round(w), Math.round(h));
    roomModal.onClose();
    fittedRef.current = false;
  };

  const handlePublish = async () => {
    setPublishing(true);
    try {
      // Resolve temp table ids, save, then publish
      let doc = editor.docRef.current;
      doc = await ensurePendingTableIds(doc);
      editor.replaceDocument(doc);
      // replaceDocument clears history — re-mark for save
      const layout = editorToLayout(doc);
      const saved = await spacesApi.putLayoutDraft(businessId, spaceId, {
        expected_revision: draftRevision,
        layout,
      });
      setDraftRevision(saved.draft_revision);
      const published = await spacesApi.publishLayout(businessId, spaceId, {
        expected_revision: saved.draft_revision,
      });
      setSpace(published.space);
      setDraftRevision(published.space.draft_revision);
      toast.success(t("publish.success"));
      publishModal.onClose();
      autosave.markClean();
    } catch (err: unknown) {
      const ax = err as {
        response?: { data?: { code?: string; validation?: { issues?: { message: string }[] } } };
      };
      if (ax?.response?.data?.code === "revision_conflict") {
        setConflictOpen(true);
      } else if (ax?.response?.data?.code === "layout_invalid") {
        toast.error(t("publish.blocked"));
      } else {
        toast.error(getSafeApiErrorMessage(err, t("publish.error")));
      }
    } finally {
      setPublishing(false);
    }
  };

  const handleDiscard = async () => {
    try {
      const res = await spacesApi.discardLayout(businessId, spaceId, {
        expected_revision: draftRevision,
      });
      setSpace(res.space);
      setDraftRevision(res.space.draft_revision);
      const layout = parseLayoutDocument(res.space.draft_layout_json);
      editor.replaceDocument(
        layoutToEditor(layout, {
          width_mm: res.space.width_mm,
          height_mm: res.space.height_mm,
          measurement_unit: res.space.measurement_unit,
        }),
      );
      toast.success(t("editor.discarded"));
      discardModal.onClose();
      autosave.markClean();
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("editor.discardError")));
    }
  };

  const handleReloadConflict = async () => {
    setConflictOpen(false);
    await load();
    toast.success(t("editor.reloaded"));
  };

  useKeyboardShortcuts({
    enabled: previewMode === "edit" && !loading,
    onUndo: editor.undo,
    onRedo: editor.redo,
    onCopy: editor.copySelection,
    onPaste: () => {
      editor.pasteClipboard();
    },
    onDuplicate: editor.duplicateSelection,
    onDelete: editor.removeSelection,
    onNudge: editor.nudgeSelection,
    onEscape: () => {
      editor.clearSelection();
      setDrawingPoints([]);
      setTool("select");
    },
    onSave: () => {
      void autosave.saveNow();
    },
    onSelectAll: () => {
      editor.select([
        ...editor.doc.tables.map((t) => ({
          kind: "table" as const,
          key: t.clientKey,
        })),
        ...editor.doc.elements.map((e) => ({
          kind: "element" as const,
          key: e.clientKey,
        })),
        ...editor.doc.regions.map((r) => ({
          kind: "region" as const,
          key: r.clientKey,
        })),
      ]);
    },
  });

  const tableQrLinks = useMemo(() => {
    const map: Record<number, string> = {};
    for (const tbl of tables) {
      const code = (tbl as SpaceTableRef & { table_code?: string }).table_code;
      if (tbl.id && code) {
        map[tbl.id] = `/t/${code}`;
      }
    }
    return map;
  }, [tables]);

  const uniqueWarnings = useMemo(() => {
    const seen = new Set<string>();
    return editor.softWarnings.filter((w) => {
      if (seen.has(w.code)) return false;
      seen.add(w.code);
      return true;
    });
  }, [editor.softWarnings]);

  if (loading) {
    return (
      <div
        className="flex h-[70vh] items-center justify-center rounded-2xl border border-warm-200 bg-warm-50"
        data-testid="space-editor-loading"
        role="status"
      >
        <p className="text-sm text-ink-500">{t("editor.loading")}</p>
      </div>
    );
  }

  if (loadError || !space) {
    return (
      <div
        className="flex h-[50vh] flex-col items-center justify-center gap-3 rounded-2xl border border-warm-200 bg-white p-6"
        data-testid="space-editor-error"
      >
        <p className="text-sm text-rose-700">{loadError || t("toasts.loadError")}</p>
        <div className="flex gap-2">
          <Button size="sm" variant="bordered" onPress={onClose}>
            {t("editor.backToSpaces")}
          </Button>
          <Button size="sm" className="bg-brand text-white" onPress={() => void load()}>
            {t("editor.retry")}
          </Button>
        </div>
      </div>
    );
  }

  const needsRoom = !editor.doc.width_mm || !editor.doc.height_mm;

  return (
    <div
      className="flex h-[min(calc(100dvh-10rem),920px)] min-h-[560px] flex-col overflow-hidden rounded-2xl border border-warm-200 bg-white shadow-sm"
      data-testid="space-editor-page"
    >
      <EditorToolbar
        t={t}
        spaceName={space.name}
        tool={tool}
        onToolChange={setTool}
        snapGrid={snapGrid}
        onSnapGridChange={setSnapGrid}
        canUndo={editor.canUndo}
        canRedo={editor.canRedo}
        onUndo={editor.undo}
        onRedo={editor.redo}
        onDuplicate={editor.duplicateSelection}
        onDelete={editor.removeSelection}
        onBringForward={editor.bringForward}
        onSendBackward={editor.sendBackward}
        onZoomIn={() => viewport.zoomIn()}
        onZoomOut={() => viewport.zoomOut()}
        onFit={() =>
          viewport.fitToView(
            editor.doc.width_mm || DEFAULT_ROOM_WIDTH_MM,
            editor.doc.height_mm || DEFAULT_ROOM_HEIGHT_MM,
            900,
            600,
            64,
          )
        }
        autosaveStatus={autosave.status}
        onSaveNow={() => void autosave.saveNow()}
        onPublish={() => publishModal.onOpen()}
        onDiscard={() => discardModal.onOpen()}
        publishing={publishing}
        onBack={onClose}
        previewMode={previewMode}
        onPreviewModeChange={setPreviewMode}
        hasSelection={editor.selection.length > 0}
        canLayerOrder={editor.selection.some((s) => s.kind === "element")}
        warningsCount={uniqueWarnings.length}
      />

      {uniqueWarnings.length > 0 && previewMode === "edit" && (
        <div
          className="flex flex-wrap gap-2 border-b border-amber-100 bg-amber-50/80 px-3 py-1.5 text-xs text-amber-900"
          data-testid="editor-warnings"
        >
          {uniqueWarnings.map((w) => (
            <span key={w.code}>{t(w.messageKey)}</span>
          ))}
        </div>
      )}

      <div
        className="min-h-0 min-w-0 overflow-hidden"
        style={
          previewMode === "edit"
            ? EDITOR_WORKSPACE_EDIT_STYLE
            : EDITOR_WORKSPACE_PREVIEW_STYLE
        }
        data-testid="editor-workspace"
        data-layout={previewMode === "edit" ? "canvas-hero" : "preview"}
      >
        {previewMode === "edit" && (
          <div
            className="border-r border-warm-200"
            style={EDITOR_PALETTE_COLUMN_STYLE}
          >
            <ElementPalette
              t={t}
              onAddTableShape={(s) => void handleAddTableShape(s)}
              onAddElement={handleAddElement}
              onStartRegion={() => {
                setDrawingPoints([]);
                setTool("draw-region");
              }}
              onStartRoomRect={() => {
                roomModal.onOpen();
              }}
              onStartRoomPolygon={() => {
                setDrawingPoints([]);
                setTool("draw-polygon");
              }}
            />
          </div>
        )}

        <div style={EDITOR_CANVAS_COLUMN_STYLE} data-testid="editor-canvas-column">
          {needsRoom && (
            <div className="absolute inset-0 z-10 flex items-center justify-center bg-warm-50/90 p-6">
              <div className="max-w-md rounded-2xl border border-warm-200 bg-white p-6 shadow-lg">
                <h2 className="text-lg font-semibold text-ink-900">
                  {t("editor.setupRoomTitle")}
                </h2>
                <p className="mt-1 text-sm text-ink-600">
                  {t("editor.setupRoomBody")}
                </p>
                <div className="mt-4 flex flex-wrap gap-2">
                  <Button
                    className="bg-brand text-white"
                    radius="full"
                    size="sm"
                    onPress={() => roomModal.onOpen()}
                    data-testid="setup-room-rect"
                  >
                    {t("editor.palette.roomRect")}
                  </Button>
                  <Button
                    variant="bordered"
                    radius="full"
                    size="sm"
                    className="border-warm-200"
                    onPress={() => {
                      setDrawingPoints([]);
                      setTool("draw-polygon");
                    }}
                  >
                    {t("editor.palette.roomPoly")}
                  </Button>
                </div>
              </div>
            </div>
          )}
          <FloorCanvas
            doc={editor.doc}
            selection={editor.selection}
            isSelected={editor.isSelected}
            onSelect={editor.select}
            tool={tool}
            previewMode={previewMode}
            snapGrid={snapGrid}
            snapElements
            viewport={viewport}
            onBeginDrag={editor.beginDrag}
            onPreviewDoc={editor.previewDoc}
            onCommitDrag={editor.commitDrag}
            onCancelDrag={editor.cancelDrag}
            onPolygonComplete={handlePolygonComplete}
            drawingPoints={drawingPoints}
            onDrawingPoint={(p) => setDrawingPoints((prev) => [...prev, p])}
            onClearDrawing={() => setDrawingPoints([])}
            t={t}
          />
        </div>

        {previewMode === "edit" && editor.selection.length > 0 && (
          <div
            className="border-l border-warm-200"
            style={EDITOR_PROPERTIES_COLUMN_STYLE}
            data-testid="editor-properties-column"
            data-properties-placement="beside"
          >
            <PropertiesPanel
              t={t}
              doc={editor.doc}
              selection={editor.selection}
              onUpdateTable={editor.updateTable}
              onUpdateElement={editor.updateElement}
              onUpdateRegion={editor.updateRegion}
              tableQrLinks={tableQrLinks}
            />
          </div>
        )}
      </div>

      {/* Room size modal */}
      <Modal isOpen={roomModal.isOpen} onOpenChange={roomModal.onOpenChange} size="md">
        <ModalContent>
          {(onCloseModal) => (
            <>
              <ModalHeader>{t("editor.roomModalTitle")}</ModalHeader>
              <ModalBody>
                <p className="text-sm text-ink-600">{t("editor.roomModalBody")}</p>
                <div className="grid grid-cols-2 gap-3">
                  <Input
                    label={t("editor.properties.width")}
                    value={roomW}
                    onValueChange={setRoomW}
                    type="number"
                    variant="bordered"
                    endContent={
                      <span className="text-xs text-ink-400">
                        {space.measurement_unit}
                      </span>
                    }
                  />
                  <Input
                    label={t("editor.properties.height")}
                    value={roomH}
                    onValueChange={setRoomH}
                    type="number"
                    variant="bordered"
                    endContent={
                      <span className="text-xs text-ink-400">
                        {space.measurement_unit}
                      </span>
                    }
                  />
                </div>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onCloseModal}>
                  {t("create.cancel")}
                </Button>
                <Button
                  className="bg-brand text-white"
                  onPress={handleRoomFromModal}
                  data-testid="room-modal-apply"
                >
                  {t("editor.applyRoom")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <ConfirmationModal
        isOpen={discardModal.isOpen}
        onOpenChange={discardModal.onOpenChange}
        title={t("editor.discard")}
        description={t("editor.discardConfirm")}
        cancelLabel={t("confirm.cancel")}
        confirmLabel={t("editor.discard")}
        isDanger
        onConfirm={() => {
          void handleDiscard();
        }}
      />

      <ConfirmationModal
        isOpen={publishModal.isOpen}
        onOpenChange={publishModal.onOpenChange}
        title={t("editor.publish")}
        description={
          uniqueWarnings.length > 0
            ? `${t("publish.ready")} ${uniqueWarnings.map((w) => t(w.messageKey)).join(" · ")}`
            : t("publish.ready")
        }
        cancelLabel={t("confirm.cancel")}
        confirmLabel={publishing ? t("editor.publishing") : t("editor.publish")}
        onConfirm={() => {
          void handlePublish();
        }}
      />

      <Modal isOpen={conflictOpen} onOpenChange={setConflictOpen} size="md">
        <ModalContent>
          {() => (
            <>
              <ModalHeader>{t("editor.conflictTitle")}</ModalHeader>
              <ModalBody>
                <p className="text-sm text-ink-700">{t("editor.revisionConflict")}</p>
              </ModalBody>
              <ModalFooter>
                <Button
                  className="bg-brand text-white"
                  onPress={() => void handleReloadConflict()}
                  data-testid="conflict-reload"
                >
                  {t("editor.reload")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
