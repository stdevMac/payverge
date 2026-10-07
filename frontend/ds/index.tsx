/**
 * Payverge design-system barrel.
 *
 * This file exists for ONE purpose: to give the /design-sync converter a single
 * entry point over the components that make up Payverge's reusable UI layer.
 * The app itself does not import from here — every export below is a re-export
 * of the real shipped component in `src/`, so the bundle uploaded to
 * claude.ai/design is the same code that runs in production.
 *
 * Scope rule: presentational primitives that a designer would compose with.
 * App screens (src/components/business/**, admin/**, staff/**) are deliberately
 * out of scope — they are wired to data, not reusable parts.
 *
 * Adding a component: re-export it here, add its src path to the local
 * design-sync config's componentSrcMap (not tracked), and re-run the sync.
 */

// MUST stay first: supplies the `process.env` values Next's client runtime
// (next/link, next/image) reads at module scope. See browser-env.ts.
import "./browser-env";

// ── Context ─────────────────────────────────────────────────────────────────
// The operator-tier translation provider. Components that read locale
// (LocalizedModal, OfflineBanner, the skeletons' aria labels) degrade to
// English without it, but wrapping is the correct usage.
export {
  SimpleTranslationProvider,
  OperatorTranslationProvider,
} from "../src/i18n/SimpleTranslationProvider";

// Drives OfflineBanner. Reads navigator.onLine on mount and listens for the
// browser's online/offline events.
export { ConnectivityProvider } from "../src/contexts/ConnectivityContext";

// ── Primitives ──────────────────────────────────────────────────────────────
export { RouteLoadingFallback, PanelFailure } from "../src/components/ui/AsyncState";
export { ComingSoonBadge } from "../src/components/ui/ComingSoonBadge";
export { DecimalInput } from "../src/components/ui/DecimalInput";
export { EmptyState } from "../src/components/ui/EmptyState";
export { default as IconTile } from "../src/components/ui/IconTile";
export { LocalizedModal } from "../src/components/ui/LocalizedModal";
export { Metric } from "../src/components/ui/Metric";
export { OfflineBanner } from "../src/components/ui/OfflineBanner";
export { StatusChip } from "../src/components/ui/StatusChip";

// ── Input ───────────────────────────────────────────────────────────────────
export { Button } from "../src/components/ui/input/button";

/**
 * The dashboard button SYSTEM is these class recipes applied to a plain
 * <button>/<a> — not the `Button` component above, which is a minimal legacy
 * primitive. Exported so designs can use the real thing.
 */
export {
  btnPrimary,
  btnPrimaryCompact,
  btnSecondary,
  btnGhostIcon,
  btnGhostIconActive,
  btnTonalSuccess,
  btnTonalDanger,
  btnQuietDanger,
  btnDangerLink,
  btnPrimaryNextUI,
  btnSecondaryNextUI,
  touchIconBtn,
} from "../src/components/ui/buttonStyles";
export { TimeField } from "../src/components/ui/fields/TimeField";

// ── Loading states ──────────────────────────────────────────────────────────
export { SkeletonCard } from "../src/components/ui/skeletons/SkeletonCard";
export { SkeletonChart } from "../src/components/ui/skeletons/SkeletonChart";
export { SkeletonKPI } from "../src/components/ui/skeletons/SkeletonKPI";
export { SkeletonLine } from "../src/components/ui/skeletons/SkeletonLine";
export { SkeletonList } from "../src/components/ui/skeletons/SkeletonList";
export { SkeletonTable } from "../src/components/ui/skeletons/SkeletonTable";
export { PrimarySpinner } from "../src/components/ui/spinners/PrimarySpinner";

// ── Navigation ──────────────────────────────────────────────────────────────
export { TopMenuShell, TopMenuInstantChrome } from "../src/components/ui/top-menu/TopMenuShell";

/**
 * NextUI's modal parts, re-exported from the SAME bundled instance
 * LocalizedModal uses. Importing them from '@nextui-org/react' in a design
 * would load a second copy of NextUI, and `useModalContext` would be
 * undefined — compose modals with these.
 */
export {
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@nextui-org/react";

// ── Composite ───────────────────────────────────────────────────────────────
export { default as CurrencyConverter, CurrencyPrice } from "../src/components/common/CurrencyConverter";
export { FiscalIdentityFields } from "../src/components/common/FiscalIdentityFields";
export { default as ErrorBoundaryShell } from "../src/components/shared/ErrorBoundaryShell";
export { Title } from "../src/components/title/Title";
