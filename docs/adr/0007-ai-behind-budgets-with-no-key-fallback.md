# 0007. AI behind mandatory budgets, with a working product when no model is configured

- Status: Accepted. Amended 2026-10-05: the public marketing concierge was
  removed and its tables dropped; the genesis baseline never had them. The
  guest AI waiter is now the only unauthenticated AI surface, and the public
  ceiling no longer has a concierge slice. Mentions of the concierge below are historical.
- Date: 2026-10-03 (records a decision already in effect)

## Context

Payverge has AI surfaces that anonymous visitors can reach (the guest AI
waiter, the public concierge) and owner tools that run multi-step agent
loops. Each model call costs money. A self-hoster may not want AI at all, or
may run a local model. A model can also say things the business never
approved: invented prices, allergy claims, links.

## Decision

- **One seam.** Every surface calls models through `llm.Provider`. One
  client speaks the OpenAI Chat Completions protocol to OpenRouter by default,
  or to any compatible endpoint set with `LLM_BASE_URL`.
- **Mandatory dollar ceilings.** Every call reserves its worst-case cost in a
  Postgres ledger before it runs and settles the real cost after. Ceilings
  apply per business (owner and guest scopes separately, $5 a day each by
  default) and to the whole instance ($20). Inside the instance ceiling, all
  unauthenticated traffic (guest waiter turns and the public concierge)
  shares a public ceiling of half the instance total, so owners keep a
  reserve guests cannot spend. The concierge's own ceiling is derived from
  the public one ($5 by default). They cannot be switched off; invalid values
  fall back to the defaults. Cheap per-session, per-IP and per-device caps
  run in front.
- **No key, no crash.** With no provider configured, the backend starts
  normally. The AI waiter answers in a deterministic basic mode from the real
  menu, other AI routes return `503 ai_not_configured`, and
  `GET /api/v1/instance` reports `features.ai: false` so the UI can hide
  them. The UI does not read that flag yet.
- **The model proposes, code decides.** Prices, availability, cart changes
  and links come from the database and are checked after the model answers.

## Consequences

- **Easier:** an instance cannot run up an unbounded bill, even if a public
  page is abused. The product works fully without AI.
- **Easier:** hermetic contract tests can script a hostile model and check
  that the code holds.
- **Harder:** the default ceilings are small, and a busy instance must raise
  them. Models on a local endpoint need a price entry for the ceilings to
  apply; models on a hosted endpoint need one to be called at all.
- **Accepted limitation:** budget enforcement is built into the one client
  rather than wrapped around any provider, so a new provider implementation
  must carry it.
- **Not yet followed everywhere:** the optional WhatsApp waiter sends the
  model's prose with only an allergen note added, and has no basic mode. See
  [ai/ai-waiter.md § WhatsApp](../ai/ai-waiter.md#whatsapp).

See [ai/README.md](../ai/README.md) and
[ai/cost-and-budgets.md](../ai/cost-and-budgets.md).
