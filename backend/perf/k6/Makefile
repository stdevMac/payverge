BASE_URL ?= http://127.0.0.1:8080
RESULT_DIR ?= /tmp/payverge-perf-results
SUMMARY_PATH := $(RESULT_DIR)/capacity-summary.json
K6_BIN ?= /tmp/payverge-k6

.PHONY: build-k6 smoke capacity burst soak_1h candidate_5x guard-local guard-external

build-k6: ## Build the pinned k6 binary with real SSE support.
	bash scripts/build-k6.sh "$(K6_BIN)"

guard-local:
	@case "$(BASE_URL)" in \
	  *payverge.io*) echo "Refusing Payverge staging/production load target: $(BASE_URL)"; exit 1 ;; \
	  *) ;; \
	esac
	@mkdir -p "$(RESULT_DIR)"
	@test -x "$(K6_BIN)" || (echo "Pinned k6 binary missing; run make build-k6"; exit 1)

guard-external: guard-local
	@test -n "$(BASE_URL)" || (echo "BASE_URL is required for external profiles"; exit 1)
	@test "$(PERF_EXTERNAL_CONFIRM)" = "isolated-perf" || \
	  (echo "Set PERF_EXTERNAL_CONFIRM=isolated-perf for burst/soak profiles"; exit 1)

smoke: guard-local ## Local fixture smoke: every D5 request surface, 20 seconds.
	BASE_URL=$(BASE_URL) PROFILE_NAME=smoke DURATION=20s \
	  RPS_MENU=1 RPS_OPERATOR=1 RPS_GUEST_QUOTE=1 RPS_GUEST_ORDER=1 RPS_CALLBACK=1 VUS_SSE=1 \
	  SUMMARY_PATH="$(SUMMARY_PATH)" "$(K6_BIN)" run scenarios/capacity.js

capacity: smoke

burst: guard-external ## Isolated perf only: two-minute service burst.
	BASE_URL=$(BASE_URL) PROFILE_NAME=burst DURATION=2m \
	  RPS_MENU=24 RPS_OPERATOR=6 RPS_GUEST_QUOTE=4 RPS_GUEST_ORDER=4 RPS_CALLBACK=2 VUS_SSE=40 \
	  SUMMARY_PATH="$(SUMMARY_PATH)" "$(K6_BIN)" run scenarios/capacity.js

soak_1h: guard-external ## Isolated perf only: one-hour projected-peak soak.
	BASE_URL=$(BASE_URL) PROFILE_NAME=soak_1h DURATION=1h \
	  RPS_MENU=12 RPS_OPERATOR=3 RPS_GUEST_QUOTE=2 RPS_GUEST_ORDER=2 RPS_CALLBACK=1 VUS_SSE=20 \
	  SUMMARY_PATH="$(SUMMARY_PATH)" "$(K6_BIN)" run scenarios/capacity.js

candidate_5x: guard-external ## External proof only: exact candidate at five times projected peak.
	@test -n "$(CANDIDATE_SHA)" || (echo "CANDIDATE_SHA is required for exact-candidate proof"; exit 1)
	BASE_URL=$(BASE_URL) PROFILE_NAME=candidate_5x CANDIDATE_SHA="$(CANDIDATE_SHA)" DURATION=1h \
	  RPS_MENU=60 RPS_OPERATOR=15 RPS_GUEST_QUOTE=10 RPS_GUEST_ORDER=10 RPS_CALLBACK=5 VUS_SSE=100 \
	  SUMMARY_PATH="$(SUMMARY_PATH)" "$(K6_BIN)" run scenarios/capacity.js
