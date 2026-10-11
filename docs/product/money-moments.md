# Money moments: payment-received card

When a payment lands, the operator dashboard shows a card in the bottom-right
corner with the amount counting up from zero, the payment method and the bill
number.

## Behaviour

The card is presentation only. It is driven by the `payment.received` SSE
event the dashboard already receives, whose payload is
`{ bill_id, amount, method }`.

- `parsePaymentEvent` rejects any payload without a finite, positive numeric
  `amount` (zero, negative, NaN, Infinity or a non-number shows no card).
- `amount` is already in dollars: the backend converts cents before
  publishing, so the frontend does not divide again.
- The card is not a toast. The `payment.received` toast and its sound belong
  to `OperationalAlertsProvider` (the `payment_received` alert), so the
  dashboard's `SSEToastBridge` only calls `celebrate(event.data)` and adds no
  fallback toast.
- At most three cards are visible at once; older ones are dropped and each
  card dismisses itself after a timeout.
- Unknown payment methods are shown title-cased instead of as raw tokens.

## Code

All files are under `frontend/src/components/dashboard/moneyMoments/`.

| File | Responsibility |
|------|----------------|
| `paymentMoment.ts` | Pure: validates and parses the SSE payload, maps payment methods to labels. |
| `useCountUp.ts` | Count-up animation hook; respects reduced motion. |
| `PaymentCelebration.tsx` | The card and the bottom-right stack, in a polite live region (`role="status"`). |
| `MoneyMomentsProvider.tsx` | Queue, auto-dismiss, visible-card cap and the `useMoneyMoments()` context. |

`MoneyMomentsProvider` wraps the dashboard page
(`app/(shop)/business/[businessId]/dashboard/page.tsx`) inside
`ToastProvider`. Strings are under `businessDashboard.moneyMoments.*` in the
en, es and es-ar operator bundles. The `money-moment-in` keyframe is in
`globals.css` and only runs under `motion-safe:`.

## Tests

`moneyMoments/__tests__/` covers payload validation, count-up with reduced
motion, card accessibility and dismissal, and the provider's queue,
auto-dismiss and cap.
