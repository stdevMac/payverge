# 0008. Apache-2.0, with the GPL-linked WhatsApp channel behind a build tag

- Status: Accepted
- Date: 2026-10-03

## Context

Payverge is released under the Apache License 2.0, a permissive licence with
an explicit patent grant. One optional feature, the WhatsApp AI-waiter
channel, uses `whatsmeow` (MPL-2.0), which depends on `go.mau.fi/libsignal`
(GPL-3.0). Linking GPL-3.0 code makes the resulting binary as a whole subject
to GPL-3.0. Most self-hosters do not need WhatsApp, and should not have to
take GPL obligations to run a restaurant.

## Decision

- All Payverge source code is Apache-2.0.
- Every file that imports `go.mau.fi/*` carries `//go:build whatsapp`. The
  default build compiles a stub instead, which reports that WhatsApp is not
  built in.
- The channel runs only when the binary is built with `-tags whatsapp`
  **and** `WHATSAPP_ENABLED=true` is set.
- Published backend images are always built **without** the tag. A
  WhatsApp-capable binary is something an operator builds themselves.

## Consequences

- **Easier:** the default build's Go binary links no GPL module, and its
  licence picture is Apache-2.0 plus permissive dependencies. The images'
  base OS packages carry their own licences (some GPL), with source from
  the distribution.
- **Easier:** the rest of the codebase compiles and is tested without the
  WhatsApp modules.
- **Harder:** two build variants. `make check-gpl-free` checks both that
  the default binary links no GPL module and that the tagged build still
  compiles. Code that touches WhatsApp must go through the interface the
  stub also implements.
- **Accepted:** an operator who distributes a tagged binary must meet
  GPL-3.0 for that binary. The WhatsApp library also creates its own tables
  at runtime, outside the migration system.

See [self-hosting/whatsapp.md](../self-hosting/whatsapp.md) and
[ai/README.md](../ai/README.md).
