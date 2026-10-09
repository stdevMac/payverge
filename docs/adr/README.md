# Architecture decision records

Short records of decisions that shape the codebase and are expensive to
reverse. Each one states the context, the decision and what it costs. They
were written when the project was opened up, so they describe decisions
already in effect; new ones are added as decisions are made.

| ADR | Decision |
|---|---|
| [0001](0001-go-gin-gorm-modular-monolith.md) | One Go backend (Gin + GORM), structured as a modular monolith |
| [0002](0002-postgres-genesis-and-numbered-sql-migrations.md) | Postgres schema owned by a genesis baseline plus numbered SQL migrations |
| [0003](0003-money-as-int64-cents.md) | Store money as `int64` cents; send dollars on the wire |
| [0004](0004-sse-instead-of-websockets.md) | Server-sent events, not WebSockets, for realtime |
| [0005](0005-plugin-registry-for-payments.md) | Payment providers as compiled-in plugins behind one contract |
| [0006](0006-same-origin-api-and-runtime-frontend-config.md) | Same-origin API and runtime frontend configuration |
| [0007](0007-ai-behind-budgets-with-no-key-fallback.md) | AI behind mandatory budgets, with a working product when no model is configured |
| [0008](0008-apache-2-with-whatsapp-behind-gpl-build-tag.md) | Apache-2.0, with the GPL-linked WhatsApp channel behind a build tag |

## Writing a new ADR

Copy this shape into `docs/adr/NNNN-short-title.md`, numbered one above the
highest, and add a row to the table above.

```markdown
# NNNN. Title in the imperative

- Status: Proposed | Accepted | Superseded by NNNN
- Date: YYYY-MM-DD

## Context

What forces are at play, and what problem needs a decision.

## Decision

What we do, in one or two paragraphs.

## Consequences

What gets easier, what gets harder, and what we accept.
```

Do not rewrite an accepted ADR when the decision changes. Write a new one,
and mark the old one "Superseded by".

For how these decisions look in the code, see
[architecture/](../architecture/overview.md).
