<!--
Thanks for contributing. Please read .github/CONTRIBUTING.md first.
The PR title becomes the commit subject, so use Conventional Commits:
  fix(payments): reject a refund larger than the captured amount
Security fixes do not go here: see .github/SECURITY.md.
-->

## Summary

<!-- What does this change, and why? Link the issue it resolves. -->

Closes #

## How it was tested

<!-- Commands you ran and what they showed. For UI changes, add screenshots
(blur any personal data). For bug fixes, confirm the regression test failed
before the fix. -->

## Checklist

<!-- Tick each item. If one does not apply, tick it and add "n/a" at the end of the line. -->

- [ ] **Tests**: new or updated tests cover the change, and the relevant suites pass locally (`go test -race` for touched backend packages; `npm run lint`, `npm run typecheck` and `npm run test` for frontend changes).
- [ ] **i18n parity**: any new or changed UI copy is in the operator tier (`en`, `es`, `es-ar`) and/or all 21 guest locales, and the validators pass (`check-operator-locales.js`, `check-guest-locales.js --strict`, `npm run i18n:check`).
- [ ] **Performance gate**: if this touches a hot path (guest or public routes, polling, payments or webhooks, list endpoints, shared middleware, queues, query shape), the before/after `-benchmem` numbers and the exact command are in this description.
- [ ] **Schema**: database changes are a new numbered migration pair in `backend/migrations/`, with no new `AutoMigrate`.
- [ ] **No secrets or personal data**: no credentials, `.env` files, production dumps or real customer or guest data are included anywhere in the diff, fixtures or screenshots.
- [ ] **Docs**: user-facing behaviour, configuration or environment variable changes are documented, and breaking changes are marked with `!` and a `BREAKING CHANGE:` footer.
- [ ] **DCO**: every commit is signed off (`git commit -s`). See [CONTRIBUTING.md](https://github.com/stdevMac/payverge/blob/main/.github/CONTRIBUTING.md#developer-certificate-of-origin-dco).
