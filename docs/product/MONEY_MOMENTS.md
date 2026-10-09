# Live Money Moments — payment-received celebration

Built 2026-05-30 as an autonomous follow-up surprise on the
`feature/money-moments` worktree.

## What it does

When a payment lands, the operator dashboard now plays a premium, on-brand
**"money moment"**: a celebratory card slides in (bottom-right) with the **real
amount counting up from zero**, the payment method, and the bill number —
instead of the old flat toast ("A payment has been processed"). It's the
emotional payoff of running a restaurant: *seeing the money come in.*

## Why it stays trustworthy (the bug-free bar)

A celebration that shows a wrong number would destroy trust in the money
figures. So it's **presentation-only**, driven entirely off the existing
`payment.received` SSE event the dashboard already receives:

- `parsePaymentEvent` **rejects** any payload without a real positive `amount`
  (0, negative, NaN, Infinity, non-numeric → no celebration). We'd rather show
  nothing than a phantom payment.
- `amount` is already dollars (backend converts cents→dollars before
  publishing — the money wire contract), so no re-division.
- **No new sound** — the dashboard already calls `playNotificationSound("payment")`.
  This feature is purely visual, so there's no double-beep.
- It **replaces** the bland payment toast (one notification, no duplication) and
  falls back to that toast only if a payload somehow lacks an amount.

## Architecture (`src/components/dashboard/moneyMoments/`)

| File | Responsibility |
|------|----------------|
| `paymentMoment.ts` | Pure: validate/parse the SSE payload, friendly method labels. |
| `useCountUp.ts` | Reduced-motion-aware count-up animation hook. |
| `PaymentCelebration.tsx` | The celebration card + bottom-right stack, in a polite live region. |
| `MoneyMomentsProvider.tsx` | Queue, auto-dismiss, max-visible cap, `useMoneyMoments()` context. |

Integration: `MoneyMomentsProvider` wraps the dashboard render (inside
`ToastProvider`); `SSEToastBridge` calls `celebrate(event.data)` on
`payment.received`. i18n under `businessDashboard.moneyMoments.*` (en / es /
es-AR). Entrance keyframe in `globals.css`, gated by `motion-safe:`.

## Testing

17 unit/component tests (parse validation, count-up reduced-motion, card a11y +
dismiss, provider queue/auto-dismiss/cap). Verified with full typecheck, lint,
`npm run build`, and the 148 dashboard/i18n/app-route jest tests — all green.
Remaining: visual QA against a live payment on a running dashboard.
