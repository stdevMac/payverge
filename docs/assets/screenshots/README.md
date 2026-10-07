# Screenshots

These images show the running app. The main [README](../../../README.md#screenshots)
and the project site (`site/assets/screenshots/`, WebP copies of a subset) use
them.

## Where they come from

All of them were captured from a local install started with demo data:

```sh
./deploy/install.sh --build --domain localhost --admin-email you@example.com --demo
```

The capture script [`tools/screenshots/capture.mjs`](../../../tools/screenshots/capture.mjs)
runs one real service against the demo venue. A guest scans a table, orders
from the menu and opens the bill. Staff approve the ticket in the kitchen. The
guest asks to pay at the counter, staff record the cash payment, and the guest
lands on the receipt. Every screen is the live app.
Nothing is mocked. The only addition is a label on the staff AI screens saying no model is connected.

- **Demo data is fictional.** The venues, owners, guests, addresses and phone
  numbers are invented for the demo seed (`backend/internal/demo/`). The dish
  and venue pictures are illustrations drawn in code by
  [`tools/demo-art/`](../../../tools/demo-art/). They are not photographs.
  [docs/licensing/CREDITS.md](../../licensing/CREDITS.md) records their provenance.
- **The AI screens show the no-model state.** A demo install has no model key.
  The AI waiter greeting and reply are the built-in scripted copy, and the
  director console shows its deterministic "not enough data" answer. The guest
  widget calls itself a menu helper and says it is not an AI; the staff AI
  screens carry a banner that says no model is connected. Do not replace them with model output
  that was pasted in or invented. Recapture them on an install with a model
  configured if you want real responses.
- **Payment.** No card processor is connected in the demo, so the payment
  screens show pay-at-counter (cash), the tender that works without any outside
  account.

## Recapturing

```sh
cd frontend && npm ci && cd ..          # playwright and sharp come from here
# an install made with `deploy/install.sh --demo` already serves the demo
# grill's storefront on "/" (PRIMARY_VENUE=parrilla-quebracho-azul)
# compose also reads OPENROUTER_API_KEY, LLM_BASE_URL and LLM_API_KEY from your
# shell: unset them before installing or the "no model" shots will have one
PAYVERGE_URL=https://localhost PAYVERGE_EMAIL=you@example.com PAYVERGE_PASSWORD=... \
  node tools/screenshots/capture.mjs
```

The script prints each file and its size. It keeps every PNG here under
300 KB and every site WebP under the 150 KB cap that `site/tools/check.mjs`
enforces. It places an order and records a payment on the install it points
at, so only point it at a throwaway demo install. Set `ONLY=name,name` to
recapture some shots, and `PAYVERGE_TABLE` to pick the table.

Desktop shots are 1440x900. Guest shots are 390x844 at 2x pixel density.

## License

The screenshots show Payverge's own UI and the project-made demo content. They
are released under the repository's [Apache License 2.0](../../../LICENSE). The
Payverge name and logo that appear in them are covered by
[TRADEMARKS.md](../../../TRADEMARKS.md), not by the code license.
