package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping sqlite: %v", err)
	}
	return db
}

type readinessBody struct {
	Status string            `json:"status"`
	Checks []ComponentResult `json:"checks"`
}

func doReady(t *testing.T, state ReadinessState) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ready", ReadinessHandler(state))
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func parseReadyBody(t *testing.T, w *httptest.ResponseRecorder) readinessBody {
	t.Helper()
	var body readinessBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v\nraw: %s", err, w.Body.String())
	}
	return body
}

// secretLookingSubstrings are patterns that must never appear in readiness JSON
// (codes and component names only — no credentials).
var secretLookingSubstrings = []string{
	"sk_live_",
	"sk_test_",
	"AKIA",
	"password=",
	"api_key=",
	"secret_key=",
	"Bearer ",
	"pm_live_",
}

func assertNoSecrets(t *testing.T, raw string) {
	t.Helper()
	lower := strings.ToLower(raw)
	for _, s := range secretLookingSubstrings {
		if strings.Contains(raw, s) || strings.Contains(lower, strings.ToLower(s)) {
			t.Fatalf("readiness body must not contain secret-looking substring %q; body=%s", s, raw)
		}
	}
}

func TestReadinessComponents(t *testing.T) {
	healthyDB := openMemoryDB(t)

	tests := []struct {
		name           string
		state          ReadinessState
		wantCode       int
		wantStatus     string
		wantFailedComp string // if non-empty, expect this component with status failed
		wantDBComp     bool
	}{
		{
			name: "failed email component with healthy DB returns 503",
			state: ReadinessState{
				DB: healthyDB,
				Components: []ComponentResult{
					{Component: "jwt", Status: "ok"},
					{Component: "plugin", Status: "ok"},
					{Component: "network", Status: "ok"},
					{Component: "email", Status: "failed", Code: "email.api_key.missing"},
					{Component: "storage", Status: "ok"},
					{Component: "proxy", Status: "ok"},
					{Component: "ai_budget", Status: "ok"},
				},
			},
			wantCode:       http.StatusServiceUnavailable,
			wantStatus:     "not ready",
			wantFailedComp: "email",
			wantDBComp:     true,
		},
		{
			name: "failed storage component with healthy DB returns 503",
			state: ReadinessState{
				DB: healthyDB,
				Components: []ComponentResult{
					{Component: "storage", Status: "failed", Code: "s3.public.missing"},
				},
			},
			wantCode:       http.StatusServiceUnavailable,
			wantStatus:     "not ready",
			wantFailedComp: "storage",
			wantDBComp:     true,
		},
		{
			name: "failed proxy component with healthy DB returns 503",
			state: ReadinessState{
				DB: healthyDB,
				Components: []ComponentResult{
					{Component: "proxy", Status: "failed", Code: "proxy.trust.missing"},
				},
			},
			wantCode:       http.StatusServiceUnavailable,
			wantStatus:     "not ready",
			wantFailedComp: "proxy",
			wantDBComp:     true,
		},
		{
			name: "failed ai_budget component with healthy DB returns 503",
			state: ReadinessState{
				DB: healthyDB,
				Components: []ComponentResult{
					{Component: "ai_budget", Status: "failed", Code: "ai_budget.missing"},
				},
			},
			wantCode:       http.StatusServiceUnavailable,
			wantStatus:     "not ready",
			wantFailedComp: "ai_budget",
			wantDBComp:     true,
		},
		{
			name: "all components ok and healthy DB returns 200 ready",
			state: ReadinessState{
				DB: healthyDB,
				Components: []ComponentResult{
					{Component: "jwt", Status: "ok"},
					{Component: "plugin", Status: "ok"},
					{Component: "network", Status: "ok"},
					{Component: "email", Status: "ok"},
					{Component: "storage", Status: "ok"},
					{Component: "proxy", Status: "ok"},
					{Component: "ai_budget", Status: "ok"},
					{Component: "billing", Status: "ok"},
				},
			},
			wantCode:   http.StatusOK,
			wantStatus: "ready",
			wantDBComp: true,
		},
		{
			name: "failed billing component with healthy DB returns 503",
			state: ReadinessState{
				DB: healthyDB,
				Components: []ComponentResult{
					{Component: "jwt", Status: "ok"},
					{Component: "billing", Status: "failed", Code: "billing.price_core_monthly.missing"},
				},
			},
			wantCode:       http.StatusServiceUnavailable,
			wantStatus:     "not ready",
			wantFailedComp: "billing",
			wantDBComp:     true,
		},
		{
			name: "ExtraComponents overrides static billing to failed",
			state: ReadinessState{
				DB: healthyDB,
				Components: []ComponentResult{
					{Component: "billing", Status: "ok"},
				},
				ExtraComponents: func() []ComponentResult {
					return []ComponentResult{
						{Component: "billing", Status: "failed", Code: "billing.webhook_secret.missing"},
					}
				},
			},
			wantCode:       http.StatusServiceUnavailable,
			wantStatus:     "not ready",
			wantFailedComp: "billing",
			wantDBComp:     true,
		},
		{
			name: "all components ok but DB nil returns 503",
			state: ReadinessState{
				DB: nil,
				Components: []ComponentResult{
					{Component: "jwt", Status: "ok"},
					{Component: "email", Status: "ok"},
					{Component: "storage", Status: "ok"},
					{Component: "proxy", Status: "ok"},
					{Component: "ai_budget", Status: "ok"},
				},
			},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "not ready",
			wantDBComp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doReady(t, tt.state)
			if w.Code != tt.wantCode {
				t.Fatalf("status code: got %d want %d body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			body := parseReadyBody(t, w)
			if body.Status != tt.wantStatus {
				t.Fatalf("status field: got %q want %q", body.Status, tt.wantStatus)
			}
			if len(body.Checks) == 0 {
				t.Fatal("expected non-empty checks array")
			}
			assertNoSecrets(t, w.Body.String())

			// Only codes / component names / status tokens — no free-form secrets.
			for _, ch := range body.Checks {
				if ch.Component == "" {
					t.Fatalf("check missing component: %+v", ch)
				}
				if ch.Status == "" {
					t.Fatalf("check missing status: %+v", ch)
				}
			}

			if tt.wantDBComp {
				foundDB := false
				for _, ch := range body.Checks {
					if ch.Component == "database" {
						foundDB = true
						break
					}
				}
				if !foundDB {
					t.Fatalf("expected database check in checks: %+v", body.Checks)
				}
			}

			if tt.wantFailedComp != "" {
				found := false
				for _, ch := range body.Checks {
					if ch.Component == tt.wantFailedComp {
						found = true
						if ch.Status != "failed" {
							t.Fatalf("component %s status: got %q want failed", tt.wantFailedComp, ch.Status)
						}
						if ch.Code == "" {
							t.Fatalf("failed component %s must include a code", tt.wantFailedComp)
						}
					}
				}
				if !found {
					t.Fatalf("expected failed component %q in checks: %+v", tt.wantFailedComp, body.Checks)
				}
			}

			if tt.wantCode == http.StatusOK {
				// Ready response must include DB + every configured component.
				wantComps := map[string]bool{"database": true}
				for _, c := range tt.state.Components {
					wantComps[c.Component] = true
				}
				gotComps := map[string]string{}
				for _, ch := range body.Checks {
					gotComps[ch.Component] = ch.Status
				}
				for comp := range wantComps {
					st, ok := gotComps[comp]
					if !ok {
						t.Fatalf("missing check for component %q; got %+v", comp, body.Checks)
					}
					if st != "ok" {
						t.Fatalf("component %q status: got %q want ok", comp, st)
					}
				}
			}
		})
	}
}

func TestReadinessShowsLiveComponent(t *testing.T) {
	state := ReadinessState{
		DB: openMemoryDB(t),
		Components: []ComponentResult{
			{Component: "jwt", Status: "ok", Source: "config"},
		},
		ExtraComponents: func() []ComponentResult {
			return []ComponentResult{
				{Component: "email_outbox", Status: "ok", Source: "live"},
			}
		},
	}

	w := doReady(t, state)
	if w.Code != http.StatusOK {
		t.Fatalf("status code: got %d want 200 body=%s", w.Code, w.Body.String())
	}
	body := parseReadyBody(t, w)

	var sawDB, sawOutbox bool
	for _, ch := range body.Checks {
		switch ch.Component {
		case "database":
			sawDB = true
			if ch.Status != "ok" {
				t.Fatalf("database status: got %q want ok", ch.Status)
			}
			if ch.Source != "live" {
				t.Fatalf("database source: got %q want live", ch.Source)
			}
		case "email_outbox":
			sawOutbox = true
			if ch.Status != "ok" {
				t.Fatalf("email_outbox status: got %q want ok", ch.Status)
			}
			if ch.Source != "live" {
				t.Fatalf("email_outbox source: got %q want live", ch.Source)
			}
		}
	}
	if !sawDB || !sawOutbox {
		t.Fatalf("expected database and email_outbox checks, got %+v", body.Checks)
	}
}
