"use client";

import React from "react";
import {
  Search,
  CornerDownLeft,
  Lock,
  Home,
  Receipt,
  ChefHat,
  Calendar,
  Bot,
  Settings,
  TrendingUp,
  Utensils,
  QrCode,
  UserCircle,
  Truck,
  Coffee,
  Boxes,
  Users,
  Globe,
  FileText,
  CreditCard,
  BarChart3,
  ExternalLink,
  Copy,
  GraduationCap,
  CircleHelp,
  Banknote,
  Printer,
  Megaphone,
  CalendarClock,
  type LucideIcon,
} from "lucide-react";
import { Command, rankCommands } from "./commandRegistry";

const ICONS: Record<string, LucideIcon> = {
  Home,
  Receipt,
  ChefHat,
  Calendar,
  Bot,
  Settings,
  TrendingUp,
  Utensils,
  QrCode,
  UserCircle,
  Truck,
  Coffee,
  Boxes,
  Users,
  Globe,
  FileText,
  CreditCard,
  BarChart3,
  ExternalLink,
  Copy,
  GraduationCap,
  CircleHelp,
  Banknote,
  Printer,
  Megaphone,
  CalendarClock,
};

export interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
  /** Access-filtered navigate commands (from buildNavCommands). */
  navCommands: Command[];
  /** Quick actions (storefront, copy link, tutorial…). */
  actionCommands: Command[];
  /** Live business records (bills, staff, menu items, tables). */
  recordCommands?: Command[];
  /** True while record index is still loading. */
  recordsLoading?: boolean;
  /** Tab keys of recently visited sections, most-recent first. */
  recentTabKeys: string[];
  /** Execute a chosen command (navigate or run an action). */
  onRun: (command: Command) => void;
  /** Localized-string lookup for palette chrome. */
  t: (key: string) => string;
}

interface Section {
  /** Group header, or null to render the section without a header. */
  label: string | null;
  items: Command[];
}

const RECENT_LIMIT = 5;

function CommandIcon({ iconKey }: { iconKey: string }) {
  const Icon = ICONS[iconKey] ?? CircleHelp;
  return <Icon className="w-4 h-4 shrink-0" aria-hidden="true" strokeWidth={2} />;
}

export default function CommandPalette({
  open,
  onClose,
  navCommands,
  actionCommands,
  recordCommands = [],
  recordsLoading = false,
  recentTabKeys,
  onRun,
  t,
}: CommandPaletteProps) {
  const [query, setQuery] = React.useState("");
  const [selected, setSelected] = React.useState(0);
  const inputRef = React.useRef<HTMLInputElement>(null);
  const listRef = React.useRef<HTMLDivElement>(null);
  const panelRef = React.useRef<HTMLDivElement>(null);
  // Element that held focus before the palette opened, so we can restore it on
  // close instead of dumping the operator back at the top of the document.
  const previouslyFocusedRef = React.useRef<HTMLElement | null>(null);

  // Reset to a clean state every time the palette opens, capture the outgoing
  // focus, move focus into the palette, and restore focus on close.
  React.useEffect(() => {
    if (!open) return;
    setQuery("");
    setSelected(0);
    previouslyFocusedRef.current =
      (document.activeElement as HTMLElement | null) ?? null;
    const id = window.requestAnimationFrame(() => inputRef.current?.focus());
    return () => {
      window.cancelAnimationFrame(id);
      // Restore focus to whatever was focused before we opened. Guard against a
      // now-detached node (e.g. the trigger unmounted while the palette was up).
      const prev = previouslyFocusedRef.current;
      previouslyFocusedRef.current = null;
      if (prev && document.contains(prev)) {
        prev.focus();
      }
    };
  }, [open]);

  // Build the visible sections for the current query.
  const sections = React.useMemo<Section[]>(() => {
    if (query.trim()) {
      const ranked = rankCommands(query, [
        ...navCommands,
        ...actionCommands,
        ...recordCommands,
      ]);
      return [{ label: null, items: ranked }];
    }

    const navByKey = new Map(navCommands.map((c) => [c.tabKey, c] as const));
    const recentSet = new Set<string>();
    const recent: Command[] = [];
    for (const key of recentTabKeys) {
      if (recent.length >= RECENT_LIMIT) break;
      const cmd = navByKey.get(key);
      if (cmd && !recentSet.has(key)) {
        recent.push(cmd);
        recentSet.add(key);
      }
    }

    const rest = navCommands.filter((c) => !c.tabKey || !recentSet.has(c.tabKey));

    const built: Section[] = [];
    if (recent.length) built.push({ label: t("commandPalette.recent"), items: recent });
    built.push({ label: t("commandPalette.navigate"), items: rest });
    if (recordCommands.length)
      built.push({
        label: t("commandPalette.records"),
        // Cap the empty-query preview so the list stays scannable; full index
        // is always available when the operator types a query.
        items: recordCommands.slice(0, 12),
      });
    if (actionCommands.length)
      built.push({ label: t("commandPalette.actions"), items: actionCommands });
    return built;
  }, [query, navCommands, actionCommands, recordCommands, recentTabKeys, t]);

  // Flatten for keyboard navigation; keep a parallel id per row for aria.
  const flat = React.useMemo(() => sections.flatMap((s) => s.items), [sections]);

  // Keep selection in range as the list changes.
  React.useEffect(() => {
    setSelected((s) => (flat.length === 0 ? 0 : Math.min(s, flat.length - 1)));
  }, [flat.length]);

  // Scroll the active row into view as the operator arrows through.
  React.useEffect(() => {
    if (!open) return;
    const el = listRef.current?.querySelector<HTMLElement>(`[data-cmdk-index="${selected}"]`);
    el?.scrollIntoView?.({ block: "nearest" });
  }, [selected, open]);

  if (!open) return null;

  // All keyboard handling lives on the panel root so it works no matter which
  // descendant holds focus: Escape closes, arrows/Home/End drive selection,
  // Enter runs the active row, and Tab is trapped inside the dialog.
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelected((s) => (flat.length === 0 ? 0 : (s + 1) % flat.length));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelected((s) => (flat.length === 0 ? 0 : (s - 1 + flat.length) % flat.length));
    } else if (e.key === "Home") {
      e.preventDefault();
      setSelected(0);
    } else if (e.key === "End") {
      e.preventDefault();
      setSelected(Math.max(0, flat.length - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const cmd = flat[selected];
      if (cmd) onRun(cmd);
    } else if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    } else if (e.key === "Tab") {
      // Focus trap: keep Tab / Shift+Tab cycling within the dialog so focus
      // never escapes to the (inert) page behind the modal.
      const panel = panelRef.current;
      if (!panel) return;
      const focusable = Array.from(
        panel.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => el.offsetParent !== null || el === document.activeElement);
      if (focusable.length === 0) {
        e.preventDefault();
        inputRef.current?.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const activeEl = document.activeElement as HTMLElement | null;
      if (e.shiftKey) {
        if (activeEl === first || !panel.contains(activeEl)) {
          e.preventDefault();
          last.focus();
        }
      } else if (activeEl === last || !panel.contains(activeEl)) {
        e.preventDefault();
        first.focus();
      }
    }
  };

  let runningIndex = -1;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-start justify-center px-4 pt-[12vh] sm:pt-[15vh]"
      role="presentation"
      onMouseDown={onClose}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-ink-950/40 backdrop-blur-sm motion-safe:animate-[cmdk-fade_120ms_ease-out]" />

      {/* Panel */}
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions -- dialog stop-propagation via onMouseDown/onKeyDown prevents backdrop close */}
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={t("commandPalette.title")}
        className="relative w-full max-w-xl overflow-hidden rounded-2xl border border-warm-200 bg-warm-50 shadow-2xl shadow-ink-950/20 motion-safe:animate-[cmdk-pop_140ms_cubic-bezier(0.16,1,0.3,1)]"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Search row */}
        <div className="flex items-center gap-3 border-b border-warm-200 px-4">
          <Search className="w-4 h-4 text-ink-400 shrink-0" aria-hidden="true" />
          <input
            ref={inputRef}
            type="text"
            role="combobox"
            aria-expanded="true"
            aria-controls="cmdk-listbox"
            aria-activedescendant={flat[selected] ? `cmdk-opt-${selected}` : undefined}
            aria-autocomplete="list"
            autoComplete="off"
            spellCheck={false}
            placeholder={t("commandPalette.placeholder")}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelected(0);
            }}
            className="w-full bg-transparent py-4 text-body text-ink-800 placeholder:text-ink-400 focus:outline-none"
          />
          <kbd className="hidden sm:inline-flex items-center rounded-md border border-warm-300 bg-warm-100 px-1.5 py-0.5 text-[11px] font-medium text-ink-400">
            esc
          </kbd>
        </div>

        {/* Results */}
        <div
          ref={listRef}
          id="cmdk-listbox"
          role="listbox"
          aria-label={t("commandPalette.title")}
          className="max-h-[min(60vh,420px)] overflow-y-auto overscroll-contain py-2"
        >
          {flat.length === 0 ? (
            <p className="px-4 py-10 text-center text-body-sm text-ink-500">
              {recordsLoading
                ? t("commandPalette.recordsLoading")
                : t("commandPalette.empty")}
            </p>
          ) : (
            sections.map((section, si) =>
              section.items.length === 0 ? null : (
                <div key={section.label ?? `s-${si}`} className="px-2 pb-1">
                  {section.label && (
                    <p className="px-2 pb-1 pt-2 text-label uppercase tracking-wider text-ink-500">
                      {section.label}
                    </p>
                  )}
                  {section.items.map((cmd) => {
                    runningIndex += 1;
                    const index = runningIndex;
                    const isActive = index === selected;
                    return (
                      <button
                        key={cmd.id}
                        type="button"
                        role="option"
                        id={`cmdk-opt-${index}`}
                        data-cmdk-index={index}
                        aria-selected={isActive}
                        onMouseMove={() => setSelected(index)}
                        onClick={() => onRun(cmd)}
                        className={`group flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors ${
                          isActive ? "bg-brand/10 text-brand-dark" : "text-ink-700"
                        }`}
                      >
                        <span
                          className={`shrink-0 ${isActive ? "text-brand" : "text-ink-400"}`}
                        >
                          <CommandIcon iconKey={cmd.iconKey} />
                        </span>
                        <span className="flex min-w-0 flex-1 flex-col">
                          <span className="truncate text-body-sm font-medium">{cmd.label}</span>
                          {cmd.description && (
                            <span className="truncate text-[12px] text-ink-500">
                              {cmd.description}
                            </span>
                          )}
                        </span>
                        {cmd.locked && (
                          <span className="inline-flex items-center gap-1 rounded-full bg-warm-100 px-2 py-0.5 text-[11px] font-medium text-ink-500">
                            <Lock className="w-3 h-3" aria-hidden="true" />
                            {t("commandPalette.locked")}
                          </span>
                        )}
                        {isActive && (
                          <CornerDownLeft
                            className="w-3.5 h-3.5 shrink-0 text-brand"
                            aria-hidden="true"
                          />
                        )}
                      </button>
                    );
                  })}
                </div>
              ),
            )
          )}
        </div>

        {/* Footer hints */}
        <div className="flex items-center justify-between border-t border-warm-200 bg-warm-100/60 px-4 py-2.5 text-[12px] text-ink-500">
          <span className="flex items-center gap-3">
            <span className="flex items-center gap-1">
              <kbd className="rounded border border-warm-300 bg-warm-50 px-1 font-sans">↑</kbd>
              <kbd className="rounded border border-warm-300 bg-warm-50 px-1 font-sans">↓</kbd>
              {t("commandPalette.footerNavigate")}
            </span>
            <span className="flex items-center gap-1">
              <kbd className="rounded border border-warm-300 bg-warm-50 px-1 font-sans">↵</kbd>
              {t("commandPalette.footerOpen")}
            </span>
          </span>
          <span className="font-medium text-ink-500">{t("commandPalette.brand")}</span>
        </div>
      </div>
    </div>
  );
}
