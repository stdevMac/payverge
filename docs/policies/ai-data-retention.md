# AI Data Retention & Privacy Policy

_Last updated: 2026-07-18._

This document describes how Payverge stores, retains, and deletes data produced
by its AI features, and the legal basis for those choices. It is the source of
truth that the retention janitors, privacy class map (`internal/llm/privacy.go`),
and the PII redaction package (`internal/pii`) implement.

## 0. Privacy class and ZDR routing

Every production model request carries a mandatory `PrivacyClass` derived from
its `Feature` tag. Sensitive and confidential features force OpenRouter
`provider.zdr=true` (zero-data-retention routing). ZDR constrains eligible
provider routing for the request; it does **not** erase messages already sent
through WhatsApp or replace third-party platform policies.

| Feature | Privacy class | ZDR | Automatic retention | Permanent deletion |
|---|---|---|---|---|
| `waiter` | customer-sensitive | yes | 90 days | business closure purge; janitor |
| `waiter_whatsapp` | customer-sensitive | yes | 90 days (Payverge transcript) | business closure purge; janitor |
| `ops_assistant` | business-confidential | yes | 30 days after the last message | janitor; closure purge |
| `director` | business-confidential | yes | account lifetime | archive vs permanent delete; closure purge |
| `wizard` / `extraction` / `image` / `marketing` | business-confidential | yes | account / draft lifetime | owner delete + closure purge |
| `guardrail` | customer-sensitive | yes | ephemeral (not transcript storage) | n/a |

**Subscription cancellation alone does not erase AI history** so an account can
resume. Owner `DeleteBusiness` and admin permanent closure call
`PurgeBusinessAIData`.

## 1. What is stored

| Data | Table(s) | Contents | Source |
|---|---|---|---|
| Guest AI waiter transcripts | `ai_waiter_conversations`, `ai_waiter_messages` | Session metadata (business, table, language, mode) and per-message role + content | Guest chat with the AI waiter (public endpoint) |
| WhatsApp AI waiter | same transcript tables + `whatsapp_business_devices` | Channel metadata; transcript under waiter retention | Linked WhatsApp device |
| Ops Assistant | `ops_assistant_*` | Operator guidance threads, tool calls, request ledger | Authenticated staff |
| Director Console threads | `director_console_threads`, `director_console_messages`, proposals/audits | Owner copilot questions and structured answers | Authenticated business owners |
| Menu wizard / extraction | `menu_wizard_*`, `menu_extraction_*` | Owner menu-building dialogue and extraction jobs | Authenticated operators with menu perms |
| AI image provenance | `ai_generated_images` | Model + storage key for generated assets | Menu/marketing image generation |

## 2. Why it is stored (purpose)

- **Guest transcripts**: short-term conversational context so a guest can scroll
  back within a session, and aggregate operational analytics (conversation
  counts, upsell success) surfaced to the business owner. There is no long-term
  need to retain individual guest message text.
- **Director / wizard threads**: these are an authenticated owner's own working
  history (a copilot notebook). They are retained for the lifetime of the
  business account, with archival available, because the owner is the data
  subject and controller of their own thread.

## 3. Retention windows

| Data | Window | Mechanism |
|---|---|---|
| Guest AI waiter transcripts | **90 days** (configurable via `AI_TRANSCRIPT_RETENTION_DAYS`; `0` disables) | Hourly retention janitor deletes conversations + messages older than the window in bounded batches |
| Ops Assistant threads | **30 days after the last message** (configurable via `OPS_ASSISTANT_RETENTION_DAYS`; `0` disables) | Hourly retention janitor deletes the thread with its messages, tool calls and request ledger rows; there is no archive or per-thread delete route |
| Director Console threads | **Indefinite, with archival** | Owner-controlled; deletion on account closure or explicit request (see §6) |
| Menu wizard conversations | **Indefinite, with archival** | Owner-controlled; same as Director threads |

Rationale for the split: guest transcripts may contain third-party personal
data (and special-category health data — see §5) collected from people who are
not Payverge account holders, so storage limitation (GDPR Art. 5(1)(e)) requires
a short, automatic window. Director/wizard threads belong to the authenticated
owner who created them; indefinite-with-archival mirrors how the owner's other
business records are retained, and the owner can delete them at will.

## 4. Redaction at ingest

Guest message text is passed through `internal/pii.Redact` **before storage**
(implemented at the transcript-ingest call site, Lane B). `Redact` removes email
addresses and phone numbers while deliberately preserving dates, quantities,
currency amounts, order/bill IDs, and percentages (contract C3). Director
Console responses are likewise redacted with the same `pii.Redact` function
(Lane C), replacing the earlier date-mangling phone regex.

Redaction is best-effort defense-in-depth, not a guarantee that no personal data
is ever stored; the 90-day window is the primary storage-limitation control.

## 5. Legal basis (GDPR Art. 5 / Art. 9)

- **Art. 5(1)(c) data minimisation**: only role + (redacted) content + minimal
  session metadata are stored; tool-call payloads are excluded from transcript
  reads.
- **Art. 5(1)(e) storage limitation**: the 90-day guest window plus the hourly
  janitor enforce that guest transcripts are not kept longer than necessary.
- **Art. 9 special-category data (health)**: guests may mention **allergies** in
  the AI waiter, which constitutes special-category health data tied to a session
  and table. This is the strongest reason for the short, automatic guest window
  and for ingest redaction; allergen handling itself (deterministic answers,
  disclaimers) is covered by the waiter rebuild (Lane B). Operators should not
  rely on transcripts as an allergy record.

## 6. Third-party processing (OpenRouter)

Guest and owner messages are sent to the LLM provider **OpenRouter**
(`internal/llm/openrouter`) for inference. OpenRouter is a third-party processor;
its sub-processor and data-handling terms govern transient processing of message
content. Payverge stores only the redacted transcript on its own
infrastructure; the prompt sent to OpenRouter is not separately retained by
Payverge beyond the transcript record.

## 7. Deletion on request

- **Guest data**: because guest transcripts auto-expire at 90 days and are keyed
  to an ephemeral session/table (no guest account), the standard pathway is to
  wait out the window or, for an expedited erasure request, delete the matching
  `ai_waiter_conversations` row **and its `ai_waiter_messages` rows together**.
  There is **no database `ON DELETE CASCADE`** on `AiWaiterMessage` (it carries a
  plain `conversation_id` index, not a cascading FK — see `models.go`), so a
  manual, out-of-band delete of a single conversation row does **not**
  auto-remove its messages. Use the same conversation-scoped two-step the janitor
  uses (`DeleteExpiredAiWaiterTranscripts` deletes messages by
  `conversation_id IN (...)` first, then the conversations): for a targeted
  erasure, delete the message rows for the conversation id(s), then the
  conversation row(s). Operators forward erasure requests to Payverge support,
  who locate rows by business + table + approximate timestamp.
- **Owner data**: Director/wizard threads are deleted when the owner archives or
  deletes them, or on account closure.

## 8. Operational controls

- Retention window: `AI_TRANSCRIPT_RETENTION_DAYS` (default 90; `0` disables and
  logs a warning at startup).
- The janitor runs hourly, deletes in batches of 500 to avoid long locks, and
  logs one summary line per sweep (`AI transcript retention janitor swept N
  conversations / M messages older than D days`).
- Indexes `idx_ai_waiter_conversations_created_at` and
  `idx_ai_waiter_messages_created_at` (migration 000068) keep the range deletes
  cheap.
