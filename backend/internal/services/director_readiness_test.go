package services

import "testing"

func TestBuildSetupResponse_GroundedNoInvention(t *testing.T) {
	s := &DirectorConsoleService{}
	payload := &directorContext{}
	payload.Business.ID = 7
	payload.DataReadiness = directorDataReadiness{State: "setup", MenuItemCount: 0, PaymentsEnabled: false, CustomerCount: 0}

	for _, locale := range []string{"en", "es", "es_ar"} {
		resp := s.buildSetupResponse(7, locale, payload)
		if resp.Summary == "" || len(resp.ActionPlan) == 0 {
			t.Fatalf("%s: empty setup response", locale)
		}
		for _, a := range resp.ActionPlan {
			if a.DeepLink == "" {
				t.Fatalf("%s: setup action missing deep link", locale)
			}
		}
	}
}

func TestClassifyReadinessState(t *testing.T) {
	cases := []struct {
		name    string
		orders  int64
		revenue bool
		want    string
	}{
		{"brand new", 0, false, "setup"},
		{"four orders no revenue", 4, false, "setup"},
		{"five orders", 5, false, "established"},
		{"any revenue", 0, true, "established"},
		{"open check no lifetime orders", 0, true, "established"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyReadinessState(c.orders, c.revenue); got != c.want {
				t.Fatalf("classifyReadinessState(%d,%v) = %q, want %q", c.orders, c.revenue, got, c.want)
			}
		})
	}
}
