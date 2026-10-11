# Stripe CLI — Local Webhook Setup

## 1. Install

```bash
brew install stripe/stripe-cli/stripe
```

## 2. Login

```bash
stripe login
```

This opens a browser to authenticate with your Stripe account.

## 3. Forward webhooks to your local backend

```bash
stripe listen --forward-to localhost:8080/api/v1/webhooks/subscription-stripe
```

This will output a **webhook signing secret** like:

```
> Ready! Your webhook signing secret is whsec_abc123...
```

## 4. Configure the signing secret

Copy the `whsec_...` value and set it in the admin panel:

**Admin → Subscription Stripe → Update Credentials → Webhook Secret**

Or set it directly in platform settings via the database.

> **Note**: The `whsec_` secret changes each time you restart `stripe listen`. During local dev you can leave the webhook secret empty in settings — the backend will skip signature verification if none is configured.

## 5. Filter to relevant events only (optional)

```bash
stripe listen \
  --forward-to localhost:8080/api/v1/webhooks/subscription-stripe \
  --events checkout.session.completed,checkout.session.expired,invoice.payment_succeeded,invoice.payment_failed,customer.subscription.updated,customer.subscription.deleted
```

## 6. Trigger test events

In a separate terminal:

```bash
# Checkout completion
stripe trigger checkout.session.completed

# Failed invoice payment
stripe trigger invoice.payment_failed

# Subscription cancellation
stripe trigger customer.subscription.deleted
```

## 7. Typical dev workflow

Run these in separate terminals:

```bash
# Terminal 1: Backend
cd backend && go run ./cmd/app

# Terminal 2: Frontend
cd frontend && npm run dev

# Terminal 3: Stripe webhook forwarding
stripe listen --forward-to localhost:8080/api/v1/webhooks/subscription-stripe
```
