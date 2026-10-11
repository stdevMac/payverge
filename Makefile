# Payverge root Makefile
#
# Targets:
#   install-hooks      — install git hooks (secret scan, i18n parity, pre-commit guards)
#   verify:backend     — cold, environment-isolated backend verification
#   verify:frontend    — cold, environment-isolated frontend verification

.PHONY: install-hooks verify\:backend verify\:frontend

install-hooks:
	@./scripts/install-hooks.sh

# Cold, environment-isolated verification entry points used by CI.
verify\:backend:
	@bash ./scripts/verify-backend.sh

verify\:frontend:
	@bash ./scripts/verify-frontend.sh

# Licence gate: shipped backend binaries (default tags) must not link GPL code
# (go.mau.fi/libsignal arrives only with -tags whatsapp). Also proves the opt-in
# whatsapp build still compiles. See docs/self-hosting/whatsapp.md.
.PHONY: check-gpl-free
check-gpl-free:
	@bash ./scripts/ci/check-gpl-free_test.sh
	@bash ./scripts/ci/check-gpl-free.sh
